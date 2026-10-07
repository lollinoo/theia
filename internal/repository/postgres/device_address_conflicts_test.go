package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestDeviceAddressWritersShareAtomicConflictChecks(t *testing.T) {
	for _, writer := range []string{"create", "update", "configuration", "replace", "import"} {
		t.Run(writer, func(t *testing.T) {
			db := setupTestDB(t)
			firstRepo, secondRepo := NewDeviceRepo(db, testKeyring, nil), NewDeviceRepo(db, testKeyring, nil)
			first := newDeviceImportTestDevice("first.example.net")
			second := newDeviceImportTestDevice("second.example.net")
			if writer != "create" && writer != "import" {
				if err := secondRepo.Create(second); err != nil {
					t.Fatal(err)
				}
			}
			first.Addresses = append(first.Addresses, domain.DeviceAddress{Address: "shared.example.net", Role: domain.DeviceAddressRoleBackup})
			second.Addresses = append(second.Addresses, domain.DeviceAddress{Address: " SHARED.EXAMPLE.NET ", Role: domain.DeviceAddressRoleManagement})
			mapID := uuid.New()
			if writer == "import" {
				insertDeviceImportTestMap(t, db, mapID)
			}
			key := deviceImportTestAddressLockKey("shared.example.net")
			release := holdDeviceImportAdvisoryLock(t, db, key)
			type result struct {
				first bool
				err   error
			}
			results := make(chan result, 2)
			go func() { results <- result{true, firstRepo.Create(first)} }()
			go func() {
				var err error
				switch writer {
				case "create":
					err = secondRepo.Create(second)
				case "update":
					err = secondRepo.Update(second)
				case "configuration":
					err = secondRepo.UpdateConfiguration(second, domain.DeviceConfigurationFields{IP: true, Addresses: true})
				case "replace":
					err = secondRepo.ReplaceDeviceAddresses(second.ID, second.Addresses)
				case "import":
					err = NewDeviceImportStore(secondRepo).CreateDeviceInMap(context.Background(), second, domain.DeviceImportPlacement{MapID: mapID})
				}
				results <- result{false, err}
			}()
			waitForDeviceImportAdvisoryWaiters(t, db, key, 2)
			release()
			successes := 0
			for range 2 {
				select {
				case got := <-results:
					if got.err == nil {
						successes++
						continue
					}
					if !errors.Is(got.err, domain.ErrDeviceAddressConflict) && !errors.Is(got.err, domain.ErrDeviceImportAddressConflict) {
						t.Fatalf("unexpected write error: %v", got.err)
					}
					if got.first {
						assertDeviceImportTargetAbsent(t, db, first.ID)
					} else if writer == "create" || writer == "import" {
						assertDeviceImportTargetAbsent(t, db, second.ID)
					} else {
						if count := importTestCount(t, db, "SELECT COUNT(*) FROM device_addresses WHERE device_id=$1 AND normalized_address='second.example.net'", second.ID); count != 1 {
							t.Fatal("failed update lost its original address")
						}
					}
				case <-time.After(5 * time.Second):
					t.Fatal("address writers did not finish")
				}
			}
			if successes != 1 || importTestCount(t, db, "SELECT COUNT(*) FROM device_addresses WHERE normalized_address='shared.example.net'") != 1 {
				t.Fatalf("successes=%d, want exactly one committed address owner", successes)
			}
		})
	}
}

func TestDeviceAddressConflictsPreserveVirtualRulesAndTypeChanges(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)
	physical := newDeviceImportTestDevice("physical.example.net")
	physical.Addresses = append(physical.Addresses, domain.DeviceAddress{Address: "shared.example.net"})
	if err := repo.Create(physical); err != nil {
		t.Fatal(err)
	}
	virtual := newDeviceImportTestDevice("virtual.example.net")
	virtual.DeviceType = domain.DeviceTypeVirtual
	virtual.Addresses = append(virtual.Addresses, domain.DeviceAddress{Address: "shared.example.net"})
	if err := repo.Create(virtual); err != nil {
		t.Fatalf("virtual secondary sharing was rejected: %v", err)
	}
	clone := newDeviceImportTestDevice("virtual.example.net")
	clone.DeviceType = domain.DeviceTypeVirtual
	if err := repo.Create(clone); err != nil {
		t.Fatalf("virtual primary sharing was rejected: %v", err)
	}
	virtual.DeviceType = domain.DeviceTypeRouter
	if err := repo.UpdateStaticDiscovery(virtual); !errors.Is(err, domain.ErrDeviceAddressConflict) {
		t.Fatalf("conversion error=%v, want address conflict", err)
	}
	stored, err := repo.GetByID(virtual.ID)
	if err != nil || stored.DeviceType != domain.DeviceTypeVirtual {
		t.Fatalf("failed conversion changed the owner: device=%v error=%v", stored, err)
	}
	if err := repo.UpdateConfiguration(physical, domain.DeviceConfigurationFields{Addresses: true}); err != nil {
		t.Fatalf("updating an owner's existing addresses was rejected: %v", err)
	}
}

func TestDeviceAddressLockHonorsImportCancellation(t *testing.T) {
	db := setupTestDB(t)
	mapID := uuid.New()
	insertDeviceImportTestMap(t, db, mapID)
	device := newDeviceImportTestDevice("cancelled.example.net")
	key := deviceImportTestAddressLockKey(device.IP)
	release := holdDeviceImportAdvisoryLock(t, db, key)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- NewDeviceImportStore(NewDeviceRepo(db, testKeyring, nil)).CreateDeviceInMap(ctx, device, domain.DeviceImportPlacement{MapID: mapID})
	}()
	waitForDeviceImportAdvisoryWaiters(t, db, key, 1)
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, domain.ErrDeviceImportStoreUnavailable) {
			t.Fatalf("cancelled import error=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled import stayed blocked on an address lock")
	}
	release()
	assertDeviceImportTargetAbsent(t, db, device.ID)
}
