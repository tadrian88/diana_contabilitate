package httpserver

import (
	"net/http"

	"diana-contabilitate/backend/internal/platform/requestactor"
)

// withTestActor is test-only fixture wiring. Production obtains RequestActor
// exclusively from Authentication V1 middleware.
func withTestActor(next http.Handler, actor requestactor.Actor) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(requestactor.WithActor(r.Context(), actor)))
	})
}
