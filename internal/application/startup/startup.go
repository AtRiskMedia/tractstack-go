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
	tenantManager := tenant.NewManager(nil)

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

	// Step 3: Initialize cache system
	log.Println("Initializing cache system...")
	cacheManager := tenantManager.GetCacheManager()

	// Step 4: Create dependency injection container (MOVED HERE - BEFORE PRE-ACTIVATION!)
	log.Println("Initializing dependency injection container...")
	appContainer := container.NewContainer(tenantManager, cacheManager)
	log.Println("✓ Dependency injection container created with singleton services.")
	logger := appContainer.Logger
	logger.Startup().Info("Container initialization complete - switching to channeled logging")
	tenantManager.SetLogger(logger)
	logger.Tenant().Info("Tenant manager logger initialized", "hasDetector", true, "hasCache", true)

	// Step 5: Pre-activate inactive tenants (NOW HAS LOGGER!)
	logger.Startup().Info("Starting tenant pre-activation...")
	if err := tenantManager.PreActivateAllTenants(); err != nil {
		return fmt.Errorf("tenant pre-activation failed: %w", err)
	}

	// Step 6: Validate tenant activation
	logger.Startup().Info("Validating tenant activation...")
	if err := tenantManager.ValidatePreActivation(); err != nil {
		return fmt.Errorf("tenant validation failed: %w", err)
	}

	// Step 7: Verify active tenant connections
	logger.Startup().Info("Verifying active tenant database connections...")
	activeCount, err := tenantManager.GetActiveTenantCount()
	if err != nil {
		return fmt.Errorf("failed to get active tenant count: %w", err)
	}
	logger.Startup().Info("Active tenant connections verified", "count", activeCount)

	// Step 8: Initialize tenant cache
	logger.Startup().Info("Initializing tenant cache...")
	for tenantID, tenantInfo := range registry.Tenants {
		if tenantInfo.Status == "active" {
			logger.Tenant().Info("Initializing cache for tenant", "tenantId", tenantID)
			cacheManager.InitializeTenant(tenantID)
		}
	}

	// Step 9: Initialize application services (handled by container)
	logger.Startup().Info("Singleton application services initialized via container")

	// Step 10: Initialize cache warming
	logger.Startup().Info("Initializing cache warming...")
	startWarmTime := time.Now()

	reporter := cleanup.NewReporter(cacheManager)
	warmingService := appContainer.WarmingService
	contentMapService := appContainer.ContentMapService
	beliefRegistryService := appContainer.BeliefRegistryService

	if err := warmingService.WarmAllTenants(tenantManager, cacheManager, contentMapService, beliefRegistryService, reporter); err != nil {
		logger.Startup().Error("Cache warming failed", "error", err.Error(), "duration", time.Since(startWarmTime))
	} else {
		logger.Startup().Info("Cache warming completed successfully", "duration", time.Since(startWarmTime))
	}

	// Step 11: Start background cleanup worker
	logger.Startup().Info("Starting background cleanup worker...")
	startWorkerTime := time.Now()

	cleanupConfig := cleanup.NewConfig()
	cleanupWorker := cleanup.NewWorker(cacheManager, tenantManager.GetDetector(), cleanupConfig, logger)
	go cleanupWorker.Start(ctx)

	logger.Startup().Info("Background cleanup worker started", "duration", time.Since(startWorkerTime))

	// Step 12: Start HTTP server
	logger.Startup().Info("Starting HTTP server...")
	startServerTime := time.Now()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	httpServer := server.New(port, appContainer)

	logger.Startup().Info("HTTP server initialized", "port", port, "duration", time.Since(startServerTime))

	// Step 13: Setup graceful shutdown
	gracefulShutdown := make(chan os.Signal, 1)
	signal.Notify(gracefulShutdown, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.System().Info("Starting HTTP server", "address", ":"+port)
		if err := httpServer.Start(); err != nil {
			logger.System().Error("HTTP server failed", "error", err.Error())
		}
	}()

	totalStartupTime := time.Since(start)
	logger.Startup().Info("Application startup complete",
		"totalDuration", totalStartupTime,
		"activeTenants", activeCount,
		"port", port)

	// Wait for shutdown signal
	<-gracefulShutdown
	logger.Shutdown().Info("Shutdown signal received, starting graceful shutdown...")

	shutdownStart := time.Now()

	// Cancel background tasks
	cancelBackgroundTasks()

	// Stop server
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger.Shutdown().Info("Stopping HTTP server...")
	if err := httpServer.Stop(shutdownCtx); err != nil {
		logger.Shutdown().Error("Error during server shutdown", "error", err.Error())
	} else {
		logger.Shutdown().Info("HTTP server stopped successfully")
	}

	// Close tenant manager
	logger.Shutdown().Info("Closing tenant manager...")
	if err := tenantManager.Close(); err != nil {
		logger.Shutdown().Error("Error closing tenant manager", "error", err.Error())
	} else {
		logger.Shutdown().Info("Tenant manager closed successfully")
	}

	elapsed := time.Since(start)
	logger.Shutdown().Info("Application shutdown complete",
		"totalUptime", elapsed,
		"shutdownDuration", time.Since(shutdownStart))

	return nil
}

// setupLogging configures application logging
func setupLogging() {
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
