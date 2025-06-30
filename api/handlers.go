// Package api provides HTTP handlers and database connectivity for the application's API.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"
)

// getTenantContext is a helper to extract tenant context from gin context
func getTenantContext(c *gin.Context) (*tenant.Context, error) {
	tenantCtx, exists := c.Get("tenant")
	if !exists {
		return nil, fmt.Errorf("no tenant context")
	}
	return tenantCtx.(*tenant.Context), nil
}

// getTenantManager is a helper to extract tenant manager from gin context
func getTenantManager(c *gin.Context) (*tenant.Manager, error) {
	manager, exists := c.Get("tenantManager")
	if !exists {
		return nil, fmt.Errorf("no tenant manager")
	}
	return manager.(*tenant.Manager), nil
}

// generateULID creates a new ULID
func generateULID() string {
	return ulid.Make().String()
}

// DBStatusHandler checks tenant status and activates if needed
func DBStatusHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Only activate if status is inactive
	if ctx.Status == "inactive" {
		if err := tenant.ActivateTenant(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("tenant activation failed: %v", err)})
			return
		}

		// Update detector's cached registry after successful activation
		manager, err := getTenantManager(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Determine database type for cache update
		dbType := "sqlite3"
		if ctx.Database.UseTurso {
			dbType = "turso"
		}

		manager.GetDetector().UpdateTenantStatus(ctx.TenantID, "active", dbType)
	}

	// If we reach here and status is active, tables are guaranteed to exist
	allTablesExist := (ctx.Status == "active")

	c.JSON(http.StatusOK, gin.H{
		"tenantId":       ctx.TenantID,
		"status":         ctx.Status,
		"database":       ctx.Database.GetConnectionInfo(),
		"allTablesExist": allTablesExist,
	})
}

func StateHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var req struct {
		Events []models.Event `json:"events"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Process events with tenant context
	for _, event := range req.Events {
		if event.Type == "Pane" && event.Verb == "CLICKED" {
			paneIDs := []string{"pane-123", "pane-456"}
			data, _ := json.Marshal(paneIDs)
			models.Broadcaster.Broadcast("reload_panes", string(data))
		}
		// TODO: Add belief events and database persistence
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "tenantId": ctx.TenantID})
}
