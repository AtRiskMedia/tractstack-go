// Package api provide menu handlers
package api

import (
	"fmt"
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models/content"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/gin-gonic/gin"
)

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

	// Activate tenant if needed
	if ctx.Status == "inactive" {
		if err := tenant.ActivateTenant(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("tenant activation failed: %v", err)})
			return
		}
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

	// Activate tenant if needed
	if ctx.Status == "inactive" {
		if err := tenant.ActivateTenant(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("tenant activation failed: %v", err)})
			return
		}
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

	// Activate tenant if needed
	if ctx.Status == "inactive" {
		if err := tenant.ActivateTenant(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("tenant activation failed: %v", err)})
			return
		}
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
