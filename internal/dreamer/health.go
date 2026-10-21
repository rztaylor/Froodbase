package dreamer

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/rztaylor/froodbase/internal/version"
)

// HealthPath is the dreamer's health endpoint.
const HealthPath = "/healthz"

const (
	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 10 * time.Second
	depthTimeout      = 3 * time.Second
)

type healthResponse struct {
	Status     string `json:"status"`
	Version    string `json:"version"`
	QueueDepth int    `json:"queue_depth"`
	// IndexTotal and IndexStale report the projection bookkeeping across
	// every area: records in all, and records not indexed at their current
	// revision (TR-57).
	IndexTotal int `json:"index_total"`
	IndexStale int `json:"index_stale"`
	// ReconsiderWaiting counts the propagation jobs (reconsider_derived)
	// waiting to run, across every area (TR-44).
	ReconsiderWaiting int `json:"reconsider_waiting"`
}

// HealthHandler answers with the version, the queue depth, the index
// staleness and the propagation jobs waiting. A database
// failure reports status degraded with 503.
func (d *Dreamer) HealthHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+HealthPath, func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), depthTimeout)
		defer cancel()
		resp := healthResponse{Status: "ok", Version: version.Version}
		depth, err := d.QueueDepth(ctx)
		if err != nil {
			resp.Status = "degraded"
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		resp.QueueDepth = depth
		if st, err := d.Staleness(ctx); err != nil {
			resp.Status = "degraded"
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			resp.IndexTotal, resp.IndexStale = st.Total, st.Stale
		}
		if n, err := d.ReconsiderWaiting(ctx); err != nil {
			resp.Status = "degraded"
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			resp.ReconsiderWaiting = n
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	return mux
}

// ServeHealth serves the health endpoint on addr until ctx is cancelled.
func (d *Dreamer) ServeHealth(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: d.HealthHandler(), ReadHeaderTimeout: readHeaderTimeout}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-errc; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
