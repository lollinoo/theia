package postgres

// This file exercises map-local link route persistence and membership lifecycle behavior.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestCanvasMapLinkRouteRepoUpsertRoundTripAndDelete(t *testing.T) {
	firstLinkID := uuid.MustParse("00000000-0000-0000-0000-000000000711")
	secondLinkID := uuid.MustParse("00000000-0000-0000-0000-000000000712")
	fixture := newCanvasMapLinkRouteTestFixture(t, firstLinkID, secondLinkID)

	firstRoute := domain.CanvasMapLinkRoute{
		LinkID:  firstLinkID,
		Version: domain.CanvasMapLinkRouteVersion,
		Waypoints: []domain.CanvasPoint{
			{X: 100, Y: 120},
			{X: 180, Y: 80},
		},
	}
	saved, err := fixture.routeRepo.UpsertForMap(context.Background(), fixture.canvasMap.ID, firstRoute)
	if err != nil {
		t.Fatalf("upsert first route: %v", err)
	}
	assertCanvasMapLinkRouteContent(t, saved, firstRoute)
	if saved.UpdatedAt.IsZero() {
		t.Fatal("saved route has zero UpdatedAt")
	}

	secondRoute := domain.CanvasMapLinkRoute{
		LinkID:    secondLinkID,
		Version:   domain.CanvasMapLinkRouteVersion,
		Waypoints: []domain.CanvasPoint{{X: -15.5, Y: 21.25}},
	}
	if _, err := fixture.routeRepo.UpsertForMap(context.Background(), fixture.canvasMap.ID, secondRoute); err != nil {
		t.Fatalf("upsert second route: %v", err)
	}

	replacement := domain.CanvasMapLinkRoute{
		LinkID:    firstLinkID,
		Version:   domain.CanvasMapLinkRouteVersion,
		Waypoints: []domain.CanvasPoint{{X: 240, Y: 90}},
	}
	updated, err := fixture.routeRepo.UpsertForMap(context.Background(), fixture.canvasMap.ID, replacement)
	if err != nil {
		t.Fatalf("replace first route: %v", err)
	}
	assertCanvasMapLinkRouteContent(t, updated, replacement)

	routes, err := fixture.routeRepo.GetAllForMap(context.Background(), fixture.canvasMap.ID)
	if err != nil {
		t.Fatalf("get routes: %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("route count = %d, want 2: %#v", len(routes), routes)
	}
	assertCanvasMapLinkRouteContent(t, routes[0], replacement)
	assertCanvasMapLinkRouteContent(t, routes[1], secondRoute)

	if err := fixture.routeRepo.DeleteForMap(context.Background(), fixture.canvasMap.ID, firstLinkID); err != nil {
		t.Fatalf("delete first route: %v", err)
	}
	routes, err = fixture.routeRepo.GetAllForMap(context.Background(), fixture.canvasMap.ID)
	if err != nil {
		t.Fatalf("get routes after delete: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("route count after delete = %d, want 1: %#v", len(routes), routes)
	}
	assertCanvasMapLinkRouteContent(t, routes[0], secondRoute)
}

func TestCanvasMapLinkRouteRepoRejectsNonMemberLink(t *testing.T) {
	memberLinkID := uuid.MustParse("00000000-0000-0000-0000-000000000721")
	nonMemberLinkID := uuid.MustParse("00000000-0000-0000-0000-000000000722")
	fixture := newCanvasMapLinkRouteTestFixture(t, memberLinkID)
	insertCanvasMapLinkRouteTestLink(t, fixture.db, nonMemberLinkID, fixture.sourceDeviceID, fixture.targetDeviceID)

	_, err := fixture.routeRepo.UpsertForMap(context.Background(), fixture.canvasMap.ID, domain.CanvasMapLinkRoute{
		LinkID:    nonMemberLinkID,
		Version:   domain.CanvasMapLinkRouteVersion,
		Waypoints: []domain.CanvasPoint{{X: 1, Y: 2}},
	})
	if !errors.Is(err, domain.ErrCanvasMapLinkRouteNotMember) {
		t.Fatalf("upsert non-member route error = %v, want ErrCanvasMapLinkRouteNotMember", err)
	}
}

func TestCanvasMapLinkRouteRepoDeleteRejectsNonMemberLink(t *testing.T) {
	memberLinkID := uuid.MustParse("00000000-0000-0000-0000-000000000723")
	nonMemberLinkID := uuid.MustParse("00000000-0000-0000-0000-000000000724")
	fixture := newCanvasMapLinkRouteTestFixture(t, memberLinkID)
	insertCanvasMapLinkRouteTestLink(t, fixture.db, nonMemberLinkID, fixture.sourceDeviceID, fixture.targetDeviceID)

	err := fixture.routeRepo.DeleteForMap(context.Background(), fixture.canvasMap.ID, nonMemberLinkID)
	if !errors.Is(err, domain.ErrCanvasMapLinkRouteNotMember) {
		t.Fatalf("delete non-member route error = %v, want ErrCanvasMapLinkRouteNotMember", err)
	}
}

func TestCanvasMapLinkRouteRepoDeleteIsScopedToMap(t *testing.T) {
	linkID := uuid.MustParse("00000000-0000-0000-0000-000000000726")
	fixture := newCanvasMapLinkRouteTestFixture(t, linkID)
	otherMap, err := fixture.mapRepo.Create(domain.CanvasMapCreate{Name: "Other Route Map"})
	if err != nil {
		t.Fatalf("create other canvas map: %v", err)
	}
	if err := fixture.mapRepo.ReplaceMembership(otherMap.ID, fixture.membership(linkID)); err != nil {
		t.Fatalf("seed other map membership: %v", err)
	}
	route := domain.CanvasMapLinkRoute{
		LinkID:    linkID,
		Version:   domain.CanvasMapLinkRouteVersion,
		Waypoints: []domain.CanvasPoint{{X: 10, Y: 20}},
	}
	if _, err := fixture.routeRepo.UpsertForMap(context.Background(), fixture.canvasMap.ID, route); err != nil {
		t.Fatalf("seed first map route: %v", err)
	}
	if _, err := fixture.routeRepo.UpsertForMap(context.Background(), otherMap.ID, route); err != nil {
		t.Fatalf("seed other map route: %v", err)
	}

	if err := fixture.routeRepo.DeleteForMap(context.Background(), fixture.canvasMap.ID, linkID); err != nil {
		t.Fatalf("delete first map route: %v", err)
	}
	firstRoutes, err := fixture.routeRepo.GetAllForMap(context.Background(), fixture.canvasMap.ID)
	if err != nil {
		t.Fatalf("get first map routes: %v", err)
	}
	if len(firstRoutes) != 0 {
		t.Fatalf("first map routes = %#v, want none", firstRoutes)
	}
	otherRoutes, err := fixture.routeRepo.GetAllForMap(context.Background(), otherMap.ID)
	if err != nil {
		t.Fatalf("get other map routes: %v", err)
	}
	if len(otherRoutes) != 1 || otherRoutes[0].LinkID != linkID {
		t.Fatalf("other map routes = %#v, want route %s untouched", otherRoutes, linkID)
	}
}

func TestCanvasMapLinkRouteRepoDeleteIsIdempotentForMemberWithoutRoute(t *testing.T) {
	linkID := uuid.MustParse("00000000-0000-0000-0000-000000000727")
	fixture := newCanvasMapLinkRouteTestFixture(t, linkID)

	for attempt := 1; attempt <= 2; attempt++ {
		if err := fixture.routeRepo.DeleteForMap(context.Background(), fixture.canvasMap.ID, linkID); err != nil {
			t.Fatalf("delete absent member route attempt %d: %v", attempt, err)
		}
	}
}

func TestCanvasMapLinkRouteRepoDeleteWaitsForMembershipRemovalAndRejectsRemovedLink(t *testing.T) {
	linkID := uuid.MustParse("00000000-0000-0000-0000-000000000728")
	fixture := newCanvasMapLinkRouteTestFixture(t, linkID)
	fixture.saveRoute(t, linkID)

	ctx := context.Background()
	removalTx, err := fixture.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin membership removal transaction: %v", err)
	}
	removalFinished := false
	t.Cleanup(func() {
		if !removalFinished {
			_ = removalTx.Rollback()
		}
	})
	var membershipMarker int
	if err := removalTx.QueryRowContext(
		ctx,
		`SELECT 1 FROM canvas_map_links WHERE map_id = $1 AND link_id = $2 FOR UPDATE`,
		fixture.canvasMap.ID.String(),
		linkID.String(),
	).Scan(&membershipMarker); err != nil {
		t.Fatalf("lock membership row for removal: %v", err)
	}

	deleteDone := make(chan error, 1)
	go func() {
		deleteDone <- fixture.routeRepo.DeleteForMap(context.Background(), fixture.canvasMap.ID, linkID)
	}()
	waitForCanvasMapLinkRouteMembershipWaiter(t, fixture.db, deleteDone)

	if _, err := removalTx.ExecContext(
		ctx,
		`DELETE FROM canvas_map_links WHERE map_id = $1 AND link_id = $2`,
		fixture.canvasMap.ID.String(),
		linkID.String(),
	); err != nil {
		t.Fatalf("remove locked membership: %v", err)
	}
	if err := removalTx.Commit(); err != nil {
		t.Fatalf("commit membership removal: %v", err)
	}
	removalFinished = true

	err = waitForCanvasMapLinkRouteOperation(t, deleteDone, "delete route after membership removal")
	if !errors.Is(err, domain.ErrCanvasMapLinkRouteNotMember) {
		t.Fatalf("delete after membership removal error = %v, want ErrCanvasMapLinkRouteNotMember", err)
	}
	fixture.assertRouteCount(t, 1)
}

func TestCanvasMapLinkRouteRepoMembershipLockHonorsCancellation(t *testing.T) {
	tests := []struct {
		name      string
		operation func(context.Context, domain.CanvasMapLinkRouteRepository, uuid.UUID, uuid.UUID) error
	}{
		{
			name: "upsert",
			operation: func(
				ctx context.Context,
				repo domain.CanvasMapLinkRouteRepository,
				mapID uuid.UUID,
				linkID uuid.UUID,
			) error {
				_, err := repo.UpsertForMap(ctx, mapID, domain.CanvasMapLinkRoute{
					LinkID:    linkID,
					Version:   domain.CanvasMapLinkRouteVersion,
					Waypoints: []domain.CanvasPoint{{X: 300, Y: 400}},
				})
				return err
			},
		},
		{
			name: "delete",
			operation: func(
				ctx context.Context,
				repo domain.CanvasMapLinkRouteRepository,
				mapID uuid.UUID,
				linkID uuid.UUID,
			) error {
				return repo.DeleteForMap(ctx, mapID, linkID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			linkID := uuid.New()
			fixture := newCanvasMapLinkRouteTestFixture(t, linkID)
			contextRepo := fixture.routeRepo
			initial := domain.CanvasMapLinkRoute{
				LinkID:    linkID,
				Version:   domain.CanvasMapLinkRouteVersion,
				Waypoints: []domain.CanvasPoint{{X: 10, Y: 20}},
			}
			if _, err := contextRepo.UpsertForMap(context.Background(), fixture.canvasMap.ID, initial); err != nil {
				t.Fatalf("seed route: %v", err)
			}

			blockerTx, err := fixture.db.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatalf("begin membership blocker transaction: %v", err)
			}
			blockerFinished := false
			t.Cleanup(func() {
				if !blockerFinished {
					_ = blockerTx.Rollback()
				}
			})
			var membershipMarker int
			if err := blockerTx.QueryRowContext(
				context.Background(),
				`SELECT 1 FROM canvas_map_links WHERE map_id = $1 AND link_id = $2 FOR UPDATE`,
				fixture.canvasMap.ID.String(),
				linkID.String(),
			).Scan(&membershipMarker); err != nil {
				t.Fatalf("lock route membership: %v", err)
			}

			operationCtx, cancel := context.WithCancel(context.Background())
			operationDone := make(chan error, 1)
			go func() {
				operationDone <- tt.operation(operationCtx, contextRepo, fixture.canvasMap.ID, linkID)
			}()
			waitForCanvasMapLinkRouteMembershipWaiter(t, fixture.db, operationDone)
			cancel()

			select {
			case err := <-operationDone:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("%s error = %v, want context.Canceled", tt.name, err)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("%s did not return promptly after context cancellation", tt.name)
			}

			routes, err := contextRepo.GetAllForMap(context.Background(), fixture.canvasMap.ID)
			if err != nil {
				t.Fatalf("get routes after canceled %s: %v", tt.name, err)
			}
			if len(routes) != 1 {
				t.Fatalf("routes after canceled %s = %#v, want original route", tt.name, routes)
			}
			assertCanvasMapLinkRouteContent(t, routes[0], initial)

			if err := blockerTx.Rollback(); err != nil {
				t.Fatalf("release membership blocker: %v", err)
			}
			blockerFinished = true
		})
	}
}

func TestCanvasMapLinkRouteRepoConcurrentMembershipRemovalPrunesLateUpsert(t *testing.T) {
	linkID := uuid.MustParse("00000000-0000-0000-0000-000000000725")
	fixture := newCanvasMapLinkRouteTestFixture(t, linkID)
	const advisoryLockKey int64 = 260026
	installCanvasMapLinkRouteInsertPause(t, fixture.db, advisoryLockKey)

	ctx := context.Background()
	lockConn, err := fixture.db.Conn(ctx)
	if err != nil {
		t.Fatalf("open advisory lock connection: %v", err)
	}
	defer lockConn.Close()
	if _, err := lockConn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockKey); err != nil {
		t.Fatalf("acquire advisory lock: %v", err)
	}
	lockHeld := true
	unlock := func() {
		t.Helper()
		if !lockHeld {
			return
		}
		if _, err := lockConn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, advisoryLockKey); err != nil {
			t.Fatalf("release advisory lock: %v", err)
		}
		lockHeld = false
	}
	defer func() {
		if lockHeld {
			_, _ = lockConn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, advisoryLockKey)
		}
	}()

	upsertDone := make(chan error, 1)
	go func() {
		_, err := fixture.routeRepo.UpsertForMap(context.Background(), fixture.canvasMap.ID, domain.CanvasMapLinkRoute{
			LinkID:    linkID,
			Version:   domain.CanvasMapLinkRouteVersion,
			Waypoints: []domain.CanvasPoint{{X: 12, Y: 24}},
		})
		upsertDone <- err
	}()
	waitForCanvasMapLinkRouteAdvisoryWaiter(t, fixture.db, advisoryLockKey)

	removeDone := make(chan error, 1)
	go func() {
		removeDone <- fixture.mapRepo.RemoveLink(fixture.canvasMap.ID, linkID)
	}()
	removeErr, removeFinished := waitForCanvasMapLinkRemovalState(t, fixture.db, removeDone)

	unlock()
	if err := waitForCanvasMapLinkRouteOperation(t, upsertDone, "upsert route"); err != nil {
		t.Fatalf("upsert route: %v", err)
	}
	if !removeFinished {
		removeErr = waitForCanvasMapLinkRouteOperation(t, removeDone, "remove membership")
	}
	if removeErr != nil {
		t.Fatalf("remove link membership: %v", removeErr)
	}
	fixture.assertRouteCount(t, 0)
}

func TestCanvasMapLinkRouteRepoValidatesRoutesOnWriteAndRead(t *testing.T) {
	linkID := uuid.MustParse("00000000-0000-0000-0000-000000000731")
	fixture := newCanvasMapLinkRouteTestFixture(t, linkID)

	_, err := fixture.routeRepo.UpsertForMap(context.Background(), fixture.canvasMap.ID, domain.CanvasMapLinkRoute{
		LinkID:    linkID,
		Version:   domain.CanvasMapLinkRouteVersion,
		Waypoints: nil,
	})
	if err == nil {
		t.Fatal("expected invalid route write to fail")
	}

	if _, err := wrapDB(fixture.db).Exec(
		`INSERT INTO canvas_map_link_routes (map_id, link_id, route_version, waypoints_json)
		 VALUES (?, ?, ?, ?::jsonb)`,
		fixture.canvasMap.ID.String(),
		linkID.String(),
		domain.CanvasMapLinkRouteVersion,
		`[]`,
	); err != nil {
		t.Fatalf("insert invalid stored route: %v", err)
	}
	if _, err := fixture.routeRepo.GetAllForMap(context.Background(), fixture.canvasMap.ID); err == nil {
		t.Fatal("expected invalid stored route read to fail")
	}
}

func TestCanvasMapRepoDuplicateCopiesLinkRoutes(t *testing.T) {
	linkID := uuid.MustParse("00000000-0000-0000-0000-000000000741")
	fixture := newCanvasMapLinkRouteTestFixture(t, linkID)
	want := domain.CanvasMapLinkRoute{
		LinkID:    linkID,
		Version:   domain.CanvasMapLinkRouteVersion,
		Waypoints: []domain.CanvasPoint{{X: 45, Y: 60}, {X: 80, Y: 95}},
	}
	if _, err := fixture.routeRepo.UpsertForMap(context.Background(), fixture.canvasMap.ID, want); err != nil {
		t.Fatalf("upsert source route: %v", err)
	}

	duplicate, err := fixture.mapRepo.Duplicate(fixture.canvasMap.ID, "Route Copy")
	if err != nil {
		t.Fatalf("duplicate map: %v", err)
	}
	routes, err := fixture.routeRepo.GetAllForMap(context.Background(), duplicate.ID)
	if err != nil {
		t.Fatalf("get duplicate routes: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("duplicate route count = %d, want 1: %#v", len(routes), routes)
	}
	assertCanvasMapLinkRouteContent(t, routes[0], want)
}

func TestCanvasMapRepoDuplicateCopiesLinkRoutesOnlyForCopiedMembership(t *testing.T) {
	initialLinkID := uuid.MustParse("00000000-0000-0000-0000-000000000742")
	lateLinkID := uuid.MustParse("00000000-0000-0000-0000-000000000743")
	fixture := newCanvasMapLinkRouteTestFixture(t, initialLinkID)
	insertCanvasMapLinkRouteTestLink(t, fixture.db, lateLinkID, fixture.sourceDeviceID, fixture.targetDeviceID)
	const advisoryLockKey int64 = 260027
	installCanvasMapLinkMembershipCopyPause(t, fixture.db, fixture.canvasMap.ID, advisoryLockKey)

	ctx := context.Background()
	lockConn, err := fixture.db.Conn(ctx)
	if err != nil {
		t.Fatalf("open advisory lock connection: %v", err)
	}
	defer lockConn.Close()
	if _, err := lockConn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockKey); err != nil {
		t.Fatalf("acquire advisory lock: %v", err)
	}
	lockHeld := true
	unlock := func() {
		t.Helper()
		if !lockHeld {
			return
		}
		if _, err := lockConn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, advisoryLockKey); err != nil {
			t.Fatalf("release advisory lock: %v", err)
		}
		lockHeld = false
	}
	defer func() {
		if lockHeld {
			_, _ = lockConn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, advisoryLockKey)
		}
	}()

	type duplicateResult struct {
		canvasMap domain.CanvasMap
		err       error
	}
	duplicateDone := make(chan duplicateResult, 1)
	go func() {
		duplicate, err := fixture.mapRepo.Duplicate(fixture.canvasMap.ID, "Concurrent Route Copy")
		duplicateDone <- duplicateResult{canvasMap: duplicate, err: err}
	}()
	waitForCanvasMapLinkRouteAdvisoryWaiter(t, fixture.db, advisoryLockKey)

	if err := fixture.mapRepo.AddDeviceMembership(
		fixture.canvasMap.ID,
		domain.CanvasMapDeviceMembership{
			DeviceID: fixture.sourceDeviceID,
			Role:     domain.CanvasMapDeviceRoleBase,
		},
		[]uuid.UUID{lateLinkID},
		nil,
	); err != nil {
		t.Fatalf("add concurrent source link membership: %v", err)
	}
	lateRoute := domain.CanvasMapLinkRoute{
		LinkID:    lateLinkID,
		Version:   domain.CanvasMapLinkRouteVersion,
		Waypoints: []domain.CanvasPoint{{X: 300, Y: 150}},
	}
	if _, err := fixture.routeRepo.UpsertForMap(context.Background(), fixture.canvasMap.ID, lateRoute); err != nil {
		t.Fatalf("upsert concurrent source route: %v", err)
	}

	unlock()
	var result duplicateResult
	select {
	case result = <-duplicateDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting to duplicate canvas map")
	}
	if result.err != nil {
		t.Fatalf("duplicate canvas map: %v", result.err)
	}

	membership, err := fixture.mapRepo.GetMembership(result.canvasMap.ID)
	if err != nil {
		t.Fatalf("get duplicate membership: %v", err)
	}
	for _, linkID := range membership.LinkIDs {
		if linkID == lateLinkID {
			t.Fatalf("duplicate unexpectedly copied late link membership %s", lateLinkID)
		}
	}
	routes, err := fixture.routeRepo.GetAllForMap(context.Background(), result.canvasMap.ID)
	if err != nil {
		t.Fatalf("get duplicate routes: %v", err)
	}
	for _, route := range routes {
		if route.LinkID == lateLinkID {
			t.Fatalf("duplicate copied route for non-member late link %s: %#v", lateLinkID, route)
		}
	}
}

func TestCanvasMapLinkRouteMembershipLifecycle(t *testing.T) {
	linkID := uuid.MustParse("00000000-0000-0000-0000-000000000751")

	t.Run("replacement preserves remaining route", func(t *testing.T) {
		fixture := newCanvasMapLinkRouteTestFixture(t, linkID)
		fixture.saveRoute(t, linkID)

		if err := fixture.mapRepo.ReplaceMembership(fixture.canvasMap.ID, fixture.membership(linkID)); err != nil {
			t.Fatalf("replace same membership: %v", err)
		}
		fixture.assertRouteCount(t, 1)
	})

	t.Run("replacement prunes removed route", func(t *testing.T) {
		fixture := newCanvasMapLinkRouteTestFixture(t, linkID)
		fixture.saveRoute(t, linkID)

		if err := fixture.mapRepo.ReplaceMembership(fixture.canvasMap.ID, fixture.membership()); err != nil {
			t.Fatalf("replace membership without link: %v", err)
		}
		fixture.assertRouteCount(t, 0)
	})

	t.Run("link removal prunes route", func(t *testing.T) {
		fixture := newCanvasMapLinkRouteTestFixture(t, linkID)
		fixture.saveRoute(t, linkID)

		if err := fixture.mapRepo.RemoveLink(fixture.canvasMap.ID, linkID); err != nil {
			t.Fatalf("remove link membership: %v", err)
		}
		fixture.assertRouteCount(t, 0)
	})

	t.Run("device removal prunes routes for removed links", func(t *testing.T) {
		fixture := newCanvasMapLinkRouteTestFixture(t, linkID)
		fixture.saveRoute(t, linkID)

		if err := fixture.mapRepo.RemoveDevice(fixture.canvasMap.ID, fixture.sourceDeviceID); err != nil {
			t.Fatalf("remove device membership: %v", err)
		}
		fixture.assertRouteCount(t, 0)
	})
}

func TestCanvasMapLinkRouteStartupMembershipMaterializationPrunesRoutes(t *testing.T) {
	db := openCanvasMapRepoTestDB(t)
	mapRepo := NewCanvasMapRepo(db)
	routeRepo := NewCanvasMapLinkRouteRepo(db)
	sourceDeviceID := uuid.MustParse("00000000-0000-0000-0000-000000000761")
	targetDeviceID := uuid.MustParse("00000000-0000-0000-0000-000000000762")
	linkID := uuid.MustParse("00000000-0000-0000-0000-000000000763")
	insertCanvasMapLinkRouteTestDevice(t, db, sourceDeviceID)
	insertCanvasMapLinkRouteTestDevice(t, db, targetDeviceID)
	insertCanvasMapLinkRouteTestLink(t, db, linkID, sourceDeviceID, targetDeviceID)

	canvasMap, err := mapRepo.Create(domain.CanvasMapCreate{
		Name:   "Materialized Routes",
		Filter: domain.CanvasMapFilter{DeviceIDs: []uuid.UUID{sourceDeviceID}},
	})
	if err != nil {
		t.Fatalf("create canvas map: %v", err)
	}
	membership := domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{
			{DeviceID: sourceDeviceID, Role: domain.CanvasMapDeviceRoleBase},
			{DeviceID: targetDeviceID, Role: domain.CanvasMapDeviceRoleBase},
		},
		LinkIDs: []uuid.UUID{linkID},
	}
	if err := mapRepo.ReplaceMembership(canvasMap.ID, membership); err != nil {
		t.Fatalf("seed route membership: %v", err)
	}
	if _, err := routeRepo.UpsertForMap(context.Background(), canvasMap.ID, domain.CanvasMapLinkRoute{
		LinkID:    linkID,
		Version:   domain.CanvasMapLinkRouteVersion,
		Waypoints: []domain.CanvasPoint{{X: 30, Y: 40}},
	}); err != nil {
		t.Fatalf("seed route: %v", err)
	}
	if _, err := wrapDB(db).Exec(
		`UPDATE canvas_maps SET membership_materialized = ? WHERE id = ?`,
		false,
		canvasMap.ID.String(),
	); err != nil {
		t.Fatalf("mark map membership stale: %v", err)
	}

	if err := migrateCanvasMapMemberships(db); err != nil {
		t.Fatalf("materialize canvas map memberships: %v", err)
	}
	routes, err := routeRepo.GetAllForMap(context.Background(), canvasMap.ID)
	if err != nil {
		t.Fatalf("get routes after materialization: %v", err)
	}
	if len(routes) != 0 {
		t.Fatalf("route count after materialization = %d, want 0: %#v", len(routes), routes)
	}
}

type canvasMapLinkRouteTestFixture struct {
	db             *sql.DB
	mapRepo        *CanvasMapRepo
	routeRepo      domain.CanvasMapLinkRouteRepository
	canvasMap      domain.CanvasMap
	sourceDeviceID uuid.UUID
	targetDeviceID uuid.UUID
}

func newCanvasMapLinkRouteTestFixture(t *testing.T, linkIDs ...uuid.UUID) canvasMapLinkRouteTestFixture {
	t.Helper()
	db := openCanvasMapRepoTestDB(t)
	mapRepo := NewCanvasMapRepo(db)
	routeRepo := NewCanvasMapLinkRouteRepo(db)
	sourceDeviceID := uuid.MustParse("00000000-0000-0000-0000-000000000701")
	targetDeviceID := uuid.MustParse("00000000-0000-0000-0000-000000000702")
	insertCanvasMapLinkRouteTestDevice(t, db, sourceDeviceID)
	insertCanvasMapLinkRouteTestDevice(t, db, targetDeviceID)
	for _, linkID := range linkIDs {
		insertCanvasMapLinkRouteTestLink(t, db, linkID, sourceDeviceID, targetDeviceID)
	}

	canvasMap, err := mapRepo.Create(domain.CanvasMapCreate{Name: "Map Link Routes"})
	if err != nil {
		t.Fatalf("create canvas map: %v", err)
	}
	fixture := canvasMapLinkRouteTestFixture{
		db:             db,
		mapRepo:        mapRepo,
		routeRepo:      routeRepo,
		canvasMap:      canvasMap,
		sourceDeviceID: sourceDeviceID,
		targetDeviceID: targetDeviceID,
	}
	if err := mapRepo.ReplaceMembership(canvasMap.ID, fixture.membership(linkIDs...)); err != nil {
		t.Fatalf("replace canvas map membership: %v", err)
	}
	return fixture
}

func (f canvasMapLinkRouteTestFixture) membership(linkIDs ...uuid.UUID) domain.CanvasMapMembership {
	return domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{
			{DeviceID: f.sourceDeviceID, Role: domain.CanvasMapDeviceRoleBase},
			{DeviceID: f.targetDeviceID, Role: domain.CanvasMapDeviceRoleBase},
		},
		LinkIDs: append([]uuid.UUID(nil), linkIDs...),
	}
}

func (f canvasMapLinkRouteTestFixture) saveRoute(t *testing.T, linkID uuid.UUID) {
	t.Helper()
	if _, err := f.routeRepo.UpsertForMap(context.Background(), f.canvasMap.ID, domain.CanvasMapLinkRoute{
		LinkID:    linkID,
		Version:   domain.CanvasMapLinkRouteVersion,
		Waypoints: []domain.CanvasPoint{{X: 10, Y: 20}},
	}); err != nil {
		t.Fatalf("save route: %v", err)
	}
}

func (f canvasMapLinkRouteTestFixture) assertRouteCount(t *testing.T, want int) {
	t.Helper()
	routes, err := f.routeRepo.GetAllForMap(context.Background(), f.canvasMap.ID)
	if err != nil {
		t.Fatalf("get routes: %v", err)
	}
	if len(routes) != want {
		t.Fatalf("route count = %d, want %d: %#v", len(routes), want, routes)
	}
}

func assertCanvasMapLinkRouteContent(t *testing.T, got, want domain.CanvasMapLinkRoute) {
	t.Helper()
	if got.LinkID != want.LinkID || got.Version != want.Version || !reflect.DeepEqual(got.Waypoints, want.Waypoints) {
		t.Fatalf("route = %#v, want content %#v", got, want)
	}
}

func insertCanvasMapLinkRouteTestDevice(t *testing.T, db *sql.DB, id uuid.UUID) {
	t.Helper()

	suffix := id.String()[len(id.String())-3:]
	if _, err := wrapDB(db).Exec(
		`INSERT INTO devices (id, hostname, ip, device_type, status, sys_name, sys_descr, sys_object_id, hardware_model, vendor, managed, tags_json, metrics_source, prometheus_label_name, prometheus_label_value, created_at, updated_at)
		 VALUES (?, ?, ?, 'router', 'up', ?, '', '', '', 'default', 1, '{}', 'none', '', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		id.String(),
		"router-"+suffix,
		"10.0.9."+suffix[1:],
		"router-"+suffix,
	); err != nil {
		t.Fatalf("insert route test device %s: %v", id, err)
	}
}

func insertCanvasMapLinkRouteTestLink(
	t *testing.T,
	db *sql.DB,
	id uuid.UUID,
	sourceDeviceID uuid.UUID,
	targetDeviceID uuid.UUID,
) {
	t.Helper()

	suffix := id.String()[len(id.String())-3:]
	if _, err := wrapDB(db).Exec(
		`INSERT INTO links (id, source_device_id, source_if_name, target_device_id, target_if_name, discovery_protocol, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 'manual', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		id.String(),
		sourceDeviceID.String(),
		"ether1-"+suffix,
		targetDeviceID.String(),
		"ether2-"+suffix,
	); err != nil {
		t.Fatalf("insert route test link %s: %v", id, err)
	}
}

func installCanvasMapLinkRouteInsertPause(t *testing.T, db *sql.DB, advisoryLockKey int64) {
	t.Helper()

	if _, err := db.Exec(`DROP TRIGGER IF EXISTS test_pause_canvas_map_link_route_insert ON canvas_map_link_routes`); err != nil {
		t.Fatalf("drop existing route insert pause trigger: %v", err)
	}
	functionSQL := fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION test_pause_canvas_map_link_route_insert()
		RETURNS trigger AS $$
		BEGIN
			PERFORM pg_advisory_xact_lock(%d);
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql`, advisoryLockKey)
	if _, err := db.Exec(functionSQL); err != nil {
		t.Fatalf("install route insert pause function: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TRIGGER test_pause_canvas_map_link_route_insert
		BEFORE INSERT ON canvas_map_link_routes
		FOR EACH ROW EXECUTE FUNCTION test_pause_canvas_map_link_route_insert()`); err != nil {
		t.Fatalf("install route insert pause trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DROP TRIGGER IF EXISTS test_pause_canvas_map_link_route_insert ON canvas_map_link_routes`)
		_, _ = db.Exec(`DROP FUNCTION IF EXISTS test_pause_canvas_map_link_route_insert()`)
	})
}

func installCanvasMapLinkMembershipCopyPause(
	t *testing.T,
	db *sql.DB,
	sourceMapID uuid.UUID,
	advisoryLockKey int64,
) {
	t.Helper()

	if _, err := db.Exec(`DROP TRIGGER IF EXISTS test_pause_canvas_map_link_membership_copy ON canvas_map_links`); err != nil {
		t.Fatalf("drop existing link membership copy pause trigger: %v", err)
	}
	functionSQL := fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION test_pause_canvas_map_link_membership_copy()
		RETURNS trigger AS $$
		BEGIN
			IF NEW.map_id <> '%s' THEN
				PERFORM pg_advisory_xact_lock(%d);
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql`, sourceMapID.String(), advisoryLockKey)
	if _, err := db.Exec(functionSQL); err != nil {
		t.Fatalf("install link membership copy pause function: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TRIGGER test_pause_canvas_map_link_membership_copy
		BEFORE INSERT ON canvas_map_links
		FOR EACH ROW EXECUTE FUNCTION test_pause_canvas_map_link_membership_copy()`); err != nil {
		t.Fatalf("install link membership copy pause trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DROP TRIGGER IF EXISTS test_pause_canvas_map_link_membership_copy ON canvas_map_links`)
		_, _ = db.Exec(`DROP FUNCTION IF EXISTS test_pause_canvas_map_link_membership_copy()`)
	})
}

func waitForCanvasMapLinkRouteAdvisoryWaiter(t *testing.T, db *sql.DB, advisoryLockKey int64) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		if err := db.QueryRow(
			`SELECT COUNT(*)
			 FROM pg_locks
			 WHERE locktype = 'advisory'
			   AND granted = FALSE
			   AND classid = 0
			   AND objid = $1::oid`,
			advisoryLockKey,
		).Scan(&count); err != nil {
			t.Fatalf("query advisory lock waiters: %v", err)
		}
		if count > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for route upsert to pause")
}

func waitForCanvasMapLinkRemovalState(
	t *testing.T,
	db *sql.DB,
	removeDone <-chan error,
) (error, bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-removeDone:
			return err, true
		default:
		}

		var count int
		if err := db.QueryRow(
			`SELECT COUNT(*)
			 FROM pg_stat_activity
			 WHERE datname = current_database()
			   AND pid <> pg_backend_pid()
			   AND wait_event_type = 'Lock'
			   AND query LIKE '%DELETE FROM canvas_map_links%'`,
		).Scan(&count); err != nil {
			t.Fatalf("query blocked membership removal: %v", err)
		}
		if count > 0 {
			return nil, false
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for membership removal state")
	return nil, false
}

func waitForCanvasMapLinkRouteMembershipWaiter(
	t *testing.T,
	db *sql.DB,
	done <-chan error,
) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("route operation completed before the membership lock was released: %v", err)
		default:
		}

		var count int
		if err := db.QueryRow(
			`SELECT COUNT(*)
			 FROM pg_stat_activity
			 WHERE datname = current_database()
			   AND pid <> pg_backend_pid()
			   AND wait_event_type = 'Lock'
			   AND query LIKE '%FROM canvas_map_links%FOR KEY SHARE%'`,
		).Scan(&count); err != nil {
			t.Fatalf("query blocked route membership checks: %v", err)
		}
		if count > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for route membership check to block")
}

func waitForCanvasMapLinkRouteOperation(t *testing.T, done <-chan error, operation string) error {
	t.Helper()

	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting to %s", operation)
		return nil
	}
}
