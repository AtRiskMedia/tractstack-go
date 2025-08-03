// Package handlers provides HTTP handlers for menu endpoints
package handlers

import (
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// MenuIDsRequest represents the request body for bulk menu loading
type MenuIDsRequest struct {
	MenuIDs []string `json:"menuIds" binding:"required"`
}

// CreateMenuRequest defines the structure for creating a new menu.
type CreateMenuRequest struct {
	Title          string              `json:"title" binding:"required"`
	Theme          string              `json:"theme" binding:"required"`
	OptionsPayload []*content.MenuLink `json:"optionsPayload" binding:"required"`
}

// UpdateMenuRequest defines the structure for updating an existing menu.
type UpdateMenuRequest struct {
	Title          string              `json:"title" binding:"required"`
	Theme          string              `json:"theme" binding:"required"`
	OptionsPayload []*content.MenuLink `json:"optionsPayload" binding:"required"`
}

// MenuHandlers contains all menu-related HTTP handlers
type MenuHandlers struct {
	menuService *services.MenuService
	logger      *logging.ChanneledLogger
}

// NewMenuHandlers creates menu handlers with injected dependencies
func NewMenuHandlers(menuService *services.MenuService, logger *logging.ChanneledLogger) *MenuHandlers {
	return &MenuHandlers{
		menuService: menuService,
		logger:      logger,
	}
}

// GetAllMenuIDs returns all menu IDs using cache-first pattern
func (h *MenuHandlers) GetAllMenuIDs(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get all menu IDs request", "method", c.Request.Method, "path", c.Request.URL.Path)
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	menuIDs, err := h.menuService.GetAllIDs(tenantCtx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Get all menu IDs request completed", "count", len(menuIDs), "duration", time.Since(start))
	c.JSON(http.StatusOK, gin.H{
		"menuIds": menuIDs,
		"count":   len(menuIDs),
	})
}

// GetMenusByIDs returns multiple menus by IDs using cache-first pattern
func (h *MenuHandlers) GetMenusByIDs(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get menus by IDs request", "method", c.Request.Method, "path", c.Request.URL.Path)
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

	menus, err := h.menuService.GetByIDs(tenantCtx, req.MenuIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Get menus by IDs request completed", "requestedCount", len(req.MenuIDs), "foundCount", len(menus), "duration", time.Since(start))
	c.JSON(http.StatusOK, gin.H{
		"menus": menus,
		"count": len(menus),
	})
}

// GetMenuByID returns a specific menu by ID using cache-first pattern
func (h *MenuHandlers) GetMenuByID(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get menu by ID request", "method", c.Request.Method, "path", c.Request.URL.Path, "menuId", c.Param("id"))
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

	menuNode, err := h.menuService.GetByID(tenantCtx, menuID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if menuNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "menu not found"})
		return
	}

	h.logger.Content().Info("Get menu by ID request completed", "menuId", menuID, "found", menuNode != nil, "duration", time.Since(start))
	c.JSON(http.StatusOK, menuNode)
}

// CreateMenu creates a new menu
func (h *MenuHandlers) CreateMenu(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received create menu request", "method", c.Request.Method, "path", c.Request.URL.Path)
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var req CreateMenuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	menu := &content.MenuNode{
		Title:          req.Title,
		Theme:          req.Theme,
		OptionsPayload: req.OptionsPayload,
	}

	if err := h.menuService.Create(tenantCtx, menu); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Create menu request completed", "menuId", menu.ID, "title", menu.Title, "duration", time.Since(start))
	c.JSON(http.StatusCreated, gin.H{
		"message": "menu created successfully",
		"menuId":  menu.ID,
	})
}

// UpdateMenu updates an existing menu
func (h *MenuHandlers) UpdateMenu(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received update menu request", "method", c.Request.Method, "path", c.Request.URL.Path, "menuId", c.Param("id"))
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

	var req UpdateMenuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	menu := &content.MenuNode{
		ID:             menuID,
		Title:          req.Title,
		Theme:          req.Theme,
		OptionsPayload: req.OptionsPayload,
	}

	if err := h.menuService.Update(tenantCtx, menu); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Update menu request completed", "menuId", menu.ID, "title", menu.Title, "duration", time.Since(start))
	c.JSON(http.StatusOK, gin.H{
		"message": "menu updated successfully",
		"menuId":  menu.ID,
	})
}

// DeleteMenu deletes a menu
func (h *MenuHandlers) DeleteMenu(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received delete menu request", "method", c.Request.Method, "path", c.Request.URL.Path, "menuId", c.Param("id"))
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

	if err := h.menuService.Delete(tenantCtx, menuID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Delete menu request completed", "menuId", menuID, "duration", time.Since(start))
	c.JSON(http.StatusOK, gin.H{
		"message": "menu deleted successfully",
		"menuId":  menuID,
	})
}
