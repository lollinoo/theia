package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	devicecache "github.com/lollinoo/theia/internal/cache"
	"github.com/lollinoo/theia/internal/domain"
)

func TestDeviceImportTopologyRunRepoPersistsLifecycle(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mapID := uuid.New()
	actorID := uuid.New()
	insertDeviceImportTestMap(t, db, mapID)
	insertDeviceImportTopologyTestUser(t, db, actorID)

	deviceRepo := NewDeviceRepo(db, testKeyring, nil)
	device := newDeviceImportTestDevice("bootstrap-router.example.net")
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("create device: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO canvas_map_devices (map_id, device_id, role, added_at)
		 VALUES ($1, $2, $3, NOW())`,
		mapID.String(), device.ID.String(), string(domain.CanvasMapDeviceRoleBase),
	); err != nil {
		t.Fatalf("insert device membership: %v", err)
	}

	repo := NewDeviceImportTopologyRunRepo(db)
	run := &domain.DeviceImportTopologyRun{
		ID:                uuid.New(),
		MapID:             mapID,
		ActorUserID:       actorID,
		FileDigest:        "sha256:test",
		LayoutScope:       domain.DeviceImportTopologyLayoutScopePreserve,
		State:             domain.DeviceImportTopologyRunStateImporting,
		AutoLayoutAllowed: true,
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.AddItem(ctx, run.ID, device.ID); err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	if err := repo.FinalizeImport(ctx, run.ID); err != nil {
		t.Fatalf("FinalizeImport: %v", err)
	}
	if err := repo.FinalizeImport(ctx, run.ID); err != nil {
		t.Fatalf("idempotent FinalizeImport: %v", err)
	}

	snapshot, err := repo.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if snapshot.Run.State != domain.DeviceImportTopologyRunStateDiscovering {
		t.Fatalf("run state = %q, want %q", snapshot.Run.State, domain.DeviceImportTopologyRunStateDiscovering)
	}
	if !snapshot.Run.AutoLayoutAllowed {
		t.Fatal("auto layout disabled, want enabled")
	}
	if snapshot.Run.LayoutInputToken != "" {
		t.Fatalf("discovering layout token = %q, want omitted until layout is ready", snapshot.Run.LayoutInputToken)
	}
	if len(snapshot.Items) != 1 || snapshot.Items[0].DeviceID != device.ID {
		t.Fatalf("items = %#v, want device %s", snapshot.Items, device.ID)
	}
	if snapshot.Items[0].State != domain.DeviceImportTopologyItemStateQueued {
		t.Fatalf("item state = %q, want queued", snapshot.Items[0].State)
	}

	startedAt := time.Date(2026, time.July, 31, 10, 0, 0, 0, time.UTC)
	runID, found, err := repo.MarkItemRunning(ctx, device.ID, startedAt)
	if err != nil {
		t.Fatalf("MarkItemRunning: %v", err)
	}
	if !found || runID != run.ID {
		t.Fatalf("running item lookup = (%s, %v), want (%s, true)", runID, found, run.ID)
	}

	completedAt := startedAt.Add(2 * time.Second)
	allTerminal, err := repo.CompleteItem(ctx, run.ID, device.ID, domain.DeviceImportTopologyItemCompletion{
		State:               domain.DeviceImportTopologyItemStateWarning,
		ResultCode:          domain.DeviceImportTopologyResultUnresolvedNeighbors,
		Message:             "Some neighbors need attention.",
		NeighborCount:       3,
		LinksCreated:        1,
		UnresolvedNeighbors: 2,
		CompletedAt:         completedAt,
	})
	if err != nil {
		t.Fatalf("CompleteItem: %v", err)
	}
	if !allTerminal {
		t.Fatal("allTerminal = false, want true")
	}

	snapshot, err = repo.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("Get after completion: %v", err)
	}
	item := snapshot.Items[0]
	if item.Attempt != 1 || item.State != domain.DeviceImportTopologyItemStateWarning {
		t.Fatalf("completed item = %#v, want warning attempt 1", item)
	}
	if item.NeighborCount != 3 || item.LinksCreated != 1 || item.UnresolvedNeighbors != 2 {
		t.Fatalf("completed item counters = %#v", item)
	}
	if err := repo.TransitionRun(ctx, run.ID, domain.DeviceImportTopologyRunStateReconciling); err != nil {
		t.Fatalf("TransitionRun(reconciling): %v", err)
	}
	reconciling, err := repo.RecoverReconciling(ctx)
	if err != nil {
		t.Fatalf("RecoverReconciling: %v", err)
	}
	if len(reconciling) != 1 || reconciling[0] != run.ID {
		t.Fatalf("reconciling runs = %#v, want [%s]", reconciling, run.ID)
	}
	if err := repo.TransitionRun(ctx, run.ID, domain.DeviceImportTopologyRunStateReadyForLayout); err != nil {
		t.Fatalf("TransitionRun(ready): %v", err)
	}
	gotMapID, err := repo.RunMapID(ctx, run.ID)
	if err != nil {
		t.Fatalf("RunMapID: %v", err)
	}
	if gotMapID != mapID {
		t.Fatalf("RunMapID = %s, want %s", gotMapID, mapID)
	}
}

func TestDeviceImportTopologyRunRepoManualEditCompletesLayoutOwnership(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mapID := uuid.New()
	actorID := uuid.New()
	insertDeviceImportTestMap(t, db, mapID)
	insertDeviceImportTopologyTestUser(t, db, actorID)
	repo := NewDeviceImportTopologyRunRepo(db)

	createRun := func(state domain.DeviceImportTopologyRunState) uuid.UUID {
		run := &domain.DeviceImportTopologyRun{
			ID: uuid.New(), MapID: mapID, ActorUserID: actorID, FileDigest: "sha256:manual",
			LayoutScope: domain.DeviceImportTopologyLayoutScopePreserve, State: domain.DeviceImportTopologyRunStateImporting,
			AutoLayoutAllowed: true,
		}
		if err := repo.Create(ctx, run); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if _, err := db.Exec(`UPDATE device_import_topology_runs SET state = $1 WHERE id = $2`, string(state), run.ID.String()); err != nil {
			t.Fatalf("set state: %v", err)
		}
		return run.ID
	}

	readyRunID := createRun(domain.DeviceImportTopologyRunStateReadyForLayout)
	if err := repo.DisableAutoLayout(ctx, readyRunID, actorID); err != nil {
		t.Fatalf("DisableAutoLayout(ready): %v", err)
	}
	readySnapshot, err := repo.Get(ctx, readyRunID)
	if err != nil {
		t.Fatalf("Get ready run: %v", err)
	}
	if readySnapshot.Run.State != domain.DeviceImportTopologyRunStateCompleted || readySnapshot.Run.CompletedAt == nil {
		t.Fatalf("ready manual run = %#v, want completed", readySnapshot.Run)
	}
	if _, err := repo.GetActiveForMap(ctx, mapID, actorID); !errors.Is(err, ErrDeviceImportTopologyRunNotFound) {
		t.Fatalf("GetActiveForMap after manual completion error = %v, want not found", err)
	}

	reconcilingRunID := createRun(domain.DeviceImportTopologyRunStateReconciling)
	if err := repo.DisableAutoLayout(ctx, reconcilingRunID, actorID); err != nil {
		t.Fatalf("DisableAutoLayout(reconciling): %v", err)
	}
	reconcilingSnapshot, err := repo.Get(ctx, reconcilingRunID)
	if err != nil {
		t.Fatalf("Get reconciling run: %v", err)
	}
	if reconcilingSnapshot.Run.State != domain.DeviceImportTopologyRunStateCompleted || reconcilingSnapshot.Run.CompletedAt == nil {
		t.Fatalf("reconciled manual run = %#v, want completed", reconcilingSnapshot.Run)
	}
}

func TestDeviceImportTopologyRunRepoSupersedesAbandonedRunOnCreate(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mapID := uuid.New()
	actorID := uuid.New()
	insertDeviceImportTestMap(t, db, mapID)
	insertDeviceImportTopologyTestUser(t, db, actorID)
	repo := NewDeviceImportTopologyRunRepo(db)
	now := time.Date(2026, time.July, 31, 8, 0, 0, 0, time.UTC)
	repo.now = func() time.Time { return now }
	newRun := func(id uuid.UUID) *domain.DeviceImportTopologyRun {
		return &domain.DeviceImportTopologyRun{
			ID: id, MapID: mapID, ActorUserID: actorID, FileDigest: "sha256:abandoned",
			LayoutScope: domain.DeviceImportTopologyLayoutScopePreserve,
		}
	}

	first := newRun(uuid.New())
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("Create first: %v", err)
	}
	if err := repo.Create(ctx, newRun(uuid.New())); !errors.Is(err, ErrDeviceImportTopologyRunConflict) {
		t.Fatalf("Create while active error = %v, want conflict", err)
	}

	now = now.Add(deviceImportTopologyRunStaleAfter + time.Second)
	second := newRun(uuid.New())
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("Create after expiry: %v", err)
	}
	firstSnapshot, err := repo.Get(ctx, first.ID)
	if err != nil {
		t.Fatalf("Get first: %v", err)
	}
	if firstSnapshot.Run.State != domain.DeviceImportTopologyRunStateSuperseded || firstSnapshot.Run.CompletedAt == nil {
		t.Fatalf("first run = %#v, want superseded", firstSnapshot.Run)
	}
	secondSnapshot, err := repo.Get(ctx, second.ID)
	if err != nil {
		t.Fatalf("Get second: %v", err)
	}
	if secondSnapshot.Run.State != domain.DeviceImportTopologyRunStateImporting {
		t.Fatalf("second state = %q, want importing", secondSnapshot.Run.State)
	}
}

func TestDeviceImportTopologyRunRepoPersistsBoundedRunFailures(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mapID := uuid.New()
	actorID := uuid.New()
	insertDeviceImportTestMap(t, db, mapID)
	insertDeviceImportTopologyTestUser(t, db, actorID)
	repo := NewDeviceImportTopologyRunRepo(db)
	run := &domain.DeviceImportTopologyRun{
		ID: uuid.New(), MapID: mapID, ActorUserID: actorID, FileDigest: "sha256:run-failure",
		LayoutScope: domain.DeviceImportTopologyLayoutScopePreserve,
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := db.Exec(
		`UPDATE device_import_topology_runs SET state = 'reconciling' WHERE id = $1`, run.ID.String(),
	); err != nil {
		t.Fatalf("set reconciling: %v", err)
	}
	failure := domain.DeviceImportTopologyRunFailure{
		Code: domain.DeviceImportTopologyResultPersistence, Message: "Automatic link creation could not complete.", Reference: "safe-reference",
	}
	retry, err := repo.RecordReconciliationFailure(ctx, run.ID, failure)
	if err != nil || !retry {
		t.Fatalf("first RecordReconciliationFailure = retry %v, error %v", retry, err)
	}
	first, err := repo.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("Get first failure: %v", err)
	}
	if first.Run.State != domain.DeviceImportTopologyRunStateReconciling || first.Run.ReconcileAttempts != 1 ||
		first.Run.FailureCode != failure.Code || first.Run.FailureReference != failure.Reference {
		t.Fatalf("first failure snapshot = %#v", first.Run)
	}
	retry, err = repo.RecordReconciliationFailure(ctx, run.ID, failure)
	if err != nil || retry {
		t.Fatalf("second RecordReconciliationFailure = retry %v, error %v", retry, err)
	}
	failed, err := repo.GetActiveForMap(ctx, mapID, actorID)
	if err != nil {
		t.Fatalf("GetActiveForMap failed run: %v", err)
	}
	if failed.Run.State != domain.DeviceImportTopologyRunStateFailed || failed.Run.ReconcileAttempts != 2 {
		t.Fatalf("failed run = %#v", failed.Run)
	}
	if err := repo.SetBackgrounded(ctx, run.ID, actorID); err != nil {
		t.Fatalf("SetBackgrounded failed run: %v", err)
	}
	completed, err := repo.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("Get completed failed run: %v", err)
	}
	if completed.Run.State != domain.DeviceImportTopologyRunStateCompleted || completed.Run.CompletedAt == nil ||
		completed.Run.AutoLayoutAllowed {
		t.Fatalf("continued failed run = %#v, want completed manual ownership", completed.Run)
	}
}

func TestDeviceImportTopologyRunRepoPersistsImportFinalizationFailure(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mapID := uuid.New()
	actorID := uuid.New()
	insertDeviceImportTestMap(t, db, mapID)
	insertDeviceImportTopologyTestUser(t, db, actorID)
	repo := NewDeviceImportTopologyRunRepo(db)
	run := &domain.DeviceImportTopologyRun{
		ID: uuid.New(), MapID: mapID, ActorUserID: actorID, FileDigest: "sha256:finalize-failure",
		LayoutScope: domain.DeviceImportTopologyLayoutScopePreserve,
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}
	failure := domain.DeviceImportTopologyRunFailure{
		Code: domain.DeviceImportTopologyResultInternal, Message: "Bootstrap scheduling could not start.", Reference: "finalize-reference",
	}
	if err := repo.FailImport(ctx, run.ID, failure); err != nil {
		t.Fatalf("FailImport: %v", err)
	}
	snapshot, err := repo.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if snapshot.Run.State != domain.DeviceImportTopologyRunStateFailed || snapshot.Run.FailureMessage != failure.Message ||
		snapshot.Run.FailureReference != failure.Reference {
		t.Fatalf("failed import run = %#v", snapshot.Run)
	}
}

func TestDeviceImportTopologyRunRepoConcurrentLastCompletionsAdvanceExactlyOnce(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mapID := uuid.New()
	actorID := uuid.New()
	insertDeviceImportTestMap(t, db, mapID)
	insertDeviceImportTopologyTestUser(t, db, actorID)
	deviceRepo := NewDeviceRepo(db, testKeyring, nil)
	repo := NewDeviceImportTopologyRunRepo(db)
	run := &domain.DeviceImportTopologyRun{
		ID: uuid.New(), MapID: mapID, ActorUserID: actorID, FileDigest: "sha256:concurrent",
		LayoutScope: domain.DeviceImportTopologyLayoutScopePreserve, State: domain.DeviceImportTopologyRunStateImporting,
		AutoLayoutAllowed: true,
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}

	deviceIDs := []uuid.UUID{uuid.New(), uuid.New()}
	for index, deviceID := range deviceIDs {
		device := newDeviceImportTestDevice(fmt.Sprintf("concurrent-%d.example.net", index))
		device.ID = deviceID
		if err := deviceRepo.Create(device); err != nil {
			t.Fatalf("create device %d: %v", index, err)
		}
		if _, err := db.Exec(
			`INSERT INTO canvas_map_devices (map_id, device_id, role, added_at) VALUES ($1, $2, $3, NOW())`,
			mapID.String(), deviceID.String(), string(domain.CanvasMapDeviceRoleBase),
		); err != nil {
			t.Fatalf("insert membership %d: %v", index, err)
		}
		if err := repo.AddItem(ctx, run.ID, deviceID); err != nil {
			t.Fatalf("AddItem %d: %v", index, err)
		}
	}
	if err := repo.FinalizeImport(ctx, run.ID); err != nil {
		t.Fatalf("FinalizeImport: %v", err)
	}
	for _, deviceID := range deviceIDs {
		if _, found, err := repo.MarkItemRunning(ctx, deviceID, time.Now().UTC()); err != nil || !found {
			t.Fatalf("MarkItemRunning(%s) = found %v, error %v", deviceID, found, err)
		}
	}

	start := make(chan struct{})
	results := make(chan bool, len(deviceIDs))
	errorsCh := make(chan error, len(deviceIDs))
	for _, deviceID := range deviceIDs {
		deviceID := deviceID
		go func() {
			<-start
			allTerminal, err := repo.CompleteItem(ctx, run.ID, deviceID, domain.DeviceImportTopologyItemCompletion{
				State: domain.DeviceImportTopologyItemStateSucceeded, ResultCode: domain.DeviceImportTopologyResultDiscovered,
				Message: "Topology discovery completed.", CompletedAt: time.Now().UTC(),
			})
			results <- allTerminal
			errorsCh <- err
		}()
	}
	close(start)
	terminalTransitions := 0
	for range deviceIDs {
		if err := <-errorsCh; err != nil {
			t.Fatalf("CompleteItem: %v", err)
		}
		if <-results {
			terminalTransitions++
		}
	}
	if terminalTransitions != 1 {
		t.Fatalf("terminal transitions = %d, want exactly 1", terminalTransitions)
	}
}

func TestDeviceImportTopologyRunRepoRecoversInterruptedItems(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mapID := uuid.New()
	actorID := uuid.New()
	insertDeviceImportTestMap(t, db, mapID)
	insertDeviceImportTopologyTestUser(t, db, actorID)
	deviceRepo := NewDeviceRepo(db, testKeyring, nil)
	device := newDeviceImportTestDevice("restart-router.example.net")
	queuedDevice := newDeviceImportTestDevice("queued-restart-router.example.net")
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("create device: %v", err)
	}
	if err := deviceRepo.Create(queuedDevice); err != nil {
		t.Fatalf("create queued device: %v", err)
	}
	for _, deviceID := range []uuid.UUID{device.ID, queuedDevice.ID} {
		if _, err := db.Exec(
			`INSERT INTO canvas_map_devices (map_id, device_id, role, added_at)
			 VALUES ($1, $2, $3, NOW())`,
			mapID.String(), deviceID.String(), string(domain.CanvasMapDeviceRoleBase),
		); err != nil {
			t.Fatalf("insert device membership: %v", err)
		}
	}

	repo := NewDeviceImportTopologyRunRepo(db, deviceRepo)
	run := &domain.DeviceImportTopologyRun{
		ID:                uuid.New(),
		MapID:             mapID,
		ActorUserID:       actorID,
		FileDigest:        "sha256:restart",
		LayoutScope:       domain.DeviceImportTopologyLayoutScopePreserve,
		State:             domain.DeviceImportTopologyRunStateImporting,
		AutoLayoutAllowed: true,
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.AddItem(ctx, run.ID, device.ID); err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	if err := repo.AddItem(ctx, run.ID, queuedDevice.ID); err != nil {
		t.Fatalf("AddItem queued: %v", err)
	}
	if err := repo.FinalizeImport(ctx, run.ID); err != nil {
		t.Fatalf("FinalizeImport: %v", err)
	}
	if _, _, err := repo.MarkItemRunning(ctx, device.ID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkItemRunning: %v", err)
	}
	if _, err := db.Exec(
		`UPDATE devices SET topology_discovery_mode = $1, topology_bootstrap_state = $2
		 WHERE id IN ($3, $4)`,
		string(domain.TopologyDiscoveryModeBootstrapOnce), string(domain.TopologyBootstrapStateCompleted),
		device.ID.String(), queuedDevice.ID.String(),
	); err != nil {
		t.Fatalf("simulate completed device state before run-item persistence: %v", err)
	}
	deviceLinkCache := devicecache.NewDeviceLinkCache(deviceRepo, NewLinkRepo(db, nil), nil)
	for _, deviceID := range []uuid.UUID{device.ID, queuedDevice.ID} {
		cachedDevice, found, cacheErr := deviceLinkCache.GetDeviceByID(deviceID)
		if cacheErr != nil {
			t.Fatalf("prime recovered device cache for %s: %v", deviceID, cacheErr)
		}
		if !found || cachedDevice.TopologyBootstrapState != domain.TopologyBootstrapStateCompleted {
			t.Fatalf("cached device %s before recovery = (%v, %q), want (true, completed)", deviceID, found, cachedDevice.TopologyBootstrapState)
		}
	}

	recovered, err := repo.RecoverInterrupted(ctx)
	if err != nil {
		t.Fatalf("RecoverInterrupted: %v", err)
	}
	wantRecovered := normalizedDeviceImportTopologyUUIDs([]uuid.UUID{device.ID, queuedDevice.ID})
	if len(recovered) != len(wantRecovered) {
		t.Fatalf("recovered device IDs = %#v, want %#v", recovered, wantRecovered)
	}
	for index := range wantRecovered {
		if recovered[index] != wantRecovered[index] {
			t.Fatalf("recovered device IDs = %#v, want %#v", recovered, wantRecovered)
		}
	}
	snapshot, err := repo.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("Get after recovery: %v", err)
	}
	for _, item := range snapshot.Items {
		if item.State != domain.DeviceImportTopologyItemStateQueued {
			t.Fatalf("recovered item state = %q, want queued", item.State)
		}
	}
	for _, deviceID := range []uuid.UUID{device.ID, queuedDevice.ID} {
		recoveredDevice, getErr := deviceRepo.GetByID(deviceID)
		if getErr != nil {
			t.Fatalf("GetByID(%s): %v", deviceID, getErr)
		}
		if recoveredDevice.TopologyBootstrapState != domain.TopologyBootstrapStatePending {
			t.Fatalf(
				"recovered device %s bootstrap state = %q, want pending",
				deviceID,
				recoveredDevice.TopologyBootstrapState,
			)
		}
		cachedDevice, found, cacheErr := deviceLinkCache.GetDeviceByID(deviceID)
		if cacheErr != nil {
			t.Fatalf("read recovered device cache for %s: %v", deviceID, cacheErr)
		}
		if !found || cachedDevice.TopologyBootstrapState != domain.TopologyBootstrapStatePending {
			t.Fatalf("cached device %s after recovery = (%v, %q), want (true, pending)", deviceID, found, cachedDevice.TopologyBootstrapState)
		}
	}
}

func TestDeviceImportTopologyRunRepoRecoversTerminalDiscoveryBeforeReconciliation(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mapID := uuid.New()
	actorID := uuid.New()
	insertDeviceImportTestMap(t, db, mapID)
	insertDeviceImportTopologyTestUser(t, db, actorID)

	deviceRepo := NewDeviceRepo(db, testKeyring, nil)
	device := newDeviceImportTestDevice("terminal-restart-router.example.net")
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("create device: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO canvas_map_devices (map_id, device_id, role, added_at)
		 VALUES ($1, $2, $3, NOW())`,
		mapID.String(), device.ID.String(), string(domain.CanvasMapDeviceRoleBase),
	); err != nil {
		t.Fatalf("insert device membership: %v", err)
	}

	repo := NewDeviceImportTopologyRunRepo(db)
	run := &domain.DeviceImportTopologyRun{
		ID: uuid.New(), MapID: mapID, ActorUserID: actorID, FileDigest: "sha256:terminal-restart",
		LayoutScope: domain.DeviceImportTopologyLayoutScopePreserve, State: domain.DeviceImportTopologyRunStateImporting,
		AutoLayoutAllowed: true,
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.AddItem(ctx, run.ID, device.ID); err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	if err := repo.FinalizeImport(ctx, run.ID); err != nil {
		t.Fatalf("FinalizeImport: %v", err)
	}
	if _, found, err := repo.MarkItemRunning(ctx, device.ID, time.Now().UTC()); err != nil || !found {
		t.Fatalf("MarkItemRunning = found %v, error %v", found, err)
	}
	if allTerminal, err := repo.CompleteItem(ctx, run.ID, device.ID, domain.DeviceImportTopologyItemCompletion{
		State: domain.DeviceImportTopologyItemStateSucceeded, ResultCode: domain.DeviceImportTopologyResultDiscovered,
		Message: "Topology discovery completed.", CompletedAt: time.Now().UTC(),
	}); err != nil || !allTerminal {
		t.Fatalf("CompleteItem = terminal %v, error %v", allTerminal, err)
	}

	if recovered, err := repo.RecoverInterrupted(ctx); err != nil || len(recovered) != 0 {
		t.Fatalf("RecoverInterrupted = %#v, error %v, want no queued devices", recovered, err)
	}
	reconciling, err := repo.RecoverReconciling(ctx)
	if err != nil {
		t.Fatalf("RecoverReconciling: %v", err)
	}
	if len(reconciling) != 1 || reconciling[0] != run.ID {
		t.Fatalf("reconciling runs = %#v, want [%s]", reconciling, run.ID)
	}
	snapshot, err := repo.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("Get after recovery: %v", err)
	}
	if snapshot.Run.State != domain.DeviceImportTopologyRunStateReconciling {
		t.Fatalf("run state = %q, want %q", snapshot.Run.State, domain.DeviceImportTopologyRunStateReconciling)
	}
}

func TestDeviceImportTopologyRunRepoAllowsOnlyOneTargetedFollowup(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mapID := uuid.New()
	actorID := uuid.New()
	insertDeviceImportTestMap(t, db, mapID)
	insertDeviceImportTopologyTestUser(t, db, actorID)
	deviceRepo := NewDeviceRepo(db, testKeyring, nil)
	device := newDeviceImportTestDevice("followup-router.example.net")
	device.TopologyDiscoveryMode = domain.TopologyDiscoveryModeBootstrapOnce
	device.TopologyBootstrapState = domain.TopologyBootstrapStatePending
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("create device: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO canvas_map_devices (map_id, device_id, role, added_at) VALUES ($1, $2, $3, NOW())`,
		mapID.String(), device.ID.String(), string(domain.CanvasMapDeviceRoleBase),
	); err != nil {
		t.Fatalf("insert membership: %v", err)
	}
	repo := NewDeviceImportTopologyRunRepo(db, deviceRepo)
	run := &domain.DeviceImportTopologyRun{
		ID: uuid.New(), MapID: mapID, ActorUserID: actorID, FileDigest: "sha256:followup",
		LayoutScope: domain.DeviceImportTopologyLayoutScopePreserve,
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.AddItem(ctx, run.ID, device.ID); err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	if err := repo.FinalizeImport(ctx, run.ID); err != nil {
		t.Fatalf("FinalizeImport: %v", err)
	}
	completeImportTopologyAttempt(t, ctx, repo, run.ID, device.ID)
	deviceLinkCache := devicecache.NewDeviceLinkCache(deviceRepo, NewLinkRepo(db, nil), nil)
	cachedCompleted, found, err := deviceLinkCache.GetDeviceByID(device.ID)
	if err != nil {
		t.Fatalf("prime follow-up device cache: %v", err)
	}
	if !found || cachedCompleted.TopologyBootstrapState != domain.TopologyBootstrapStateCompleted {
		t.Fatalf("cached device before follow-up = (%v, %q), want (true, completed)", found, cachedCompleted.TopologyBootstrapState)
	}
	if err := repo.TransitionRun(ctx, run.ID, domain.DeviceImportTopologyRunStateReconciling); err != nil {
		t.Fatalf("TransitionRun(reconciling): %v", err)
	}
	requeued, err := repo.RequeueFollowupItems(ctx, run.ID, []uuid.UUID{device.ID})
	if err != nil {
		t.Fatalf("RequeueFollowupItems: %v", err)
	}
	if len(requeued) != 1 || requeued[0] != device.ID {
		t.Fatalf("follow-up requeued = %#v, want [%s]", requeued, device.ID)
	}
	cachedPending, found, err := deviceLinkCache.GetDeviceByID(device.ID)
	if err != nil {
		t.Fatalf("read follow-up device cache: %v", err)
	}
	if !found || cachedPending.TopologyBootstrapState != domain.TopologyBootstrapStatePending {
		t.Fatalf("cached device after follow-up = (%v, %q), want (true, pending)", found, cachedPending.TopologyBootstrapState)
	}
	snapshot, err := repo.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("Get follow-up: %v", err)
	}
	if snapshot.Run.State != domain.DeviceImportTopologyRunStateFollowup ||
		snapshot.Items[0].State != domain.DeviceImportTopologyItemStateQueued {
		t.Fatalf("follow-up snapshot = %#v", snapshot)
	}

	completeImportTopologyAttempt(t, ctx, repo, run.ID, device.ID)
	if err := repo.TransitionRun(ctx, run.ID, domain.DeviceImportTopologyRunStateReconciling); err != nil {
		t.Fatalf("TransitionRun(second reconciling): %v", err)
	}
	requeued, err = repo.RequeueFollowupItems(ctx, run.ID, []uuid.UUID{device.ID})
	if err != nil {
		t.Fatalf("second RequeueFollowupItems: %v", err)
	}
	if len(requeued) != 0 {
		t.Fatalf("second follow-up requeued = %#v, want none", requeued)
	}
}

func TestDeviceImportTopologyRunRepoPersistsFinalMapScopedItemResults(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mapID := uuid.New()
	actorID := uuid.New()
	insertDeviceImportTestMap(t, db, mapID)
	insertDeviceImportTopologyTestUser(t, db, actorID)
	deviceRepo := NewDeviceRepo(db, testKeyring, nil)
	device := newDeviceImportTestDevice("reconciled-router.example.net")
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("create device: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO canvas_map_devices (map_id, device_id, role, added_at) VALUES ($1, $2, $3, NOW())`,
		mapID.String(), device.ID.String(), string(domain.CanvasMapDeviceRoleBase),
	); err != nil {
		t.Fatalf("insert membership: %v", err)
	}
	repo := NewDeviceImportTopologyRunRepo(db)
	run := &domain.DeviceImportTopologyRun{
		ID: uuid.New(), MapID: mapID, ActorUserID: actorID, FileDigest: "sha256:reconciled",
		LayoutScope: domain.DeviceImportTopologyLayoutScopePreserve,
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.AddItem(ctx, run.ID, device.ID); err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	if err := repo.FinalizeImport(ctx, run.ID); err != nil {
		t.Fatalf("FinalizeImport: %v", err)
	}
	if _, found, err := repo.MarkItemRunning(ctx, device.ID, time.Now().UTC()); err != nil || !found {
		t.Fatalf("MarkItemRunning = found %v, error %v", found, err)
	}
	if terminal, err := repo.CompleteItem(ctx, run.ID, device.ID, domain.DeviceImportTopologyItemCompletion{
		State: domain.DeviceImportTopologyItemStateSucceeded, ResultCode: domain.DeviceImportTopologyResultDiscovered,
		Message: "Topology discovery completed.", NeighborCount: 1, CompletedAt: time.Now().UTC(),
	}); err != nil || !terminal {
		t.Fatalf("CompleteItem = terminal %v, error %v", terminal, err)
	}
	if err := repo.TransitionRun(ctx, run.ID, domain.DeviceImportTopologyRunStateReconciling); err != nil {
		t.Fatalf("TransitionRun(reconciling): %v", err)
	}

	if err := repo.ApplyReconciliationResults(ctx, run.ID, []domain.DeviceImportTopologyItemReconciliation{{
		DeviceID: device.ID, UnresolvedNeighbors: 1,
	}}); err != nil {
		t.Fatalf("ApplyReconciliationResults(unresolved): %v", err)
	}
	warningSnapshot, err := repo.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("Get warning snapshot: %v", err)
	}
	warningItem := warningSnapshot.Items[0]
	if warningItem.State != domain.DeviceImportTopologyItemStateWarning ||
		warningItem.ResultCode != domain.DeviceImportTopologyResultUnresolvedNeighbors ||
		warningItem.UnresolvedNeighbors != 1 || warningItem.LinksCreated != 0 {
		t.Fatalf("warning item = %#v", warningItem)
	}

	if err := repo.ApplyReconciliationResults(ctx, run.ID, []domain.DeviceImportTopologyItemReconciliation{{
		DeviceID: device.ID, LinksCreated: 1,
	}}); err != nil {
		t.Fatalf("ApplyReconciliationResults(resolved): %v", err)
	}
	resolvedSnapshot, err := repo.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("Get resolved snapshot: %v", err)
	}
	resolvedItem := resolvedSnapshot.Items[0]
	if resolvedItem.State != domain.DeviceImportTopologyItemStateSucceeded ||
		resolvedItem.ResultCode != domain.DeviceImportTopologyResultDiscovered ||
		resolvedItem.UnresolvedNeighbors != 0 || resolvedItem.LinksCreated != 1 {
		t.Fatalf("resolved item = %#v", resolvedItem)
	}
}

func completeImportTopologyAttempt(
	t *testing.T,
	ctx context.Context,
	repo *DeviceImportTopologyRunRepo,
	runID, deviceID uuid.UUID,
) {
	t.Helper()
	if _, found, err := repo.MarkItemRunning(ctx, deviceID, time.Now().UTC()); err != nil || !found {
		t.Fatalf("MarkItemRunning = found %v error %v", found, err)
	}
	if _, err := repo.CompleteItem(ctx, runID, deviceID, domain.DeviceImportTopologyItemCompletion{
		State: domain.DeviceImportTopologyItemStateWarning, ResultCode: domain.DeviceImportTopologyResultIncompletePorts,
		Message: "Some interface details are incomplete.", CompletedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CompleteItem: %v", err)
	}
}

func TestDeviceImportTopologyRunRepoActorScopedControlsAndRetry(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mapID := uuid.New()
	actorID := uuid.New()
	insertDeviceImportTestMap(t, db, mapID)
	insertDeviceImportTopologyTestUser(t, db, actorID)
	deviceRepo := NewDeviceRepo(db, testKeyring, nil)
	device := newDeviceImportTestDevice("retry-router.example.net")
	device.TopologyDiscoveryMode = domain.TopologyDiscoveryModeBootstrapOnce
	device.TopologyBootstrapState = domain.TopologyBootstrapStatePending
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("create device: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO canvas_map_devices (map_id, device_id, role, added_at) VALUES ($1, $2, $3, NOW())`,
		mapID.String(), device.ID.String(), string(domain.CanvasMapDeviceRoleBase),
	); err != nil {
		t.Fatalf("insert device membership: %v", err)
	}

	repo := NewDeviceImportTopologyRunRepo(db)
	run := &domain.DeviceImportTopologyRun{
		ID: uuid.New(), MapID: mapID, ActorUserID: actorID, FileDigest: "sha256:controls",
		LayoutScope: domain.DeviceImportTopologyLayoutScopePreserve,
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.AddItem(ctx, run.ID, device.ID); err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	if err := repo.FinalizeImport(ctx, run.ID); err != nil {
		t.Fatalf("FinalizeImport: %v", err)
	}
	if _, _, err := repo.MarkItemRunning(ctx, device.ID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkItemRunning: %v", err)
	}
	if _, err := repo.CompleteItem(ctx, run.ID, device.ID, domain.DeviceImportTopologyItemCompletion{
		State: domain.DeviceImportTopologyItemStateFailed, ResultCode: domain.DeviceImportTopologyResultSNMPUnreachable,
		Message: "SNMP topology discovery did not complete.", CompletedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CompleteItem: %v", err)
	}
	completedDevice, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID after terminal item: %v", err)
	}
	if completedDevice.TopologyBootstrapState != domain.TopologyBootstrapStateCompleted {
		t.Fatalf(
			"terminal topology state = %q, want completed",
			completedDevice.TopologyBootstrapState,
		)
	}
	if err := repo.TransitionRun(ctx, run.ID, domain.DeviceImportTopologyRunStateReconciling); err != nil {
		t.Fatalf("TransitionRun(reconciling): %v", err)
	}
	if err := repo.TransitionRun(ctx, run.ID, domain.DeviceImportTopologyRunStateReadyForLayout); err != nil {
		t.Fatalf("TransitionRun(ready): %v", err)
	}

	active, err := repo.GetActiveForMap(ctx, mapID, actorID)
	if err != nil {
		t.Fatalf("GetActiveForMap: %v", err)
	}
	if active.Run.ID != run.ID {
		t.Fatalf("active run = %s, want %s", active.Run.ID, run.ID)
	}
	if _, err := repo.GetForActor(ctx, run.ID, uuid.New()); !errors.Is(err, ErrDeviceImportTopologyRunNotFound) {
		t.Fatalf("GetForActor(other) error = %v, want not found", err)
	}
	if err := repo.SetBackgrounded(ctx, run.ID, actorID); err != nil {
		t.Fatalf("SetBackgrounded: %v", err)
	}
	requeued, err := repo.RequeueItems(ctx, run.ID, actorID, []uuid.UUID{device.ID})
	if err != nil {
		t.Fatalf("RequeueItems: %v", err)
	}
	if len(requeued) != 1 || requeued[0] != device.ID {
		t.Fatalf("requeued = %#v, want [%s]", requeued, device.ID)
	}
	if err := repo.DisableAutoLayout(ctx, run.ID, actorID); err != nil {
		t.Fatalf("DisableAutoLayout: %v", err)
	}

	snapshot, err := repo.GetForActor(ctx, run.ID, actorID)
	if err != nil {
		t.Fatalf("GetForActor: %v", err)
	}
	if !snapshot.Run.Backgrounded || snapshot.Run.AutoLayoutAllowed {
		t.Fatalf("run flags = backgrounded %v auto-layout %v", snapshot.Run.Backgrounded, snapshot.Run.AutoLayoutAllowed)
	}
	if snapshot.Run.State != domain.DeviceImportTopologyRunStateCompleted || snapshot.Items[0].State != domain.DeviceImportTopologyItemStateQueued {
		t.Fatalf("retried snapshot = %#v", snapshot)
	}
	storedDevice, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if storedDevice.TopologyBootstrapState != domain.TopologyBootstrapStatePending {
		t.Fatalf("retried topology state = %q, want pending", storedDevice.TopologyBootstrapState)
	}
}

func TestDeviceImportTopologyRunRepoPublishesTerminalDeviceStateToLoadedCache(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mapID := uuid.New()
	actorID := uuid.New()
	insertDeviceImportTestMap(t, db, mapID)
	insertDeviceImportTopologyTestUser(t, db, actorID)

	deviceRepo := NewDeviceRepo(db, testKeyring, nil)
	device := newDeviceImportTestDevice("cached-offline-router.example.net")
	device.TopologyDiscoveryMode = domain.TopologyDiscoveryModeBootstrapOnce
	device.TopologyBootstrapState = domain.TopologyBootstrapStatePending
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("create device: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO canvas_map_devices (map_id, device_id, role, added_at) VALUES ($1, $2, $3, NOW())`,
		mapID.String(), device.ID.String(), string(domain.CanvasMapDeviceRoleBase),
	); err != nil {
		t.Fatalf("insert device membership: %v", err)
	}

	deviceLinkCache := devicecache.NewDeviceLinkCache(deviceRepo, NewLinkRepo(db, nil), nil)
	cachedBefore, found, err := deviceLinkCache.GetDeviceByID(device.ID)
	if err != nil {
		t.Fatalf("prime device cache: %v", err)
	}
	if !found || cachedBefore.TopologyBootstrapState != domain.TopologyBootstrapStatePending {
		t.Fatalf("cached bootstrap state before completion = (%v, %q), want (true, pending)", found, cachedBefore.TopologyBootstrapState)
	}

	repo := NewDeviceImportTopologyRunRepo(db, deviceRepo)
	run := &domain.DeviceImportTopologyRun{
		ID: uuid.New(), MapID: mapID, ActorUserID: actorID, FileDigest: "sha256:cached-terminal-state",
		LayoutScope: domain.DeviceImportTopologyLayoutScopePreserve,
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.AddItem(ctx, run.ID, device.ID); err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	if err := repo.FinalizeImport(ctx, run.ID); err != nil {
		t.Fatalf("FinalizeImport: %v", err)
	}
	if _, _, err := repo.MarkItemRunning(ctx, device.ID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkItemRunning: %v", err)
	}
	if _, err := repo.CompleteItem(ctx, run.ID, device.ID, domain.DeviceImportTopologyItemCompletion{
		State: domain.DeviceImportTopologyItemStateFailed, ResultCode: domain.DeviceImportTopologyResultSNMPUnreachable,
		Message: "SNMP topology discovery did not complete.", CompletedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CompleteItem: %v", err)
	}

	cachedAfter, found, err := deviceLinkCache.GetDeviceByID(device.ID)
	if err != nil {
		t.Fatalf("read device cache after completion: %v", err)
	}
	if !found || cachedAfter.TopologyBootstrapState != domain.TopologyBootstrapStateCompleted {
		t.Fatalf("cached bootstrap state after completion = (%v, %q), want (true, completed)", found, cachedAfter.TopologyBootstrapState)
	}

	requeued, err := repo.RequeueItems(ctx, run.ID, actorID, []uuid.UUID{device.ID})
	if err != nil {
		t.Fatalf("RequeueItems: %v", err)
	}
	if len(requeued) != 1 || requeued[0] != device.ID {
		t.Fatalf("requeued devices = %#v, want [%s]", requeued, device.ID)
	}
	cachedRetry, found, err := deviceLinkCache.GetDeviceByID(device.ID)
	if err != nil {
		t.Fatalf("read device cache after retry: %v", err)
	}
	if !found || cachedRetry.TopologyBootstrapState != domain.TopologyBootstrapStatePending {
		t.Fatalf("cached bootstrap state after retry = (%v, %q), want (true, pending)", found, cachedRetry.TopologyBootstrapState)
	}
}

func TestDeviceImportTopologyRunRepoAppliesLayoutAtomicallyAndIdempotently(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mapID := uuid.New()
	actorID := uuid.New()
	insertDeviceImportTestMap(t, db, mapID)
	insertDeviceImportTopologyTestUser(t, db, actorID)
	deviceRepo := NewDeviceRepo(db, testKeyring, nil)
	source := newDeviceImportTestDevice("layout-source.example.net")
	target := newDeviceImportTestDevice("layout-target.example.net")
	if err := deviceRepo.Create(source); err != nil {
		t.Fatalf("create source: %v", err)
	}
	if err := deviceRepo.Create(target); err != nil {
		t.Fatalf("create target: %v", err)
	}
	for _, deviceID := range []uuid.UUID{source.ID, target.ID} {
		if _, err := db.Exec(
			`INSERT INTO canvas_map_devices (map_id, device_id, role, added_at) VALUES ($1, $2, $3, NOW())`,
			mapID.String(), deviceID.String(), string(domain.CanvasMapDeviceRoleBase),
		); err != nil {
			t.Fatalf("insert device membership: %v", err)
		}
	}
	linkID := uuid.New()
	if _, err := db.Exec(
		`INSERT INTO links
		 (id, source_device_id, source_if_name, target_device_id, target_if_name, discovery_protocol, created_at, updated_at)
		 VALUES ($1, $2, 'ether1', $3, 'ether48', 'lldp', NOW(), NOW())`,
		linkID.String(), source.ID.String(), target.ID.String(),
	); err != nil {
		t.Fatalf("insert link: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO canvas_map_links (map_id, link_id, added_at) VALUES ($1, $2, NOW())`,
		mapID.String(), linkID.String(),
	); err != nil {
		t.Fatalf("insert link membership: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO canvas_map_link_routes (map_id, link_id, route_version, waypoints_json)
		 VALUES ($1, $2, 1, '[{"x":10,"y":20}]'::jsonb)`,
		mapID.String(), linkID.String(),
	); err != nil {
		t.Fatalf("insert link route: %v", err)
	}

	repo := NewDeviceImportTopologyRunRepo(db)
	run := &domain.DeviceImportTopologyRun{
		ID: uuid.New(), MapID: mapID, ActorUserID: actorID, FileDigest: "sha256:layout",
		LayoutScope: domain.DeviceImportTopologyLayoutScopeReorganize,
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := db.Exec(
		`UPDATE device_import_topology_runs SET state = 'ready_for_layout' WHERE id = $1`, run.ID.String(),
	); err != nil {
		t.Fatalf("ready run: %v", err)
	}
	snapshot, err := repo.GetForActor(ctx, run.ID, actorID)
	if err != nil {
		t.Fatalf("GetForActor: %v", err)
	}
	if snapshot.Run.LayoutInputToken == "" {
		t.Fatal("layout input token is empty")
	}
	request := domain.DeviceImportTopologyLayoutApply{
		InputToken: snapshot.Run.LayoutInputToken,
		Positions: []domain.DevicePosition{
			{DeviceID: source.ID, X: 120, Y: 240, Pinned: false},
			{DeviceID: target.ID, X: 600, Y: 240, Pinned: false},
		},
	}
	if err := repo.ApplyLayout(ctx, run.ID, actorID, request); err != nil {
		t.Fatalf("ApplyLayout: %v", err)
	}
	if err := repo.ApplyLayout(ctx, run.ID, actorID, request); err != nil {
		t.Fatalf("idempotent ApplyLayout: %v", err)
	}

	var x, y float64
	if err := db.QueryRow(
		`SELECT x, y FROM canvas_map_positions WHERE map_id = $1 AND device_id = $2`,
		mapID.String(), source.ID.String(),
	).Scan(&x, &y); err != nil {
		t.Fatalf("read saved position: %v", err)
	}
	if x != 120 || y != 240 {
		t.Fatalf("saved position = (%v,%v)", x, y)
	}
	var routeCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM canvas_map_link_routes WHERE map_id = $1 AND link_id = $2`,
		mapID.String(), linkID.String(),
	).Scan(&routeCount); err != nil {
		t.Fatalf("count routes: %v", err)
	}
	if routeCount != 0 {
		t.Fatalf("route count = %d, want 0", routeCount)
	}
	completed, err := repo.GetForActor(ctx, run.ID, actorID)
	if err != nil {
		t.Fatalf("GetForActor after layout: %v", err)
	}
	if completed.Run.State != domain.DeviceImportTopologyRunStateCompleted {
		t.Fatalf("run state = %q, want completed", completed.Run.State)
	}
}

func TestDeviceImportTopologyRunRepoEnforcesPreserveLayoutAndDerivesRouteResets(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mapID := uuid.New()
	actorID := uuid.New()
	insertDeviceImportTestMap(t, db, mapID)
	insertDeviceImportTopologyTestUser(t, db, actorID)
	deviceRepo := NewDeviceRepo(db, testKeyring, nil)
	imported := newDeviceImportTestDevice("preserve-imported.example.net")
	existing := newDeviceImportTestDevice("preserve-existing.example.net")
	for _, device := range []*domain.Device{imported, existing} {
		if err := deviceRepo.Create(device); err != nil {
			t.Fatalf("create device: %v", err)
		}
		if _, err := db.Exec(
			`INSERT INTO canvas_map_devices (map_id, device_id, role, added_at) VALUES ($1, $2, $3, NOW())`,
			mapID.String(), device.ID.String(), string(domain.CanvasMapDeviceRoleBase),
		); err != nil {
			t.Fatalf("insert membership: %v", err)
		}
	}
	if _, err := db.Exec(
		`INSERT INTO canvas_map_positions (map_id, device_id, x, y, pinned) VALUES ($1, $2, 0, 0, TRUE)`,
		mapID.String(), existing.ID.String(),
	); err != nil {
		t.Fatalf("insert existing position: %v", err)
	}
	linkID := uuid.New()
	if _, err := db.Exec(
		`INSERT INTO links
		 (id, source_device_id, source_if_name, target_device_id, target_if_name, discovery_protocol, created_at, updated_at)
		 VALUES ($1, $2, 'ether1', $3, 'ether2', 'lldp', NOW(), NOW())`,
		linkID.String(), imported.ID.String(), existing.ID.String(),
	); err != nil {
		t.Fatalf("insert link: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO canvas_map_links (map_id, link_id, added_at) VALUES ($1, $2, NOW())`,
		mapID.String(), linkID.String(),
	); err != nil {
		t.Fatalf("insert link membership: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO canvas_map_link_routes (map_id, link_id, route_version, waypoints_json)
		 VALUES ($1, $2, 1, '[{"x":20,"y":30}]'::jsonb)`,
		mapID.String(), linkID.String(),
	); err != nil {
		t.Fatalf("insert link route: %v", err)
	}

	repo := NewDeviceImportTopologyRunRepo(db)
	run := &domain.DeviceImportTopologyRun{
		ID: uuid.New(), MapID: mapID, ActorUserID: actorID, FileDigest: "sha256:preserve-contract",
		LayoutScope: domain.DeviceImportTopologyLayoutScopePreserve,
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.AddItem(ctx, run.ID, imported.ID); err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	if _, err := db.Exec(
		`UPDATE device_import_topology_runs SET state = 'ready_for_layout' WHERE id = $1`, run.ID.String(),
	); err != nil {
		t.Fatalf("ready run: %v", err)
	}
	snapshot, err := repo.GetForActor(ctx, run.ID, actorID)
	if err != nil {
		t.Fatalf("GetForActor: %v", err)
	}

	for name, positions := range map[string][]domain.DevicePosition{
		"empty payload":   nil,
		"existing device": {{DeviceID: existing.ID, X: 900, Y: 0}},
		"overlap":         {{DeviceID: imported.ID, X: 100, Y: 100}},
	} {
		t.Run(name, func(t *testing.T) {
			err := repo.ApplyLayout(ctx, run.ID, actorID, domain.DeviceImportTopologyLayoutApply{
				InputToken: snapshot.Run.LayoutInputToken,
				Positions:  positions,
			})
			if !errors.Is(err, ErrDeviceImportTopologyRunConflict) {
				t.Fatalf("ApplyLayout error = %v, want conflict", err)
			}
		})
	}

	if err := repo.ApplyLayout(ctx, run.ID, actorID, domain.DeviceImportTopologyLayoutApply{
		InputToken: snapshot.Run.LayoutInputToken,
		Positions:  []domain.DevicePosition{{DeviceID: imported.ID, X: 500, Y: 0}},
	}); err != nil {
		t.Fatalf("ApplyLayout valid preserve layout: %v", err)
	}
	var routeCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM canvas_map_link_routes WHERE map_id = $1 AND link_id = $2`,
		mapID.String(), linkID.String(),
	).Scan(&routeCount); err != nil {
		t.Fatalf("count derived route reset: %v", err)
	}
	if routeCount != 0 {
		t.Fatalf("route count = %d, want server-derived reset", routeCount)
	}
	var existingX, existingY float64
	if err := db.QueryRow(
		`SELECT x, y FROM canvas_map_positions WHERE map_id = $1 AND device_id = $2`,
		mapID.String(), existing.ID.String(),
	).Scan(&existingX, &existingY); err != nil {
		t.Fatalf("read existing position: %v", err)
	}
	if existingX != 0 || existingY != 0 {
		t.Fatalf("existing position changed to (%v,%v)", existingX, existingY)
	}
}

func TestDeviceImportTopologyRunRepoRejectsStaleLayoutWithoutWrites(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mapID := uuid.New()
	actorID := uuid.New()
	insertDeviceImportTestMap(t, db, mapID)
	insertDeviceImportTopologyTestUser(t, db, actorID)
	deviceRepo := NewDeviceRepo(db, testKeyring, nil)
	device := newDeviceImportTestDevice("stale-layout.example.net")
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("create device: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO canvas_map_devices (map_id, device_id, role, added_at) VALUES ($1, $2, $3, NOW())`,
		mapID.String(), device.ID.String(), string(domain.CanvasMapDeviceRoleBase),
	); err != nil {
		t.Fatalf("insert membership: %v", err)
	}
	repo := NewDeviceImportTopologyRunRepo(db)
	run := &domain.DeviceImportTopologyRun{
		ID: uuid.New(), MapID: mapID, ActorUserID: actorID, FileDigest: "sha256:stale",
		LayoutScope: domain.DeviceImportTopologyLayoutScopePreserve,
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.AddItem(ctx, run.ID, device.ID); err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	if _, err := db.Exec(
		`UPDATE device_import_topology_runs SET state = 'ready_for_layout' WHERE id = $1`, run.ID.String(),
	); err != nil {
		t.Fatalf("ready run: %v", err)
	}
	snapshot, err := repo.GetForActor(ctx, run.ID, actorID)
	if err != nil {
		t.Fatalf("GetForActor: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO canvas_map_positions (map_id, device_id, x, y, pinned) VALUES ($1, $2, 5, 6, FALSE)`,
		mapID.String(), device.ID.String(),
	); err != nil {
		t.Fatalf("mutate layout input: %v", err)
	}
	err = repo.ApplyLayout(ctx, run.ID, actorID, domain.DeviceImportTopologyLayoutApply{
		InputToken: snapshot.Run.LayoutInputToken,
		Positions:  []domain.DevicePosition{{DeviceID: device.ID, X: 100, Y: 200}},
	})
	if !errors.Is(err, domain.ErrDeviceImportTopologyLayoutStale) {
		t.Fatalf("ApplyLayout error = %v, want stale", err)
	}
	var x, y float64
	if err := db.QueryRow(
		`SELECT x, y FROM canvas_map_positions WHERE map_id = $1 AND device_id = $2`,
		mapID.String(), device.ID.String(),
	).Scan(&x, &y); err != nil {
		t.Fatalf("read unchanged position: %v", err)
	}
	if x != 5 || y != 6 {
		t.Fatalf("position changed to (%v,%v)", x, y)
	}
}

func insertDeviceImportTopologyTestUser(t *testing.T, db *sql.DB, userID uuid.UUID) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO users
		 (id, username, username_normalized, email, email_normalized, password_hash, display_name, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'active')`,
		userID.String(), "topology-"+userID.String(), strings.ToLower("topology-"+userID.String()),
		userID.String()+"@example.test", strings.ToLower(userID.String()+"@example.test"),
		"test-hash", "Topology test user",
	); err != nil {
		t.Fatalf("insert topology test user: %v", err)
	}
}
