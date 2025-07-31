// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// EpinetService orchestrates epinet operations with cache-first repository pattern
type EpinetService struct {
	// No stored dependencies - all passed via tenant context
}

// NewEpinetService creates a new epinet service singleton
func NewEpinetService() *EpinetService {
	return &EpinetService{}
}

// GetAllIDs returns all epinet IDs for a tenant (cache-first)
func (s *EpinetService) GetAllIDs(tenantCtx *tenant.Context) ([]string, error) {
	epinetRepo := tenantCtx.EpinetRepo()

	epinets, err := epinetRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get all epinets: %w", err)
	}

	ids := make([]string, len(epinets))
	for i, epinet := range epinets {
		ids[i] = epinet.ID
	}

	return ids, nil
}

// GetByID returns an epinet by ID (cache-first)
func (s *EpinetService) GetByID(tenantCtx *tenant.Context, id string) (*content.EpinetNode, error) {
	if id == "" {
		return nil, fmt.Errorf("epinet ID cannot be empty")
	}

	epinetRepo := tenantCtx.EpinetRepo()
	epinet, err := epinetRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get epinet %s: %w", id, err)
	}

	return epinet, nil
}

// GetByIDs returns multiple epinets by IDs (cache-first with bulk loading)
func (s *EpinetService) GetByIDs(tenantCtx *tenant.Context, ids []string) ([]*content.EpinetNode, error) {
	if len(ids) == 0 {
		return []*content.EpinetNode{}, nil
	}

	epinetRepo := tenantCtx.EpinetRepo()
	epinets, err := epinetRepo.FindByIDs(tenantCtx.TenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get epinets by IDs: %w", err)
	}

	return epinets, nil
}
