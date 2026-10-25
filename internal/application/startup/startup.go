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

	// --- One Dark ANSI Color Codes ---
	const (
		brandGreen = "\033[38;2;200;223;140m" // c8df8c
		darkGrey   = "\033[90m"
		purple     = "\033[38;2;198;120;221m" // One Dark Purple
		blue       = "\033[38;2;97;175;239m"  // One Dark Blue
		cyan       = "\033[38;2;86;182;194m"  // One Dark Cyan/Teal
		green      = "\033[38;2;152;195;121m" // One Dark Green
		yellow     = "\033[38;2;229;192;123m" // One Dark Yellow
		white      = "\033[97m"
		reset      = "\033[0m"
	)

	totalStart := time.Now()
	ctx, cancelBackgroundTasks := context.WithCancel(context.Background())
	defer cancelBackgroundTasks()

	log.Println(brandGreen + `

 ▄██▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄██▄▄▄▄▄▄▄██▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄ ▄▄▄
  ██  ██ ██ ▀▀ ██ ██ ▀▀ ██ ██ ▀▀ ██ ▀▀ ██ ██ ▀▀ ██ ██
  ██  ██▀█▄ ██▀██ ██ ▄▄ ██ ▀▀▀██ ██ ██▀██ ██ ▄▄ ██▀█▄
  ██  ██ ██ ██▄██ ██▄██ ██ ██▄██ ██ ██▄██ ██▄██ ██ ██
   ▀▀                   ▀▀       ▀▀             ▀▀ ▀▀▀
` + darkGrey + `
  made by At Risk Media
` + reset)

	// --- Step 1: Initialize Core Systems ---
	stepStart := time.Now()
	tenantManager := tenant.NewManager(nil)
	cacheManager := tenantManager.GetCacheManager()
	log.Printf("%s✓ Core Systems Initialized %s(%s)%s", cyan, darkGrey, time.Since(stepStart), reset)

	// --- Step 2: Load Tenant Registry ---
	stepStart = time.Now()
	registry, err := tenant.LoadTenantRegistry()
	if err != nil {
		return fmt.Errorf("failed to load tenant registry: %w", err)
	}
	// This is the sole legitimate caller of tenant.RegisterTenant. It only fires
	// when tenants.json exists but has an empty tenants map (LoadTenantRegistry
	// already seeds default when the file is absent), seeding the default tenant.
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
	log.Printf("%s✓ Tenant Registry Loaded   %s(%d tenants in %s)%s", cyan, darkGrey, len(registry.Tenants), time.Since(stepStart), reset)

	// --- Step 3: Create Dependency Injection Container ---
	stepStart = time.Now()
	appContainer := container.NewContainer(tenantManager, cacheManager)
	logger := appContainer.Logger
	tenantManager.SetLogger(logger)
	tenantManager.SetService(appContainer.Service)
	log.Printf("%s✓ DI Container Built       %s(%s)%s", cyan, darkGrey, time.Since(stepStart), reset)

	// --- Step 4: Run Startup Migrations for ALL Tenants ---
	stepStart = time.Now()
	log.Printf("\n%s✓ RUNNING STARTUP MIGRATIONS FOR ALL TENANTS%s", purple, reset)
	if err := tenantManager.RunStartupMigrations(); err != nil {
		return fmt.Errorf("startup migrations failed: %w", err)
	}
	log.Printf("%s✓ Migrations Complete      %s(%s)%s", cyan, darkGrey, time.Since(stepStart), reset)

	// --- Step 5: Activate Reserved Tenants ---
	stepStart = time.Now()
	log.Printf("\n%s✓ ACTIVATING RESERVED TENANTS%s", purple, reset)
	if err := tenantManager.PreActivateAllTenants(); err != nil {
		return fmt.Errorf("tenant pre-activation failed: %w", err)
	}
	activeCount, err := tenantManager.GetActiveTenantCount()
	if err != nil {
		return fmt.Errorf("failed to get active tenant count: %w", err)
	}
	log.Printf("%s✓ Activation Complete      %s(%d active in %s)%s", cyan, darkGrey, activeCount, time.Since(stepStart), reset)

	// --- Step 6: Initialize and Warm Tenant Caches ---
	stepStart = time.Now()
	log.Printf("\n%s✓ WARMING CACHE FOR %d TENANTS%s", purple, activeCount, reset)
	reporter := cleanup.NewReporter(cacheManager)
	if err := appContainer.WarmingService.WarmAllTenants(tenantManager, cacheManager, appContainer.ContentMapService, appContainer.BeliefRegistryService, reporter); err != nil {
		log.Printf("%s✗ Cache warming failed for %d tenants%s", yellow, activeCount, reset)
		return fmt.Errorf("cache warming failed: %w", err)
	}
	log.Printf("%s✓ Caches Warmed          %s(%s)%s", cyan, darkGrey, time.Since(stepStart), reset)

	// --- Step 7: Start Background Services ---
	stepStart = time.Now()
	cleanupConfig := cleanup.NewConfig()
	cleanupWorker := cleanup.NewWorker(cacheManager, tenantManager.GetDetector(), cleanupConfig, logger)
	go cleanupWorker.Start(ctx)
	if appContainer.ShopifyReconciliationWorker != nil {
		go appContainer.ShopifyReconciliationWorker.Start(ctx)
	}
	if appContainer.BookingReconciliationWorker != nil {
		go appContainer.BookingReconciliationWorker.Start(ctx)
	}
	log.Printf("%s✓ Background Services Up   %s(%s)%s", cyan, darkGrey, time.Since(stepStart), reset)

	// --- Step 8: Start HTTP Server ---
	stepStart = time.Now()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	httpServer := server.New(port, appContainer)
	go func() {
		if err := httpServer.Start(); err != nil {
			logger.System().Error("HTTP server failed", "error", err.Error())
		}
	}()
	log.Printf("%s✓ HTTP Server Listening    %s(port: %s, started in %s)%s", cyan, darkGrey, port, time.Since(stepStart), reset)

	// --- Startup Complete ---
	log.Printf("\n%sApplication startup complete %s(%s total)%s", blue, darkGrey, time.Since(totalStart), reset)
	log.Printf("%sServer is now accepting connections at %shttp://localhost:%s%s", white, green, port, reset)

	// --- Graceful Shutdown Sequence ---
	gracefulShutdown := make(chan os.Signal, 1)
	signal.Notify(gracefulShutdown, syscall.SIGINT, syscall.SIGTERM)
	<-gracefulShutdown

	shutdownStart := time.Now()
	log.Printf("\n%s[Shutdown]%s Signal received, starting graceful shutdown...", yellow, reset)
	cancelBackgroundTasks()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Stop(shutdownCtx); err != nil {
		logger.Shutdown().Error("Error during server shutdown", "error", err.Error())
	} else {
		log.Printf("%s[Shutdown]%s HTTP server stopped.", yellow, reset)
	}

	if err := tenantManager.Close(); err != nil {
		logger.Shutdown().Error("Error closing tenant manager", "error", err.Error())
	} else {
		log.Printf("%s[Shutdown]%s Tenant manager closed.", yellow, reset)
	}

	log.Printf("%s[Shutdown]%s Complete. Total uptime: %s, shutdown duration: %s.", yellow, reset,
		time.Since(totalStart).Round(time.Second), time.Since(shutdownStart))

	return nil
}

// setupLogging configures application logging
func setupLogging() {
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	log.SetFlags(0) // Remove flags to allow for custom formatting
}
