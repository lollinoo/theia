package service

// This file coordinates worker completion with durable one-time topology import state.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/logging"
	"github.com/lollinoo/theia/internal/observability"
)

type deviceImportTopologyCoordinatorStore interface {
	MarkItemRunning(context.Context, uuid.UUID, time.Time) (uuid.UUID, bool, error)
	CompleteItem(context.Context, uuid.UUID, uuid.UUID, domain.DeviceImportTopologyItemCompletion) (bool, error)
	TransitionRun(context.Context, uuid.UUID, domain.DeviceImportTopologyRunState) error
	RunMapID(context.Context, uuid.UUID) (uuid.UUID, error)
	RecoverInterrupted(context.Context) ([]uuid.UUID, error)
	RecoverReconciling(context.Context) ([]uuid.UUID, error)
	ApplyReconciliationResults(context.Context, uuid.UUID, []domain.DeviceImportTopologyItemReconciliation) error
	RecordReconciliationFailure(context.Context, uuid.UUID, domain.DeviceImportTopologyRunFailure) (bool, error)
	GetForActor(context.Context, uuid.UUID, uuid.UUID) (domain.DeviceImportTopologyRunSnapshot, error)
	GetActiveForMap(context.Context, uuid.UUID, uuid.UUID) (domain.DeviceImportTopologyRunSnapshot, error)
	RequeueItems(context.Context, uuid.UUID, uuid.UUID, []uuid.UUID) ([]uuid.UUID, error)
	RequeueFollowupItems(context.Context, uuid.UUID, []uuid.UUID) ([]uuid.UUID, error)
	SetBackgrounded(context.Context, uuid.UUID, uuid.UUID) error
	DisableAutoLayout(context.Context, uuid.UUID, uuid.UUID) error
	ApplyLayout(context.Context, uuid.UUID, uuid.UUID, domain.DeviceImportTopologyLayoutApply) error
}

// ApplyLayout atomically commits a topology-derived layout after optimistic validation.
func (c *DeviceImportTopologyCoordinator) ApplyLayout(
	ctx context.Context,
	runID, actorUserID uuid.UUID,
	request domain.DeviceImportTopologyLayoutApply,
) error {
	if c == nil || c.store == nil {
		return domain.ErrDeviceImportStoreUnavailable
	}
	if err := c.store.ApplyLayout(normalizeDeviceImportContext(ctx), runID, actorUserID, request); err != nil {
		result := "error"
		switch {
		case errors.Is(err, domain.ErrDeviceImportTopologyLayoutStale):
			result = "stale"
		case errors.Is(err, domain.ErrDeviceImportTopologyRunConflict):
			result = "conflict"
		}
		observability.Default().IncDeviceImportTopologyLayout(result)
		return err
	}
	observability.Default().IncDeviceImportTopologyLayout("success")
	c.notifyRun(ctx, runID)
	return nil
}

// GetRun returns one actor-scoped authoritative progress snapshot.
func (c *DeviceImportTopologyCoordinator) GetRun(
	ctx context.Context,
	runID, actorUserID uuid.UUID,
) (domain.DeviceImportTopologyRunSnapshot, error) {
	if c == nil || c.store == nil {
		return domain.DeviceImportTopologyRunSnapshot{}, domain.ErrDeviceImportStoreUnavailable
	}
	return c.store.GetForActor(normalizeDeviceImportContext(ctx), runID, actorUserID)
}

// GetActiveRun resumes the actor's unfinished run after a browser refresh.
func (c *DeviceImportTopologyCoordinator) GetActiveRun(
	ctx context.Context,
	mapID, actorUserID uuid.UUID,
) (domain.DeviceImportTopologyRunSnapshot, error) {
	if c == nil || c.store == nil {
		return domain.DeviceImportTopologyRunSnapshot{}, domain.ErrDeviceImportStoreUnavailable
	}
	return c.store.GetActiveForMap(normalizeDeviceImportContext(ctx), mapID, actorUserID)
}

// Continue persists the non-blocking partial-map choice while discovery continues.
func (c *DeviceImportTopologyCoordinator) Continue(ctx context.Context, runID, actorUserID uuid.UUID) error {
	if c == nil || c.store == nil {
		return domain.ErrDeviceImportStoreUnavailable
	}
	if err := c.store.SetBackgrounded(normalizeDeviceImportContext(ctx), runID, actorUserID); err != nil {
		return err
	}
	observability.Default().IncDeviceImportTopologyRunEvent("backgrounded")
	c.notifyRun(ctx, runID)
	return nil
}

// MarkManualEdit disables automatic layout before the first user-authored canvas mutation.
func (c *DeviceImportTopologyCoordinator) MarkManualEdit(ctx context.Context, runID, actorUserID uuid.UUID) error {
	if c == nil || c.store == nil {
		return domain.ErrDeviceImportStoreUnavailable
	}
	if err := c.store.DisableAutoLayout(normalizeDeviceImportContext(ctx), runID, actorUserID); err != nil {
		return err
	}
	observability.Default().IncDeviceImportTopologyRunEvent("manual_edit")
	observability.Default().IncDeviceImportTopologyLayout("skipped_manual")
	c.notifyRun(ctx, runID)
	return nil
}

// Retry requeues terminal devices and schedules exactly those durable items.
func (c *DeviceImportTopologyCoordinator) Retry(
	ctx context.Context,
	runID, actorUserID uuid.UUID,
	deviceIDs []uuid.UUID,
) error {
	if c == nil || c.store == nil {
		return domain.ErrDeviceImportStoreUnavailable
	}
	requeued, err := c.store.RequeueItems(normalizeDeviceImportContext(ctx), runID, actorUserID, deviceIDs)
	if err != nil {
		return err
	}
	observability.Default().IncDeviceImportTopologyRetry("manual")
	if c.schedule != nil {
		for _, deviceID := range normalizedTopologyBootstrapDeviceIDs(requeued) {
			if !c.schedule(ctx, deviceID) {
				logging.Errorf("topology import retry scheduling deferred run_id=%s device_id=%s", runID, deviceID)
			}
		}
	}
	c.notifyRun(ctx, runID)
	return nil
}

// DeviceImportTopologyBootstrapOutcome carries only data needed for safe run progress reporting.
type DeviceImportTopologyBootstrapOutcome struct {
	DiscoveryErr        error
	PersistenceErr      error
	NeighborCount       int
	ProtocolIssues      int
	LinksCreated        int
	UnresolvedNeighbors int
	CompletedAt         time.Time
}

// DeviceImportTopologyRecoveryPlan contains durable work repaired before runtime workers start.
// Queued devices are intentionally not scheduled here: scheduler startup loads their restored
// pending state exactly once, while reconciling runs resume only after the pipeline is running.
type DeviceImportTopologyRecoveryPlan struct {
	QueuedDeviceIDs   []uuid.UUID
	ReconcilingRunIDs []uuid.UUID
}

// DeviceImportTopologyReconciliation carries final item summaries and an optional follow-up set.
type DeviceImportTopologyReconciliation struct {
	Items             []domain.DeviceImportTopologyItemReconciliation
	FollowupDeviceIDs []uuid.UUID
}

// DeviceImportTopologyCoordinator serializes run progress around the existing bootstrap scheduler.
type DeviceImportTopologyCoordinator struct {
	store     deviceImportTopologyCoordinatorStore
	schedule  func(context.Context, uuid.UUID) bool
	reconcile func(context.Context, uuid.UUID) (DeviceImportTopologyReconciliation, error)
	notify    func(uuid.UUID, uuid.UUID)
}

// NewDeviceImportTopologyCoordinator creates a durable bootstrap result coordinator.
func NewDeviceImportTopologyCoordinator(
	store deviceImportTopologyCoordinatorStore,
	schedule func(context.Context, uuid.UUID) bool,
	reconcile func(context.Context, uuid.UUID) (DeviceImportTopologyReconciliation, error),
	notify func(uuid.UUID, uuid.UUID),
) *DeviceImportTopologyCoordinator {
	return &DeviceImportTopologyCoordinator{
		store: store, schedule: schedule, reconcile: reconcile, notify: notify,
	}
}

// Launch schedules each imported device once after its run and map membership are durable.
func (c *DeviceImportTopologyCoordinator) Launch(ctx context.Context, runID uuid.UUID, deviceIDs []uuid.UUID) {
	if c == nil || c.schedule == nil || runID == uuid.Nil {
		return
	}
	for _, deviceID := range normalizedTopologyBootstrapDeviceIDs(deviceIDs) {
		if !c.schedule(normalizeDeviceImportContext(ctx), deviceID) {
			logging.Errorf("topology import bootstrap scheduling deferred run_id=%s device_id=%s", runID, deviceID)
		}
	}
	observability.Default().IncDeviceImportTopologyRunEvent("launched")
	c.notifyRun(ctx, runID)
}

// PrepareRecovery repairs interrupted durable state before scheduler startup can dispatch work.
func (c *DeviceImportTopologyCoordinator) PrepareRecovery(
	ctx context.Context,
) (DeviceImportTopologyRecoveryPlan, error) {
	var recovery DeviceImportTopologyRecoveryPlan
	if c == nil || c.store == nil {
		return recovery, nil
	}
	deviceIDs, err := c.store.RecoverInterrupted(normalizeDeviceImportContext(ctx))
	if err != nil {
		return recovery, fmt.Errorf("recovering topology import runs: %w", err)
	}
	recovery.QueuedDeviceIDs = normalizedTopologyBootstrapDeviceIDs(deviceIDs)
	reconcilingRunIDs, err := c.store.RecoverReconciling(normalizeDeviceImportContext(ctx))
	if err != nil {
		return recovery, fmt.Errorf("recovering topology import reconciliation: %w", err)
	}
	recovery.ReconcilingRunIDs = normalizedTopologyBootstrapDeviceIDs(reconcilingRunIDs)
	return recovery, nil
}

// ResumeRecovery restarts idempotent reconciliation after the runtime pipeline is available.
func (c *DeviceImportTopologyCoordinator) ResumeRecovery(
	ctx context.Context,
	recovery DeviceImportTopologyRecoveryPlan,
) error {
	if c == nil || c.store == nil {
		return nil
	}
	for _, runID := range normalizedTopologyBootstrapDeviceIDs(recovery.ReconcilingRunIDs) {
		if err := c.finishReconciliation(ctx, runID); err != nil {
			return fmt.Errorf("finishing recovered topology reconciliation %s: %w", runID, err)
		}
	}
	if len(recovery.QueuedDeviceIDs) > 0 || len(recovery.ReconcilingRunIDs) > 0 {
		observability.Default().IncDeviceImportTopologyRunEvent("recovered")
	}
	return nil
}

// Recover repairs and resumes state for callers that do not own pipeline startup ordering.
func (c *DeviceImportTopologyCoordinator) Recover(ctx context.Context) error {
	recovery, err := c.PrepareRecovery(ctx)
	if err != nil {
		return err
	}
	return c.ResumeRecovery(ctx, recovery)
}

// BootstrapStarted claims a queued import item before the worker performs network I/O.
func (c *DeviceImportTopologyCoordinator) BootstrapStarted(
	ctx context.Context,
	deviceID uuid.UUID,
	startedAt time.Time,
) (uuid.UUID, bool) {
	if c == nil || c.store == nil {
		return uuid.Nil, false
	}
	runID, found, err := c.store.MarkItemRunning(normalizeDeviceImportContext(ctx), deviceID, startedAt)
	if err != nil {
		logging.Errorf("topology import bootstrap claim failed reference=%s device_id=%s: %v", uuid.NewString(), deviceID, err)
		return uuid.Nil, false
	}
	if found {
		c.notifyRun(ctx, runID)
	}
	return runID, found
}

// BootstrapCompleted records a sanitized worker outcome and advances a terminal batch to layout.
func (c *DeviceImportTopologyCoordinator) BootstrapCompleted(
	ctx context.Context,
	runID, deviceID uuid.UUID,
	outcome DeviceImportTopologyBootstrapOutcome,
) {
	if c == nil || c.store == nil || runID == uuid.Nil || deviceID == uuid.Nil {
		return
	}
	completion := classifyDeviceImportTopologyOutcome(outcome)
	allTerminal, err := c.store.CompleteItem(normalizeDeviceImportContext(ctx), runID, deviceID, completion)
	if err != nil {
		logging.Errorf("topology import bootstrap completion failed reference=%s run_id=%s device_id=%s: %v", uuid.NewString(), runID, deviceID, err)
		return
	}
	observability.Default().IncDeviceImportTopologyItemOutcome(string(completion.ResultCode))
	c.notifyRun(ctx, runID)
	if !allTerminal {
		return
	}
	if err := c.store.TransitionRun(ctx, runID, domain.DeviceImportTopologyRunStateReconciling); err != nil {
		logging.Errorf("topology import reconciliation transition failed reference=%s run_id=%s: %v", uuid.NewString(), runID, err)
		return
	}
	c.notifyRun(ctx, runID)
	if err := c.finishReconciliation(ctx, runID); err != nil {
		logging.Errorf("topology import reconciliation completion failed reference=%s run_id=%s: %v", uuid.NewString(), runID, err)
	}
}

func (c *DeviceImportTopologyCoordinator) finishReconciliation(ctx context.Context, runID uuid.UUID) error {
	for {
		var reconciliation DeviceImportTopologyReconciliation
		if c.reconcile != nil {
			var reconcileErr error
			reconciliation, reconcileErr = c.reconcile(ctx, runID)
			if reconcileErr != nil {
				retry, err := c.recordReconciliationFailure(ctx, runID, "reconciliation", reconcileErr)
				if err != nil {
					return err
				}
				if retry {
					continue
				}
				return nil
			}
			if err := c.store.ApplyReconciliationResults(
				normalizeDeviceImportContext(ctx),
				runID,
				reconciliation.Items,
			); err != nil {
				retry, recordErr := c.recordReconciliationFailure(ctx, runID, "result persistence", err)
				if recordErr != nil {
					return recordErr
				}
				if retry {
					continue
				}
				return nil
			}
		}
		if len(reconciliation.FollowupDeviceIDs) > 0 {
			requeued, err := c.store.RequeueFollowupItems(
				normalizeDeviceImportContext(ctx),
				runID,
				reconciliation.FollowupDeviceIDs,
			)
			if err != nil {
				retry, recordErr := c.recordReconciliationFailure(ctx, runID, "follow-up preparation", err)
				if recordErr != nil {
					return recordErr
				}
				if retry {
					continue
				}
				return nil
			}
			if len(requeued) > 0 {
				observability.Default().IncDeviceImportTopologyRetry("automatic_followup")
				if c.schedule != nil {
					for _, followupDeviceID := range normalizedTopologyBootstrapDeviceIDs(requeued) {
						if !c.schedule(ctx, followupDeviceID) {
							logging.Errorf("topology import follow-up scheduling deferred run_id=%s device_id=%s", runID, followupDeviceID)
						}
					}
				}
				c.notifyRun(ctx, runID)
				return nil
			}
		}
		if err := c.store.TransitionRun(ctx, runID, domain.DeviceImportTopologyRunStateReadyForLayout); err != nil {
			return err
		}
		observability.Default().IncDeviceImportTopologyRunEvent("ready")
		c.notifyRun(ctx, runID)
		return nil
	}
}

func (c *DeviceImportTopologyCoordinator) recordReconciliationFailure(
	ctx context.Context,
	runID uuid.UUID,
	stage string,
	cause error,
) (bool, error) {
	reference := uuid.NewString()
	logging.Errorf("topology import %s failed reference=%s run_id=%s: %v", stage, reference, runID, cause)
	retry, err := c.store.RecordReconciliationFailure(
		normalizeDeviceImportContext(ctx),
		runID,
		domain.DeviceImportTopologyRunFailure{
			Code:      domain.DeviceImportTopologyResultPersistence,
			Message:   "Automatic link creation could not complete.",
			Reference: reference,
		},
	)
	if err != nil {
		return false, fmt.Errorf("recording topology reconciliation failure: %w", err)
	}
	observability.Default().IncDeviceImportTopologyRunEvent("reconciliation_failed")
	c.notifyRun(ctx, runID)
	if retry {
		observability.Default().IncDeviceImportTopologyRetry("reconciliation")
	}
	return retry, nil
}

func (c *DeviceImportTopologyCoordinator) notifyRun(ctx context.Context, runID uuid.UUID) {
	if c.notify == nil {
		return
	}
	mapID, err := c.store.RunMapID(normalizeDeviceImportContext(ctx), runID)
	if err != nil {
		return
	}
	c.notify(runID, mapID)
}

func classifyDeviceImportTopologyOutcome(outcome DeviceImportTopologyBootstrapOutcome) domain.DeviceImportTopologyItemCompletion {
	completedAt := outcome.CompletedAt
	if completedAt.IsZero() {
		completedAt = time.Now().UTC()
	}
	completion := domain.DeviceImportTopologyItemCompletion{
		State:               domain.DeviceImportTopologyItemStateSucceeded,
		ResultCode:          domain.DeviceImportTopologyResultDiscovered,
		Message:             "Topology discovery completed.",
		NeighborCount:       max(outcome.NeighborCount, 0),
		LinksCreated:        max(outcome.LinksCreated, 0),
		UnresolvedNeighbors: max(outcome.UnresolvedNeighbors, 0),
		CompletedAt:         completedAt.UTC(),
	}
	switch {
	case outcome.PersistenceErr != nil:
		completion.State = domain.DeviceImportTopologyItemStateFailed
		completion.ResultCode = domain.DeviceImportTopologyResultPersistence
		completion.Message = "Discovery data could not be saved."
		completion.Reference = uuid.NewString()
		logging.Errorf("topology import persistence failed reference=%s: %v", completion.Reference, outcome.PersistenceErr)
	case outcome.DiscoveryErr != nil:
		completion.State = domain.DeviceImportTopologyItemStateFailed
		completion.ResultCode = domain.DeviceImportTopologyResultSNMPUnreachable
		completion.Message = "SNMP topology discovery did not complete."
		completion.Reference = uuid.NewString()
		logging.Errorf("topology import discovery failed reference=%s: %v", completion.Reference, outcome.DiscoveryErr)
	case outcome.UnresolvedNeighbors > 0:
		completion.State = domain.DeviceImportTopologyItemStateWarning
		completion.ResultCode = domain.DeviceImportTopologyResultUnresolvedNeighbors
		completion.Message = "Some discovered neighbors could not be linked automatically."
	case outcome.ProtocolIssues > 0:
		completion.State = domain.DeviceImportTopologyItemStateWarning
		completion.ResultCode = domain.DeviceImportTopologyResultPartialProtocol
		completion.Message = "Topology discovery completed with partial protocol support."
	case outcome.NeighborCount == 0:
		completion.State = domain.DeviceImportTopologyItemStateWarning
		completion.ResultCode = domain.DeviceImportTopologyResultNoNeighbors
		completion.Message = "No LLDP or CDP neighbors were discovered."
	}
	return completion
}

func normalizedTopologyBootstrapDeviceIDs(deviceIDs []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(deviceIDs))
	normalized := make([]uuid.UUID, 0, len(deviceIDs))
	for _, deviceID := range deviceIDs {
		if deviceID == uuid.Nil {
			continue
		}
		if _, ok := seen[deviceID]; ok {
			continue
		}
		seen[deviceID] = struct{}{}
		normalized = append(normalized, deviceID)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].String() < normalized[j].String()
	})
	return normalized
}
