package postgres

// This file exercises canvas map repo behavior so refactors preserve the documented contract.

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func openCanvasMapRepoTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestDB(t)
	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	return db
}

func TestCanvasMapRepoCreatesListsAndRejectsSecondDefault(t *testing.T) {
	db := openCanvasMapRepoTestDB(t)
	repo := NewCanvasMapRepo(db)

	maps, err := repo.List()
	if err != nil {
		t.Fatalf("list maps: %v", err)
	}
	if len(maps) != 1 || !maps[0].IsDefault || maps[0].Name != "Default" {
		t.Fatalf("expected seeded default map, got %#v", maps)
	}

	created, err := repo.Create(domain.CanvasMapCreate{
		Name:        "Backbone",
		Description: "Backbone operational map",
		Filter: domain.CanvasMapFilter{
			IncludeCrossAreaLinks: true,
			IncludeGhostDevices:   true,
		},
	})
	if err != nil {
		t.Fatalf("create map: %v", err)
	}
	if created.ID == uuid.Nil || created.Name != "Backbone" || created.IsDefault {
		t.Fatalf("unexpected created map: %#v", created)
	}

	if _, err := repo.Create(domain.CanvasMapCreate{Name: "Second Default", IsDefault: true}); err == nil {
		t.Fatal("expected second default map create to fail")
	}
}

func TestCanvasMapSelectQueryAggregatesCountsBeforeJoiningMaps(t *testing.T) {
	query := canvasMapSelectQuery("")

	for _, joinedTable := range []string{
		"canvas_map_devices cmd",
		"canvas_map_links cml",
		"canvas_map_positions cmp",
	} {
		if strings.Contains(query, joinedTable) {
			t.Fatalf("canvas map list query directly joins %s; counts must be pre-aggregated by map_id to avoid fan-out", joinedTable)
		}
	}

	for _, countAlias := range []string{
		"device_counts",
		"link_counts",
		"position_counts",
	} {
		if !strings.Contains(query, countAlias) {
			t.Fatalf("canvas map list query missing pre-aggregated %s subquery", countAlias)
		}
	}
}

func TestCanvasMapRepoDuplicatesMetadataAndPositions(t *testing.T) {
	db := openCanvasMapRepoTestDB(t)
	mapRepo := NewCanvasMapRepo(db)
	positionRepo := NewCanvasMapPositionRepo(db)

	sourceFilter := domain.CanvasMapFilter{
		IncludeCrossAreaLinks: true,
		Tags:                  map[string]string{"pop": "milano"},
	}
	source, err := mapRepo.Create(domain.CanvasMapCreate{
		Name:        "POP Milano",
		Description: "Milano aggregation layout",
		Filter:      sourceFilter,
	})
	if err != nil {
		t.Fatalf("create source map: %v", err)
	}

	deviceID := uuid.MustParse("00000000-0000-0000-0000-000000000201")
	insertCanvasMapRepoTestDevice(t, db, deviceID)
	if err := positionRepo.SaveAllForMap(source.ID, []domain.DevicePosition{{DeviceID: deviceID, X: 10, Y: 20, Pinned: true}}); err != nil {
		t.Fatalf("save source positions: %v", err)
	}

	copy, err := mapRepo.Duplicate(source.ID, "Copy of POP Milano")
	if err != nil {
		t.Fatalf("duplicate map: %v", err)
	}
	if copy.ID == source.ID || copy.Name != "Copy of POP Milano" || copy.Description != source.Description || copy.FilterJSON != source.FilterJSON || copy.IsDefault {
		t.Fatalf("unexpected copied map metadata: %#v, source %#v", copy, source)
	}

	positions, err := positionRepo.GetAllForMap(copy.ID)
	if err != nil {
		t.Fatalf("get copy positions: %v", err)
	}
	if len(positions) != 1 || positions[0].DeviceID != deviceID || positions[0].X != 10 || !positions[0].Pinned {
		t.Fatalf("unexpected copied positions: %#v", positions)
	}
}

func TestCanvasMapMembershipRepoReplaceAndGetPersistsDevicesLinksAreas(t *testing.T) {
	db := openCanvasMapRepoTestDB(t)
	repo := NewCanvasMapRepo(db)

	canvasMap, err := repo.Create(domain.CanvasMapCreate{Name: "Materialized POP"})
	if err != nil {
		t.Fatalf("create map: %v", err)
	}
	baseDeviceID := uuid.MustParse("00000000-0000-0000-0000-000000000601")
	ghostDeviceID := uuid.MustParse("00000000-0000-0000-0000-000000000602")
	insertCanvasMapRepoTestDevice(t, db, baseDeviceID)
	insertCanvasMapRepoTestDevice(t, db, ghostDeviceID)

	linkID := uuid.MustParse("00000000-0000-0000-0000-000000000603")
	insertCanvasMapRepoTestLink(t, db, linkID, baseDeviceID, ghostDeviceID)

	areaID := uuid.MustParse("00000000-0000-0000-0000-000000000604")
	membership := domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{
			{DeviceID: baseDeviceID, Role: domain.CanvasMapDeviceRoleBase},
			{DeviceID: ghostDeviceID, Role: domain.CanvasMapDeviceRoleGhost},
		},
		LinkIDs: []uuid.UUID{linkID},
		Areas: []domain.CanvasMapAreaMembership{
			{
				AreaID:      areaID,
				Name:        "North POP",
				Description: "map-local north aggregation",
				Color:       "#123456",
			},
		},
	}

	if err := repo.ReplaceMembership(canvasMap.ID, membership); err != nil {
		t.Fatalf("replace membership: %v", err)
	}

	got, err := repo.GetMembership(canvasMap.ID)
	if err != nil {
		t.Fatalf("get membership: %v", err)
	}
	assertCanvasMapMembershipEqual(t, got, membership)

	replacement := domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{
			{DeviceID: ghostDeviceID, Role: domain.CanvasMapDeviceRoleGhost},
		},
	}
	if err := repo.ReplaceMembership(canvasMap.ID, replacement); err != nil {
		t.Fatalf("replace membership with smaller set: %v", err)
	}

	got, err = repo.GetMembership(canvasMap.ID)
	if err != nil {
		t.Fatalf("get replacement membership: %v", err)
	}
	assertCanvasMapMembershipEqual(t, got, replacement)
}

func TestCanvasMapRepoAddDeviceMembershipIsIdempotentAndPreservesOtherMaps(t *testing.T) {
	db := openCanvasMapRepoTestDB(t)
	repo := NewCanvasMapRepo(db)

	firstMap, err := repo.Create(domain.CanvasMapCreate{Name: "First"})
	if err != nil {
		t.Fatalf("create first map: %v", err)
	}
	secondMap, err := repo.Create(domain.CanvasMapCreate{Name: "Second"})
	if err != nil {
		t.Fatalf("create second map: %v", err)
	}

	deviceA := uuid.MustParse("00000000-0000-0000-0000-000000000701")
	deviceB := uuid.MustParse("00000000-0000-0000-0000-000000000702")
	insertCanvasMapRepoTestDevice(t, db, deviceA)
	insertCanvasMapRepoTestDevice(t, db, deviceB)
	linkID := uuid.MustParse("00000000-0000-0000-0000-000000000703")
	insertCanvasMapRepoTestLink(t, db, linkID, deviceA, deviceB)
	areaID := uuid.MustParse("00000000-0000-0000-0000-000000000704")

	initial := domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{
			{DeviceID: deviceA, Role: domain.CanvasMapDeviceRoleBase},
		},
	}
	if err := repo.ReplaceMembership(firstMap.ID, initial); err != nil {
		t.Fatalf("replace first membership: %v", err)
	}
	if err := repo.ReplaceMembership(secondMap.ID, initial); err != nil {
		t.Fatalf("replace second membership: %v", err)
	}

	addedDevice := domain.CanvasMapDeviceMembership{
		DeviceID: deviceB,
		Role:     domain.CanvasMapDeviceRoleBase,
	}
	addedArea := domain.CanvasMapAreaMembership{
		AreaID:      areaID,
		Name:        "Backbone",
		Description: "Backbone devices",
		Color:       "#00AEEF",
	}
	for i := 0; i < 2; i++ {
		if err := repo.AddDeviceMembership(firstMap.ID, addedDevice, []uuid.UUID{linkID}, []domain.CanvasMapAreaMembership{addedArea}); err != nil {
			t.Fatalf("add device membership iteration %d: %v", i, err)
		}
	}

	firstMembership, err := repo.GetMembership(firstMap.ID)
	if err != nil {
		t.Fatalf("get first membership: %v", err)
	}
	assertCanvasMapMembershipEqual(t, firstMembership, domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{
			{DeviceID: deviceA, Role: domain.CanvasMapDeviceRoleBase},
			{DeviceID: deviceB, Role: domain.CanvasMapDeviceRoleBase},
		},
		LinkIDs: []uuid.UUID{linkID},
		Areas:   []domain.CanvasMapAreaMembership{addedArea},
	})

	secondMembership, err := repo.GetMembership(secondMap.ID)
	if err != nil {
		t.Fatalf("get second membership: %v", err)
	}
	assertCanvasMapMembershipEqual(t, secondMembership, initial)
}

func TestCanvasMapRepoListMemberDeviceIDsReturnsDistinctSavedMapMembers(t *testing.T) {
	db := openCanvasMapRepoTestDB(t)
	repo := NewCanvasMapRepo(db)

	firstMap, err := repo.Create(domain.CanvasMapCreate{Name: "First Members"})
	if err != nil {
		t.Fatalf("create first map: %v", err)
	}
	secondMap, err := repo.Create(domain.CanvasMapCreate{Name: "Second Members"})
	if err != nil {
		t.Fatalf("create second map: %v", err)
	}
	memberA := uuid.MustParse("00000000-0000-0000-0000-000000000241")
	memberB := uuid.MustParse("00000000-0000-0000-0000-000000000242")
	orphan := uuid.MustParse("00000000-0000-0000-0000-000000000243")
	insertCanvasMapRepoTestDevice(t, db, memberA)
	insertCanvasMapRepoTestDevice(t, db, memberB)
	insertCanvasMapRepoTestDevice(t, db, orphan)

	if err := repo.ReplaceMembership(firstMap.ID, domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{
			{DeviceID: memberA, Role: domain.CanvasMapDeviceRoleBase},
			{DeviceID: memberB, Role: domain.CanvasMapDeviceRoleGhost},
		},
	}); err != nil {
		t.Fatalf("replace first membership: %v", err)
	}
	if err := repo.ReplaceMembership(secondMap.ID, domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{
			{DeviceID: memberA, Role: domain.CanvasMapDeviceRoleBase},
		},
	}); err != nil {
		t.Fatalf("replace second membership: %v", err)
	}

	deviceIDs, err := repo.ListMemberDeviceIDs()
	if err != nil {
		t.Fatalf("ListMemberDeviceIDs: %v", err)
	}
	if len(deviceIDs) != 2 {
		t.Fatalf("len(deviceIDs) = %d, want 2; ids = %#v", len(deviceIDs), deviceIDs)
	}
	got := map[uuid.UUID]bool{}
	for _, id := range deviceIDs {
		got[id] = true
	}
	if !got[memberA] || !got[memberB] || got[orphan] {
		t.Fatalf("deviceIDs = %#v, want distinct members %s and %s only", deviceIDs, memberA, memberB)
	}
}

func TestCanvasMapRepoUpdateDeviceAreaMembershipsIsMapLocal(t *testing.T) {
	db := openCanvasMapRepoTestDB(t)
	repo := NewCanvasMapRepo(db)

	firstMap, err := repo.Create(domain.CanvasMapCreate{Name: "First Area Scope"})
	if err != nil {
		t.Fatalf("create first map: %v", err)
	}
	secondMap, err := repo.Create(domain.CanvasMapCreate{Name: "Second Area Scope"})
	if err != nil {
		t.Fatalf("create second map: %v", err)
	}

	deviceID := uuid.MustParse("00000000-0000-0000-0000-000000000711")
	areaA := uuid.MustParse("00000000-0000-0000-0000-000000000712")
	areaB := uuid.MustParse("00000000-0000-0000-0000-000000000713")
	insertCanvasMapRepoTestDevice(t, db, deviceID)
	insertCanvasMapRepoTestArea(t, db, areaA, "Original Area", "#111111")
	insertCanvasMapRepoTestArea(t, db, areaB, "Copy Area", "#222222")
	insertCanvasMapRepoTestDeviceArea(t, db, deviceID, areaA)

	membership := domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{
			{DeviceID: deviceID, Role: domain.CanvasMapDeviceRoleBase, AreaIDs: []uuid.UUID{areaA}},
		},
		Areas: []domain.CanvasMapAreaMembership{
			{AreaID: areaA, Name: "Original Area", Color: "#111111"},
			{AreaID: areaB, Name: "Copy Area", Color: "#222222"},
		},
	}
	if err := repo.ReplaceMembership(firstMap.ID, membership); err != nil {
		t.Fatalf("replace first membership: %v", err)
	}
	if err := repo.ReplaceMembership(secondMap.ID, membership); err != nil {
		t.Fatalf("replace second membership: %v", err)
	}

	if err := repo.UpdateDeviceAreaMemberships(firstMap.ID, []uuid.UUID{deviceID}, []uuid.UUID{areaB}); err != nil {
		t.Fatalf("update first map device areas: %v", err)
	}

	firstMembership, err := repo.GetMembership(firstMap.ID)
	if err != nil {
		t.Fatalf("get first membership: %v", err)
	}
	if len(firstMembership.Devices) != 1 || !uuidSlicesEqual(firstMembership.Devices[0].AreaIDs, []uuid.UUID{areaB}) {
		t.Fatalf("first map device areas = %#v, want only %s", firstMembership.Devices, areaB)
	}

	secondMembership, err := repo.GetMembership(secondMap.ID)
	if err != nil {
		t.Fatalf("get second membership: %v", err)
	}
	if len(secondMembership.Devices) != 1 || !uuidSlicesEqual(secondMembership.Devices[0].AreaIDs, []uuid.UUID{areaA}) {
		t.Fatalf("second map device areas = %#v, want original %s", secondMembership.Devices, areaA)
	}

	var globalAreaID string
	if err := db.QueryRow(`SELECT area_id FROM device_areas WHERE device_id = ?`, deviceID.String()).Scan(&globalAreaID); err != nil {
		t.Fatalf("query global device area: %v", err)
	}
	if globalAreaID != areaA.String() {
		t.Fatalf("global device area = %s, want %s", globalAreaID, areaA)
	}
}

func TestCanvasMapRepoUpdateDeviceVisualColorPersistsAndClearsMapLocalOverride(t *testing.T) {
	db := openCanvasMapRepoTestDB(t)
	repo := NewCanvasMapRepo(db)

	firstMap, err := repo.Create(domain.CanvasMapCreate{Name: "First Visual Scope"})
	if err != nil {
		t.Fatalf("create first map: %v", err)
	}
	secondMap, err := repo.Create(domain.CanvasMapCreate{Name: "Second Visual Scope"})
	if err != nil {
		t.Fatalf("create second map: %v", err)
	}

	deviceID := uuid.MustParse("00000000-0000-0000-0000-000000000731")
	insertCanvasMapRepoTestDevice(t, db, deviceID)
	initial := domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{
			{DeviceID: deviceID, Role: domain.CanvasMapDeviceRoleBase},
		},
	}
	if err := repo.ReplaceMembership(firstMap.ID, initial); err != nil {
		t.Fatalf("replace first membership: %v", err)
	}
	if err := repo.ReplaceMembership(secondMap.ID, initial); err != nil {
		t.Fatalf("replace second membership: %v", err)
	}

	color := "#123ABC"
	if err := repo.UpdateDeviceVisualColor(firstMap.ID, deviceID, &color); err != nil {
		t.Fatalf("update first map visual color: %v", err)
	}

	firstMembership, err := repo.GetMembership(firstMap.ID)
	if err != nil {
		t.Fatalf("get first membership: %v", err)
	}
	if len(firstMembership.Devices) != 1 ||
		firstMembership.Devices[0].VisualColor == nil ||
		*firstMembership.Devices[0].VisualColor != color {
		t.Fatalf("first map visual color = %#v, want %s", firstMembership.Devices, color)
	}

	secondMembership, err := repo.GetMembership(secondMap.ID)
	if err != nil {
		t.Fatalf("get second membership: %v", err)
	}
	if len(secondMembership.Devices) != 1 || secondMembership.Devices[0].VisualColor != nil {
		t.Fatalf("second map visual color = %#v, want nil", secondMembership.Devices)
	}

	var tagsJSON string
	if err := db.QueryRow(`SELECT tags_json FROM devices WHERE id = ?`, deviceID.String()).Scan(&tagsJSON); err != nil {
		t.Fatalf("query global device tags: %v", err)
	}
	if tagsJSON != "{}" {
		t.Fatalf("global device tags_json = %s, want unchanged empty object", tagsJSON)
	}

	if err := repo.UpdateDeviceVisualColor(firstMap.ID, deviceID, nil); err != nil {
		t.Fatalf("clear first map visual color: %v", err)
	}
	firstMembership, err = repo.GetMembership(firstMap.ID)
	if err != nil {
		t.Fatalf("get cleared first membership: %v", err)
	}
	if len(firstMembership.Devices) != 1 || firstMembership.Devices[0].VisualColor != nil {
		t.Fatalf("cleared first map visual color = %#v, want nil", firstMembership.Devices)
	}
}

func TestCanvasMapRepoDuplicateCopiesMembershipAndPositions(t *testing.T) {
	db := openCanvasMapRepoTestDB(t)
	mapRepo := NewCanvasMapRepo(db)
	positionRepo := NewCanvasMapPositionRepo(db)

	source, err := mapRepo.Create(domain.CanvasMapCreate{Name: "Source Materialized"})
	if err != nil {
		t.Fatalf("create source map: %v", err)
	}
	deviceID := uuid.MustParse("00000000-0000-0000-0000-000000000611")
	ghostDeviceID := uuid.MustParse("00000000-0000-0000-0000-000000000612")
	insertCanvasMapRepoTestDevice(t, db, deviceID)
	insertCanvasMapRepoTestDevice(t, db, ghostDeviceID)

	linkID := uuid.MustParse("00000000-0000-0000-0000-000000000613")
	insertCanvasMapRepoTestLink(t, db, linkID, deviceID, ghostDeviceID)
	visualColor := "#7C4DFF"

	membership := domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{
			{DeviceID: deviceID, Role: domain.CanvasMapDeviceRoleBase, VisualColor: &visualColor},
			{DeviceID: ghostDeviceID, Role: domain.CanvasMapDeviceRoleGhost},
		},
		LinkIDs: []uuid.UUID{linkID},
		Areas: []domain.CanvasMapAreaMembership{
			{
				AreaID:      uuid.MustParse("00000000-0000-0000-0000-000000000614"),
				Name:        "Source Area",
				Description: "copied map-local area",
				Color:       "#abcdef",
			},
		},
	}
	if err := mapRepo.ReplaceMembership(source.ID, membership); err != nil {
		t.Fatalf("replace source membership: %v", err)
	}
	if err := positionRepo.SaveAllForMap(source.ID, []domain.DevicePosition{{DeviceID: deviceID, X: 10, Y: 20, Pinned: true}}); err != nil {
		t.Fatalf("save source positions: %v", err)
	}

	copy, err := mapRepo.Duplicate(source.ID, "Copied Materialized")
	if err != nil {
		t.Fatalf("duplicate map: %v", err)
	}

	gotMembership, err := mapRepo.GetMembership(copy.ID)
	if err != nil {
		t.Fatalf("get copy membership: %v", err)
	}
	assertCanvasMapMembershipEqual(t, gotMembership, membership)

	positions, err := positionRepo.GetAllForMap(copy.ID)
	if err != nil {
		t.Fatalf("get copy positions: %v", err)
	}
	if len(positions) != 1 || positions[0].DeviceID != deviceID || positions[0].X != 10 || positions[0].Y != 20 || !positions[0].Pinned {
		t.Fatalf("unexpected copied positions: %#v", positions)
	}

	copyVisualColor := "#00AEEF"
	if err := mapRepo.UpdateDeviceVisualColor(copy.ID, deviceID, &copyVisualColor); err != nil {
		t.Fatalf("update copy visual color: %v", err)
	}
	sourceMembership, err := mapRepo.GetMembership(source.ID)
	if err != nil {
		t.Fatalf("get source membership after copy edit: %v", err)
	}
	if sourceMembership.Devices[0].VisualColor == nil || *sourceMembership.Devices[0].VisualColor != visualColor {
		t.Fatalf("source visual color = %#v, want %s", sourceMembership.Devices[0].VisualColor, visualColor)
	}
}

func TestCanvasMapRepoCreateAndUpdateCanonicalizeFilterJSON(t *testing.T) {
	db := openCanvasMapRepoTestDB(t)
	repo := NewCanvasMapRepo(db)
	firstDeviceID := uuid.MustParse("00000000-0000-0000-0000-000000000401")
	secondDeviceID := uuid.MustParse("00000000-0000-0000-0000-000000000402")

	createFilter := domain.CanvasMapFilter{
		DeviceIDs:             []uuid.UUID{secondDeviceID, firstDeviceID, secondDeviceID},
		IncludeCrossAreaLinks: true,
		Tags:                  map[string]string{"site": "milano"},
	}
	wantCreateFilterJSON, err := domain.CanonicalCanvasMapFilterJSON(createFilter)
	if err != nil {
		t.Fatalf("canonical create filter: %v", err)
	}

	created, err := repo.Create(domain.CanvasMapCreate{Name: "Filtered", Filter: createFilter})
	if err != nil {
		t.Fatalf("create filtered map: %v", err)
	}
	if created.FilterJSON != wantCreateFilterJSON {
		t.Fatalf("created filter_json = %s, want %s", created.FilterJSON, wantCreateFilterJSON)
	}

	var storedFilterJSON string
	if err := db.QueryRow(`SELECT filter_json FROM canvas_maps WHERE id = ?`, created.ID.String()).Scan(&storedFilterJSON); err != nil {
		t.Fatalf("query stored create filter: %v", err)
	}
	if storedFilterJSON != wantCreateFilterJSON {
		t.Fatalf("stored create filter_json = %s, want %s", storedFilterJSON, wantCreateFilterJSON)
	}

	updatedName := "Filtered Updated"
	updatedDescription := "Updated filter map"
	updateFilter := domain.CanvasMapFilter{
		DeviceIDs:           []uuid.UUID{secondDeviceID, firstDeviceID, firstDeviceID},
		IncludeGhostDevices: true,
	}
	wantUpdateFilterJSON, err := domain.CanonicalCanvasMapFilterJSON(updateFilter)
	if err != nil {
		t.Fatalf("canonical update filter: %v", err)
	}

	updated, err := repo.Update(created.ID, domain.CanvasMapUpdate{
		Name:        &updatedName,
		Description: &updatedDescription,
		Filter:      &updateFilter,
	})
	if err != nil {
		t.Fatalf("update filtered map: %v", err)
	}
	if updated.Name != updatedName || updated.Description != updatedDescription || updated.FilterJSON != wantUpdateFilterJSON {
		t.Fatalf("unexpected updated map: %#v, want filter_json %s", updated, wantUpdateFilterJSON)
	}

	if err := db.QueryRow(`SELECT filter_json FROM canvas_maps WHERE id = ?`, created.ID.String()).Scan(&storedFilterJSON); err != nil {
		t.Fatalf("query stored update filter: %v", err)
	}
	if storedFilterJSON != wantUpdateFilterJSON {
		t.Fatalf("stored update filter_json = %s, want %s", storedFilterJSON, wantUpdateFilterJSON)
	}
}

func TestCanvasMapRepoUpdatePersistsSourceAreaID(t *testing.T) {
	db := openCanvasMapRepoTestDB(t)
	repo := NewCanvasMapRepo(db)

	areaID := uuid.New()
	if _, err := db.Exec(
		`INSERT INTO areas (id, name, description, color, created_at, updated_at)
		 VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
		areaID.String(),
		"Source Area",
		"source area",
		"#2979FF",
	); err != nil {
		t.Fatalf("insert area: %v", err)
	}

	canvasMap, err := repo.Create(domain.CanvasMapCreate{Name: "Area Update"})
	if err != nil {
		t.Fatalf("create map: %v", err)
	}
	updated, err := repo.Update(canvasMap.ID, domain.CanvasMapUpdate{
		SourceAreaID:    &areaID,
		SourceAreaIDSet: true,
	})
	if err != nil {
		t.Fatalf("update source area: %v", err)
	}
	if updated.SourceAreaID == nil || *updated.SourceAreaID != areaID {
		t.Fatalf("updated source area = %#v, want %s", updated.SourceAreaID, areaID)
	}

	renamed := "Area Update Renamed"
	updated, err = repo.Update(canvasMap.ID, domain.CanvasMapUpdate{Name: &renamed})
	if err != nil {
		t.Fatalf("update name without source area: %v", err)
	}
	if updated.Name != renamed {
		t.Fatalf("updated name = %q, want %q", updated.Name, renamed)
	}
	if updated.SourceAreaID == nil || *updated.SourceAreaID != areaID {
		t.Fatalf("source area after name-only update = %#v, want %s", updated.SourceAreaID, areaID)
	}
}

func TestCanvasMapRepoDeleteRejectsDefaultMap(t *testing.T) {
	db := openCanvasMapRepoTestDB(t)
	repo := NewCanvasMapRepo(db)

	defaultMap, err := repo.GetDefault()
	if err != nil {
		t.Fatalf("get default map: %v", err)
	}
	if err := repo.Delete(defaultMap.ID); err == nil {
		t.Fatal("expected deleting default map to fail")
	}

	maps, err := repo.List()
	if err != nil {
		t.Fatalf("list maps after failed delete: %v", err)
	}
	if len(maps) != 1 || !maps[0].IsDefault {
		t.Fatalf("default map missing after failed delete: %#v", maps)
	}
}

func TestCanvasMapRepoSetPrimaryMovesDefaultFlag(t *testing.T) {
	db := openCanvasMapRepoTestDB(t)
	repo := NewCanvasMapRepo(db)

	oldDefault, err := repo.GetDefault()
	if err != nil {
		t.Fatalf("get default map: %v", err)
	}
	branch, err := repo.Create(domain.CanvasMapCreate{Name: "Branch"})
	if err != nil {
		t.Fatalf("create branch map: %v", err)
	}

	primary, err := repo.SetPrimary(branch.ID)
	if err != nil {
		t.Fatalf("set primary map: %v", err)
	}
	if primary.ID != branch.ID || !primary.IsDefault {
		t.Fatalf("primary map = %#v, want branch map marked default", primary)
	}

	currentDefault, err := repo.GetDefault()
	if err != nil {
		t.Fatalf("get promoted default map: %v", err)
	}
	if currentDefault.ID != branch.ID {
		t.Fatalf("default map id = %s, want %s", currentDefault.ID, branch.ID)
	}

	reloadedOldDefault, err := repo.GetByID(oldDefault.ID)
	if err != nil {
		t.Fatalf("reload old default map: %v", err)
	}
	if reloadedOldDefault.IsDefault {
		t.Fatalf("old default still marked default: %#v", reloadedOldDefault)
	}
	if err := repo.Delete(oldDefault.ID); err != nil {
		t.Fatalf("delete old default after primary move: %v", err)
	}
}

func TestCanvasMapPositionRepoRejectsNilIdentifiers(t *testing.T) {
	db := openCanvasMapRepoTestDB(t)
	mapRepo := NewCanvasMapRepo(db)
	positionRepo := NewCanvasMapPositionRepo(db)

	canvasMap, err := mapRepo.Create(domain.CanvasMapCreate{Name: "Validation"})
	if err != nil {
		t.Fatalf("create map: %v", err)
	}

	if err := positionRepo.SaveAllForMap(uuid.Nil, []domain.DevicePosition{{DeviceID: uuid.New(), X: 1, Y: 2}}); err == nil {
		t.Fatal("expected nil map id to fail")
	}
	if err := positionRepo.SaveAllForMap(canvasMap.ID, []domain.DevicePosition{{DeviceID: uuid.Nil, X: 1, Y: 2}}); err == nil {
		t.Fatal("expected nil device id to fail")
	}
}

func TestCanvasMapPositionRepoSaveAllForMapUpsertsExistingCoordinates(t *testing.T) {
	db := openCanvasMapRepoTestDB(t)
	mapRepo := NewCanvasMapRepo(db)
	positionRepo := NewCanvasMapPositionRepo(db)

	canvasMap, err := mapRepo.Create(domain.CanvasMapCreate{Name: "Upsert"})
	if err != nil {
		t.Fatalf("create map: %v", err)
	}
	deviceID := uuid.MustParse("00000000-0000-0000-0000-000000000501")
	insertCanvasMapRepoTestDevice(t, db, deviceID)

	if err := positionRepo.SaveAllForMap(canvasMap.ID, []domain.DevicePosition{{DeviceID: deviceID, X: 10, Y: 20, Pinned: false}}); err != nil {
		t.Fatalf("save first position: %v", err)
	}
	if err := positionRepo.SaveAllForMap(canvasMap.ID, []domain.DevicePosition{{DeviceID: deviceID, X: 30, Y: 40, Pinned: true}}); err != nil {
		t.Fatalf("save updated position: %v", err)
	}

	positions, err := positionRepo.GetAllForMap(canvasMap.ID)
	if err != nil {
		t.Fatalf("get positions: %v", err)
	}
	if len(positions) != 1 {
		t.Fatalf("positions count = %d, want 1", len(positions))
	}
	got := positions[0]
	if got.DeviceID != deviceID || got.X != 30 || got.Y != 40 || !got.Pinned {
		t.Fatalf("updated position = %#v, want device %s at (30, 40) pinned", got, deviceID)
	}
}

func TestCanvasMapPositionRepoRejectsPositionsForNonMemberDevices(t *testing.T) {
	db := openCanvasMapRepoTestDB(t)
	mapRepo := NewCanvasMapRepo(db)
	positionRepo := NewCanvasMapPositionRepo(db)

	legacyMap, err := mapRepo.Create(domain.CanvasMapCreate{Name: "Legacy Positions"})
	if err != nil {
		t.Fatalf("create legacy map: %v", err)
	}
	memberID := uuid.MustParse("00000000-0000-0000-0000-000000000621")
	nonMemberID := uuid.MustParse("00000000-0000-0000-0000-000000000622")
	insertCanvasMapRepoTestDevice(t, db, memberID)
	insertCanvasMapRepoTestDevice(t, db, nonMemberID)

	if err := positionRepo.SaveAllForMap(legacyMap.ID, []domain.DevicePosition{{DeviceID: nonMemberID, X: 1, Y: 2}}); err != nil {
		t.Fatalf("save position before membership exists: %v", err)
	}

	materializedMap, err := mapRepo.Create(domain.CanvasMapCreate{Name: "Materialized Positions"})
	if err != nil {
		t.Fatalf("create materialized map: %v", err)
	}
	if err := mapRepo.ReplaceMembership(materializedMap.ID, domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{{DeviceID: memberID, Role: domain.CanvasMapDeviceRoleBase}},
	}); err != nil {
		t.Fatalf("replace materialized membership: %v", err)
	}

	if err := positionRepo.SaveAllForMap(materializedMap.ID, []domain.DevicePosition{{DeviceID: nonMemberID, X: 3, Y: 4}}); err == nil {
		t.Fatal("expected non-member device position to fail")
	}
	if err := positionRepo.SaveAllForMap(materializedMap.ID, []domain.DevicePosition{{DeviceID: memberID, X: 5, Y: 6, Pinned: true}}); err != nil {
		t.Fatalf("save member position: %v", err)
	}
}

func TestCanvasMapRepoReplaceMembershipPrunesNonMemberPositions(t *testing.T) {
	db := openCanvasMapRepoTestDB(t)
	mapRepo := NewCanvasMapRepo(db)
	positionRepo := NewCanvasMapPositionRepo(db)

	canvasMap, err := mapRepo.Create(domain.CanvasMapCreate{Name: "Pruned Positions"})
	if err != nil {
		t.Fatalf("create map: %v", err)
	}
	memberID := uuid.MustParse("00000000-0000-0000-0000-000000000631")
	removedID := uuid.MustParse("00000000-0000-0000-0000-000000000632")
	insertCanvasMapRepoTestDevice(t, db, memberID)
	insertCanvasMapRepoTestDevice(t, db, removedID)

	if err := mapRepo.ReplaceMembership(canvasMap.ID, domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{
			{DeviceID: memberID, Role: domain.CanvasMapDeviceRoleBase},
			{DeviceID: removedID, Role: domain.CanvasMapDeviceRoleBase},
		},
	}); err != nil {
		t.Fatalf("replace initial membership: %v", err)
	}
	if err := positionRepo.SaveAllForMap(canvasMap.ID, []domain.DevicePosition{
		{DeviceID: memberID, X: 1, Y: 2},
		{DeviceID: removedID, X: 3, Y: 4},
	}); err != nil {
		t.Fatalf("save positions: %v", err)
	}

	if err := mapRepo.ReplaceMembership(canvasMap.ID, domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{{DeviceID: memberID, Role: domain.CanvasMapDeviceRoleBase}},
	}); err != nil {
		t.Fatalf("replace pruned membership: %v", err)
	}

	positions, err := positionRepo.GetAllForMap(canvasMap.ID)
	if err != nil {
		t.Fatalf("get positions: %v", err)
	}
	if len(positions) != 1 || positions[0].DeviceID != memberID {
		t.Fatalf("positions after pruning = %#v, want only %s", positions, memberID)
	}
}

func assertCanvasMapMembershipEqual(t *testing.T, got, want domain.CanvasMapMembership) {
	t.Helper()

	if len(got.Devices) != len(want.Devices) {
		t.Fatalf("device membership count = %d, want %d: %#v", len(got.Devices), len(want.Devices), got.Devices)
	}
	for i := range got.Devices {
		if got.Devices[i].DeviceID != want.Devices[i].DeviceID ||
			got.Devices[i].Role != want.Devices[i].Role ||
			!optionalStringEqual(got.Devices[i].VisualColor, want.Devices[i].VisualColor) ||
			!uuidSlicesEqual(got.Devices[i].AreaIDs, want.Devices[i].AreaIDs) {
			t.Fatalf("device membership[%d] = %#v, want %#v", i, got.Devices[i], want.Devices[i])
		}
	}

	if len(got.LinkIDs) != len(want.LinkIDs) {
		t.Fatalf("link membership count = %d, want %d: %#v", len(got.LinkIDs), len(want.LinkIDs), got.LinkIDs)
	}
	for i := range got.LinkIDs {
		if got.LinkIDs[i] != want.LinkIDs[i] {
			t.Fatalf("link membership[%d] = %s, want %s", i, got.LinkIDs[i], want.LinkIDs[i])
		}
	}

	if len(got.Areas) != len(want.Areas) {
		t.Fatalf("area membership count = %d, want %d: %#v", len(got.Areas), len(want.Areas), got.Areas)
	}
	for i := range got.Areas {
		if got.Areas[i] != want.Areas[i] {
			t.Fatalf("area membership[%d] = %#v, want %#v", i, got.Areas[i], want.Areas[i])
		}
	}
}

func optionalStringEqual(got, want *string) bool {
	if got == nil || want == nil {
		return got == want
	}
	return *got == *want
}

func uuidSlicesEqual(got, want []uuid.UUID) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func insertCanvasMapRepoTestDevice(t *testing.T, db *sql.DB, id uuid.UUID) {
	t.Helper()

	suffix := id.String()[len(id.String())-3:]
	if _, err := db.Exec(
		`INSERT INTO devices (id, hostname, ip, device_type, status, sys_name, sys_descr, sys_object_id, hardware_model, vendor, managed, tags_json, metrics_source, prometheus_label_name, prometheus_label_value, created_at, updated_at)
		 VALUES (?, ?, ?, 'router', 'up', ?, '', '', '', 'default', 1, '{}', 'none', '', '', datetime('now'), datetime('now'))`,
		id.String(),
		"router-"+suffix,
		"10.0.9."+suffix[1:],
		"router-"+suffix,
	); err != nil {
		t.Fatalf("insert device %s: %v", id, err)
	}
}

func insertCanvasMapRepoTestArea(t *testing.T, db *sql.DB, id uuid.UUID, name string, color string) {
	t.Helper()

	if _, err := db.Exec(
		`INSERT INTO areas (id, name, description, color, created_at, updated_at)
		 VALUES (?, ?, '', ?, datetime('now'), datetime('now'))`,
		id.String(),
		name,
		color,
	); err != nil {
		t.Fatalf("insert area %s: %v", id, err)
	}
}

func insertCanvasMapRepoTestDeviceArea(t *testing.T, db *sql.DB, deviceID uuid.UUID, areaID uuid.UUID) {
	t.Helper()

	if _, err := db.Exec(
		`INSERT INTO device_areas (device_id, area_id) VALUES (?, ?)`,
		deviceID.String(),
		areaID.String(),
	); err != nil {
		t.Fatalf("insert device area %s/%s: %v", deviceID, areaID, err)
	}
}

func insertCanvasMapRepoTestLink(t *testing.T, db *sql.DB, id, sourceDeviceID, targetDeviceID uuid.UUID) {
	t.Helper()

	if _, err := db.Exec(
		`INSERT INTO links (id, source_device_id, source_if_name, target_device_id, target_if_name, discovery_protocol, created_at, updated_at)
		 VALUES (?, ?, 'ether1', ?, 'ether2', 'manual', datetime('now'), datetime('now'))`,
		id.String(),
		sourceDeviceID.String(),
		targetDeviceID.String(),
	); err != nil {
		t.Fatalf("insert link %s: %v", id, err)
	}
}
