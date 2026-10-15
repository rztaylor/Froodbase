package httpserver

import (
	"log/slog"
	"net/http"

	"github.com/rztaylor/froodbase/internal/version"
)

// ClientVersion returns a middleware that reads the froodbase-mcp version
// header and logs a warning when the client is below the minimum the API
// publishes. The request is still served in the local MVP; the cloud move
// turns the warning into an update message (TR-102). Requests without the
// header (curl, other clients) pass silently.
func ClientVersion(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.Header.Get(version.ClientHeader); v != "" && !version.Compatible(v) {
			log.Warn("froodbase-mcp client below the minimum version; ask the user to update",
				"client_version", v, "min_client_version", version.MinClientVersion)
		}
		next.ServeHTTP(w, r)
	})
}
