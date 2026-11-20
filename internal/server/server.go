// Package server runs the HTTP server with Kubernetes-friendly graceful shutdown.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Drainer is told when shutdown begins so readiness can fail before the server stops.
type Drainer interface {
	StartDraining()
}

// Timeouts controls the shutdown sequence.
type Timeouts struct {
	// Drain is how long to keep serving (with /readyz = 503) so Kubernetes removes the pod from endpoints.
	Drain time.Duration
	// Shutdown bounds how long in-flight requests get to finish.
	Shutdown time.Duration
}

// Server owns the http.Server lifecycle.
type Server struct {
	httpServer *http.Server
	drainer    Drainer
	timeouts   Timeouts
	logger     *slog.Logger
}

// New builds a Server; call Serve to run it.
//
//	srv := server.New(routes, health, server.Timeouts{Drain: 5 * time.Second, Shutdown: 15 * time.Second}, logger)
func New(routes http.Handler, drainer Drainer, timeouts Timeouts, logger *slog.Logger) *Server {
	return &Server{
		httpServer: &http.Server{
			Handler:           routes,
			ReadHeaderTimeout: 5 * time.Second, // mitigates Slowloris (gosec G112)
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
		drainer:  drainer,
		timeouts: timeouts,
		logger:   logger,
	}
}

// Serve accepts connections on listener until ctx is cancelled, then drains and shuts down.
// It returns nil after a graceful shutdown.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	serveErr := make(chan error, 1)
	go func() { serveErr <- s.httpServer.Serve(listener) }()
	s.logger.Info("server listening", "addr", listener.Addr().String())

	select {
	case err := <-serveErr:
		return fmt.Errorf("http server stopped unexpectedly: %w", err)
	case <-ctx.Done():
		return s.shutdown()
	}
}

func (s *Server) shutdown() error {
	s.logger.Info("shutdown started, draining", "drain", s.timeouts.Drain.String())
	s.drainer.StartDraining()
	time.Sleep(s.timeouts.Drain)

	ctx, cancel := context.WithTimeout(context.Background(), s.timeouts.Shutdown)
	defer cancel()
	if err := s.httpServer.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("graceful shutdown failed after %s: %w", s.timeouts.Shutdown, err)
	}
	s.logger.Info("shutdown complete")
	return nil
}
