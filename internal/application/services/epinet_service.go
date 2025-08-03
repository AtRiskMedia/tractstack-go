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

// EpinetService orchestrates epinet operations with cache-first repository pattern
type EpinetService struct {
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// NewEpinetService creates a new epinet service singleton
func NewEpinetService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *EpinetService {
	return &EpinetService{
		logger:      logger,
		perfTracker: perfTracker,
	}
}

// GetAllIDs returns all epinet IDs for a tenant by leveraging the robust repository.
func (s *EpinetService) GetAllIDs(tenantCtx *tenant.Context) ([]string, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_all_epinet_ids", tenantCtx.TenantID)
	defer marker.Complete()
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
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetAllEpinetIDs", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	return ids, nil
}

// GetByID returns an epinet by ID (cache-first via repository)
func (s *EpinetService) GetByID(tenantCtx *tenant.Context, id string) (*content.EpinetNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_epinet_by_id", tenantCtx.TenantID)
	defer marker.Complete()
	if id == "" {
		return nil, fmt.Errorf("epinet ID cannot be empty")
	}

	epinetRepo := tenantCtx.EpinetRepo()
	epinet, err := epinetRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get epinet %s: %w", id, err)
	}

	s.logger.Content().Info("Successfully retrieved epinet by ID", "tenantId", tenantCtx.TenantID, "epinetId", id, "found", epinet != nil, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetEpinetByID", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "epinetId", id)

	return epinet, nil
}

// GetByIDs returns multiple epinets by IDs (cache-first with bulk loading via repository)
func (s *EpinetService) GetByIDs(tenantCtx *tenant.Context, ids []string) ([]*content.EpinetNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_epinets_by_ids", tenantCtx.TenantID)
	defer marker.Complete()
	if len(ids) == 0 {
		return []*content.EpinetNode{}, nil
	}

	epinetRepo := tenantCtx.EpinetRepo()
	epinets, err := epinetRepo.FindByIDs(tenantCtx.TenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get epinets by IDs from repository: %w", err)
	}

	s.logger.Content().Info("Successfully retrieved epinets by IDs", "tenantId", tenantCtx.TenantID, "requestedCount", len(ids), "foundCount", len(epinets), "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetEpinetsByIDs", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "requestedCount", len(ids))

	return epinets, nil
}
