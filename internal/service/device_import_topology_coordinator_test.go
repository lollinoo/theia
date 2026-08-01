package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestDeviceImportTopologyCoordinatorCompletesRunAfterReconciliation(t *testing.T) {
	runID := uuid.New()
	mapID := uuid.New()
	deviceID := uuid.New()
	repo := &fakeImportTopologyCoordinatorRepo{
		runID:       runID,
		mapID:       mapID,
		claimFound:  true,
		allTerminal: true,
	}
	var reconciledRunID uuid.UUID
	var notifications []uuid.UUID
	coordinator := NewDeviceImportTopologyCoordinator(
		repo,
		func(context.Context, uuid.UUID) bool { return true },
		func(_ context.Context, gotRunID uuid.UUID) (DeviceImportTopologyReconciliation, error) {
			reconciledRunID = gotRunID
			return DeviceImportTopologyReconciliation{Items: []domain.DeviceImportTopologyItemReconciliation{{
				DeviceID: deviceID, LinksCreated: 2,
			}}}, nil
		},
		func(gotRunID, _ uuid.UUID) { notifications = append(notifications, gotRunID) },
	)

	claimedRunID, tracked := coordinator.BootstrapStarted(context.Background(), deviceID, time.Now().UTC())
	if !tracked || claimedRunID != runID {
		t.Fatalf("BootstrapStarted = (%s, %v), want (%s, true)", claimedRunID, tracked, runID)
	}
	coordinator.BootstrapCompleted(context.Background(), runID, deviceID, DeviceImportTopologyBootstrapOutcome{
		NeighborCount:  3,
		LinksCreated:   2,
		CompletedAt:    time.Now().UTC(),
		ProtocolIssues: 1,
	})

	if len(repo.completions) != 1 {
		t.Fatalf("completion count = %d, want 1", len(repo.completions))
	}
	completion := repo.completions[0]
	if completion.State != domain.DeviceImportTopologyItemStateWarning || completion.ResultCode != domain.DeviceImportTopologyResultPartialProtocol {
		t.Fatalf("completion = %#v, want partial-protocol warning", completion)
	}
	if completion.NeighborCount != 3 || completion.LinksCreated != 2 {
		t.Fatalf("completion counters = %#v", completion)
	}
	wantStates := []domain.DeviceImportTopologyRunState{
		domain.DeviceImportTopologyRunStateReconciling,
		domain.DeviceImportTopologyRunStateReadyForLayout,
	}
	if len(repo.states) != len(wantStates) {
		t.Fatalf("run states = %#v, want %#v", repo.states, wantStates)
	}
	for index := range wantStates {
		if repo.states[index] != wantStates[index] {
			t.Fatalf("run states = %#v, want %#v", repo.states, wantStates)
		}
	}
	if reconciledRunID != runID {
		t.Fatalf("reconciled run = %s, want %s", reconciledRunID, runID)
	}
	if len(repo.reconciliationResults) != 1 || len(repo.reconciliationResults[0]) != 1 ||
		repo.reconciliationResults[0][0].DeviceID != deviceID ||
		repo.reconciliationResults[0][0].LinksCreated != 2 {
		t.Fatalf("persisted reconciliation results = %#v", repo.reconciliationResults)
	}
	if len(notifications) < 2 || notifications[len(notifications)-1] != runID {
		t.Fatalf("notifications = %#v", notifications)
	}
}

func TestDeviceImportTopologyCoordinatorSchedulesOneTargetedFollowup(t *testing.T) {
	runID := uuid.New()
	deviceID := uuid.New()
	repo := &fakeImportTopologyCoordinatorRepo{
		runID: runID, mapID: uuid.New(), allTerminal: true,
		followupRequeued: []uuid.UUID{deviceID},
	}
	var scheduled []uuid.UUID
	coordinator := NewDeviceImportTopologyCoordinator(
		repo,
		func(_ context.Context, id uuid.UUID) bool {
			scheduled = append(scheduled, id)
			return true
		},
		func(context.Context, uuid.UUID) (DeviceImportTopologyReconciliation, error) {
			return DeviceImportTopologyReconciliation{
				Items:             []domain.DeviceImportTopologyItemReconciliation{{DeviceID: deviceID}},
				FollowupDeviceIDs: []uuid.UUID{deviceID},
			}, nil
		},
		nil,
	)

	coordinator.BootstrapCompleted(context.Background(), runID, deviceID, DeviceImportTopologyBootstrapOutcome{
		NeighborCount: 1,
		CompletedAt:   time.Now().UTC(),
	})

	if repo.followupCalls != 1 {
		t.Fatalf("follow-up calls = %d, want 1", repo.followupCalls)
	}
	if len(scheduled) != 1 || scheduled[0] != deviceID {
		t.Fatalf("scheduled follow-up = %#v, want [%s]", scheduled, deviceID)
	}
	if len(repo.states) != 1 || repo.states[0] != domain.DeviceImportTopologyRunStateReconciling {
		t.Fatalf("run states = %#v, want reconciliation without ready layout", repo.states)
	}
}

func TestDeviceImportTopologyCoordinatorPersistsBoundedReconciliationFailure(t *testing.T) {
	runID := uuid.New()
	deviceID := uuid.New()
	repo := &fakeImportTopologyCoordinatorRepo{
		runID: runID, mapID: uuid.New(), allTerminal: true,
		failureRetries: []bool{true, false},
	}
	reconcileCalls := 0
	coordinator := NewDeviceImportTopologyCoordinator(
		repo,
		nil,
		func(context.Context, uuid.UUID) (DeviceImportTopologyReconciliation, error) {
			reconcileCalls++
			return DeviceImportTopologyReconciliation{}, errors.New("database error containing a secret")
		},
		nil,
	)

	coordinator.BootstrapCompleted(context.Background(), runID, deviceID, DeviceImportTopologyBootstrapOutcome{
		CompletedAt: time.Now().UTC(),
	})

	if reconcileCalls != 2 || len(repo.failures) != 2 {
		t.Fatalf("reconciliation calls=%d failures=%#v, want two bounded attempts", reconcileCalls, repo.failures)
	}
	for _, failure := range repo.failures {
		if failure.Code != domain.DeviceImportTopologyResultPersistence || failure.Message == "" || failure.Reference == "" {
			t.Fatalf("unsafe failure metadata = %#v", failure)
		}
		if failure.Message == "database error containing a secret" {
			t.Fatal("run failure leaked reconciliation error")
		}
	}
	if len(repo.states) != 1 || repo.states[0] != domain.DeviceImportTopologyRunStateReconciling {
		t.Fatalf("run states = %#v, want no false layout-ready transition", repo.states)
	}
}

func TestDeviceImportTopologyCoordinatorDoesNotAdvanceAfterFollowupPreparationFailure(t *testing.T) {
	runID := uuid.New()
	deviceID := uuid.New()
	repo := &fakeImportTopologyCoordinatorRepo{
		runID: runID, mapID: uuid.New(), allTerminal: true,
		failureRetries: []bool{true, false}, followupErr: errors.New("follow-up store unavailable"),
	}
	reconcileCalls := 0
	coordinator := NewDeviceImportTopologyCoordinator(
		repo,
		nil,
		func(context.Context, uuid.UUID) (DeviceImportTopologyReconciliation, error) {
			reconcileCalls++
			return DeviceImportTopologyReconciliation{
				Items:             []domain.DeviceImportTopologyItemReconciliation{{DeviceID: deviceID}},
				FollowupDeviceIDs: []uuid.UUID{deviceID},
			}, nil
		},
		nil,
	)

	coordinator.BootstrapCompleted(context.Background(), runID, deviceID, DeviceImportTopologyBootstrapOutcome{
		CompletedAt: time.Now().UTC(),
	})

	if reconcileCalls != 2 || repo.followupCalls != 2 || len(repo.failures) != 2 {
		t.Fatalf(
			"reconciliation calls=%d follow-up calls=%d failures=%d, want two bounded attempts",
			reconcileCalls,
			repo.followupCalls,
			len(repo.failures),
		)
	}
	if len(repo.states) != 1 || repo.states[0] != domain.DeviceImportTopologyRunStateReconciling {
		t.Fatalf("run states = %#v, want no false layout-ready transition", repo.states)
	}
}

func TestDeviceImportTopologyCoordinatorSanitizesDiscoveryFailure(t *testing.T) {
	runID := uuid.New()
	deviceID := uuid.New()
	repo := &fakeImportTopologyCoordinatorRepo{runID: runID, mapID: uuid.New(), claimFound: true}
	coordinator := NewDeviceImportTopologyCoordinator(repo, nil, nil, nil)
	coordinator.BootstrapCompleted(context.Background(), runID, deviceID, DeviceImportTopologyBootstrapOutcome{
		DiscoveryErr: errors.New("secret community public-private failed"),
		CompletedAt:  time.Now().UTC(),
	})

	if len(repo.completions) != 1 {
		t.Fatalf("completion count = %d, want 1", len(repo.completions))
	}
	completion := repo.completions[0]
	if completion.State != domain.DeviceImportTopologyItemStateFailed || completion.ResultCode != domain.DeviceImportTopologyResultSNMPUnreachable {
		t.Fatalf("completion = %#v, want SNMP failure", completion)
	}
	if completion.Message == "" || completion.Reference == "" {
		t.Fatalf("safe failure metadata = %#v", completion)
	}
	if completion.Message == "secret community public-private failed" {
		t.Fatal("completion leaked discovery error")
	}
}

func TestDeviceImportTopologyCoordinatorPreparesRecoveryBeforeWorkersAndResumesReconciliation(t *testing.T) {
	runID := uuid.New()
	firstID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	secondID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	repo := &fakeImportTopologyCoordinatorRepo{
		runID:       runID,
		mapID:       uuid.New(),
		recovered:   []uuid.UUID{secondID},
		reconciling: []uuid.UUID{runID},
	}
	var scheduled []uuid.UUID
	var reconciled []uuid.UUID
	coordinator := NewDeviceImportTopologyCoordinator(
		repo,
		func(_ context.Context, deviceID uuid.UUID) bool {
			scheduled = append(scheduled, deviceID)
			return true
		},
		func(_ context.Context, recoveredRunID uuid.UUID) (DeviceImportTopologyReconciliation, error) {
			reconciled = append(reconciled, recoveredRunID)
			return DeviceImportTopologyReconciliation{}, nil
		},
		nil,
	)
	coordinator.Launch(context.Background(), runID, []uuid.UUID{secondID, firstID, secondID})
	if len(scheduled) != 2 || scheduled[0] != firstID || scheduled[1] != secondID {
		t.Fatalf("scheduled launch devices = %#v, want sorted unique [%s %s]", scheduled, firstID, secondID)
	}

	scheduled = nil
	recovery, err := coordinator.PrepareRecovery(context.Background())
	if err != nil {
		t.Fatalf("PrepareRecovery: %v", err)
	}
	if len(scheduled) != 0 || len(reconciled) != 0 {
		t.Fatalf("recovery started runtime work before workers: scheduled=%#v reconciled=%#v", scheduled, reconciled)
	}
	if len(recovery.QueuedDeviceIDs) != 1 || recovery.QueuedDeviceIDs[0] != secondID {
		t.Fatalf("queued recovery devices = %#v, want [%s]", recovery.QueuedDeviceIDs, secondID)
	}

	if err := coordinator.ResumeRecovery(context.Background(), recovery); err != nil {
		t.Fatalf("ResumeRecovery: %v", err)
	}
	if len(scheduled) != 0 {
		t.Fatalf("recovery explicitly rescheduled devices already loaded by scheduler: %#v", scheduled)
	}
	if len(reconciled) != 1 || reconciled[0] != runID {
		t.Fatalf("resumed reconciliations = %#v, want [%s]", reconciled, runID)
	}
}

func TestDeviceImportTopologyCoordinatorResumesInterruptedReconciliation(t *testing.T) {
	runID := uuid.New()
	repo := &fakeImportTopologyCoordinatorRepo{
		runID: runID, mapID: uuid.New(), reconciling: []uuid.UUID{runID},
	}
	var reconciled []uuid.UUID
	coordinator := NewDeviceImportTopologyCoordinator(
		repo,
		nil,
		func(_ context.Context, recoveredRunID uuid.UUID) (DeviceImportTopologyReconciliation, error) {
			reconciled = append(reconciled, recoveredRunID)
			return DeviceImportTopologyReconciliation{}, nil
		},
		nil,
	)

	if err := coordinator.Recover(context.Background()); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if len(reconciled) != 1 || reconciled[0] != runID {
		t.Fatalf("reconciled runs = %#v, want [%s]", reconciled, runID)
	}
	if len(repo.states) != 1 || repo.states[0] != domain.DeviceImportTopologyRunStateReadyForLayout {
		t.Fatalf("run states = %#v, want ready for layout", repo.states)
	}
}

func TestDeviceImportTopologyCoordinatorActorControlsRetryAndManualEdit(t *testing.T) {
	runID := uuid.New()
	mapID := uuid.New()
	actorID := uuid.New()
	deviceID := uuid.New()
	repo := &fakeImportTopologyCoordinatorRepo{
		runID: runID,
		mapID: mapID,
		snapshot: domain.DeviceImportTopologyRunSnapshot{Run: domain.DeviceImportTopologyRun{
			ID: runID, MapID: mapID, ActorUserID: actorID,
		}},
		requeued: []uuid.UUID{deviceID},
	}
	var scheduled []uuid.UUID
	coordinator := NewDeviceImportTopologyCoordinator(
		repo,
		func(_ context.Context, id uuid.UUID) bool {
			scheduled = append(scheduled, id)
			return true
		},
		nil,
		nil,
	)

	if _, err := coordinator.GetRun(context.Background(), runID, actorID); err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if _, err := coordinator.GetActiveRun(context.Background(), mapID, actorID); err != nil {
		t.Fatalf("GetActiveRun: %v", err)
	}
	if err := coordinator.Continue(context.Background(), runID, actorID); err != nil {
		t.Fatalf("Continue: %v", err)
	}
	if err := coordinator.MarkManualEdit(context.Background(), runID, actorID); err != nil {
		t.Fatalf("MarkManualEdit: %v", err)
	}
	if err := coordinator.Retry(context.Background(), runID, actorID, []uuid.UUID{deviceID}); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	if err := coordinator.ApplyLayout(context.Background(), runID, actorID, domain.DeviceImportTopologyLayoutApply{
		InputToken: "sha256:test",
	}); err != nil {
		t.Fatalf("ApplyLayout: %v", err)
	}
	if repo.backgroundCalls != 1 || repo.manualEditCalls != 1 || repo.layoutCalls != 1 {
		t.Fatalf("control calls background=%d manual=%d layout=%d", repo.backgroundCalls, repo.manualEditCalls, repo.layoutCalls)
	}
	if len(scheduled) != 1 || scheduled[0] != deviceID {
		t.Fatalf("retry scheduled = %#v, want [%s]", scheduled, deviceID)
	}
}

type fakeImportTopologyCoordinatorRepo struct {
	runID                 uuid.UUID
	mapID                 uuid.UUID
	claimFound            bool
	allTerminal           bool
	completions           []domain.DeviceImportTopologyItemCompletion
	states                []domain.DeviceImportTopologyRunState
	recovered             []uuid.UUID
	reconciling           []uuid.UUID
	requeued              []uuid.UUID
	snapshot              domain.DeviceImportTopologyRunSnapshot
	backgroundCalls       int
	manualEditCalls       int
	layoutCalls           int
	followupCalls         int
	followupRequeued      []uuid.UUID
	followupErr           error
	failures              []domain.DeviceImportTopologyRunFailure
	failureRetries        []bool
	reconciliationResults [][]domain.DeviceImportTopologyItemReconciliation
}

func (r *fakeImportTopologyCoordinatorRepo) MarkItemRunning(
	context.Context,
	uuid.UUID,
	time.Time,
) (uuid.UUID, bool, error) {
	return r.runID, r.claimFound, nil
}

func (r *fakeImportTopologyCoordinatorRepo) CompleteItem(
	_ context.Context,
	_, _ uuid.UUID,
	completion domain.DeviceImportTopologyItemCompletion,
) (bool, error) {
	r.completions = append(r.completions, completion)
	return r.allTerminal, nil
}

func (r *fakeImportTopologyCoordinatorRepo) TransitionRun(
	_ context.Context,
	_ uuid.UUID,
	state domain.DeviceImportTopologyRunState,
) error {
	r.states = append(r.states, state)
	return nil
}

func (r *fakeImportTopologyCoordinatorRepo) RunMapID(context.Context, uuid.UUID) (uuid.UUID, error) {
	return r.mapID, nil
}

func (r *fakeImportTopologyCoordinatorRepo) RecoverInterrupted(context.Context) ([]uuid.UUID, error) {
	return append([]uuid.UUID(nil), r.recovered...), nil
}

func (r *fakeImportTopologyCoordinatorRepo) RecoverReconciling(context.Context) ([]uuid.UUID, error) {
	return append([]uuid.UUID(nil), r.reconciling...), nil
}

func (r *fakeImportTopologyCoordinatorRepo) ApplyReconciliationResults(
	_ context.Context,
	_ uuid.UUID,
	results []domain.DeviceImportTopologyItemReconciliation,
) error {
	r.reconciliationResults = append(
		r.reconciliationResults,
		append([]domain.DeviceImportTopologyItemReconciliation(nil), results...),
	)
	return nil
}

func (r *fakeImportTopologyCoordinatorRepo) RecordReconciliationFailure(
	_ context.Context,
	_ uuid.UUID,
	failure domain.DeviceImportTopologyRunFailure,
) (bool, error) {
	r.failures = append(r.failures, failure)
	if len(r.failureRetries) == 0 {
		return false, nil
	}
	retry := r.failureRetries[0]
	r.failureRetries = r.failureRetries[1:]
	return retry, nil
}

func (r *fakeImportTopologyCoordinatorRepo) GetForActor(context.Context, uuid.UUID, uuid.UUID) (domain.DeviceImportTopologyRunSnapshot, error) {
	return r.snapshot, nil
}

func (r *fakeImportTopologyCoordinatorRepo) GetActiveForMap(context.Context, uuid.UUID, uuid.UUID) (domain.DeviceImportTopologyRunSnapshot, error) {
	return r.snapshot, nil
}

func (r *fakeImportTopologyCoordinatorRepo) RequeueItems(context.Context, uuid.UUID, uuid.UUID, []uuid.UUID) ([]uuid.UUID, error) {
	return append([]uuid.UUID(nil), r.requeued...), nil
}

func (r *fakeImportTopologyCoordinatorRepo) RequeueFollowupItems(
	context.Context,
	uuid.UUID,
	[]uuid.UUID,
) ([]uuid.UUID, error) {
	r.followupCalls++
	return append([]uuid.UUID(nil), r.followupRequeued...), r.followupErr
}

func (r *fakeImportTopologyCoordinatorRepo) SetBackgrounded(context.Context, uuid.UUID, uuid.UUID) error {
	r.backgroundCalls++
	return nil
}

func (r *fakeImportTopologyCoordinatorRepo) DisableAutoLayout(context.Context, uuid.UUID, uuid.UUID) error {
	r.manualEditCalls++
	return nil
}

func (r *fakeImportTopologyCoordinatorRepo) ApplyLayout(
	context.Context,
	uuid.UUID,
	uuid.UUID,
	domain.DeviceImportTopologyLayoutApply,
) error {
	r.layoutCalls++
	return nil
}
