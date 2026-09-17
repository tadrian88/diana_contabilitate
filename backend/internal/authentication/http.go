package authentication

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/platform/requestactor"
	"github.com/redis/go-redis/v9"
)

const (
	SessionCookie = "diana_session"
	CSRFCookie    = "diana_csrf"
)

type Config struct {
	SecureCookies bool
	SessionTTL    time.Duration
	FrontendURL   string
	LoginLimit    int64
	LoginWindow   time.Duration
}

type HTTP struct {
	store             Store
	redis             *redis.Client
	config            Config
	logger            *slog.Logger
	now               func() time.Time
	dummyPasswordHash string
	allowLoginForTest func(context.Context, string) bool
}

func NewHTTP(store Store, redisClient *redis.Client, config Config, logger *slog.Logger) *HTTP {
	if config.SessionTTL <= 0 {
		config.SessionTTL = 12 * time.Hour
	}
	if config.LoginLimit <= 0 {
		config.LoginLimit = 5
	}
	if config.LoginWindow <= 0 {
		config.LoginWindow = 15 * time.Minute
	}
	dummyHash, _ := HashPassword("Diana-invalid-login-only-2026!")
	return &HTTP{store: store, redis: redisClient, config: config, logger: logger, now: time.Now, dummyPasswordHash: dummyHash}
}

func (h *HTTP) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/auth/login" && r.Method == http.MethodPost:
			h.login(w, r)
			return
		case r.URL.Path == "/api/v1/auth/session" && r.Method == http.MethodGet:
			h.session(w, r)
			return
		case r.URL.Path == "/api/v1/auth/logout" && r.Method == http.MethodPost:
			h.logout(w, r)
			return
		case r.URL.Path == "/api/v1/integrations/anaf/callback", r.URL.Path == "/healthz", r.URL.Path == "/readyz":
			next.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		session, ok := h.authenticate(w, r)
		if !ok {
			return
		}
		if isUnsafe(r.Method) && !h.validCSRF(r, session) {
			writeAuthError(w, http.StatusForbidden, "CSRF_REJECTED", "Cererea nu a putut fi verificată.")
			return
		}
		actor := requestactor.Actor{ID: session.User.ID, Display: session.User.Email, Persona: session.User.Persona, AllClients: session.User.AllClients, AuthorizedClientIDs: session.User.AuthorizedClientIDs}
		next.ServeHTTP(w, r.WithContext(requestactor.WithActor(r.Context(), actor)))
	})
}

func (h *HTTP) login(w http.ResponseWriter, r *http.Request) {
	if !h.validOrigin(r) {
		writeAuthError(w, http.StatusForbidden, "ORIGIN_REJECTED", "Cererea nu a putut fi verificată.")
		return
	}
	var input struct{ Email, Password string }
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
		writeAuthError(w, http.StatusBadRequest, "INVALID_REQUEST", "Emailul și parola sunt obligatorii.")
		return
	}
	normalized := NormalizeEmail(input.Email)
	key := h.loginRateKey(normalized, clientIP(r))
	if !h.allowLogin(r.Context(), key) {
		writeAuthError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Prea multe încercări. Încearcă din nou mai târziu.")
		return
	}
	user, err := h.store.UserByEmail(r.Context(), normalized)
	valid := err == nil
	hash := user.PasswordHash
	if !valid {
		hash = h.dummyPasswordHash
	}
	passwordValid, verifyErr := VerifyPassword(hash, input.Password)
	valid = valid && passwordValid && verifyErr == nil
	if !valid || err != nil || user.Status != "ACTIVE" {
		var id *string
		if user.ID != "" {
			id = &user.ID
		}
		_ = h.store.RecordEvent(r.Context(), "LOGIN_FAILURE", id, h.now())
		writeAuthError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email sau parolă incorectă.")
		return
	}
	token, err := randomSecret()
	if err != nil {
		h.internal(w, err)
		return
	}
	csrf, err := randomSecret()
	if err != nil {
		h.internal(w, err)
		return
	}
	now := h.now()
	if _, err = h.store.CreateSession(r.Context(), user.ID, TokenHash(token), TokenHash(csrf), now, now.Add(h.config.SessionTTL)); err != nil {
		h.internal(w, err)
		return
	}
	if h.redis != nil {
		_ = h.redis.Del(r.Context(), key).Err()
	}
	_ = h.store.RecordEvent(r.Context(), "LOGIN_SUCCESS", &user.ID, now)
	h.setCookies(w, token, csrf, now.Add(h.config.SessionTTL))
	writeUser(w, user)
}

func (h *HTTP) session(w http.ResponseWriter, r *http.Request) {
	session, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	writeUser(w, session.User)
}

func (h *HTTP) logout(w http.ResponseWriter, r *http.Request) {
	session, ok := h.authenticate(w, r)
	if !ok {
		h.clearCookies(w)
		return
	}
	if !h.validOrigin(r) || !h.validCSRF(r, session) {
		writeAuthError(w, http.StatusForbidden, "CSRF_REJECTED", "Cererea nu a putut fi verificată.")
		return
	}
	cookie, _ := r.Cookie(SessionCookie)
	_ = h.store.RevokeSession(r.Context(), TokenHash(cookie.Value), h.now())
	_ = h.store.RecordEvent(r.Context(), "LOGOUT", &session.User.ID, h.now())
	h.clearCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTP) authenticate(w http.ResponseWriter, r *http.Request) (Session, bool) {
	cookie, err := r.Cookie(SessionCookie)
	if err != nil || cookie.Value == "" {
		writeAuthError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Autentificare necesară.")
		return Session{}, false
	}
	session, err := h.store.SessionByTokenHash(r.Context(), TokenHash(cookie.Value), h.now())
	if err != nil {
		h.clearCookies(w)
		writeAuthError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Sesiunea a expirat.")
		return Session{}, false
	}
	return session, true
}

func (h *HTTP) validCSRF(r *http.Request, session Session) bool {
	cookie, err := r.Cookie(CSRFCookie)
	header := r.Header.Get("X-CSRF-Token")
	if err != nil || cookie.Value == "" || header == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(header)) != 1 {
		return false
	}
	return subtle.ConstantTimeCompare(TokenHash(header), session.CSRFHash) == 1
}

func (h *HTTP) validOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	want, err := url.Parse(h.config.FrontendURL)
	got, parseErr := url.Parse(origin)
	return err == nil && parseErr == nil && strings.EqualFold(want.Scheme, got.Scheme) && strings.EqualFold(want.Host, got.Host)
}

func (h *HTTP) allowLogin(ctx context.Context, key string) bool {
	if h.allowLoginForTest != nil {
		return h.allowLoginForTest(ctx, key)
	}
	if h.redis == nil {
		return false
	}
	count, err := h.redis.Eval(ctx, `
local count = redis.call('INCR', KEYS[1])
if count == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
return count`, []string{key}, h.config.LoginWindow.Milliseconds()).Int64()
	if err != nil {
		return false
	}
	return count <= h.config.LoginLimit
}

func (h *HTTP) loginRateKey(email, ip string) string {
	digest := TokenHash(email + "\x00" + ip)
	return "diana:auth:login:" + hex.EncodeToString(digest)
}

func (h *HTTP) setCookies(w http.ResponseWriter, token, csrf string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: token, Path: "/", Expires: expires, MaxAge: int(time.Until(expires).Seconds()), Secure: h.config.SecureCookies, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	http.SetCookie(w, &http.Cookie{Name: CSRFCookie, Value: csrf, Path: "/", Expires: expires, MaxAge: int(time.Until(expires).Seconds()), Secure: h.config.SecureCookies, HttpOnly: false, SameSite: http.SameSiteLaxMode})
}

func (h *HTTP) clearCookies(w http.ResponseWriter) {
	for _, name := range []string{SessionCookie, CSRFCookie} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(1, 0), Secure: h.config.SecureCookies, HttpOnly: name == SessionCookie, SameSite: http.SameSiteLaxMode})
	}
}

func (h *HTTP) internal(w http.ResponseWriter, err error) {
	h.logger.Error("authentication operation failed", "error", err)
	writeAuthError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Autentificarea nu a putut fi finalizată.")
}

func writeUser(w http.ResponseWriter, user User) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"user": map[string]any{"id": user.ID, "email": user.Email, "persona": user.Persona}})
}

func writeAuthError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message})
}

func randomSecret() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func clientIP(r *http.Request) string {
	if raw := r.Header.Get("X-Forwarded-For"); raw != "" {
		parts := strings.Split(raw, ",")
		// Google Front Ends append the actual client and load-balancer addresses
		// after any caller-supplied values. The second-to-last value is therefore
		// the trustworthy client address at Cloud Run; a single value supports
		// local reverse proxies.
		index := len(parts) - 1
		if len(parts) >= 2 {
			index = len(parts) - 2
		}
		if forwarded := strings.TrimSpace(parts[index]); forwarded != "" {
			return forwarded
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func isUnsafe(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}
