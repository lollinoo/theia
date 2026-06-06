package postgres

// This file exercises position repo behavior so refactors preserve the documented contract.

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestPositionRepo_GetAllEmpty(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPositionRepo(db)

	positions, err := repo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}

	if positions == nil {
		t.Fatal("expected empty slice, got nil")
	}
	if len(positions) != 0 {
		t.Fatalf("positions count = %d, want 0", len(positions))
	}
}

func TestPositionRepo_SaveAllAndGetAll(t *testing.T) {
	db := setupTestDB(t)
	deviceIDs := createTestDevices(t, db, 3)
	repo := NewPositionRepo(db)

	input := []domain.DevicePosition{
		{DeviceID: deviceIDs[0], X: 100.5, Y: 200.25, Pinned: false},
		{DeviceID: deviceIDs[1], X: 320, Y: 180, Pinned: true},
		{DeviceID: deviceIDs[2], X: -40, Y: 512.75, Pinned: false},
	}

	if err := repo.SaveAll(input); err != nil {
		t.Fatalf("SaveAll: %v", err)
	}

	positions, err := repo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}

	if len(positions) != 3 {
		t.Fatalf("positions count = %d, want 3", len(positions))
	}

	byDevice := map[uuid.UUID]domain.DevicePosition{}
	for _, position := range positions {
		byDevice[position.DeviceID] = position
	}

	if got := byDevice[deviceIDs[0]]; got.X != 100.5 || got.Y != 200.25 || got.Pinned {
		t.Fatalf("position[0] = %+v, want x=100.5 y=200.25 pinned=false", got)
	}
	if got := byDevice[deviceIDs[1]]; got.X != 320 || got.Y != 180 || !got.Pinned {
		t.Fatalf("position[1] = %+v, want x=320 y=180 pinned=true", got)
	}
	if got := byDevice[deviceIDs[2]]; got.X != -40 || got.Y != 512.75 || got.Pinned {
		t.Fatalf("position[2] = %+v, want x=-40 y=512.75 pinned=false", got)
	}
}

func TestPositionRepo_SaveAllUpsert(t *testing.T) {
	db := setupTestDB(t)
	deviceIDs := createTestDevices(t, db, 1)
	repo := NewPositionRepo(db)

	first := []domain.DevicePosition{
		{DeviceID: deviceIDs[0], X: 10, Y: 20, Pinned: false},
	}
	if err := repo.SaveAll(first); err != nil {
		t.Fatalf("SaveAll first: %v", err)
	}

	second := []domain.DevicePosition{
		{DeviceID: deviceIDs[0], X: 300, Y: 400, Pinned: true},
	}
	if err := repo.SaveAll(second); err != nil {
		t.Fatalf("SaveAll second: %v", err)
	}

	positions, err := repo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(positions) != 1 {
		t.Fatalf("positions count = %d, want 1", len(positions))
	}

	got := positions[0]
	if got.X != 300 || got.Y != 400 {
		t.Fatalf("updated position = (%v, %v), want (300, 400)", got.X, got.Y)
	}
	if !got.Pinned {
		t.Fatal("expected pinned=true after upsert")
	}
}

func TestPositionRepo_DeleteByDeviceID(t *testing.T) {
	db := setupTestDB(t)
	deviceIDs := createTestDevices(t, db, 2)
	repo := NewPositionRepo(db)

	if err := repo.SaveAll([]domain.DevicePosition{
		{DeviceID: deviceIDs[0], X: 10, Y: 20, Pinned: true},
		{DeviceID: deviceIDs[1], X: 30, Y: 40, Pinned: false},
	}); err != nil {
		t.Fatalf("SaveAll: %v", err)
	}

	if err := repo.DeleteByDeviceID(deviceIDs[0]); err != nil {
		t.Fatalf("DeleteByDeviceID: %v", err)
	}

	positions, err := repo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(positions) != 1 {
		t.Fatalf("positions count = %d, want 1", len(positions))
	}
	if positions[0].DeviceID != deviceIDs[1] {
		t.Fatalf("remaining device = %s, want %s", positions[0].DeviceID, deviceIDs[1])
	}
}

func createTestDevices(t *testing.T, db *sql.DB, count int) []uuid.UUID {
	t.Helper()

	ids := make([]uuid.UUID, 0, count)
	for i := 0; i < count; i++ {
		id := uuid.New()
		ids = append(ids, id)

		if _, err := db.Exec(
			`INSERT INTO devices (
				id, hostname, ip, snmp_credentials_json, device_type, status,
				sys_name, sys_descr, sys_object_id, hardware_model, managed,
				tags_json, created_at, updated_at
			) VALUES (?, ?, ?, '{}', 'unknown', 'unknown', '', '', '', '', 1, '{}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			id.String(),
			"device-"+id.String()[:8],
			fmt.Sprintf("10.0.0.%d", i+1),
		); err != nil {
			t.Fatalf("inserting test device %d: %v", i, err)
		}
	}

	return ids
}
