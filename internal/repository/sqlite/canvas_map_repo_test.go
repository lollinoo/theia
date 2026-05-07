package sqlite

import (
	"database/sql"
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

func TestPrimaryDataCopySpecsIncludeCanvasMaps(t *testing.T) {
	required := map[string]bool{
		"canvas_maps":          false,
		"canvas_map_positions": false,
	}
	for _, spec := range primaryDataCopySpecs {
		if _, ok := required[spec.name]; ok {
			required[spec.name] = true
		}
	}
	for name, found := range required {
		if !found {
			t.Fatalf("expected primaryDataCopySpecs to include %s", name)
		}
	}
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
