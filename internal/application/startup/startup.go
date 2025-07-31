// Package startup prepares the application server
package startup

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/container"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/cleanup"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/server"
	"github.com/gin-gonic/gin"
)

// Initialize performs the complete multi-tenant startup sequence
func Initialize() error {
	setupLogging()

	start := time.Now().UTC()

	ctx, cancelBackgroundTasks := context.WithCancel(context.Background())
	defer cancelBackgroundTasks()

	log.Println("\033[32m" + `

 ▄██▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄██▄▄▄▄▄▄▄██▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄ ▄▄▄
  ██  ██ ██ ▀▀ ██ ██ ▀▀ ██ ██ ▀▀ ██ ▀▀ ██ ██ ▀▀ ██ ██
  ██  ██▀█▄ ██▀██ ██ ▄▄ ██ ▀▀▀██ ██ ██▀██ ██ ▄▄ ██▀█▄
  ██  ██ ██ ██▄██ ██▄██ ██ ██▄██ ██ ██▄██ ██▄██ ██ ██
   ▀▀                   ▀▀       ▀▀             ▀▀ ▀▀▀
` + "\033[97m" + `
  made by At Risk Media
` + "\033[0m")

	// Step 1: Initialize tenant system
	log.Println("Initializing...")
	tenantManager := tenant.NewManager()

	// Step 2: Load tenant registry to discover all tenants
	log.Println("Loading tenant registry...")
	registry, err := tenant.LoadTenantRegistry()
	if err != nil {
		return fmt.Errorf("failed to load tenant registry: %w", err)
	}

	if len(registry.Tenants) == 0 {
		log.Println("No tenants found in registry - creating default tenant")
		if err := tenant.RegisterTenant("default"); err != nil {
			return fmt.Errorf("failed to register default tenant: %w", err)
		}
		registry, err = tenant.LoadTenantRegistry()
		if err != nil {
			return fmt.Errorf("failed to reload registry: %w", err)
		}
	}

	log.Printf("Found %d tenants in registry", len(registry.Tenants))

	// Step 3: Pre-activate inactive tenants only
	log.Println("Starting tenant pre-activation...")
	if err := tenantManager.PreActivateAllTenants(); err != nil {
		return fmt.Errorf("tenant pre-activation failed: %w", err)
	}

	// Step 4: Validate tenant activation
	log.Println("Validating tenant activation...")
	if err := tenantManager.ValidatePreActivation(); err != nil {
		return fmt.Errorf("tenant validation failed: %w", err)
	}

	// Step 5: Verify active tenant connections
	log.Println("Verifying active tenant database connections...")
	activeCount, err := tenantManager.GetActiveTenantCount()
	if err != nil {
		return fmt.Errorf("failed to get active tenant count: %w", err)
	}
	log.Printf("✓ %d active tenant connections verified", activeCount)

	// Step 6: Initialize cache system
	log.Println("Initializing cache system...")
	cacheManager := tenantManager.GetCacheManager()

	for tenantID, tenantInfo := range registry.Tenants {
		if tenantInfo.Status == "active" {
			log.Printf("  Initializing cache for tenant: %s", tenantID)
			cacheManager.InitializeTenant(tenantID)
		}
	}

	// Step 7: Create dependency injection container
	log.Println("Initializing dependency injection container...")
	appContainer := container.NewContainer(tenantManager, cacheManager)
	log.Println("✓ Dependency injection container created with singleton services.")

	// Step 8: Initialize application services (handled by container)
	log.Println("Initializing singleton application services...")
	log.Println("✓ Singleton application services initialized via container.")

	// Step 9: Initialize cache warming
	log.Println("Initializing cache warming...")
	reporter := cleanup.NewReporter(cacheManager)

	warmingService := appContainer.WarmingService
	contentMapService := appContainer.ContentMapService
	beliefRegistryService := appContainer.BeliefRegistryService

	if err := warmingService.WarmAllTenants(tenantManager, cacheManager, contentMapService, beliefRegistryService, reporter); err != nil {
		log.Printf("WARNING: Cache warming failed: %v", err)
	}

	// Step 10: Start background cleanup worker
	log.Println("Starting background cleanup worker...")
	// CORRECT: Create the config struct from the central config package.
	cleanupConfig := cleanup.NewConfig()
	// CORRECT: Inject the config into the worker's constructor.
	cleanupWorker := cleanup.NewWorker(cacheManager, tenantManager.GetDetector(), cleanupConfig)
	go cleanupWorker.Start(ctx)
	log.Println("✓ Background cleanup worker started.")

	// Step 11: Start HTTP server
	log.Println("Starting HTTP server...")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	httpServer := server.New(port, appContainer)

	// Step 12: Setup graceful shutdown
	gracefulShutdown := make(chan os.Signal, 1)
	signal.Notify(gracefulShutdown, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := httpServer.Start(); err != nil {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-gracefulShutdown
	log.Println("Shutdown signal received, starting graceful shutdown...")

	// Cancel background tasks
	cancelBackgroundTasks()

	// Stop server
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := httpServer.Stop(shutdownCtx); err != nil {
		log.Printf("Error during server shutdown: %v", err)
	}

	// Close tenant manager
	if err := tenantManager.Close(); err != nil {
		log.Printf("Error closing tenant manager: %v", err)
	}

	elapsed := time.Since(start)
	log.Printf("Application ran for %v", elapsed)
	log.Println("Application shutdown complete.")

	return nil
}

// setupLogging configures application logging
func setupLogging() {
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
