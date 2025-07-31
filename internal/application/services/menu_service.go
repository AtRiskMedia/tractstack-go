// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/repositories"
)

// MenuService orchestrates menu operations with cache-first repository pattern
type MenuService struct {
	menuRepo repositories.MenuRepository
}

// NewMenuService creates a new menu application service
func NewMenuService(menuRepo repositories.MenuRepository) *MenuService {
	return &MenuService{
		menuRepo: menuRepo,
	}
}

// GetAllIDs returns all menu IDs for a tenant (cache-first)
func (s *MenuService) GetAllIDs(tenantID string) ([]string, error) {
	menus, err := s.menuRepo.FindAll(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get all menus: %w", err)
	}

	ids := make([]string, len(menus))
	for i, menu := range menus {
		ids[i] = menu.ID
	}

	return ids, nil
}

// GetByID returns a menu by ID (cache-first)
func (s *MenuService) GetByID(tenantID, id string) (*content.MenuNode, error) {
	if id == "" {
		return nil, fmt.Errorf("menu ID cannot be empty")
	}

	menu, err := s.menuRepo.FindByID(tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get menu %s: %w", id, err)
	}

	return menu, nil
}

// GetByIDs returns multiple menus by IDs (cache-first with bulk loading)
func (s *MenuService) GetByIDs(tenantID string, ids []string) ([]*content.MenuNode, error) {
	if len(ids) == 0 {
		return []*content.MenuNode{}, nil
	}

	menus, err := s.menuRepo.FindByIDs(tenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get menus by IDs: %w", err)
	}

	return menus, nil
}

// Create creates a new menu
func (s *MenuService) Create(tenantID string, menu *content.MenuNode) error {
	if menu == nil {
		return fmt.Errorf("menu cannot be nil")
	}
	if menu.ID == "" {
		return fmt.Errorf("menu ID cannot be empty")
	}
	if menu.Title == "" {
		return fmt.Errorf("menu title cannot be empty")
	}

	err := s.menuRepo.Store(tenantID, menu)
	if err != nil {
		return fmt.Errorf("failed to create menu %s: %w", menu.ID, err)
	}

	return nil
}

// Update updates an existing menu
func (s *MenuService) Update(tenantID string, menu *content.MenuNode) error {
	if menu == nil {
		return fmt.Errorf("menu cannot be nil")
	}
	if menu.ID == "" {
		return fmt.Errorf("menu ID cannot be empty")
	}
	if menu.Title == "" {
		return fmt.Errorf("menu title cannot be empty")
	}

	// Verify menu exists before updating
	existing, err := s.menuRepo.FindByID(tenantID, menu.ID)
	if err != nil {
		return fmt.Errorf("failed to verify menu %s exists: %w", menu.ID, err)
	}
	if existing == nil {
		return fmt.Errorf("menu %s not found", menu.ID)
	}

	err = s.menuRepo.Update(tenantID, menu)
	if err != nil {
		return fmt.Errorf("failed to update menu %s: %w", menu.ID, err)
	}

	return nil
}

// Delete deletes a menu
func (s *MenuService) Delete(tenantID, id string) error {
	if id == "" {
		return fmt.Errorf("menu ID cannot be empty")
	}

	// Verify menu exists before deleting
	existing, err := s.menuRepo.FindByID(tenantID, id)
	if err != nil {
		return fmt.Errorf("failed to verify menu %s exists: %w", id, err)
	}
	if existing == nil {
		return fmt.Errorf("menu %s not found", id)
	}

	err = s.menuRepo.Delete(tenantID, id)
	if err != nil {
		return fmt.Errorf("failed to delete menu %s: %w", id, err)
	}

	return nil
}
