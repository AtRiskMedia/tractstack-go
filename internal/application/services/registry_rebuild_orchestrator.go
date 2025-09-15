// in: internal/application/services/registry_rebuild_orchestrator.go

package services

import (
	"encoding/json"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// rebuildJob defines the data needed to perform a registry rebuild.
type rebuildJob struct {
	tenantID        string
	storyfragmentID string
}

// RegistryRebuildOrchestrator manages an asynchronous queue for rebuilding
// StoryfragmentBeliefRegistry entries to prevent stale cache data.
type RegistryRebuildOrchestrator struct {
	logger            *logging.ChanneledLogger
	jobQueue          chan rebuildJob
	tenantManager     *tenant.Manager
	beliefRegistrySvc *BeliefRegistryService
	paneSvc           *PaneService
}

// NewRegistryRebuildOrchestrator creates and starts a new orchestrator.
func NewRegistryRebuildOrchestrator(
	logger *logging.ChanneledLogger,
	tenantManager *tenant.Manager,
	brs *BeliefRegistryService,
	// PaneService is omitted here to be set later, breaking the dependency cycle.
) *RegistryRebuildOrchestrator {
	orchestrator := &RegistryRebuildOrchestrator{
		logger:            logger,
		jobQueue:          make(chan rebuildJob, 100), // Buffered channel for 100 jobs
		tenantManager:     tenantManager,
		beliefRegistrySvc: brs,
		// paneSvc is intentionally nil at this stage.
	}

	go orchestrator.startWorker()

	logger.Startup().Info("RegistryRebuildOrchestrator started with a background worker.")
	return orchestrator
}

// SetPaneService injects the PaneService after initialization to break a dependency cycle.
func (o *RegistryRebuildOrchestrator) SetPaneService(ps *PaneService) {
	o.paneSvc = ps
}

// EnqueueRebuild adds a new storyfragment ID to the queue for a belief registry rebuild.
func (o *RegistryRebuildOrchestrator) EnqueueRebuild(tenantID, storyfragmentID string) {
	if storyfragmentID == "" || tenantID == "" {
		o.logger.Cache().Warn("Attempted to enqueue rebuild job with empty tenant or storyfragment ID.")
		return
	}

	select {
	case o.jobQueue <- rebuildJob{tenantID: tenantID, storyfragmentID: storyfragmentID}:
		o.logger.Cache().Debug("Enqueued belief registry rebuild job", "tenantId", tenantID, "storyfragmentId", storyfragmentID)
	default:
		o.logger.Cache().Warn("Belief registry rebuild queue is full. Job dropped.", "tenantId", tenantID, "storyfragmentId", storyfragmentID)
	}
}

// startWorker is the background goroutine that processes rebuild jobs sequentially.
func (o *RegistryRebuildOrchestrator) startWorker() {
	for job := range o.jobQueue {
		// Defensive check to ensure PaneService was injected before worker runs.
		if o.paneSvc == nil {
			o.logger.Cache().Error("Orchestrator worker cannot run: PaneService was not injected.", "storyfragmentId", job.storyfragmentID)
			continue
		}

		o.logger.Cache().Info("Worker picked up belief registry rebuild job", "tenantId", job.tenantID, "storyfragmentId", job.storyfragmentID)

		tenantCtx, err := o.tenantManager.NewContextFromID(job.tenantID)
		if err != nil {
			o.logger.Cache().Error("Worker failed to create tenant context", "error", err, "tenantId", job.tenantID)
			continue
		}

		sf, err := tenantCtx.StoryFragmentRepo().FindByID(tenantCtx.TenantID, job.storyfragmentID)
		if err != nil || sf == nil {
			o.logger.Cache().Error("Worker could not find storyfragment for rebuild", "error", err, "storyfragmentId", job.storyfragmentID)
			if closeErr := tenantCtx.Close(); closeErr != nil {
				o.logger.Cache().Error("Worker failed to close tenant context after error", "error", closeErr, "tenantId", job.tenantID)
			}
			continue
		}

		panes, err := o.paneSvc.GetByIDs(tenantCtx, sf.PaneIDs)
		if err != nil {
			o.logger.Cache().Error("Worker could not find panes for rebuild", "error", err, "storyfragmentId", job.storyfragmentID)
			if closeErr := tenantCtx.Close(); closeErr != nil {
				o.logger.Cache().Error("Worker failed to close tenant context after error", "error", closeErr, "tenantId", job.tenantID)
			}
			continue
		}

		if len(panes) == 0 {
			o.logger.Cache().Debug("Worker found 0 panes for this storyfragment.", "storyfragmentId", job.storyfragmentID)
		}
		for _, pane := range panes {
			// Using JSON marshalling to get a clean printout of the map.
			heldBeliefsJSON, _ := json.Marshal(pane.HeldBeliefs)
			withheldBeliefsJSON, _ := json.Marshal(pane.WithheldBeliefs)
			o.logger.Cache().Debug(
				"Worker is building with this pane data",
				"paneId", pane.ID,
				"paneTitle", pane.Title,
				"heldBeliefs", string(heldBeliefsJSON),
				"withheldBeliefs", string(withheldBeliefsJSON),
			)
		}

		tenantCtx.CacheManager.InvalidateStoryfragmentBeliefRegistry(tenantCtx.TenantID, job.storyfragmentID)

		if _, err := o.beliefRegistrySvc.BuildRegistryFromLoadedPanes(tenantCtx, job.storyfragmentID, panes); err != nil {
			o.logger.Cache().Error("Worker failed to build belief registry", "error", err, "storyfragmentId", job.storyfragmentID)
		} else {
			o.logger.Cache().Info("Worker successfully rebuilt belief registry", "storyfragmentId", job.storyfragmentID)
		}

		if err := tenantCtx.Close(); err != nil {
			o.logger.Cache().Error("Worker failed to close tenant context", "error", err, "tenantId", job.tenantID)
		}
	}
}
