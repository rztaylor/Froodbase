package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/rztaylor/froodbase/internal/version"
)

// HealthPath is the liveness endpoint. It answers 200 as soon as the process
// is serving; it does not check the database.
const HealthPath = "/healthz"

const (
	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 10 * time.Second
)

// MCPPath is where the Streamable HTTP MCP endpoint is mounted.
const MCPPath = "/mcp"

// Server is the froodbase-api HTTP server.
type Server struct {
	log *slog.Logger
	mux *http.ServeMux
	srv *http.Server
}

// New builds a server with the health route mounted. It does not listen.
func New(log *slog.Logger) *Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+HealthPath, handleHealth)
	return &Server{
		log: log,
		mux: mux,
		srv: &http.Server{Handler: mux, ReadHeaderTimeout: readHeaderTimeout},
	}
}

// Mount adds a handler at pattern (a net/http ServeMux pattern).
func (s *Server) Mount(pattern string, h http.Handler) {
	s.mux.Handle(pattern, h)
}

// Handler returns the router, for tests and for mounting in other servers.
func (s *Server) Handler() http.Handler {
	return s.srv.Handler
}

// Run listens on addr and serves until ctx is cancelled, then shuts down
// gracefully. It returns nil after a clean shutdown.
func (s *Server) Run(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return s.Serve(ctx, ln)
}

// Serve serves on an existing listener until ctx is cancelled, then shuts
// down gracefully. It takes ownership of ln.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	s.log.Info("froodbase-api listening", "addr", ln.Addr().String(), "version", version.Version)

	errc := make(chan error, 1)
	go func() { errc <- s.srv.Serve(ln) }()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	s.log.Info("froodbase-api stopping")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := s.srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-errc; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(healthResponse{Status: "ok", Version: version.Version})
}
