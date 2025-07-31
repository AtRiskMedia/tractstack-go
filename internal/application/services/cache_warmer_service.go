// Package services provides cache warming orchestration
package services

import (
	"fmt"
	"log"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// CacheWarmerService orchestrates cache warming operations
type CacheWarmerService struct {
	// No stored dependencies - all passed via tenant context
}

// NewCacheWarmerService creates a new cache warmer service singleton
func NewCacheWarmerService() *CacheWarmerService {
	return &CacheWarmerService{}
}

// WarmHourlyEpinetData warms epinet analytics data for the specified hours back
func (cws *CacheWarmerService) WarmHourlyEpinetData(tenantCtx *tenant.Context, hoursBack int) error {
	// FIXED: Removed unused variables that were causing compilation errors
	// Previously declared but unused:
	// epinetRepo := tenantCtx.EpinetRepo()
	// paneRepo := tenantCtx.PaneRepo()
	// storyFragmentRepo := tenantCtx.StoryFragmentRepo()

	log.Printf("Starting cache warming for tenant %s, %d hours back", tenantCtx.TenantID, hoursBack)

	// Calculate hour range to warm
	now := time.Now().UTC()
	startTime := now.Add(-time.Duration(hoursBack) * time.Hour)

	// Generate hour keys for the range
	hourKeys := make([]string, 0, hoursBack)
	for i := 0; i < hoursBack; i++ {
		hourTime := startTime.Add(time.Duration(i) * time.Hour)
		hourKey := hourTime.Format("2006-01-02-15")
		hourKeys = append(hourKeys, hourKey)
	}

	// Warm epinet data using analytics cache manager
	for _, hourKey := range hourKeys {
		err := cws.warmEpinetHour(tenantCtx, hourKey)
		if err != nil {
			log.Printf("Failed to warm epinet data for hour %s: %v", hourKey, err)
			// Continue with other hours even if one fails
		}
	}

	// Update the last full hour in analytics cache
	lastHour := hourKeys[len(hourKeys)-1]
	tenantCtx.CacheManager.UpdateLastFullHour(tenantCtx.TenantID, lastHour)

	log.Printf("Completed cache warming for tenant %s", tenantCtx.TenantID)
	return nil
}

// warmEpinetHour warms cache data for a specific hour
func (cws *CacheWarmerService) warmEpinetHour(tenantCtx *tenant.Context, hourKey string) error {
	// This would implement the actual warming logic
	// For now, just log the operation
	log.Printf("Warming epinet data for hour %s", hourKey)

	// TODO: Implement actual cache warming:
	// 1. Query analytics data for the hour
	// 2. Aggregate into bins
	// 3. Store in analytics cache

	return nil
}

// WarmContentCache warms content cache with frequently accessed items
func (cws *CacheWarmerService) WarmContentCache(tenantCtx *tenant.Context) error {
	log.Printf("Starting content cache warming for tenant %s", tenantCtx.TenantID)

	// Use repository factory pattern from tenant context
	tractStackRepo := tenantCtx.TractStackRepo()
	storyFragmentRepo := tenantCtx.StoryFragmentRepo()
	menuRepo := tenantCtx.MenuRepo()

	// Pre-load frequently accessed content
	tractStacks, err := tractStackRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return fmt.Errorf("failed to warm tractstack cache: %w", err)
	}
	log.Printf("Warmed %d tractstacks", len(tractStacks))

	storyFragments, err := storyFragmentRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return fmt.Errorf("failed to warm storyfragment cache: %w", err)
	}
	log.Printf("Warmed %d storyfragments", len(storyFragments))

	menus, err := menuRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return fmt.Errorf("failed to warm menu cache: %w", err)
	}
	log.Printf("Warmed %d menus", len(menus))

	log.Printf("Completed content cache warming for tenant %s", tenantCtx.TenantID)
	return nil
}

// WarmUserStateCache initializes user state cache structures
func (cws *CacheWarmerService) WarmUserStateCache(tenantCtx *tenant.Context) error {
	log.Printf("Starting user state cache warming for tenant %s", tenantCtx.TenantID)

	// Initialize cache structures through manager
	// The cache manager will handle proper initialization
	tenantCtx.CacheManager.InitializeTenant(tenantCtx.TenantID)

	log.Printf("Completed user state cache warming for tenant %s", tenantCtx.TenantID)
	return nil
}
