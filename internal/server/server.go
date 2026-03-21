package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/symphony-go/symphony/internal/orchestrator"
)

// Server is the optional HTTP server for observability.
type Server struct {
	port       int
	orch       *orchestrator.Orchestrator
	httpServer *http.Server
	listener   net.Listener
}

// New creates a new HTTP server.
func New(port int, orch *orchestrator.Orchestrator) *Server {
	return &Server{
		port: port,
		orch: orch,
	}
}

// Start binds and starts the HTTP server.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", s.handleDashboard)
	mux.HandleFunc("GET /api/v1/state", s.handleGetState)
	mux.HandleFunc("GET /api/v1/{identifier}", s.handleGetIssue)
	mux.HandleFunc("POST /api/v1/refresh", s.handlePostRefresh)
	mux.HandleFunc("POST /api/internal/agent/callback", s.handleAgentCallback)

	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to bind %s: %w", addr, err)
	}
	s.listener = listener

	actualPort := listener.Addr().(*net.TCPAddr).Port
	if s.port == 0 {
		slog.Info("HTTP server bound to ephemeral port", "port", actualPort)
	}

	s.httpServer = &http.Server{Handler: mux}

	go func() {
		if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
		}
	}()

	return nil
}

// Port returns the actual bound port (useful when port=0).
func (s *Server) Port() int {
	if s.listener != nil {
		return s.listener.Addr().(*net.TCPAddr).Port
	}
	return s.port
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}
