// Package api provide belief handlers
package api

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/models/content"
	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"
)

// BeliefIDsRequest represents the request body for bulk belief loading
type BeliefIDsRequest struct {
	BeliefIDs []string `json:"beliefIds" binding:"required"`
}

// CreateBeliefRequest represents the request body for creating beliefs
type CreateBeliefRequest struct {
	Title        string   `json:"title" binding:"required"`
	Slug         string   `json:"slug" binding:"required"`
	Scale        string   `json:"scale" binding:"required"`
	CustomValues []string `json:"customValues,omitempty"`
}

// UpdateBeliefRequest represents the request body for updating beliefs
type UpdateBeliefRequest struct {
	Title        string   `json:"title" binding:"required"`
	Slug         string   `json:"slug" binding:"required"`
	Scale        string   `json:"scale" binding:"required"`
	CustomValues []string `json:"customValues,omitempty"`
}

// GetAllBeliefIDsHandler returns all belief IDs using cache-first pattern
func GetAllBeliefIDsHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Use cache-first belief service with global cache manager
	beliefService := content.NewBeliefService(ctx, cache.GetGlobalManager())
	beliefIDs, err := beliefService.GetAllIDs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"beliefIds": beliefIDs,
		"count":     len(beliefIDs),
	})
}

// GetBeliefsByIDsHandler returns multiple beliefs by IDs using cache-first pattern
func GetBeliefsByIDsHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Parse request body
	var req BeliefIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if len(req.BeliefIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "beliefIds array cannot be empty"})
		return
	}

	// Use cache-first belief service with global cache manager
	beliefService := content.NewBeliefService(ctx, cache.GetGlobalManager())
	beliefs, err := beliefService.GetByIDs(req.BeliefIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"beliefs": beliefs,
		"count":   len(beliefs),
	})
}

// GetBeliefByIDHandler returns a specific belief by ID using cache-first pattern
func GetBeliefByIDHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	beliefID := c.Param("id")
	if beliefID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "belief ID is required"})
		return
	}

	// Use cache-first belief service with global cache manager
	beliefService := content.NewBeliefService(ctx, cache.GetGlobalManager())
	beliefNode, err := beliefService.GetByID(beliefID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if beliefNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "belief not found"})
		return
	}

	c.JSON(http.StatusOK, beliefNode)
}

// GetBeliefBySlugHandler returns a specific belief by slug using cache-first pattern
func GetBeliefBySlugHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "belief slug is required"})
		return
	}

	// Use cache-first belief service with global cache manager
	beliefService := content.NewBeliefService(ctx, cache.GetGlobalManager())
	beliefNode, err := beliefService.GetBySlug(slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if beliefNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "belief not found"})
		return
	}

	c.JSON(http.StatusOK, beliefNode)
}

// CreateBeliefHandler creates a new belief with authentication
func CreateBeliefHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Authentication - Admin OR Editor required
	if !validateAdminOrEditor(c, ctx) {
		return
	}

	// Parse request
	var req CreateBeliefRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format", "details": err.Error()})
		return
	}

	// Validate belief data
	if err := validateBeliefRequest(req.Title, req.Slug, req.Scale, req.CustomValues); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check for slug uniqueness
	var existingID string
	err = ctx.Database.Conn.QueryRow("SELECT id FROM beliefs WHERE slug = ?", req.Slug).Scan(&existingID)
	if err != sql.ErrNoRows {
		if err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "Belief with this slug already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Generate new ID
	beliefID := ulid.Make().String()

	// Convert custom values to database format
	var customValuesString *string
	if len(req.CustomValues) > 0 {
		joined := strings.Join(req.CustomValues, ",")
		customValuesString = &joined
	}

	// Insert into database
	query := `INSERT INTO beliefs (id, title, slug, scale, custom_values) VALUES (?, ?, ?, ?, ?)`
	_, err = ctx.Database.Conn.Exec(query, beliefID, req.Title, req.Slug, req.Scale, customValuesString)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create belief"})
		return
	}

	// Create response belief node
	beliefNode := &models.BeliefNode{
		ID:           beliefID,
		Title:        req.Title,
		Slug:         req.Slug,
		Scale:        req.Scale,
		CustomValues: req.CustomValues,
	}

	// Cache invalidation cascade
	cache.GetGlobalManager().SetBelief(ctx.TenantID, beliefNode)
	cache.GetGlobalManager().InvalidateFullContentMap(ctx.TenantID)
	cache.GetGlobalManager().InvalidateOrphanAnalysis(ctx.TenantID)

	c.JSON(http.StatusCreated, beliefNode)
}

// UpdateBeliefHandler updates an existing belief with authentication
func UpdateBeliefHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Authentication - Admin OR Editor required
	if !validateAdminOrEditor(c, ctx) {
		return
	}

	beliefID := c.Param("id")
	if beliefID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Belief ID is required"})
		return
	}

	// Parse request
	var req UpdateBeliefRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format", "details": err.Error()})
		return
	}

	// Validate belief data
	if err := validateBeliefRequest(req.Title, req.Slug, req.Scale, req.CustomValues); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if belief exists
	var existingID string
	err = ctx.Database.Conn.QueryRow("SELECT id FROM beliefs WHERE id = ?", beliefID).Scan(&existingID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Belief not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Check for slug uniqueness (excluding current belief)
	var conflictingID string
	err = ctx.Database.Conn.QueryRow("SELECT id FROM beliefs WHERE slug = ? AND id != ?", req.Slug, beliefID).Scan(&conflictingID)
	if err != sql.ErrNoRows {
		if err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "Another belief with this slug already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Convert custom values to database format
	var customValuesString *string
	if len(req.CustomValues) > 0 {
		joined := strings.Join(req.CustomValues, ",")
		customValuesString = &joined
	}

	// Update database
	query := `UPDATE beliefs SET title = ?, slug = ?, scale = ?, custom_values = ? WHERE id = ?`
	_, err = ctx.Database.Conn.Exec(query, req.Title, req.Slug, req.Scale, customValuesString, beliefID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update belief"})
		return
	}

	// Create response belief node
	beliefNode := &models.BeliefNode{
		ID:           beliefID,
		Title:        req.Title,
		Slug:         req.Slug,
		Scale:        req.Scale,
		CustomValues: req.CustomValues,
	}

	// Cache invalidation cascade
	cache.GetGlobalManager().SetBelief(ctx.TenantID, beliefNode)
	cache.GetGlobalManager().InvalidateFullContentMap(ctx.TenantID)
	cache.GetGlobalManager().InvalidateOrphanAnalysis(ctx.TenantID)

	c.JSON(http.StatusOK, beliefNode)
}

// DeleteBeliefHandler deletes a belief with authentication and usage check
func DeleteBeliefHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Authentication - Admin OR Editor required
	if !validateAdminOrEditor(c, ctx) {
		return
	}

	beliefID := c.Param("id")
	if beliefID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Belief ID is required"})
		return
	}

	// Check if belief exists
	var existingID string
	err = ctx.Database.Conn.QueryRow("SELECT id FROM beliefs WHERE id = ?", beliefID).Scan(&existingID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Belief not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Check for foreign key constraints (heldbeliefs using this belief)
	var usageCount int
	err = ctx.Database.Conn.QueryRow("SELECT COUNT(*) FROM heldbeliefs WHERE belief_id = ?", beliefID).Scan(&usageCount)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check belief usage"})
		return
	}

	if usageCount > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error":      "Cannot delete belief: it has recorded belief states",
			"usageCount": usageCount,
		})
		return
	}

	// Delete from database
	_, err = ctx.Database.Conn.Exec("DELETE FROM beliefs WHERE id = ?", beliefID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete belief"})
		return
	}

	// Cache invalidation cascade
	cache.GetGlobalManager().InvalidateBelief(ctx.TenantID, beliefID)
	cache.GetGlobalManager().InvalidateFullContentMap(ctx.TenantID)
	cache.GetGlobalManager().InvalidateOrphanAnalysis(ctx.TenantID)

	c.JSON(http.StatusOK, gin.H{"message": "Belief deleted successfully"})
}

// validateBeliefRequest validates belief creation/update data
func validateBeliefRequest(title, slug, scale string, customValues []string) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("title is required")
	}

	if strings.TrimSpace(slug) == "" {
		return fmt.Errorf("slug is required")
	}

	if strings.TrimSpace(scale) == "" {
		return fmt.Errorf("scale is required")
	}

	// Validate scale values
	validScales := map[string]bool{
		"likert":    true,
		"agreement": true,
		"interest":  true,
		"yn":        true,
		"tf":        true,
		"custom":    true,
	}

	if !validScales[scale] {
		return fmt.Errorf("invalid scale: %s", scale)
	}

	// If scale is custom, validate custom values
	if scale == "custom" {
		if len(customValues) == 0 {
			return fmt.Errorf("custom scale requires at least one custom value")
		}

		for i, value := range customValues {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("custom value %d cannot be empty", i+1)
			}
		}
	}

	return nil
}
