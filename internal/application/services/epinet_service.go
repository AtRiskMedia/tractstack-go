// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/repositories"
)

// EpinetService orchestrates epinet operations with cache-first repository pattern
type EpinetService struct {
	epinetRepo repositories.EpinetRepository
}

// NewEpinetService creates a new epinet application service
func NewEpinetService(epinetRepo repositories.EpinetRepository) *EpinetService {
	return &EpinetService{
		epinetRepo: epinetRepo,
	}
}

// GetAllIDs returns all epinet IDs for a tenant (cache-first)
func (s *EpinetService) GetAllIDs(tenantID string) ([]string, error) {
	epinets, err := s.epinetRepo.FindAll(tenantID)
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
func (s *EpinetService) GetByID(tenantID, id string) (*content.EpinetNode, error) {
	if id == "" {
		return nil, fmt.Errorf("epinet ID cannot be empty")
	}

	epinet, err := s.epinetRepo.FindByID(tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get epinet %s: %w", id, err)
	}

	return epinet, nil
}

// GetByIDs returns multiple epinets by IDs (cache-first with bulk loading)
func (s *EpinetService) GetByIDs(tenantID string, ids []string) ([]*content.EpinetNode, error) {
	if len(ids) == 0 {
		return []*content.EpinetNode{}, nil
	}

	epinets, err := s.epinetRepo.FindByIDs(tenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get epinets by IDs: %w", err)
	}

	return epinets, nil
}
