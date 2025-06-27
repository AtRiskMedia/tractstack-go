package api

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/AtRiskMedia/tractstack-go/utils"
)

type VisitRowData struct {
	ID            string    `json:"id"`
	FingerprintID string    `json:"fingerprint_id"`
	CampaignID    *string   `json:"campaign_id"`
	CreatedAt     time.Time `json:"created_at"`
}

type FingerprintRowData struct {
	ID        string    `json:"id"`
	LeadID    *string   `json:"lead_id"`
	CreatedAt time.Time `json:"created_at"`
}

type VisitService struct {
	ctx *tenant.Context
}

func NewVisitService(ctx *tenant.Context, _ any) *VisitService {
	return &VisitService{
		ctx: ctx,
	}
}

func (vs *VisitService) GetLatestVisitByFingerprint(fingerprintID string) (*VisitRowData, error) {
	query := `SELECT id, fingerprint_id, campaign_id, created_at 
              FROM visits 
              WHERE fingerprint_id = ? 
              ORDER BY created_at DESC 
              LIMIT 1`

	row := vs.ctx.Database.Conn.QueryRow(query, fingerprintID)

	var visit VisitRowData
	var campaignID sql.NullString

	err := row.Scan(&visit.ID, &visit.FingerprintID, &campaignID, &visit.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan visit: %w", err)
	}

	if campaignID.Valid {
		visit.CampaignID = &campaignID.String
	}

	return &visit, nil
}

func (vs *VisitService) CreateVisit(visitID, fingerprintID string, campaignID *string) error {
	query := `INSERT INTO visits (id, fingerprint_id, campaign_id, created_at) 
              VALUES (?, ?, ?, ?)`

	_, err := vs.ctx.Database.Conn.Exec(query, visitID, fingerprintID, campaignID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to create visit: %w", err)
	}

	return nil
}

func (vs *VisitService) FingerprintExists(fingerprintID string) (bool, error) {
	query := `SELECT 1 FROM fingerprints WHERE id = ? LIMIT 1`

	var exists int
	err := vs.ctx.Database.Conn.QueryRow(query, fingerprintID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check fingerprint existence: %w", err)
	}

	return true, nil
}

func (vs *VisitService) CreateFingerprint(fingerprintID string, leadID *string) error {
	query := `INSERT INTO fingerprints (id, lead_id, created_at) 
              VALUES (?, ?, ?)`

	_, err := vs.ctx.Database.Conn.Exec(query, fingerprintID, leadID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to create fingerprint: %w", err)
	}

	return nil
}

func (vs *VisitService) GetFingerprintLeadID(fingerprintID string) (*string, error) {
	query := `SELECT lead_id FROM fingerprints WHERE id = ? LIMIT 1`
	var leadID sql.NullString
	err := vs.ctx.Database.Conn.QueryRow(query, fingerprintID).Scan(&leadID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get fingerprint lead_id: %w", err)
	}
	if leadID.Valid {
		return &leadID.String, nil
	}
	return nil, nil
}

func (vs *VisitService) IsVisitExpired(visit *VisitRowData) bool {
	if visit == nil {
		return true
	}
	return time.Since(visit.CreatedAt) > 2*time.Hour
}

func (vs *VisitService) HandleVisitSession(requestFpID, requestVisitID *string, hasProfile bool) (string, string, *string, error) {
	var fpID, visitID string

	// Use provided fingerprint or generate new one
	if requestFpID != nil && *requestFpID != "" {
		fpID = *requestFpID
	} else {
		fpID = utils.GenerateULID()
	}

	// Check if fingerprint exists in database
	fpExists, err := vs.FingerprintExists(fpID)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to check fingerprint: %w", err)
	}

	// Create fingerprint if it doesn't exist
	if !fpExists {
		var leadID *string
		// leadID would be set if this is a known user

		if err := vs.CreateFingerprint(fpID, leadID); err != nil {
			return "", "", nil, fmt.Errorf("failed to create fingerprint: %w", err)
		}

		// Update cache with known fingerprint status
		cache.GetGlobalManager().SetKnownFingerprint(vs.ctx.TenantID, fpID, leadID != nil)
	}

	shouldCreateNewVisit := true

	// Check if we should reuse existing visit
	if requestVisitID != nil && *requestVisitID != "" {
		latestVisit, err := vs.GetLatestVisitByFingerprint(fpID)
		if err != nil {
			return "", "", nil, fmt.Errorf("failed to get latest visit: %w", err)
		}

		// Reuse visit if it matches requested visit ID and hasn't expired
		if latestVisit != nil && latestVisit.ID == *requestVisitID && !vs.IsVisitExpired(latestVisit) {
			visitID = *requestVisitID
			shouldCreateNewVisit = false
		}
	}

	// Create new visit if needed
	if shouldCreateNewVisit {
		visitID = utils.GenerateULID()
		if err := vs.CreateVisit(visitID, fpID, nil); err != nil {
			return "", "", nil, fmt.Errorf("failed to create visit: %w", err)
		}

		// Update cache with new visit state
		visitState := &models.VisitState{
			VisitID:       visitID,
			FingerprintID: fpID,
			StartTime:     time.Now(),
			LastActivity:  time.Now(),
			CurrentPage:   "/",
		}
		cache.GetGlobalManager().SetVisitState(vs.ctx.TenantID, visitState)
	}

	// Get lead_id for profile restoration
	leadID, err := vs.GetFingerprintLeadID(fpID)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to check fingerprint lead: %w", err)
	}

	return fpID, visitID, leadID, nil
}
