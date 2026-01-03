// Package middleware provides HTTP middleware for the presentation layer.
package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/gin-gonic/gin"
)

// GetTenantContext retrieves the tenant context from gin context.
func GetTenantContext(c *gin.Context) (*tenant.Context, bool) {
	tenantCtx, exists := c.Get("tenant")
	if !exists {
		return nil, false
	}

	ctx, ok := tenantCtx.(*tenant.Context)
	return ctx, ok
}

// TenantMiddleware creates middleware that extracts tenant information and creates a full tenant context.
// TenantMiddleware creates middleware that extracts tenant information and creates a full tenant context.
func TenantMiddleware(tenantManager *tenant.Manager, perfTracker *performance.Tracker) gin.HandlerFunc {
	logger := tenantManager.GetLogger()

	return func(c *gin.Context) {
		start := time.Now()
		marker := perfTracker.StartOperation("middleware_tenant_resolution", "unknown")
		defer marker.Complete()

		tenantID := c.GetHeader("X-Tenant-ID")
		if tenantID == "" {
			tenantID = c.Query("tenantId") // Fallback for SSE
		}

		marker.AddMetadata("path", c.Request.URL.Path)
		marker.AddMetadata("method", c.Request.Method)
		if tenantID != "" {
			marker.TenantID = tenantID
		}

		if tenantID == "" {
			errMsg := "X-Tenant-ID header or tenantId query param is required"
			logger.Tenant().Warn(errMsg, "path", c.Request.URL.Path)
			marker.SetSuccess(false)
			marker.SetError(fmt.Errorf("%s", errMsg))
			c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
			c.Abort()
			return
		}

		tenantCtx, err := tenantManager.GetContext(c)
		if err != nil {
			// Check if this is default tenant setup scenario
			if tenantID == "default" {
				detector := tenantManager.GetDetector()
				if detector.GetTenantStatus("default") == "inactive" {
					// explicitly leave the Database as nil to prevent unsafe access.
					brandConfig, _ := tenant.LoadBrandConfig("default")

					safeConfig := &tenant.Config{
						TenantID:    "default",
						Status:      "inactive",
						BrandConfig: brandConfig,
					}

					safeCtx := &tenant.Context{
						TenantID: "default",
						Status:   "inactive",
						Config:   safeConfig,
						Logger:   logger,
						// Database: nil, // Implicitly nil to prevent unsafe queries
					}

					c.Set("tenant", safeCtx)

					// Set flags for health handler and continue
					c.Set("setupNeeded", true)
					c.Set("tenantId", "default")
					marker.SetSuccess(true)
					c.Next()
					return
				}
			}

			errMsg := fmt.Sprintf("tenant '%s' not found or failed to initialize", tenantID)
			logger.Tenant().Error(errMsg, "error", err, "tenantId", tenantID)
			marker.SetSuccess(false)
			marker.SetError(err)
			c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
			c.Abort()
			return
		}

		// Hybrid Restoration: Check for persistence cookie if memory is empty
		cookieSessionID, _ := c.Cookie("tractstack_session_id")
		cookieFingerprintID, _ := c.Cookie("tractstack_fingerprint")

		if cookieFingerprintID != "" && tenantCtx.Database != nil {
			_, sessionExists := tenantCtx.CacheManager.GetSession(tenantCtx.TenantID, cookieSessionID)

			if !sessionExists && cookieSessionID != "" {
				var dbFingerprintID string
				var latestVisitID string

				// We need a valid VisitID to reconstruct the session.
				// Query: Verify fingerprint exists AND fetch its most recent visit ID
				query := `
					SELECT f.id, v.id 
					FROM fingerprints f 
					JOIN visits v ON f.id = v.fingerprint_id 
					WHERE f.id = ? 
					ORDER BY v.created_at DESC 
					LIMIT 1`

				err := tenantCtx.Database.Conn.QueryRowContext(c.Request.Context(), query, cookieFingerprintID).Scan(&dbFingerprintID, &latestVisitID)

				if err == nil && dbFingerprintID == cookieFingerprintID {
					restoredSession := &types.SessionData{
						SessionID:     cookieSessionID,
						FingerprintID: cookieFingerprintID,
						VisitID:       latestVisitID,
						CreatedAt:     time.Now().UTC(),
						LastActivity:  time.Now().UTC(),
						ExpiresAt:     time.Now().UTC().Add(24 * time.Hour),
						IsExpired:     false,
					}
					tenantCtx.CacheManager.SetSession(tenantCtx.TenantID, restoredSession)

					logger.Tenant().Info("Restored session from fingerprint cookie",
						"sessionId", cookieSessionID,
						"fingerprintId", cookieFingerprintID,
						"visitId", latestVisitID,
						"tenantId", tenantCtx.TenantID)
				}
			}

			// Store fingerprint in context for handlers to access even if session restoration failed
			c.Set("fingerprint_id", cookieFingerprintID)
		}

		logger.Tenant().Debug("Tenant context resolved successfully",
			"tenantId", tenantCtx.TenantID,
			"duration", time.Since(start),
			"database", tenantCtx.GetDatabaseInfo(),
		)
		marker.SetSuccess(true)

		c.Set("tenant", tenantCtx)

		c.Next()
	}
}
