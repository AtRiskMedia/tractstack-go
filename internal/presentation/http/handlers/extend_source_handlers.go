package handlers

import (
	"errors"
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// ExtendSourceHandlers serves /api/v1/extend/source/*.
type ExtendSourceHandlers struct {
	sourceService   *services.ExtendSourceService
	registryService *services.ExtendRegistryService
	usageService    *services.ExtendUsageService
	logger          *logging.ChanneledLogger
	perfTracker     *performance.Tracker
}

// NewExtendSourceHandlers constructs source handlers.
func NewExtendSourceHandlers(
	sourceService *services.ExtendSourceService,
	registryService *services.ExtendRegistryService,
	usageService *services.ExtendUsageService,
	logger *logging.ChanneledLogger,
	perfTracker *performance.Tracker,
) *ExtendSourceHandlers {
	return &ExtendSourceHandlers{
		sourceService:   sourceService,
		registryService: registryService,
		usageService:    usageService,
		logger:          logger,
		perfTracker:     perfTracker,
	}
}

func (h *ExtendSourceHandlers) tenantID(c *gin.Context) (string, bool) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return "", false
	}
	return tenantCtx.TenantID, true
}

func (h *ExtendSourceHandlers) writeSourceError(c *gin.Context, err error) {
	var hookInUse *services.ErrHookInUse
	var routeInUse *services.ErrRouteInUse
	switch {
	case errors.As(err, &hookInUse):
		c.JSON(http.StatusConflict, gin.H{"error": hookInUse.Error(), "usages": hookInUse.Usages})
	case errors.As(err, &routeInUse):
		c.JSON(http.StatusConflict, gin.H{"error": routeInUse.Error(), "usages": routeInUse.Usages})
	case errors.Is(err, services.ErrExtendDisabled):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrSourceNotFound), errors.Is(err, services.ErrExtendNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrSourceTooLarge), errors.Is(err, services.ErrSourceTreeTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrSourceInvalidPath), errors.Is(err, services.ErrSourceForbidden),
		errors.Is(err, services.ErrExtendBadID):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrSourceNotText):
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrExtendCoreID), errors.Is(err, services.ErrExtendCollision),
		errors.Is(err, services.ErrExtendTrunkExists):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		h.logger.System().Error("extend source error", "error", err.Error(), "path", c.Request.URL.Path)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

// GetTree handles GET /api/v1/extend/source/tree
func (h *ExtendSourceHandlers) GetTree(c *gin.Context) {
	tid, ok := h.tenantID(c)
	if !ok {
		return
	}
	nodes, err := h.sourceService.Tree(tid)
	if err != nil {
		h.writeSourceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tree": nodes})
}

// GetFile handles GET /api/v1/extend/source/file?path=
func (h *ExtendSourceHandlers) GetFile(c *gin.Context) {
	tid, ok := h.tenantID(c)
	if !ok {
		return
	}
	path := c.Query("path")
	content, err := h.sourceService.GetFile(tid, path)
	if err != nil {
		h.writeSourceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"path": path, "content": content})
}

type putSourceFileBody struct {
	Path    string `json:"path" binding:"required"`
	Content string `json:"content"`
}

// PutFile handles PUT /api/v1/extend/source/file
func (h *ExtendSourceHandlers) PutFile(c *gin.Context) {
	tid, ok := h.tenantID(c)
	if !ok {
		return
	}
	var body putSourceFileBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path and content required"})
		return
	}
	if err := h.sourceService.PutFile(tid, body.Path, body.Content); err != nil {
		h.writeSourceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "path": body.Path})
}

// GetRegistry handles GET /api/v1/extend/source/registry
func (h *ExtendSourceHandlers) GetRegistry(c *gin.Context) {
	tid, ok := h.tenantID(c)
	if !ok {
		return
	}
	reg, err := h.registryService.GetRegistry(tid)
	if err != nil {
		h.writeSourceError(c, err)
		return
	}
	c.JSON(http.StatusOK, reg)
}

type createHookBody struct {
	ID   string `json:"id" binding:"required"`
	Kind string `json:"kind"`
}

// CreateHook handles POST /api/v1/extend/source/hooks
func (h *ExtendSourceHandlers) CreateHook(c *gin.Context) {
	tid, ok := h.tenantID(c)
	if !ok {
		return
	}
	var body createHookBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id required"})
		return
	}
	res, err := h.registryService.CreateHook(tid, body.ID, body.Kind)
	if err != nil {
		h.writeSourceError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// DeleteHook handles DELETE /api/v1/extend/source/hooks/:id
func (h *ExtendSourceHandlers) DeleteHook(c *gin.Context) {
	tid, ok := h.tenantID(c)
	if !ok {
		return
	}
	id := c.Param("id")
	res, err := h.registryService.DeleteHook(tid, id)
	if err != nil {
		h.writeSourceError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// GetHookUsages handles GET /api/v1/extend/source/hooks/:id/usages
func (h *ExtendSourceHandlers) GetHookUsages(c *gin.Context) {
	tid, ok := h.tenantID(c)
	if !ok {
		return
	}
	usages, err := h.usageService.FindHookUsages(tid, c.Param("id"))
	if err != nil {
		h.writeSourceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"usages": usages})
}

type createRouteBody struct {
	Name string `json:"name" binding:"required"`
}

// CreateRoute handles POST /api/v1/extend/source/routes
func (h *ExtendSourceHandlers) CreateRoute(c *gin.Context) {
	tid, ok := h.tenantID(c)
	if !ok {
		return
	}
	var body createRouteBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	res, err := h.registryService.CreateRoute(tid, body.Name)
	if err != nil {
		h.writeSourceError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// DeleteRoute handles DELETE /api/v1/extend/source/routes/:name
func (h *ExtendSourceHandlers) DeleteRoute(c *gin.Context) {
	tid, ok := h.tenantID(c)
	if !ok {
		return
	}
	res, err := h.registryService.DeleteRoute(tid, c.Param("name"))
	if err != nil {
		h.writeSourceError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// GetRouteUsages handles GET /api/v1/extend/source/routes/:name/usages
func (h *ExtendSourceHandlers) GetRouteUsages(c *gin.Context) {
	tid, ok := h.tenantID(c)
	if !ok {
		return
	}
	usages, err := h.usageService.FindRouteUsages(tid, c.Param("name"))
	if err != nil {
		h.writeSourceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"usages": usages})
}
