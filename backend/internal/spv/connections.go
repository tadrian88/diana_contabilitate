package spv

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/platform/requestactor"
)

const IdentityValidationUnavailable = "NOT_AVAILABLE"

type ConnectionStatus string

const (
	StatusNotConnected          ConnectionStatus = "NOT_CONNECTED"
	StatusConnected             ConnectionStatus = "CONNECTED"
	StatusNeedsReauthentication ConnectionStatus = "NEEDS_REAUTHENTICATION"
	StatusError                 ConnectionStatus = "ERROR"
	StatusDisabled              ConnectionStatus = "DISABLED"
)

type ConnectionView struct {
	Status                                        ConnectionStatus
	Environment                                   string
	ConnectedAt, LastSyncAt, LastSuccessfulSyncAt *time.Time
	LastSyncStatus                                string
	SafeErrorCode, SafeError                      string
	ImportAutomatic, ConfigurationReady           bool
	IdentityValidation                            string
}

type ConnectionManagerConfig struct {
	Environment, AuthorizeURL, OAuthClientID, OAuthClientSecret string
	RedirectURI, FrontendBaseURL                                string
	StateTTL                                                    time.Duration
	Enabled                                                     bool
}

type ConnectionManager struct {
	store     ConnectionStore
	oauth     OAuthClient
	cipher    TokenCipher
	publisher SyncPublisher
	config    ConnectionManagerConfig
	clock     func() time.Time
}

func NewConnectionManager(store ConnectionStore, oauth OAuthClient, cipher TokenCipher, publisher SyncPublisher, cfg ConnectionManagerConfig) *ConnectionManager {
	if cfg.StateTTL <= 0 {
		cfg.StateTTL = 10 * time.Minute
	}
	return &ConnectionManager{store: store, oauth: oauth, cipher: cipher, publisher: publisher, config: cfg, clock: func() time.Time { return time.Now().UTC() }}
}

func (m *ConnectionManager) Get(ctx context.Context, clientID string) (ConnectionView, error) {
	if _, err := m.store.ClientCUI(ctx, clientID); err != nil {
		return ConnectionView{}, err
	}
	connection, err := m.store.ConnectionByClient(ctx, clientID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return ConnectionView{Status: StatusNotConnected, LastSyncStatus: "NEVER", ConfigurationReady: m.configured(), IdentityValidation: IdentityValidationUnavailable}, nil
	}
	if err != nil {
		return ConnectionView{}, err
	}
	view := m.view(*connection)
	if guard, ok := m.store.(interface {
		ClientOperational(context.Context, string) (bool, error)
	}); ok {
		allowed, e := guard.ClientOperational(ctx, clientID)
		if e != nil {
			return ConnectionView{}, e
		}
		if !allowed {
			view.ImportAutomatic = false
			view.SafeErrorCode = "CLIENT_INACTIVE"
			view.SafeError = "Client inactiv: operațiile ANAF noi sunt oprite."
		}
	}
	return view, nil
}

func (m *ConnectionManager) StartOAuth(ctx context.Context, clientID string) (string, error) {
	return m.StartOAuthCommand(ctx, clientID, "", Actor{})
}
func (m *ConnectionManager) StartOAuthCommand(ctx context.Context, clientID, commandID string, actor Actor) (string, error) {
	if !m.configured() {
		return "", ErrConfiguration
	}
	if _, err := m.store.ClientCUI(ctx, clientID); err != nil {
		return "", err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate OAuth state: %w", err)
	}
	state := hex.EncodeToString(raw)
	digest := sha256.Sum256([]byte(state))
	hash := hex.EncodeToString(digest[:])
	now := m.clock()
	returnPath := "/clients/" + url.PathEscape(clientID)
	attempt := OAuthState{ID: "spvoauth-" + hash[:24], StateHash: hash, ClientID: clientID, Environment: m.config.Environment, ReturnPath: returnPath, CreatedAt: now, ExpiresAt: now.Add(m.config.StateTTL)}
	if store, ok := m.store.(interface {
		CreateClientOAuthAttempt(context.Context, OAuthState, string, string, Actor) (string, error)
	}); ok && commandID != "" {
		if strings.TrimSpace(commandID) == "" || len(commandID) > 200 {
			return "", apperrors.ErrValidation
		}
		encrypted, err := m.cipher.Encrypt(state)
		if err != nil {
			return "", err
		}
		replay, err := store.CreateClientOAuthAttempt(ctx, attempt, encrypted, "client-oauth:"+actor.ID+":"+clientID+":"+commandID, actor)
		if err != nil {
			return "", err
		}
		state, err = m.cipher.Decrypt(replay)
		if err != nil {
			return "", err
		}
	} else if err := m.store.CreateOAuthState(ctx, attempt); err != nil {
		return "", err
	}
	return AuthorizationURLAt(m.config.AuthorizeURL, m.config.OAuthClientID, m.config.RedirectURI, state), nil
}

func (m *ConnectionManager) Callback(ctx context.Context, rawState, code, providerError string, actor Actor) (string, error) {
	if strings.TrimSpace(rawState) == "" {
		return m.errorRedirect("", "invalid_state"), ErrInvalidOAuthState
	}
	digest := sha256.Sum256([]byte(rawState))
	state, err := m.store.ConsumeOAuthState(ctx, hex.EncodeToString(digest[:]), m.clock())
	if err != nil {
		return m.errorRedirect("", "invalid_state"), ErrInvalidOAuthState
	}
	if current, ok := requestactor.FromContext(ctx); ok && !current.AllowsClient(state.ClientID) {
		return m.errorRedirect("", "invalid_state"), apperrors.ErrNotFound
	}
	if state.Environment != m.config.Environment {
		return m.errorRedirect(state.ReturnPath, "environment_mismatch"), apperrors.ErrValidation
	}
	if providerError != "" {
		return m.errorRedirect(state.ReturnPath, "authorization_cancelled"), apperrors.ErrValidation
	}
	if strings.TrimSpace(code) == "" {
		return m.errorRedirect(state.ReturnPath, "missing_code"), apperrors.ErrValidation
	}
	if !m.configured() {
		return m.errorRedirect(state.ReturnPath, "configuration"), ErrConfiguration
	}
	token, err := m.oauth.ExchangeToken(ctx, code, m.config.OAuthClientID, m.config.OAuthClientSecret, m.config.RedirectURI)
	if err != nil {
		return m.errorRedirect(state.ReturnPath, "token_exchange"), err
	}
	access, err := m.cipher.Encrypt(token.AccessToken)
	if err != nil {
		return m.errorRedirect(state.ReturnPath, "token_security"), err
	}
	refresh, err := m.cipher.Encrypt(token.RefreshToken)
	if err != nil {
		return m.errorRedirect(state.ReturnPath, "token_security"), err
	}
	now := m.clock()
	accessExpires := now.Add(token.ExpiresIn)
	var refreshExpires *time.Time
	if token.RefreshExpiresIn > 0 {
		value := now.Add(token.RefreshExpiresIn)
		refreshExpires = &value
	}
	if _, _, err = m.store.CompleteOAuthConnection(ctx, state, access, refresh, accessExpires, refreshExpires, actor, now); err != nil {
		return m.errorRedirect(state.ReturnPath, "persistence"), err
	}
	return m.resultRedirect(state.ReturnPath, "connected", ""), nil
}

func (m *ConnectionManager) Disconnect(ctx context.Context, clientID, commandID string, actor Actor) (ConnectionView, error) {
	connection, _, err := m.store.DisconnectSPVConnection(ctx, clientID, "spv-disconnect:"+actor.ID+":"+clientID+":"+commandID, actor, m.clock())
	if err != nil {
		return ConnectionView{}, err
	}
	return m.view(connection), nil
}

func (m *ConnectionManager) RequestSync(ctx context.Context, clientID, commandID string, actor Actor) (ConnectionView, error) {
	if !m.configured() {
		return ConnectionView{}, ErrConfiguration
	}
	connection, err := m.store.ConnectionByClient(ctx, clientID)
	if err != nil {
		return ConnectionView{}, err
	}
	if guard, ok := m.store.(interface {
		ClientOperational(context.Context, string) (bool, error)
	}); ok {
		allowed, e := guard.ClientOperational(ctx, clientID)
		if e != nil {
			return ConnectionView{}, e
		}
		if !allowed {
			return ConnectionView{}, ErrConnectionInactive
		}
	}
	if connection.Status != "ACTIVE" {
		return ConnectionView{}, ErrConnectionInactive
	}
	if _, err = m.publisher.PublishSPVSync(ctx, connection.ID); err != nil {
		return ConnectionView{}, fmt.Errorf("%w: enqueue SPV sync", ErrTransient)
	}
	if _, err = m.store.RecordManualSyncRequested(ctx, *connection, "spv-sync:"+actor.ID+":"+clientID+":"+commandID, actor, m.clock()); err != nil {
		return ConnectionView{}, err
	}
	return m.view(*connection), nil
}

func (m *ConnectionManager) configured() bool {
	return m.config.Enabled && m.oauth != nil && m.cipher != nil && m.publisher != nil && m.config.OAuthClientID != "" && m.config.OAuthClientSecret != "" && m.config.RedirectURI != "" && m.config.AuthorizeURL != ""
}

func (m *ConnectionManager) view(c Connection) ConnectionView {
	status := StatusError
	switch c.Status {
	case "ACTIVE":
		status = StatusConnected
	case "EXPIRED":
		status = StatusNeedsReauthentication
	case "REVOKED":
		status = StatusDisabled
	}
	// Expired access is refreshable; missing/expired refresh credentials require authorization.
	if status == StatusConnected && (c.AccessTokenCiphertext == "" || c.RefreshTokenCiphertext == "" || c.RefreshTokenExpiresAt != nil && !c.RefreshTokenExpiresAt.After(m.clock())) {
		status = StatusNeedsReauthentication
	}
	view := ConnectionView{Status: status, Environment: c.Environment, ConnectedAt: c.ConnectedAt, LastSyncAt: c.LastSyncFinishedAt, LastSuccessfulSyncAt: c.LastSuccessfulSyncAt, LastSyncStatus: c.LastSyncStatus, ImportAutomatic: status == StatusConnected && m.configured(), ConfigurationReady: m.configured(), IdentityValidation: IdentityValidationUnavailable}
	if c.LastError != "" {
		view.SafeErrorCode = "SYNC_FAILED"
		view.SafeError = "Ultima sincronizare nu a putut fi finalizată."
	}
	if status == StatusNeedsReauthentication {
		view.SafeErrorCode = "REAUTHENTICATION_REQUIRED"
		view.SafeError = "Conexiunea ANAF necesită reconectare."
	}
	return view
}

func (m *ConnectionManager) resultRedirect(returnPath, result, reason string) string {
	base, err := url.Parse(m.config.FrontendBaseURL)
	if err != nil {
		return "/clients"
	}
	if strings.HasPrefix(returnPath, "/clients/") {
		base.Path = path.Clean(returnPath)
	} else {
		base.Path = "/clients"
	}
	q := base.Query()
	q.Set("integration", "anaf")
	q.Set("result", result)
	if reason != "" {
		q.Set("reason", reason)
	}
	base.RawQuery = q.Encode()
	base.Fragment = "anaf-spv"
	return base.String()
}
func (m *ConnectionManager) errorRedirect(returnPath, reason string) string {
	return m.resultRedirect(returnPath, "error", reason)
}
