package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// ExtendHistoryHandlers serves /api/v1/extend/history/*.
type ExtendHistoryHandlers struct {
	historyService  *services.ExtendHistoryService
	registryService *services.ExtendRegistryService
	tenantManager   *tenant.Manager
	logger          *logging.ChanneledLogger
	perfTracker     *performance.Tracker
}

// NewExtendHistoryHandlers constructs history handlers.
func NewExtendHistoryHandlers(
	historyService *services.ExtendHistoryService,
	registryService *services.ExtendRegistryService,
	tenantManager *tenant.Manager,
	logger *logging.ChanneledLogger,
	perfTracker *performance.Tracker,
) *ExtendHistoryHandlers {
	return &ExtendHistoryHandlers{
		historyService:  historyService,
		registryService: registryService,
		tenantManager:   tenantManager,
		logger:          logger,
		perfTracker:     perfTracker,
	}
}

func (h *ExtendHistoryHandlers) tenantCtx(c *gin.Context) (*tenant.Context, bool) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return nil, false
	}
	return tenantCtx, true
}

func (h *ExtendHistoryHandlers) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrHistoryNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrHistoryInvalidPath), errors.Is(err, services.ErrHistoryInvalidJob):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrExtendDisabled):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrHistoryNoCustomCSS):
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	default:
		h.logger.System().Error("extend history error", "error", err.Error(), "path", c.Request.URL.Path)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

func (h *ExtendHistoryHandlers) activeTenantIDs() []string {
	ids := []string{}
	if h.tenantManager == nil {
		return []string{"default"}
	}
	det := h.tenantManager.GetDetector()
	if det == nil {
		return []string{"default"}
	}
	reg := det.GetRegistry()
	if reg == nil || reg.Tenants == nil {
		return []string{"default"}
	}
	for id, info := range reg.Tenants {
		if info.Status == "inactive" {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return []string{"default"}
	}
	return ids
}

type restoreFileBody struct {
	Path  string `json:"path" binding:"required"`
	JobID string `json:"jobId" binding:"required"`
}

type restoreCSSBody struct {
	JobID string `json:"jobId" binding:"required"`
}

// GetSourceSlots handles GET /extend/history/source?path=
func (h *ExtendHistoryHandlers) GetSourceSlots(c *gin.Context) {
	tenantCtx, ok := h.tenantCtx(c)
	if !ok {
		return
	}
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}
	ns := services.HistoryNsSource(path)
	slots, err := h.historyService.ListFileSlots(tenantCtx.TenantID, ns)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"path": path, "slots": slots})
}

// RestoreSource handles POST /extend/history/source/restore
func (h *ExtendHistoryHandlers) RestoreSource(c *gin.Context) {
	tenantCtx, ok := h.tenantCtx(c)
	if !ok {
		return
	}
	var body restoreFileBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path and jobId required"})
		return
	}
	ns := services.HistoryNsSource(body.Path)
	if err := h.historyService.RestoreFileSlot(tenantCtx, ns, body.JobID); err != nil {
		h.writeError(c, err)
		return
	}
	if err := h.registryService.RegenerateAll(h.activeTenantIDs()); err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "path": body.Path, "jobId": body.JobID, "regenerated": true})
}

// GetAssetSlots handles GET /extend/history/assets?path=
func (h *ExtendHistoryHandlers) GetAssetSlots(c *gin.Context) {
	tenantCtx, ok := h.tenantCtx(c)
	if !ok {
		return
	}
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}
	ns := services.HistoryNsAsset(path)
	slots, err := h.historyService.ListFileSlots(tenantCtx.TenantID, ns)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"path": path, "slots": slots})
}

// RestoreAsset handles POST /extend/history/assets/restore
func (h *ExtendHistoryHandlers) RestoreAsset(c *gin.Context) {
	tenantCtx, ok := h.tenantCtx(c)
	if !ok {
		return
	}
	var body restoreFileBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path and jobId required"})
		return
	}
	ns := services.HistoryNsAsset(body.Path)
	if err := h.historyService.RestoreFileSlot(tenantCtx, ns, body.JobID); err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "path": body.Path, "jobId": body.JobID})
}

// GetCustomCSSSlots handles GET /extend/history/custom-css
func (h *ExtendHistoryHandlers) GetCustomCSSSlots(c *gin.Context) {
	tenantCtx, ok := h.tenantCtx(c)
	if !ok {
		return
	}
	slots, err := h.historyService.ListFileSlots(tenantCtx.TenantID, services.HistoryNsCustomCSS())
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"path": services.CustomCSSPublicPath, "slots": slots})
}

// RestoreCustomCSS handles POST /extend/history/custom-css/restore
func (h *ExtendHistoryHandlers) RestoreCustomCSS(c *gin.Context) {
	tenantCtx, ok := h.tenantCtx(c)
	if !ok {
		return
	}
	var body restoreCSSBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "jobId required"})
		return
	}
	if err := h.historyService.RestoreFileSlot(tenantCtx, services.HistoryNsCustomCSS(), body.JobID); err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "jobId": body.JobID})
}

// GetSitePoints handles GET /extend/history/site
func (h *ExtendHistoryHandlers) GetSitePoints(c *gin.Context) {
	tenantCtx, ok := h.tenantCtx(c)
	if !ok {
		return
	}
	stamp := h.historyService.CurrentStamp()
	points, err := h.historyService.ListSitePoints(tenantCtx.TenantID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	// Join optional builtAt/status from jobs/<jobId>.json when present.
	for i := range points {
		jobPath := filepath.Join(services.JobsDir(), points[i].JobID+".json")
		b, err := os.ReadFile(jobPath)
		if err != nil {
			continue
		}
		var job services.RebuildJob
		if json.Unmarshal(b, &job) != nil {
			continue
		}
		points[i].BuiltAt = job.BuiltAt
		points[i].Status = job.Status
	}
	c.JSON(http.StatusOK, gin.H{
		"stamp":  gin.H{"jobId": stamp.JobID, "builtAt": stamp.BuiltAt},
		"points": points,
	})
}

// GetSiteManifest handles GET /extend/history/site/:jobId
func (h *ExtendHistoryHandlers) GetSiteManifest(c *gin.Context) {
	tenantCtx, ok := h.tenantCtx(c)
	if !ok {
		return
	}
	m, err := h.historyService.ReadManifest(tenantCtx.TenantID, c.Param("jobId"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, m)
}

// RestoreSite handles POST /extend/history/site/:jobId/restore
func (h *ExtendHistoryHandlers) RestoreSite(c *gin.Context) {
	tenantCtx, ok := h.tenantCtx(c)
	if !ok {
		return
	}
	jobID := c.Param("jobId")
	res, err := h.historyService.RestoreSitePoint(tenantCtx, jobID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	if err := h.registryService.RegenerateAll(h.activeTenantIDs()); err != nil {
		h.writeError(c, err)
		return
	}
	if tenantCtx.CacheManager != nil {
		tenantCtx.CacheManager.InvalidateTenant(tenantCtx.TenantID)
	}
	written := []string{}
	deleted := []string{}
	if res != nil {
		written = res.Written
		deleted = res.Deleted
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":          true,
		"jobId":       jobID,
		"written":     written,
		"deleted":     deleted,
		"regenerated": true,
		"invalidated": true,
	})
}
