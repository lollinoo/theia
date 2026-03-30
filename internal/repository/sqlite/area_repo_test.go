package sqlite

import (
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestAreaRepo_CreateAndGetByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAreaRepo(db)

	area := &domain.Area{
		Name:        "Backbone",
		Description: "Core routers",
		Color:       "#2979FF",
	}

	if err := repo.Create(area); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(area.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Backbone" {
		t.Errorf("Name = %q, want %q", got.Name, "Backbone")
	}
	if got.Color != "#2979FF" {
		t.Errorf("Color = %q, want %q", got.Color, "#2979FF")
	}
}

func TestAreaRepo_GetAllWithDeviceCount(t *testing.T) {
	db := setupTestDB(t)
	areaRepo := NewAreaRepo(db)
	deviceRepo := NewDeviceRepo(db, testKey, nil)

	area := &domain.Area{Name: "Edge", Color: "#00E676"}
	if err := areaRepo.Create(area); err != nil {
		t.Fatalf("Create area: %v", err)
	}

	// Create a device assigned to this area
	dev := &domain.Device{
		Hostname: "edge-sw-01",
		IP:       "10.0.0.1",
		AreaIDs:  []uuid.UUID{area.ID},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
		DeviceType: domain.DeviceTypeSwitch,
		Status:     domain.DeviceStatusUp,
		Managed:    true,
	}
	if err := deviceRepo.Create(dev); err != nil {
		t.Fatalf("Create device: %v", err)
	}

	areas, err := areaRepo.GetAllWithDeviceCount()
	if err != nil {
		t.Fatalf("GetAllWithDeviceCount: %v", err)
	}
	if len(areas) != 1 {
		t.Fatalf("expected 1 area, got %d", len(areas))
	}
	if areas[0].DeviceCount != 1 {
		t.Errorf("DeviceCount = %d, want 1", areas[0].DeviceCount)
	}
}

func TestAreaRepo_UniqueNameConstraint(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAreaRepo(db)

	a1 := &domain.Area{Name: "Backbone", Color: "#00E676"}
	if err := repo.Create(a1); err != nil {
		t.Fatalf("Create first: %v", err)
	}

	a2 := &domain.Area{Name: "Backbone", Color: "#2979FF"}
	err := repo.Create(a2)
	if err == nil {
		t.Fatal("expected error for duplicate name, got nil")
	}
}

func TestAreaRepo_DeleteSetsDeviceAreaIDToNull(t *testing.T) {
	// AREA-03: ON DELETE SET NULL -- deleting an area should set device.area_id to NULL
	db := setupTestDB(t)
	areaRepo := NewAreaRepo(db)
	deviceRepo := NewDeviceRepo(db, testKey, nil)

	area := &domain.Area{Name: "ToDelete", Color: "#FF1744"}
	if err := areaRepo.Create(area); err != nil {
		t.Fatalf("Create area: %v", err)
	}

	dev := &domain.Device{
		Hostname: "orphan-sw-01",
		IP:       "10.0.1.1",
		AreaIDs:  []uuid.UUID{area.ID},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
		DeviceType: domain.DeviceTypeSwitch,
		Status:     domain.DeviceStatusUp,
		Managed:    true,
	}
	if err := deviceRepo.Create(dev); err != nil {
		t.Fatalf("Create device: %v", err)
	}

	// Delete the area
	if err := areaRepo.Delete(area.ID); err != nil {
		t.Fatalf("Delete area: %v", err)
	}

	// Verify device's area_ids no longer contains the deleted area
	got, err := deviceRepo.GetByID(dev.ID)
	if err != nil {
		t.Fatalf("GetByID device: %v", err)
	}
	if len(got.AreaIDs) != 0 {
		t.Errorf("expected AreaIDs to be empty after area deletion, got %v", got.AreaIDs)
	}
}

func TestAreaRepo_UpdateAndDelete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAreaRepo(db)

	area := &domain.Area{Name: "Original", Description: "desc", Color: "#00E676"}
	if err := repo.Create(area); err != nil {
		t.Fatalf("Create: %v", err)
	}

	area.Name = "Renamed"
	area.Color = "#FF6D00"
	if err := repo.Update(area); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(area.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if got.Name != "Renamed" {
		t.Errorf("Name = %q, want %q", got.Name, "Renamed")
	}
	if got.Color != "#FF6D00" {
		t.Errorf("Color = %q, want %q", got.Color, "#FF6D00")
	}

	if err := repo.Delete(area.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = repo.GetByID(area.ID)
	if err == nil {
		t.Fatal("expected error after delete, got nil")
	}
}

func TestAreaRepo_GetAll_OrderByName(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAreaRepo(db)

	// Create in reverse alphabetical order
	for _, name := range []string{"Zeta", "Alpha", "Mid"} {
		a := &domain.Area{Name: name, Color: "#00E676"}
		if err := repo.Create(a); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
	}

	areas, err := repo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(areas) != 3 {
		t.Fatalf("expected 3 areas, got %d", len(areas))
	}
	if areas[0].Name != "Alpha" || areas[1].Name != "Mid" || areas[2].Name != "Zeta" {
		t.Errorf("order = [%s, %s, %s], want [Alpha, Mid, Zeta]", areas[0].Name, areas[1].Name, areas[2].Name)
	}
}
