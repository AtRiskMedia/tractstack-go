// Package server provides HTTP server initialization and management.
package server

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/application/container"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/routes"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

// Server wraps the HTTP server with configuration and dependency injection
type Server struct {
	httpServer *http.Server
	container  *container.Container
}

// New creates a new HTTP server instance with dependency injection
func New(port string, container *container.Container) *Server {
	router := routes.SetupRoutes(container)

	httpServer := &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  config.ServerReadTimeout,
		WriteTimeout: config.ServerWriteTimeout,
		IdleTimeout:  config.ServerIdleTimeout,
	}

	return &Server{
		httpServer: httpServer,
		container:  container,
	}
}

// Start begins listening for HTTP requests
func (s *Server) Start() error {
	log.Printf("Starting HTTP server on %s", s.httpServer.Addr)

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	return nil
}

// Stop gracefully shuts down the HTTP server
func (s *Server) Stop(ctx context.Context) error {
	log.Println("Shutting down HTTP server...")
	return s.httpServer.Shutdown(ctx)
}
