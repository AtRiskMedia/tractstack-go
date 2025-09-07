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

	// Determine server address based on configuration
	addr := config.BindAddress + ":" + port

	httpServer := &http.Server{
		Addr:    addr,
		Handler: router,
		// ReadTimeout protects against slow clients on initial request.
		ReadTimeout: config.ServerReadTimeout,
		// WriteTimeout is removed to allow long-lived streaming responses like SSE.
		// IdleTimeout is also removed as it can prematurely close SSE connections.
	}

	return &Server{
		httpServer: httpServer,
		container:  container,
	}
}

// Start begins listening for HTTP requests
func (s *Server) Start() error {
	log.Printf("Starting HTTP server on %s", s.httpServer.Addr)

	if config.SSLEnabled && config.SSLCertPath != "" && config.SSLKeyPath != "" {
		log.Printf("SSL enabled - using certificates: %s", config.SSLCertPath)
		if err := s.httpServer.ListenAndServeTLS(config.SSLCertPath, config.SSLKeyPath); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("failed to start HTTPS server: %w", err)
		}
	} else {
		log.Printf("Starting HTTP server (no SSL)")
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("failed to start HTTP server: %w", err)
		}
	}

	return nil
}

// Stop gracefully shuts down the HTTP server
func (s *Server) Stop(ctx context.Context) error {
	log.Println("Shutting down HTTP server...")
	return s.httpServer.Shutdown(ctx)
}
