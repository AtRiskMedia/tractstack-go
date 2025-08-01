// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"sort"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

type UserCount struct {
	ID      string `json:"id"`
	Count   int    `json:"count"`
	IsKnown bool   `json:"isKnown"`
}

type SankeyFilters struct {
	VisitorType    string  `json:"visitorType"`
	SelectedUserID *string `json:"selectedUserID,omitempty"`
	StartHour      *int    `json:"startHour,omitempty"`
	EndHour        *int    `json:"endHour,omitempty"`
}

type AnalyticsService struct{}

func NewAnalyticsService() *AnalyticsService {
	return &AnalyticsService{}
}

func (s *AnalyticsService) GetFilteredVisitorCounts(tenantCtx *tenant.Context, epinetID string, visitorType string, startHour, endHour *int) ([]UserCount, error) {
	var hourKeys []string
	if startHour != nil && endHour != nil {
		hourKeys = s.getHourKeysForCustomRange(*startHour, *endHour)
	} else {
		hourKeys = s.getHourKeysForTimeRange(168)
	}

	knownFingerprints, err := s.getKnownFingerprints(tenantCtx)
	if err != nil {
		return nil, err
	}

	visitorEventCount := make(map[string]int)
	for _, hourKey := range hourKeys {
		bin, exists := tenantCtx.CacheManager.GetHourlyEpinetBin(tenantCtx.TenantID, epinetID, hourKey)
		if !exists {
			continue
		}
		for _, stepData := range bin.Data.Steps {
			for visitorID := range stepData.Visitors {
				if s.shouldIncludeVisitor(visitorID, &SankeyFilters{VisitorType: visitorType}, knownFingerprints) {
					visitorEventCount[visitorID]++
				}
			}
		}
	}

	var userCounts []UserCount
	for id, count := range visitorEventCount {
		userCounts = append(userCounts, UserCount{
			ID:      id,
			Count:   count,
			IsKnown: knownFingerprints[id],
		})
	}

	sort.Slice(userCounts, func(i, j int) bool {
		if userCounts[i].Count != userCounts[j].Count {
			return userCounts[i].Count > userCounts[j].Count
		}
		return userCounts[i].ID < userCounts[j].ID
	})

	return userCounts, nil
}

func (s *AnalyticsService) shouldIncludeVisitor(visitorID string, filters *SankeyFilters, knownFingerprints map[string]bool) bool {
	if filters == nil {
		return true
	}

	if filters.SelectedUserID != nil && *filters.SelectedUserID != visitorID {
		return false
	}

	isKnown := knownFingerprints[visitorID]
	switch filters.VisitorType {
	case "known":
		return isKnown
	case "anonymous":
		return !isKnown
	default:
		return true
	}
}

func (s *AnalyticsService) getKnownFingerprints(tenantCtx *tenant.Context) (map[string]bool, error) {
	query := `SELECT id, CASE WHEN lead_id IS NOT NULL THEN 1 ELSE 0 END as is_known FROM fingerprints`
	rows, err := tenantCtx.Database.Conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	knownFingerprints := make(map[string]bool)
	for rows.Next() {
		var fingerprintID string
		var isKnown bool
		if err := rows.Scan(&fingerprintID, &isKnown); err != nil {
			return nil, err
		}
		knownFingerprints[fingerprintID] = isKnown
	}
	return knownFingerprints, nil
}

func (s *AnalyticsService) getHourKeysForTimeRange(hoursBack int) []string {
	hourKeys := make([]string, hoursBack)
	now := time.Now().UTC()

	for i := 0; i < hoursBack; i++ {
		hourTime := now.Add(-time.Duration(i) * time.Hour)
		hourKeys[i] = hourTime.Format("2006-01-02-15")
	}

	return hourKeys
}

func (s *AnalyticsService) getHourKeysForCustomRange(startHour, endHour int) []string {
	if startHour <= endHour {
		return []string{}
	}

	hourKeys := make([]string, startHour-endHour)
	now := time.Now().UTC()

	for i := 0; i < startHour-endHour; i++ {
		hourTime := now.Add(-time.Duration(endHour+i) * time.Hour)
		hourKeys[i] = hourTime.Format("2006-01-02-15")
	}

	return hourKeys
}
