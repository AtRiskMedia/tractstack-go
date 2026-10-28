package handlers

import (
	"errors"
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/gin-gonic/gin"
)

// ExtendRebuildHandlers serves /api/v1/extend/rebuild*.
type ExtendRebuildHandlers struct {
	rebuildService *services.ExtendRebuildService
	logger         *logging.ChanneledLogger
	perfTracker    *performance.Tracker
}

// NewExtendRebuildHandlers constructs rebuild handlers.
func NewExtendRebuildHandlers(
	rebuildService *services.ExtendRebuildService,
	logger *logging.ChanneledLogger,
	perfTracker *performance.Tracker,
) *ExtendRebuildHandlers {
	return &ExtendRebuildHandlers{
		rebuildService: rebuildService,
		logger:         logger,
		perfTracker:    perfTracker,
	}
}

func (h *ExtendRebuildHandlers) writeRebuildError(c *gin.Context, err error) {
	var busy *services.ErrRebuildBusy
	switch {
	case errors.As(err, &busy):
		c.JSON(http.StatusConflict, gin.H{"jobId": busy.JobID})
	case errors.Is(err, services.ErrRebuildUnavailable):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrRebuildNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		h.logger.System().Error("extend rebuild error", "error", err.Error(), "path", c.Request.URL.Path)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

// PostRebuild handles POST /api/v1/extend/rebuild
func (h *ExtendRebuildHandlers) PostRebuild(c *gin.Context) {
	jobID, err := h.rebuildService.QueueRebuild()
	if err != nil {
		h.writeRebuildError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"jobId": jobID})
}

// GetLatest handles GET /api/v1/extend/rebuild/latest
func (h *ExtendRebuildHandlers) GetLatest(c *gin.Context) {
	job, err := h.rebuildService.GetLatest()
	if err != nil {
		h.writeRebuildError(c, err)
		return
	}
	if job == nil {
		c.JSON(http.StatusOK, nil)
		return
	}
	c.JSON(http.StatusOK, job)
}

// GetJob handles GET /api/v1/extend/rebuild/:jobId
func (h *ExtendRebuildHandlers) GetJob(c *gin.Context) {
	jobID := c.Param("jobId")
	resp, err := h.rebuildService.GetJob(jobID)
	if err != nil {
		h.writeRebuildError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}
