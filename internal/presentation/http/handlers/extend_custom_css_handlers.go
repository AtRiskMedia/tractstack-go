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

// ExtendCustomCSSHandlers serves /api/v1/extend/custom-css.
type ExtendCustomCSSHandlers struct {
	service     *services.ExtendCustomCSSService
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// NewExtendCustomCSSHandlers constructs custom.css handlers.
func NewExtendCustomCSSHandlers(
	service *services.ExtendCustomCSSService,
	logger *logging.ChanneledLogger,
	perfTracker *performance.Tracker,
) *ExtendCustomCSSHandlers {
	return &ExtendCustomCSSHandlers{
		service:     service,
		logger:      logger,
		perfTracker: perfTracker,
	}
}

func (h *ExtendCustomCSSHandlers) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrCustomCSSNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrCustomCSSTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrCustomCSSNotText):
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": err.Error()})
	default:
		h.logger.System().Error("extend custom.css error", "error", err.Error(), "path", c.Request.URL.Path)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

// Get handles GET /api/v1/extend/custom-css
func (h *ExtendCustomCSSHandlers) Get(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}
	content, mtime, err := h.service.Get(tenantCtx.TenantID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"path":    services.CustomCSSPublicPath,
		"content": content,
		"mtime":   mtime,
	})
}

type putCustomCSSBody struct {
	Content *string `json:"content" binding:"required"`
}

// Put handles PUT /api/v1/extend/custom-css
func (h *ExtendCustomCSSHandlers) Put(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}
	var body putCustomCSSBody
	if err := c.ShouldBindJSON(&body); err != nil || body.Content == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content required"})
		return
	}
	stylesVer, err := h.service.Put(tenantCtx, *body.Content)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "stylesVer": stylesVer})
}
