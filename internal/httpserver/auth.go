package httpserver

import (
	"net/http"

	"github.com/rztaylor/froodbase/internal/authz"
)

// Authenticator checks a username and password and returns the principal on
// success. staticusers.Store satisfies it.
type Authenticator interface {
	Authenticate(username, password string) (authz.Principal, bool)
}

// BasicAuth returns a middleware that authenticates every request with HTTP
// basic auth, builds the authorisation context and attaches it to the
// request context. Requests without valid credentials receive 401 and never
// reach next.
func BasicAuth(auth Authenticator, builder authz.Builder, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok {
			unauthorised(w)
			return
		}
		principal, ok := auth.Authenticate(user, pass)
		if !ok {
			unauthorised(w)
			return
		}
		ac, err := builder.Build(r.Context(), principal)
		if err != nil {
			unauthorised(w)
			return
		}
		next.ServeHTTP(w, r.WithContext(authz.WithContext(r.Context(), ac)))
	})
}

func unauthorised(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="froodbase"`)
	http.Error(w, "unauthorised", http.StatusUnauthorized)
}
