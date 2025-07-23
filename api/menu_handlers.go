// Package api provide menu handlers
package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/models/content"
	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"
)

type CreateMenuRequest struct {
	Title          string            `json:"title" binding:"required"`
	Theme          string            `json:"theme" binding:"required"`
	OptionsPayload []models.MenuLink `json:"optionsPayload" binding:"required"`
}

type UpdateMenuRequest struct {
	Title          string            `json:"title" binding:"required"`
	Theme          string            `json:"theme" binding:"required"`
	OptionsPayload []models.MenuLink `json:"optionsPayload" binding:"required"`
}

// MenuIDsRequest represents the request body for bulk menu loading
type MenuIDsRequest struct {
	MenuIDs []string `json:"menuIds" binding:"required"`
}

// GetAllMenuIDsHandler returns all menu IDs using cache-first pattern
func GetAllMenuIDsHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Use cache-first menu service with global cache manager
	menuService := content.NewMenuService(ctx, cache.GetGlobalManager())
	menuIDs, err := menuService.GetAllIDs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"menuIds": menuIDs,
		"count":   len(menuIDs),
	})
}

// GetMenusByIDsHandler returns multiple menus by IDs using cache-first pattern
func GetMenusByIDsHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Parse request body
	var req MenuIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if len(req.MenuIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "menuIds array cannot be empty"})
		return
	}

	// Use cache-first menu service with global cache manager
	menuService := content.NewMenuService(ctx, cache.GetGlobalManager())
	menus, err := menuService.GetByIDs(req.MenuIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"menus": menus,
		"count": len(menus),
	})
}

// GetMenuByIDHandler returns a specific menu by ID using cache-first pattern
func GetMenuByIDHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	menuID := c.Param("id")
	if menuID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "menu ID is required"})
		return
	}

	// Use cache-first menu service with global cache manager
	menuService := content.NewMenuService(ctx, cache.GetGlobalManager())
	menuNode, err := menuService.GetByID(menuID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if menuNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "menu not found"})
		return
	}

	c.JSON(http.StatusOK, menuNode)
}

// CreateMenuHandler creates a new menu with authentication
func CreateMenuHandler(c *gin.Context) {
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
	var req CreateMenuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format", "details": err.Error()})
		return
	}

	// Validate menu data
	if err := validateMenuRequest(req.Title, req.Theme, req.OptionsPayload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Generate new ID
	menuID := generateULID()

	// Convert to database format
	optionsPayloadJSON, err := json.Marshal(req.OptionsPayload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to serialize options payload"})
		return
	}

	// Insert into database
	query := `INSERT INTO menus (id, title, theme, options_payload) VALUES (?, ?, ?, ?)`
	_, err = ctx.Database.Conn.Exec(query, menuID, req.Title, req.Theme, string(optionsPayloadJSON))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create menu"})
		return
	}

	// Create response menu node
	menuNode := &models.MenuNode{
		ID:             menuID,
		Title:          req.Title,
		Theme:          req.Theme,
		OptionsPayload: req.OptionsPayload,
	}

	// Cache invalidation cascade
	cache.GetGlobalManager().SetMenu(ctx.TenantID, menuNode)
	cache.GetGlobalManager().InvalidateFullContentMap(ctx.TenantID)
	cache.GetGlobalManager().InvalidateOrphanAnalysis(ctx.TenantID)

	c.JSON(http.StatusCreated, menuNode)
}

// UpdateMenuHandler updates an existing menu with authentication
func UpdateMenuHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Authentication - Admin OR Editor required
	if !validateAdminOrEditor(c, ctx) {
		return
	}

	menuID := c.Param("id")
	if menuID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Menu ID is required"})
		return
	}

	// Parse request
	var req UpdateMenuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format", "details": err.Error()})
		return
	}

	// Validate menu data
	if err := validateMenuRequest(req.Title, req.Theme, req.OptionsPayload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if menu exists
	var existingID string
	err = ctx.Database.Conn.QueryRow("SELECT id FROM menus WHERE id = ?", menuID).Scan(&existingID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Menu not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Convert to database format
	optionsPayloadJSON, err := json.Marshal(req.OptionsPayload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to serialize options payload"})
		return
	}

	// Update database
	query := `UPDATE menus SET title = ?, theme = ?, options_payload = ? WHERE id = ?`
	_, err = ctx.Database.Conn.Exec(query, req.Title, req.Theme, string(optionsPayloadJSON), menuID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update menu"})
		return
	}

	// Create response menu node
	menuNode := &models.MenuNode{
		ID:             menuID,
		Title:          req.Title,
		Theme:          req.Theme,
		OptionsPayload: req.OptionsPayload,
	}

	// Cache invalidation cascade
	cache.GetGlobalManager().SetMenu(ctx.TenantID, menuNode)
	cache.GetGlobalManager().InvalidateFullContentMap(ctx.TenantID)
	cache.GetGlobalManager().InvalidateOrphanAnalysis(ctx.TenantID)

	c.JSON(http.StatusOK, menuNode)
}

// DeleteMenuHandler deletes a menu with authentication and orphan check
func DeleteMenuHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Authentication - Admin OR Editor required
	if !validateAdminOrEditor(c, ctx) {
		return
	}

	menuID := c.Param("id")
	if menuID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Menu ID is required"})
		return
	}

	// Check if menu exists
	var existingID string
	err = ctx.Database.Conn.QueryRow("SELECT id FROM menus WHERE id = ?", menuID).Scan(&existingID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Menu not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Check for foreign key constraints (story fragments using this menu)
	var usageCount int
	err = ctx.Database.Conn.QueryRow("SELECT COUNT(*) FROM storyfragments WHERE menu_id = ?", menuID).Scan(&usageCount)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check menu usage"})
		return
	}

	if usageCount > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error":      "Cannot delete menu: it is currently used by story fragments",
			"usageCount": usageCount,
		})
		return
	}

	// Delete from database
	_, err = ctx.Database.Conn.Exec("DELETE FROM menus WHERE id = ?", menuID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete menu"})
		return
	}

	// Cache invalidation cascade
	cache.GetGlobalManager().InvalidateMenu(ctx.TenantID, menuID)
	cache.GetGlobalManager().InvalidateFullContentMap(ctx.TenantID)
	cache.GetGlobalManager().InvalidateOrphanAnalysis(ctx.TenantID)

	c.JSON(http.StatusOK, gin.H{"message": "Menu deleted successfully"})
}

// validateMenuRequest validates menu creation/update data
func validateMenuRequest(title, theme string, optionsPayload []models.MenuLink) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("title is required")
	}

	if strings.TrimSpace(theme) == "" {
		return fmt.Errorf("theme is required")
	}

	// Validate each menu link
	for i, link := range optionsPayload {
		if strings.TrimSpace(link.Name) == "" {
			return fmt.Errorf("menu link %d: name is required", i+1)
		}
		if strings.TrimSpace(link.ActionLisp) == "" {
			return fmt.Errorf("menu link %d: actionLisp is required", i+1)
		}
		// Basic ActionLisp format validation
		if !strings.HasPrefix(link.ActionLisp, "(goto ") {
			return fmt.Errorf("menu link %d: actionLisp must start with '(goto '", i+1)
		}
		if !strings.HasSuffix(link.ActionLisp, "))") {
			return fmt.Errorf("menu link %d: actionLisp must end with '))'", i+1)
		}
	}

	return nil
}

// generateULID creates a new ULID for menu IDs
func generateULID() string {
	// This should use the same ULID generation as other parts of the system
	// Assuming there's a utility function available
	return ulid.Make().String()
}
