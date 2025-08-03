// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"fmt"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// MenuService orchestrates menu operations with cache-first repository pattern
type MenuService struct {
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// NewMenuService creates a new menu service singleton
func NewMenuService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *MenuService {
	return &MenuService{
		logger:      logger,
		perfTracker: perfTracker,
	}
}

// GetAllIDs returns all menu IDs for a tenant by leveraging the robust repository.
func (s *MenuService) GetAllIDs(tenantCtx *tenant.Context) ([]string, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_all_menu_ids", tenantCtx.TenantID)
	defer marker.Complete()
	menuRepo := tenantCtx.MenuRepo()

	// The repository's FindAll method is now the cache-aware entry point.
	// It will handle the full cache-miss-fallback-and-set logic.
	menus, err := menuRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get all menus from repository: %w", err)
	}

	// We just need to extract the IDs from the full objects.
	ids := make([]string, len(menus))
	for i, menu := range menus {
		ids[i] = menu.ID
	}

	s.logger.Content().Info("Successfully retrieved all menu IDs",
		"tenantId", tenantCtx.TenantID,
		"count", len(ids),
		"duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetAllMenuIDs", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	return ids, nil
}

// GetByID returns a menu by ID (cache-first via repository)
func (s *MenuService) GetByID(tenantCtx *tenant.Context, id string) (*content.MenuNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_menu_by_id", tenantCtx.TenantID)
	defer marker.Complete()
	if id == "" {
		return nil, fmt.Errorf("menu ID cannot be empty")
	}

	menuRepo := tenantCtx.MenuRepo()
	menu, err := menuRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get menu %s: %w", id, err)
	}

	s.logger.Content().Info("Successfully retrieved menu by ID",
		"tenantId", tenantCtx.TenantID,
		"menuId", id,
		"found", menu != nil,
		"duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetMenuByID", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "menuId", id)

	return menu, nil
}

// GetByIDs returns multiple menus by IDs (cache-first with bulk loading via repository)
func (s *MenuService) GetByIDs(tenantCtx *tenant.Context, ids []string) ([]*content.MenuNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_menus_by_ids", tenantCtx.TenantID)
	defer marker.Complete()
	if len(ids) == 0 {
		return []*content.MenuNode{}, nil
	}

	menuRepo := tenantCtx.MenuRepo()
	menus, err := menuRepo.FindByIDs(tenantCtx.TenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get menus by IDs from repository: %w", err)
	}

	s.logger.Content().Info("Successfully retrieved menus by IDs", "tenantId", tenantCtx.TenantID, "requestedCount", len(ids), "foundCount", len(menus), "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetMenusByIDs", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "requestedCount", len(ids))

	return menus, nil
}

// Create creates a new menu
func (s *MenuService) Create(tenantCtx *tenant.Context, menu *content.MenuNode) error {
	start := time.Now()
	marker := s.perfTracker.StartOperation("create_menu", tenantCtx.TenantID)
	defer marker.Complete()
	if menu == nil {
		return fmt.Errorf("menu cannot be nil")
	}
	if menu.ID == "" {
		return fmt.Errorf("menu ID cannot be empty")
	}
	if menu.Title == "" {
		return fmt.Errorf("menu title cannot be empty")
	}

	menuRepo := tenantCtx.MenuRepo()
	err := menuRepo.Store(tenantCtx.TenantID, menu)
	if err != nil {
		return fmt.Errorf("failed to create menu %s: %w", menu.ID, err)
	}

	s.logger.Content().Info("Successfully created menu", "tenantId", tenantCtx.TenantID, "menuId", menu.ID, "title", menu.Title, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for CreateMenu", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "menuId", menu.ID)

	return nil
}

// Update updates an existing menu
func (s *MenuService) Update(tenantCtx *tenant.Context, menu *content.MenuNode) error {
	start := time.Now()
	marker := s.perfTracker.StartOperation("update_menu", tenantCtx.TenantID)
	defer marker.Complete()
	if menu == nil {
		return fmt.Errorf("menu cannot be nil")
	}
	if menu.ID == "" {
		return fmt.Errorf("menu ID cannot be empty")
	}
	if menu.Title == "" {
		return fmt.Errorf("menu title cannot be empty")
	}

	menuRepo := tenantCtx.MenuRepo()

	// Verify menu exists before updating
	existing, err := menuRepo.FindByID(tenantCtx.TenantID, menu.ID)
	if err != nil {
		return fmt.Errorf("failed to verify menu %s exists: %w", menu.ID, err)
	}
	if existing == nil {
		return fmt.Errorf("menu %s not found", menu.ID)
	}

	err = menuRepo.Update(tenantCtx.TenantID, menu)
	if err != nil {
		return fmt.Errorf("failed to update menu %s: %w", menu.ID, err)
	}

	s.logger.Content().Info("Successfully updated menu", "tenantId", tenantCtx.TenantID, "menuId", menu.ID, "title", menu.Title, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for UpdateMenu", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "menuId", menu.ID)

	return nil
}

// Delete deletes a menu
func (s *MenuService) Delete(tenantCtx *tenant.Context, id string) error {
	start := time.Now()
	marker := s.perfTracker.StartOperation("delete_menu", tenantCtx.TenantID)
	defer marker.Complete()
	if id == "" {
		return fmt.Errorf("menu ID cannot be empty")
	}

	menuRepo := tenantCtx.MenuRepo()

	// Verify menu exists before deleting
	existing, err := menuRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return fmt.Errorf("failed to verify menu %s exists: %w", id, err)
	}
	if existing == nil {
		return fmt.Errorf("menu %s not found", id)
	}

	err = menuRepo.Delete(tenantCtx.TenantID, id)
	if err != nil {
		return fmt.Errorf("failed to delete menu %s: %w", id, err)
	}

	s.logger.Content().Info("Successfully deleted menu", "tenantId", tenantCtx.TenantID, "menuId", id, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for DeleteMenu", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "menuId", id)

	return nil
}
