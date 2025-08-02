package services

import (
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

type LeadMetrics struct {
	TotalLeads       int            `json:"totalLeads"`
	NewLeads         int            `json:"newLeads"`
	ConversionRate   float64        `json:"conversionRate"`
	LeadSources      map[string]int `json:"leadSources"`
	ConversionFunnel map[string]int `json:"conversionFunnel"`
	Attribution      map[string]any `json:"attribution"`
}

type LeadAnalyticsService struct{}

func NewLeadAnalyticsService() *LeadAnalyticsService {
	return &LeadAnalyticsService{}
}

func (s *LeadAnalyticsService) ComputeLeadMetrics(tenantCtx *tenant.Context, startHour, endHour int) (*LeadMetrics, error) {
	hourKeys := s.getHourKeysForCustomRange(startHour, endHour)

	totalVisitors := s.getTotalVisitors(tenantCtx, hourKeys)
	totalLeads := s.getTotalLeads(tenantCtx)
	newLeads := s.getNewLeads(tenantCtx, hourKeys)

	var conversionRate float64
	if totalVisitors > 0 {
		conversionRate = float64(totalLeads) / float64(totalVisitors) * 100
	}

	leadSources := s.getLeadSources(tenantCtx)
	conversionFunnel := s.getConversionFunnel(tenantCtx, hourKeys)
	attribution := s.getAttribution(tenantCtx)

	return &LeadMetrics{
		TotalLeads:       totalLeads,
		NewLeads:         newLeads,
		ConversionRate:   conversionRate,
		LeadSources:      leadSources,
		ConversionFunnel: conversionFunnel,
		Attribution:      attribution,
	}, nil
}

func (s *LeadAnalyticsService) getTotalVisitors(tenantCtx *tenant.Context, hourKeys []string) int {
	uniqueVisitors := make(map[string]bool)

	epinetRepo := tenantCtx.EpinetRepo()
	epinets, err := epinetRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return 0
	}

	for _, epinet := range epinets {
		if epinet == nil {
			continue
		}
		for _, hourKey := range hourKeys {
			bin, exists := tenantCtx.CacheManager.GetHourlyEpinetBin(tenantCtx.TenantID, epinet.ID, hourKey)
			if !exists {
				continue
			}
			for _, stepData := range bin.Data.Steps {
				for visitorID := range stepData.Visitors {
					uniqueVisitors[visitorID] = true
				}
			}
		}
	}

	return len(uniqueVisitors)
}

func (s *LeadAnalyticsService) getTotalLeads(tenantCtx *tenant.Context) int {
	query := `SELECT COUNT(DISTINCT fingerprint_id) FROM fingerprints WHERE lead_id IS NOT NULL`

	var count int
	err := tenantCtx.Database.Conn.QueryRow(query).Scan(&count)
	if err != nil {
		return 0
	}

	return count
}

func (s *LeadAnalyticsService) getNewLeads(tenantCtx *tenant.Context, hourKeys []string) int {
	if len(hourKeys) == 0 {
		return 0
	}

	oldestHourKey := hourKeys[len(hourKeys)-1]
	oldestTime, err := time.Parse("2006-01-02-15", oldestHourKey)
	if err != nil {
		return 0
	}

	query := `SELECT COUNT(*) FROM leads WHERE created_at >= ?`

	var count int
	err = tenantCtx.Database.Conn.QueryRow(query, oldestTime.Format("2006-01-02 15:04:05")).Scan(&count)
	if err != nil {
		return 0
	}

	return count
}

func (s *LeadAnalyticsService) getLeadSources(tenantCtx *tenant.Context) map[string]int {
	leadSources := make(map[string]int)

	query := `
		SELECT COALESCE(v.campaign_id, 'direct') as source, COUNT(*) as count
		FROM fingerprints f
		LEFT JOIN visits v ON f.id = v.fingerprint_id
		WHERE f.lead_id IS NOT NULL
		GROUP BY COALESCE(v.campaign_id, 'direct')
	`

	rows, err := tenantCtx.Database.Conn.Query(query)
	if err != nil {
		return leadSources
	}
	defer rows.Close()

	for rows.Next() {
		var source string
		var count int
		if err := rows.Scan(&source, &count); err != nil {
			continue
		}
		leadSources[source] = count
	}

	return leadSources
}

func (s *LeadAnalyticsService) getConversionFunnel(tenantCtx *tenant.Context, hourKeys []string) map[string]int {
	funnel := map[string]int{
		"visitors":  s.getTotalVisitors(tenantCtx, hourKeys),
		"engaged":   s.getEngagedVisitors(tenantCtx, hourKeys),
		"leads":     s.getTotalLeads(tenantCtx),
		"activated": s.getActivatedLeads(tenantCtx),
	}

	return funnel
}

func (s *LeadAnalyticsService) getEngagedVisitors(tenantCtx *tenant.Context, hourKeys []string) int {
	engagedVisitors := make(map[string]bool)

	epinetRepo := tenantCtx.EpinetRepo()
	epinets, err := epinetRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return 0
	}

	for _, epinet := range epinets {
		if epinet == nil {
			continue
		}
		for _, hourKey := range hourKeys {
			bin, exists := tenantCtx.CacheManager.GetHourlyEpinetBin(tenantCtx.TenantID, epinet.ID, hourKey)
			if !exists {
				continue
			}

			visitorEventCount := make(map[string]int)
			for _, stepData := range bin.Data.Steps {
				for visitorID := range stepData.Visitors {
					visitorEventCount[visitorID]++
				}
			}

			for visitorID, eventCount := range visitorEventCount {
				if eventCount >= 3 {
					engagedVisitors[visitorID] = true
				}
			}
		}
	}

	return len(engagedVisitors)
}

func (s *LeadAnalyticsService) getActivatedLeads(tenantCtx *tenant.Context) int {
	query := `
		SELECT COUNT(DISTINCT f.id)
		FROM fingerprints f
		JOIN leads l ON f.lead_id = l.id
		WHERE f.lead_id IS NOT NULL
		AND l.first_name != ''
		AND l.email != ''
	`

	var count int
	err := tenantCtx.Database.Conn.QueryRow(query).Scan(&count)
	if err != nil {
		return 0
	}

	return count
}

func (s *LeadAnalyticsService) getAttribution(tenantCtx *tenant.Context) map[string]any {
	attribution := make(map[string]any)

	attribution["direct"] = 0
	attribution["campaign"] = 0
	attribution["referral"] = 0

	query := `
		SELECT 
			CASE 
				WHEN v.campaign_id IS NOT NULL THEN 'campaign'
				ELSE 'direct'
			END as attribution_type,
			COUNT(*) as count
		FROM fingerprints f
		LEFT JOIN visits v ON f.id = v.fingerprint_id
		WHERE f.lead_id IS NOT NULL
		GROUP BY attribution_type
	`

	rows, err := tenantCtx.Database.Conn.Query(query)
	if err != nil {
		return attribution
	}
	defer rows.Close()

	for rows.Next() {
		var attributionType string
		var count int
		if err := rows.Scan(&attributionType, &count); err != nil {
			continue
		}
		attribution[attributionType] = count
	}

	return attribution
}

func (s *LeadAnalyticsService) getHourKeysForCustomRange(startHour, endHour int) []string {
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
