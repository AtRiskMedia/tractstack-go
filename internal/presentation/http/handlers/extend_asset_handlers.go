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

// ExtendAssetHandlers serves /api/v1/extend/assets/*.
type ExtendAssetHandlers struct {
	assetService *services.ExtendAssetService
	logger       *logging.ChanneledLogger
	perfTracker  *performance.Tracker
}

// NewExtendAssetHandlers constructs asset handlers.
func NewExtendAssetHandlers(assetService *services.ExtendAssetService, logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *ExtendAssetHandlers {
	return &ExtendAssetHandlers{
		assetService: assetService,
		logger:       logger,
		perfTracker:  perfTracker,
	}
}

func (h *ExtendAssetHandlers) tenantID(c *gin.Context) (string, bool) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return "", false
	}
	return tenantCtx.TenantID, true
}

func (h *ExtendAssetHandlers) writeAssetError(c *gin.Context, err error) {
	var inUse *services.AssetInUseError
	switch {
	case errors.As(err, &inUse):
		c.JSON(http.StatusConflict, gin.H{"error": "asset is referenced", "references": inUse.References})
	case errors.Is(err, services.ErrAssetNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrAssetNotText):
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrAssetInvalidPath), errors.Is(err, services.ErrAssetForbidden), errors.Is(err, services.ErrAssetUnzipRejected):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrAssetExists):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrAssetTooLarge), errors.Is(err, services.ErrAssetTreeTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": err.Error()})
	default:
		h.logger.System().Error("extend asset error", "error", err.Error(), "path", c.Request.URL.Path)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

// GetTree handles GET /api/v1/extend/assets/tree
func (h *ExtendAssetHandlers) GetTree(c *gin.Context) {
	tenantID, ok := h.tenantID(c)
	if !ok {
		return
	}
	nodes, err := h.assetService.Tree(tenantID)
	if err != nil {
		h.writeAssetError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tree": nodes})
}

// GetFile handles GET /api/v1/extend/assets/file?path=
func (h *ExtendAssetHandlers) GetFile(c *gin.Context) {
	tenantID, ok := h.tenantID(c)
	if !ok {
		return
	}
	path := c.Query("path")
	content, err := h.assetService.GetFile(tenantID, path)
	if err != nil {
		h.writeAssetError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"path": path, "content": content})
}

type putFileBody struct {
	Path    string `json:"path" binding:"required"`
	Content string `json:"content"`
}

// PutFile handles PUT /api/v1/extend/assets/file
func (h *ExtendAssetHandlers) PutFile(c *gin.Context) {
	tenantID, ok := h.tenantID(c)
	if !ok {
		return
	}
	var body putFileBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.assetService.PutFile(tenantID, body.Path, body.Content); err != nil {
		h.writeAssetError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "path": body.Path})
}

// Upload handles POST /api/v1/extend/assets/upload
func (h *ExtendAssetHandlers) Upload(c *gin.Context) {
	tenantID, ok := h.tenantID(c)
	if !ok {
		return
	}
	mr, err := c.Request.MultipartReader()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "multipart required: " + err.Error()})
		return
	}
	if err := h.assetService.UploadFromMultipart(tenantID, mr); err != nil {
		h.writeAssetError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type pathBody struct {
	Path string `json:"path" binding:"required"`
}

// Mkdir handles POST /api/v1/extend/assets/mkdir
func (h *ExtendAssetHandlers) Mkdir(c *gin.Context) {
	tenantID, ok := h.tenantID(c)
	if !ok {
		return
	}
	var body pathBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.assetService.Mkdir(tenantID, body.Path); err != nil {
		h.writeAssetError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "path": body.Path})
}

type renameBody struct {
	From string `json:"from" binding:"required"`
	To   string `json:"to" binding:"required"`
}

// Rename handles POST /api/v1/extend/assets/rename
func (h *ExtendAssetHandlers) Rename(c *gin.Context) {
	tenantID, ok := h.tenantID(c)
	if !ok {
		return
	}
	var body renameBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.assetService.Rename(tenantID, body.From, body.To); err != nil {
		h.writeAssetError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "from": body.From, "to": body.To})
}

// DeleteFile handles DELETE /api/v1/extend/assets/file?path=&force=
func (h *ExtendAssetHandlers) DeleteFile(c *gin.Context) {
	tenantID, ok := h.tenantID(c)
	if !ok {
		return
	}
	path := c.Query("path")
	force := c.Query("force") == "1"
	if err := h.assetService.Delete(tenantID, path, force); err != nil {
		h.writeAssetError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "path": path})
}

type unzipBody struct {
	Path   string `json:"path" binding:"required"`
	Target string `json:"target" binding:"required"`
	Keep   int    `json:"keep"`
}

// Unzip handles POST /api/v1/extend/assets/unzip
func (h *ExtendAssetHandlers) Unzip(c *gin.Context) {
	tenantID, ok := h.tenantID(c)
	if !ok {
		return
	}
	var body unzipBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.assetService.Unzip(tenantID, body.Path, body.Target, body.Keep == 1); err != nil {
		h.writeAssetError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GetReferences handles GET /api/v1/extend/assets/references?path=
func (h *ExtendAssetHandlers) GetReferences(c *gin.Context) {
	tenantID, ok := h.tenantID(c)
	if !ok {
		return
	}
	path := c.Query("path")
	refs, err := h.assetService.FindReferences(tenantID, path)
	if err != nil {
		h.writeAssetError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"path": path, "references": refs})
}
