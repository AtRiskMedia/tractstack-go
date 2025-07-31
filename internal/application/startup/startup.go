// Package startup prepares the application server
package startup

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/cleanup"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/manager"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/bulk"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/database"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/server"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
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
	activatedTenants, err := preActivateInactiveTenants()
	if err != nil {
		return fmt.Errorf("tenant pre-activation failed: %w", err)
	}

	// Step 4: Validate only the tenants that were just activated
	if len(activatedTenants) > 0 {
		log.Println("Validating newly activated tenants...")
		if err := validateNewlyActivatedTenants(activatedTenants); err != nil {
			return fmt.Errorf("newly activated tenant validation failed: %w", err)
		}
	}

	// Step 5: Verify ALL active tenant database connections and log modes
	log.Println("Verifying active tenant database connections...")
	if err := verifyActiveTenantConnections(); err != nil {
		return fmt.Errorf("active tenant connection verification failed: %w", err)
	}

	// Step 6: Initialize cache system
	log.Println("Initializing cache system...")
	cacheManager := manager.NewManager()

	for tenantID, tenantInfo := range registry.Tenants {
		if tenantInfo.Status == "active" {
			log.Printf("  Initializing cache for tenant: %s", tenantID)
			cacheManager.InitializeTenant(tenantID)
		}
	}

	// Step 7: Initialize application services THAT ARE TRUE SINGLETONS
	log.Println("Initializing singleton application services...")
	defaultCtx, err := tenantManager.NewContextFromID("default")
	if err != nil {
		return fmt.Errorf("failed to create bootstrap context for default tenant: %w", err)
	}
	defer defaultCtx.Close()

	bootstrapDB := &database.DB{DB: defaultCtx.Database.Conn}
	bulkRepo := bulk.NewRepository(bootstrapDB)
	contentMapService := services.NewContentMapService(bulkRepo)

	log.Println("✓ Singleton application services initialized.")

	// Step 8: Initialize cache warming
	log.Println("Initializing cache warming...")
	reporter := cleanup.NewReporter(cacheManager)
	// Warming service creates its own tenant contexts, but needs one repo instance for initialization.
	beliefRepoForWarming := content.NewBeliefRepository(bootstrapDB.DB, cacheManager)
	warmingService := services.NewWarmingService(cacheManager, bulkRepo, contentMapService, nil, beliefRepoForWarming, reporter)
	if err := warmingService.WarmAllTenants(); err != nil {
		log.Printf("WARNING: Cache warming failed: %v", err)
	}

	// Step 9: Start background cleanup worker
	log.Println("Starting background cleanup worker...")
	cleanupConfig := cleanup.NewConfig()
	cleanupWorker := cleanup.NewWorker(cacheManager, tenantManager.GetDetector(), cleanupConfig)
	go cleanupWorker.Start(ctx)
	log.Println("✓ Background cleanup worker started.")

	// Step 10: Setup and start HTTP server
	log.Println("Setting up HTTP server...")
	httpServer := server.New(config.Port, tenantManager)

	elapsed := time.Since(start)
	log.Printf("=== Application startup complete in %v ===", elapsed)

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- httpServer.Start()
	}()

	log.Printf("Application is ready to serve requests on :%s", config.Port)

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		return fmt.Errorf("server error: %w", err)
	case sig := <-shutdown:
		log.Printf("Received signal %v, starting graceful shutdown...", sig)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		cancelBackgroundTasks()

		if err := httpServer.Stop(shutdownCtx); err != nil {
			return fmt.Errorf("failed to stop server gracefully: %w", err)
		}

		log.Println("Server stopped gracefully")
	}

	return nil
}

func setupLogging() {
	if os.Getenv("ENV") == "production" {
		gin.SetMode(gin.ReleaseMode)
		logDir := filepath.Join(os.Getenv("HOME"), "t8k-go-server", "log")
		if err := os.MkdirAll(logDir, 0755); err != nil {
			log.Fatalf("Failed to create log directory: %v", err)
		}
		logFile, err := os.OpenFile(
			filepath.Join(logDir, "tractstack.log"),
			os.O_CREATE|os.O_WRONLY|os.O_APPEND,
			0644,
		)
		if err != nil {
			log.Fatalf("Failed to open log file: %v", err)
		}
		log.SetOutput(logFile)
		log.Println("Production logging initialized.")
	}
}

// preActivateInactiveTenants activates only inactive tenants and returns list of activated tenant IDs
func preActivateInactiveTenants() ([]string, error) {
	registry, err := tenant.LoadTenantRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to load tenant registry for pre-activation: %w", err)
	}

	if len(registry.Tenants) == 0 {
		log.Println("✓ No tenants found in registry")
		return []string{}, nil
	}

	inactiveTenants := make([]string, 0)
	for tenantID, tenantInfo := range registry.Tenants {
		if tenantInfo.Status != "active" && tenantInfo.Status != "reserved" {
			inactiveTenants = append(inactiveTenants, tenantID)
		}
	}

	if len(inactiveTenants) == 0 {
		log.Println("✓ No tenants required pre-activation")
		return []string{}, nil
	}

	log.Println("=== Starting tenant pre-activation ===")
	start := time.Now().UTC()
	log.Printf("Found %d tenants requiring activation", len(inactiveTenants))

	activatedTenants := make([]string, 0)
	failedTenants := make([]string, 0)

	for _, tenantID := range inactiveTenants {
		log.Printf("Pre-activating tenant: '%s'", tenantID)

		if err := preActivateSingleTenant(tenantID); err != nil {
			log.Printf("ERROR: Failed to pre-activate tenant '%s': %v", tenantID, err)
			failedTenants = append(failedTenants, tenantID)
			continue
		}

		activatedTenants = append(activatedTenants, tenantID)
		log.Printf("✓ Successfully pre-activated tenant: '%s'", tenantID)
	}

	elapsed := time.Since(start)
	log.Printf("=== Pre-activation complete in %v ===", elapsed)
	log.Printf("  - Activated: %d tenants", len(activatedTenants))
	log.Printf("  - Failed: %d tenants", len(failedTenants))

	if len(failedTenants) > 0 {
		log.Printf("Failed tenants: %v", failedTenants)
		return nil, fmt.Errorf("pre-activation failed for %d tenants: %v", len(failedTenants), failedTenants)
	}

	log.Printf("✓ Successfully pre-activated %d tenants: %v", len(activatedTenants), activatedTenants)
	return activatedTenants, nil
}

// preActivateSingleTenant activates a single tenant during startup
func preActivateSingleTenant(tenantID string) error {
	tenantStart := time.Now().UTC()

	config, err := loadTenantConfig(tenantID)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	registry, err := tenant.LoadTenantRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	tenantInfo, exists := registry.Tenants[tenantID]
	if !exists {
		return fmt.Errorf("tenant %s not found in registry", tenantID)
	}

	if tenantInfo.Status == "reserved" {
		log.Printf("  - Tenant '%s' has reserved status - skipping activation", tenantID)
		return nil
	}

	database, err := tenant.NewDatabase(config)
	if err != nil {
		return fmt.Errorf("failed to create database connection: %w", err)
	}
	defer database.Close()

	if err := database.Conn.Ping(); err != nil {
		return fmt.Errorf("database connection test failed: %w", err)
	}

	connType := "SQLite"
	if database.UseTurso {
		connType = "Turso"
	}
	log.Printf("  - Database connection (%s): %s", connType, database.GetConnectionInfo())

	elapsed := time.Since(tenantStart)
	log.Printf("  - Tenant '%s' activated in %v", tenantID, elapsed)
	return nil
}

// loadTenantConfig loads configuration for a specific tenant
func loadTenantConfig(tenantID string) (*tenant.Config, error) {
	return tenant.LoadTenantConfig(tenantID)
}

// validateNewlyActivatedTenants verifies only the tenants that were just activated
func validateNewlyActivatedTenants(activatedTenants []string) error {
	log.Printf("=== Validating %d newly activated tenants ===", len(activatedTenants))

	registry, err := tenant.LoadTenantRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry for validation: %w", err)
	}

	stillInactiveTenants := make([]string, 0)

	for _, tenantID := range activatedTenants {
		tenantInfo, exists := registry.Tenants[tenantID]
		if !exists {
			stillInactiveTenants = append(stillInactiveTenants, tenantID)
			continue
		}

		if tenantInfo.Status != "active" {
			stillInactiveTenants = append(stillInactiveTenants, tenantID)
		}
	}

	if len(stillInactiveTenants) > 0 {
		log.Printf("Validation failed - tenants still inactive: %v", stillInactiveTenants)
		return fmt.Errorf("validation failed - %d tenants still inactive: %v",
			len(stillInactiveTenants), stillInactiveTenants)
	}

	log.Printf("✓ Validation passed - all %d newly activated tenants are now active", len(activatedTenants))
	return nil
}

// verifyActiveTenantConnections tests database connections for ALL active tenants and logs their modes
func verifyActiveTenantConnections() error {
	start := time.Now().UTC()

	registry, err := tenant.LoadTenantRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry for connection verification: %w", err)
	}

	activeTenants := make([]string, 0)
	for tenantID, tenantInfo := range registry.Tenants {
		if tenantInfo.Status == "active" {
			activeTenants = append(activeTenants, tenantID)
		}
	}

	if len(activeTenants) == 0 {
		log.Println("No active tenants to verify")
		return nil
	}

	log.Printf("Testing database connections for %d active tenants...", len(activeTenants))

	tursoCount := 0
	sqliteCount := 0
	failedTenants := make([]string, 0)

	for _, tenantID := range activeTenants {
		config, err := loadTenantConfig(tenantID)
		if err != nil {
			log.Printf("ERROR: Failed to load config for tenant '%s': %v", tenantID, err)
			failedTenants = append(failedTenants, tenantID)
			continue
		}

		database, err := tenant.NewDatabase(config)
		if err != nil {
			log.Printf("ERROR: Failed to create database connection for tenant '%s': %v", tenantID, err)
			failedTenants = append(failedTenants, tenantID)
			continue
		}

		if err := database.Conn.Ping(); err != nil {
			log.Printf("ERROR: Database ping failed for tenant '%s': %v", tenantID, err)
			database.Close()
			failedTenants = append(failedTenants, tenantID)
			continue
		}

		if database.UseTurso {
			tursoCount++
		} else {
			sqliteCount++
		}

		log.Printf("✓ Tenant '%s': %s", tenantID, database.GetConnectionInfo())
		database.Close()
	}

	elapsed := time.Since(start)

	if len(failedTenants) > 0 {
		log.Printf("Connection verification failed for %d tenants: %v", len(failedTenants), failedTenants)
		return fmt.Errorf("connection verification failed for %d tenants: %v", len(failedTenants), failedTenants)
	}

	log.Printf("=== Connection verification complete in %v ===", elapsed)
	log.Printf("✓ All %d active tenants have working database connections", len(activeTenants))
	log.Printf("  - Turso: %d tenants", tursoCount)
	log.Printf("  - SQLite: %d tenants", sqliteCount)

	return nil
}
