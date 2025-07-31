// Package handlers provides HTTP handlers for menu endpoints
package handlers

import (
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	persistence "github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/content"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// MenuIDsRequest represents the request body for bulk menu loading
type MenuIDsRequest struct {
	MenuIDs []string `json:"menuIds" binding:"required"`
}

// GetAllMenuIDsHandler returns all menu IDs using cache-first pattern
func GetAllMenuIDsHandler(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	menuRepo := persistence.NewMenuRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	menuService := services.NewMenuService(menuRepo)

	menuIDs, err := menuService.GetAllIDs(tenantCtx.TenantID)
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
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var req MenuIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if len(req.MenuIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "menuIds array cannot be empty"})
		return
	}

	menuRepo := persistence.NewMenuRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	menuService := services.NewMenuService(menuRepo)

	menus, err := menuService.GetByIDs(tenantCtx.TenantID, req.MenuIDs)
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
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	menuID := c.Param("id")
	if menuID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "menu ID is required"})
		return
	}

	menuRepo := persistence.NewMenuRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	menuService := services.NewMenuService(menuRepo)

	menuNode, err := menuService.GetByID(tenantCtx.TenantID, menuID)
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

// CreateMenuHandler creates a new menu
func CreateMenuHandler(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var menu content.MenuNode
	if err := c.ShouldBindJSON(&menu); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	menuRepo := persistence.NewMenuRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	menuService := services.NewMenuService(menuRepo)

	err := menuService.Create(tenantCtx.TenantID, &menu)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "menu created successfully",
		"menuId":  menu.ID,
	})
}

// UpdateMenuHandler updates an existing menu
func UpdateMenuHandler(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	menuID := c.Param("id")
	if menuID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "menu ID is required"})
		return
	}

	var menu content.MenuNode
	if err := c.ShouldBindJSON(&menu); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	// Ensure ID matches URL parameter
	menu.ID = menuID

	menuRepo := persistence.NewMenuRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	menuService := services.NewMenuService(menuRepo)

	err := menuService.Update(tenantCtx.TenantID, &menu)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "menu updated successfully",
		"menuId":  menu.ID,
	})
}

// DeleteMenuHandler deletes a menu
func DeleteMenuHandler(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	menuID := c.Param("id")
	if menuID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "menu ID is required"})
		return
	}

	menuRepo := persistence.NewMenuRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	menuService := services.NewMenuService(menuRepo)

	err := menuService.Delete(tenantCtx.TenantID, menuID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "menu deleted successfully",
		"menuId":  menuID,
	})
}
