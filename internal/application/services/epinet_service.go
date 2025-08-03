// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"fmt"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// EpinetService orchestrates epinet operations with cache-first repository pattern
type EpinetService struct {
	logger *logging.ChanneledLogger
}

// NewEpinetService creates a new epinet service singleton
func NewEpinetService(logger *logging.ChanneledLogger) *EpinetService {
	return &EpinetService{
		logger: logger,
	}
}

// GetAllIDs returns all epinet IDs for a tenant by leveraging the robust repository.
func (s *EpinetService) GetAllIDs(tenantCtx *tenant.Context) ([]string, error) {
	start := time.Now()
	epinetRepo := tenantCtx.EpinetRepo()

	// The repository's FindAll method is now the cache-aware entry point.
	epinets, err := epinetRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get all epinets from repository: %w", err)
	}

	// Extract IDs from the full objects.
	ids := make([]string, len(epinets))
	for i, epinet := range epinets {
		ids[i] = epinet.ID
	}

	s.logger.Content().Info("Successfully retrieved all epinet IDs", "tenantId", tenantCtx.TenantID, "count", len(ids), "duration", time.Since(start))

	return ids, nil
}

// GetByID returns an epinet by ID (cache-first via repository)
func (s *EpinetService) GetByID(tenantCtx *tenant.Context, id string) (*content.EpinetNode, error) {
	start := time.Now()
	if id == "" {
		return nil, fmt.Errorf("epinet ID cannot be empty")
	}

	epinetRepo := tenantCtx.EpinetRepo()
	epinet, err := epinetRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get epinet %s: %w", id, err)
	}

	s.logger.Content().Info("Successfully retrieved epinet by ID", "tenantId", tenantCtx.TenantID, "epinetId", id, "found", epinet != nil, "duration", time.Since(start))

	return epinet, nil
}

// GetByIDs returns multiple epinets by IDs (cache-first with bulk loading via repository)
func (s *EpinetService) GetByIDs(tenantCtx *tenant.Context, ids []string) ([]*content.EpinetNode, error) {
	start := time.Now()
	if len(ids) == 0 {
		return []*content.EpinetNode{}, nil
	}

	epinetRepo := tenantCtx.EpinetRepo()
	epinets, err := epinetRepo.FindByIDs(tenantCtx.TenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get epinets by IDs from repository: %w", err)
	}

	s.logger.Content().Info("Successfully retrieved epinets by IDs", "tenantId", tenantCtx.TenantID, "requestedCount", len(ids), "foundCount", len(epinets), "duration", time.Since(start))

	return epinets, nil
}
