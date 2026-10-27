package handlers

import (
	"errors"
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/gin-gonic/gin"
)

// ExtendSourceHandlers serves /api/v1/extend/source/*.
type ExtendSourceHandlers struct {
	sourceService   *services.ExtendSourceService
	registryService *services.ExtendRegistryService
	logger          *logging.ChanneledLogger
	perfTracker     *performance.Tracker
}

// NewExtendSourceHandlers constructs source handlers.
func NewExtendSourceHandlers(
	sourceService *services.ExtendSourceService,
	registryService *services.ExtendRegistryService,
	logger *logging.ChanneledLogger,
	perfTracker *performance.Tracker,
) *ExtendSourceHandlers {
	return &ExtendSourceHandlers{
		sourceService:   sourceService,
		registryService: registryService,
		logger:          logger,
		perfTracker:     perfTracker,
	}
}

func (h *ExtendSourceHandlers) writeSourceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrExtendDisabled):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrSourceNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrSourceTooLarge), errors.Is(err, services.ErrSourceTreeTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrSourceInvalidPath), errors.Is(err, services.ErrSourceForbidden):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrSourceNotText):
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": err.Error()})
	default:
		h.logger.System().Error("extend source error", "error", err.Error(), "path", c.Request.URL.Path)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

// GetTree handles GET /api/v1/extend/source/tree
func (h *ExtendSourceHandlers) GetTree(c *gin.Context) {
	nodes, err := h.sourceService.Tree()
	if err != nil {
		h.writeSourceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tree": nodes})
}

// GetFile handles GET /api/v1/extend/source/file?path=
func (h *ExtendSourceHandlers) GetFile(c *gin.Context) {
	path := c.Query("path")
	content, err := h.sourceService.GetFile(path)
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
	var body putSourceFileBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path and content required"})
		return
	}
	if err := h.sourceService.PutFile(body.Path, body.Content); err != nil {
		h.writeSourceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "path": body.Path})
}

// GetRegistry handles GET /api/v1/extend/source/registry
func (h *ExtendSourceHandlers) GetRegistry(c *gin.Context) {
	reg, err := h.registryService.GetRegistry()
	if err != nil {
		h.writeSourceError(c, err)
		return
	}
	c.JSON(http.StatusOK, reg)
}
