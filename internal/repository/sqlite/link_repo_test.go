package sqlite

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	_ "github.com/mattn/go-sqlite3"
)

func createTestDevicePair(t *testing.T, repo *DeviceRepo) (uuid.UUID, uuid.UUID) {
	t.Helper()
	d1 := &domain.Device{
		IP:       "10.1.0.1",
		Hostname: "device-A",
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
		DeviceType: domain.DeviceTypeRouter,
		Status:     domain.DeviceStatusUp,
		Managed:    true,
	}
	d2 := &domain.Device{
		IP:       "10.1.0.2",
		Hostname: "device-B",
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
		DeviceType: domain.DeviceTypeSwitch,
		Status:     domain.DeviceStatusUp,
		Managed:    true,
	}

	if err := repo.Create(d1); err != nil {
		t.Fatalf("Create device A: %v", err)
	}
	if err := repo.Create(d2); err != nil {
		t.Fatalf("Create device B: %v", err)
	}

	return d1.ID, d2.ID
}

func TestLinkRepo_CreateAndGetByDeviceID(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db, testKey, nil)
	linkRepo := NewLinkRepo(db, nil)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)

	link := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether1",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether1",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}

	if err := linkRepo.Create(link); err != nil {
		t.Fatalf("Create link: %v", err)
	}

	if link.ID == uuid.Nil {
		t.Fatal("Expected link ID to be assigned")
	}

	// Should appear when querying by source device
	links, err := linkRepo.GetByDeviceID(d1ID)
	if err != nil {
		t.Fatalf("GetByDeviceID(source): %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("Expected 1 link for source device, got %d", len(links))
	}
	if links[0].SourceIfName != "ether1" {
		t.Errorf("SourceIfName = %q, want %q", links[0].SourceIfName, "ether1")
	}
	if links[0].DiscoveryProtocol != domain.DiscoveryProtocolLLDP {
		t.Errorf("DiscoveryProtocol = %q, want %q", links[0].DiscoveryProtocol, domain.DiscoveryProtocolLLDP)
	}

	// Should also appear when querying by target device
	links2, err := linkRepo.GetByDeviceID(d2ID)
	if err != nil {
		t.Fatalf("GetByDeviceID(target): %v", err)
	}
	if len(links2) != 1 {
		t.Fatalf("Expected 1 link for target device, got %d", len(links2))
	}
}

func TestLinkRepo_Upsert_InsertNew(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db, testKey, nil)
	linkRepo := NewLinkRepo(db, nil)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)

	link := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether2",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether2",
		DiscoveryProtocol: domain.DiscoveryProtocolCDP,
	}

	if _, err := linkRepo.Upsert(link); err != nil {
		t.Fatalf("Upsert (insert): %v", err)
	}

	if link.ID == uuid.Nil {
		t.Fatal("Expected link ID to be assigned after Upsert")
	}

	all, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("Expected 1 link, got %d", len(all))
	}
	if all[0].DiscoveryProtocol != domain.DiscoveryProtocolCDP {
		t.Errorf("DiscoveryProtocol = %q, want %q", all[0].DiscoveryProtocol, domain.DiscoveryProtocolCDP)
	}
}

func TestLinkRepo_Upsert_UpdateExisting(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db, testKey, nil)
	linkRepo := NewLinkRepo(db, nil)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)

	// First upsert (insert)
	link1 := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether3",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether3",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if _, err := linkRepo.Upsert(link1); err != nil {
		t.Fatalf("Upsert (insert): %v", err)
	}
	originalID := link1.ID

	// Second upsert (update) — same interface pair, different protocol
	link2 := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether3",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether3",
		DiscoveryProtocol: domain.DiscoveryProtocolCDP,
	}
	if _, err := linkRepo.Upsert(link2); err != nil {
		t.Fatalf("Upsert (update): %v", err)
	}

	// Should still have only 1 link
	all, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("Expected 1 link after upsert, got %d", len(all))
	}

	// ID should be preserved from the first insert
	if all[0].ID != originalID {
		t.Errorf("Link ID changed after upsert: got %s, want %s", all[0].ID, originalID)
	}
	// Protocol should be updated
	if all[0].DiscoveryProtocol != domain.DiscoveryProtocolCDP {
		t.Errorf("DiscoveryProtocol = %q, want %q", all[0].DiscoveryProtocol, domain.DiscoveryProtocolCDP)
	}
}

func TestLinkRepo_UpsertDetailed_NoopDoesNotUpdateTimestampOrNotify(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db, testKey, nil)
	changes := make(chan struct{}, 1)
	linkRepo := NewLinkRepo(db, changes)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)

	link := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether3",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether9",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if err := linkRepo.Create(link); err != nil {
		t.Fatalf("Create link: %v", err)
	}
	original, err := linkRepo.GetByID(link.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	select {
	case <-changes:
	default:
	}

	result, err := linkRepo.UpsertDetailed(&domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether3",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether9",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	})
	if err != nil {
		t.Fatalf("UpsertDetailed: %v", err)
	}
	if result.Created {
		t.Fatal("expected no-op upsert not to create a row")
	}
	if result.Changed {
		t.Fatal("expected no-op upsert not to mark change")
	}
	if result.Kind != domain.LinkUpsertKindNoop {
		t.Fatalf("Kind = %q, want %q", result.Kind, domain.LinkUpsertKindNoop)
	}

	stored, err := linkRepo.GetByID(link.ID)
	if err != nil {
		t.Fatalf("GetByID after noop: %v", err)
	}
	if !stored.UpdatedAt.Equal(original.UpdatedAt) {
		t.Fatalf("UpdatedAt changed on no-op upsert: before=%s after=%s", original.UpdatedAt, stored.UpdatedAt)
	}
	select {
	case <-changes:
		t.Fatal("expected no invalidation signal on no-op upsert")
	default:
	}
}

func TestLinkRepo_Delete(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db, testKey, nil)
	linkRepo := NewLinkRepo(db, nil)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)

	link := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether1",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether1",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if err := linkRepo.Create(link); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := linkRepo.Delete(link.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	all, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("Expected 0 links after delete, got %d", len(all))
	}
}

func TestLinkRepo_GetAll(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db, testKey, nil)
	linkRepo := NewLinkRepo(db, nil)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)

	links := []*domain.Link{
		{SourceDeviceID: d1ID, SourceIfName: "ether1", TargetDeviceID: d2ID, TargetIfName: "ether1", DiscoveryProtocol: domain.DiscoveryProtocolLLDP},
		{SourceDeviceID: d1ID, SourceIfName: "ether2", TargetDeviceID: d2ID, TargetIfName: "ether2", DiscoveryProtocol: domain.DiscoveryProtocolCDP},
	}

	for i, l := range links {
		if err := linkRepo.Create(l); err != nil {
			t.Fatalf("Create link %d: %v", i, err)
		}
	}

	all, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("GetAll returned %d links, want 2", len(all))
	}
}

func TestLinkRepo_CreateManualIdempotent_PreservesDiscoveredDuplicate(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db, testKey, nil)
	linkRepo := NewLinkRepo(db, nil)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)
	existing := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether1",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether2",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if err := linkRepo.Create(existing); err != nil {
		t.Fatalf("Create discovered link: %v", err)
	}

	stored, created, err := linkRepo.CreateManualIdempotent(&domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether1",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether2",
		DiscoveryProtocol: domain.DiscoveryProtocolManual,
	}, false)
	if err != nil {
		t.Fatalf("CreateManualIdempotent duplicate: %v", err)
	}
	if created {
		t.Fatal("expected discovered duplicate not to create a row")
	}
	if stored == nil {
		t.Fatal("expected existing link to be returned")
	}
	if stored.ID != existing.ID {
		t.Fatalf("returned ID = %s, want %s", stored.ID, existing.ID)
	}
	if stored.DiscoveryProtocol != domain.DiscoveryProtocolLLDP {
		t.Fatalf("returned protocol = %q, want %q", stored.DiscoveryProtocol, domain.DiscoveryProtocolLLDP)
	}

	all, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 stored link, got %d", len(all))
	}
	if all[0].DiscoveryProtocol != domain.DiscoveryProtocolLLDP {
		t.Fatalf("stored protocol = %q, want %q", all[0].DiscoveryProtocol, domain.DiscoveryProtocolLLDP)
	}
}

func TestLinkRepo_CreateManualIdempotent_ReturnsCanonicalReverseDuplicate(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db, testKey, nil)
	linkRepo := NewLinkRepo(db, nil)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)
	existing := &domain.Link{
		SourceDeviceID:    d2ID,
		SourceIfName:      "ether2",
		TargetDeviceID:    d1ID,
		TargetIfName:      "ether1",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if err := linkRepo.Create(existing); err != nil {
		t.Fatalf("Create reverse discovered link: %v", err)
	}

	stored, created, err := linkRepo.CreateManualIdempotent(&domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether1",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether2",
		DiscoveryProtocol: domain.DiscoveryProtocolManual,
	}, false)
	if err != nil {
		t.Fatalf("CreateManualIdempotent reverse duplicate: %v", err)
	}
	if created {
		t.Fatal("expected reverse duplicate not to create a row")
	}
	if stored == nil {
		t.Fatal("expected existing link to be returned")
	}
	if stored.ID != existing.ID {
		t.Fatalf("returned ID = %s, want %s", stored.ID, existing.ID)
	}
	if stored.SourceDeviceID != existing.SourceDeviceID || stored.SourceIfName != existing.SourceIfName ||
		stored.TargetDeviceID != existing.TargetDeviceID || stored.TargetIfName != existing.TargetIfName {
		t.Fatalf("returned orientation = %+v, want stored orientation %+v", *stored, *existing)
	}
	if stored.DiscoveryProtocol != domain.DiscoveryProtocolLLDP {
		t.Fatalf("returned protocol = %q, want %q", stored.DiscoveryProtocol, domain.DiscoveryProtocolLLDP)
	}

	all, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 stored link, got %d", len(all))
	}
}

func TestLinkRepo_CreateManualIdempotent_AllowsParallelManualLinks(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db, testKey, nil)
	linkRepo := NewLinkRepo(db, nil)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)
	first, firstCreated, err := linkRepo.CreateManualIdempotent(&domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether1",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether2",
		DiscoveryProtocol: domain.DiscoveryProtocolManual,
	}, false)
	if err != nil {
		t.Fatalf("CreateManualIdempotent first link: %v", err)
	}
	if !firstCreated {
		t.Fatal("expected first manual link to be created")
	}

	second, secondCreated, err := linkRepo.CreateManualIdempotent(&domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether3",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether4",
		DiscoveryProtocol: domain.DiscoveryProtocolManual,
	}, false)
	if err != nil {
		t.Fatalf("CreateManualIdempotent second link: %v", err)
	}
	if !secondCreated {
		t.Fatal("expected parallel manual link to be created")
	}
	if first.ID == second.ID {
		t.Fatalf("parallel manual links returned the same ID %s", first.ID)
	}

	all, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 stored manual links, got %d", len(all))
	}
}

func TestLinkRepo_CreateManualIdempotent_BrowserMigrationUsesDevicePair(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db, testKey, nil)
	linkRepo := NewLinkRepo(db, nil)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)
	existing := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether1",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether2",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if err := linkRepo.Create(existing); err != nil {
		t.Fatalf("Create discovered link: %v", err)
	}

	stored, created, err := linkRepo.CreateManualIdempotent(&domain.Link{
		SourceDeviceID:    d2ID,
		SourceIfName:      "",
		TargetDeviceID:    d1ID,
		TargetIfName:      "",
		DiscoveryProtocol: domain.DiscoveryProtocolManual,
	}, true)
	if err != nil {
		t.Fatalf("CreateManualIdempotent browser migration duplicate: %v", err)
	}
	if created {
		t.Fatal("expected browser migration device-pair duplicate not to create a row")
	}
	if stored == nil {
		t.Fatal("expected existing link to be returned")
	}
	if stored.ID != existing.ID {
		t.Fatalf("returned ID = %s, want %s", stored.ID, existing.ID)
	}
	if stored.SourceIfName != "ether1" || stored.TargetIfName != "ether2" {
		t.Fatalf("returned interfaces = %q/%q, want ether1/ether2", stored.SourceIfName, stored.TargetIfName)
	}

	all, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 stored link, got %d", len(all))
	}
	if all[0].SourceIfName == "" || all[0].TargetIfName == "" {
		t.Fatalf("expected no empty-interface duplicate, stored link: %+v", all[0])
	}
}

func TestLinkRepo_CreateManualIdempotent_ConcurrentReverseCreatesCreateOneRow(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db, testKey, nil)
	linkRepo := NewLinkRepo(db, nil)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)
	start := make(chan struct{})
	type result struct {
		created bool
		err     error
	}
	results := make(chan result, 2)

	var wg sync.WaitGroup
	for _, link := range []*domain.Link{
		{
			SourceDeviceID:    d1ID,
			SourceIfName:      "ether1",
			TargetDeviceID:    d2ID,
			TargetIfName:      "ether2",
			DiscoveryProtocol: domain.DiscoveryProtocolManual,
		},
		{
			SourceDeviceID:    d2ID,
			SourceIfName:      "ether2",
			TargetDeviceID:    d1ID,
			TargetIfName:      "ether1",
			DiscoveryProtocol: domain.DiscoveryProtocolManual,
		},
	} {
		link := link
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, created, err := linkRepo.CreateManualIdempotent(link, false)
			results <- result{created: created, err: err}
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	createdCount := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("CreateManualIdempotent concurrent create: %v", result.err)
		}
		if result.created {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("created result count = %d, want 1", createdCount)
	}

	all, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 stored link after concurrent reverse creates, got %d", len(all))
	}
}

func TestLinkRepo_Upsert_PreservesDistinctParallelUplinks(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db, testKey, nil)
	linkRepo := NewLinkRepo(db, nil)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)

	first := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "sfp-sfpplus1",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether1",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if created, err := linkRepo.Upsert(first); err != nil {
		t.Fatalf("Upsert first uplink: %v", err)
	} else if !created {
		t.Fatal("Expected first uplink to be inserted")
	}

	second := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "sfp-sfpplus2",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether2",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if created, err := linkRepo.Upsert(second); err != nil {
		t.Fatalf("Upsert second uplink: %v", err)
	} else if !created {
		t.Fatal("Expected second uplink to be inserted separately")
	}

	all, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 stored parallel uplinks, got %d", len(all))
	}
}

// TestLinkRepo_Upsert_CleansUpBrokenLink verifies that upserting a link with a
// non-empty SourceIfName deletes any existing link for the same physical link
// that has an empty SourceIfName (a "broken" link from an incomplete discovery).
func TestLinkRepo_Upsert_CleansUpBrokenLink(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db, testKey, nil)
	linkRepo := NewLinkRepo(db, nil)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)

	// Insert a broken link with empty SourceIfName
	broken := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "",
		TargetDeviceID:    d2ID,
		TargetIfName:      "",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if err := linkRepo.Create(broken); err != nil {
		t.Fatalf("Create broken link: %v", err)
	}

	// Upsert a corrected link with SourceIfName populated
	corrected := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether1",
		TargetDeviceID:    d2ID,
		TargetIfName:      "",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if _, err := linkRepo.Upsert(corrected); err != nil {
		t.Fatalf("Upsert corrected link: %v", err)
	}

	all, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 link after cleanup, got %d", len(all))
	}
	if all[0].SourceIfName != "ether1" {
		t.Errorf("expected SourceIfName 'ether1', got %q", all[0].SourceIfName)
	}
}

// TestLinkRepo_Upsert_NoBrokenLinkNoDeletion verifies that upserting with a
// non-empty SourceIfName when no broken link exists works as a normal insert.
func TestLinkRepo_Upsert_NoBrokenLinkNoDeletion(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db, testKey, nil)
	linkRepo := NewLinkRepo(db, nil)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)

	// No pre-existing link — upsert should just insert
	link := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether1",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether2",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if _, err := linkRepo.Upsert(link); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	all, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 link, got %d", len(all))
	}
	if all[0].SourceIfName != "ether1" {
		t.Errorf("expected SourceIfName 'ether1', got %q", all[0].SourceIfName)
	}
}

// TestLinkRepo_Upsert_EmptySourceIfNamePreservesExisting verifies that upserting a link
// with an empty SourceIfName for a physical link that already has a populated link does
// NOT overwrite the existing port names.
func TestLinkRepo_Upsert_EmptySourceIfNamePreservesExisting(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db, testKey, nil)
	linkRepo := NewLinkRepo(db, nil)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)

	// Insert a valid link first
	valid := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether1",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether2",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if err := linkRepo.Create(valid); err != nil {
		t.Fatalf("Create valid link: %v", err)
	}

	// Upsert a new link with empty SourceIfName in the same direction — must match
	// existing record by physical interface pair and preserve the populated port names.
	empty := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether2",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if _, err := linkRepo.Upsert(empty); err != nil {
		t.Fatalf("Upsert empty: %v", err)
	}

	all, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	// Should still be exactly 1 link; existing port names must be preserved.
	if len(all) != 1 {
		t.Fatalf("expected 1 link after upsert of empty-port link, got %d", len(all))
	}
	if all[0].SourceIfName != "ether1" {
		t.Errorf("SourceIfName overwritten: got %q, want %q", all[0].SourceIfName, "ether1")
	}
	if all[0].TargetIfName != "ether2" {
		t.Errorf("TargetIfName overwritten: got %q, want %q", all[0].TargetIfName, "ether2")
	}
}

// TestLinkRepo_Upsert_BidirectionalDedup verifies that when device A discovers
// device B (A→B) and then device B discovers device A (B→A), only one link record
// is created for the same physical interface pair.
func TestLinkRepo_Upsert_BidirectionalDedup(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db, testKey, nil)
	linkRepo := NewLinkRepo(db, nil)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)

	// Device A (d1) discovers neighbor B on its local port "ether4";
	// B's remote port is "ether8".
	fromA := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether4 - uplink potenza centro",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether8 - uplink lavangone",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if _, err := linkRepo.Upsert(fromA); err != nil {
		t.Fatalf("Upsert from A: %v", err)
	}
	firstID := fromA.ID

	// Device B (d2) discovers neighbor A on its local port "ether8";
	// A's remote port is "ether4".
	fromB := &domain.Link{
		SourceDeviceID:    d2ID,
		SourceIfName:      "ether8 - uplink lavangone",
		TargetDeviceID:    d1ID,
		TargetIfName:      "ether4 - uplink potenza centro",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if _, err := linkRepo.Upsert(fromB); err != nil {
		t.Fatalf("Upsert from B: %v", err)
	}

	all, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 link after bidirectional upsert, got %d", len(all))
	}
	if all[0].ID != firstID {
		t.Errorf("link ID changed: got %s, want %s", all[0].ID, firstID)
	}
	if all[0].SourceIfName != "ether4 - uplink potenza centro" {
		t.Errorf("SourceIfName changed: got %q, want %q",
			all[0].SourceIfName, "ether4 - uplink potenza centro")
	}
	if all[0].TargetIfName != "ether8 - uplink lavangone" {
		t.Errorf("TargetIfName changed: got %q, want %q",
			all[0].TargetIfName, "ether8 - uplink lavangone")
	}
}

func TestLinkRepo_Upsert_BidirectionalDedup_MatchesAnchoredInterfaceLabels(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db, testKey, nil)
	linkRepo := NewLinkRepo(db, nil)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)

	fromA := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether4 - uplink potenza centro",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether8 - uplink lavangone",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if _, err := linkRepo.Upsert(fromA); err != nil {
		t.Fatalf("Upsert from A: %v", err)
	}
	firstID := fromA.ID

	fromB := &domain.Link{
		SourceDeviceID:    d2ID,
		SourceIfName:      "ether8",
		TargetDeviceID:    d1ID,
		TargetIfName:      "ether4",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if _, err := linkRepo.Upsert(fromB); err != nil {
		t.Fatalf("Upsert from B: %v", err)
	}

	all, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 link after anchored reverse upsert, got %d", len(all))
	}
	if all[0].ID != firstID {
		t.Errorf("link ID changed: got %s, want %s", all[0].ID, firstID)
	}
}

func TestLinkRepo_Upsert_BidirectionalDedup_EnrichesEmptyReverseInterface(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db, testKey, nil)
	linkRepo := NewLinkRepo(db, nil)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)

	fromA := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether8",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if _, err := linkRepo.Upsert(fromA); err != nil {
		t.Fatalf("Upsert from A: %v", err)
	}
	firstID := fromA.ID

	fromB := &domain.Link{
		SourceDeviceID:    d2ID,
		SourceIfName:      "ether8",
		TargetDeviceID:    d1ID,
		TargetIfName:      "ether4",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if _, err := linkRepo.Upsert(fromB); err != nil {
		t.Fatalf("Upsert from B: %v", err)
	}

	all, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 link after reverse enrichment, got %d", len(all))
	}
	if all[0].ID != firstID {
		t.Errorf("link ID changed: got %s, want %s", all[0].ID, firstID)
	}
	if all[0].SourceIfName != "ether4" {
		t.Errorf("SourceIfName = %q, want %q", all[0].SourceIfName, "ether4")
	}
	if all[0].TargetIfName != "ether8" {
		t.Errorf("TargetIfName = %q, want %q", all[0].TargetIfName, "ether8")
	}
}

func TestLinkRepo_Upsert_BidirectionalDedup_ReorientsWhenIncomingOnlyKnowsNewSourcePort(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db, testKey, nil)
	linkRepo := NewLinkRepo(db, nil)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)

	// Existing reverse-direction record from the neighbor's perspective.
	// It only knows the newly added device's port, so the saved source side is empty.
	fromNeighbor := &domain.Link{
		SourceDeviceID:    d2ID,
		SourceIfName:      "",
		TargetDeviceID:    d1ID,
		TargetIfName:      "ether4",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if _, err := linkRepo.Upsert(fromNeighbor); err != nil {
		t.Fatalf("Upsert from neighbor: %v", err)
	}
	firstID := fromNeighbor.ID

	// The newly added device then discovers the same link and only knows its
	// own local port. The saved link should reorient so the known port stays on
	// the source side of the newly added device.
	fromNewDevice := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether4",
		TargetDeviceID:    d2ID,
		TargetIfName:      "",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if _, err := linkRepo.Upsert(fromNewDevice); err != nil {
		t.Fatalf("Upsert from new device: %v", err)
	}

	all, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 link after reverse reorientation, got %d", len(all))
	}
	if all[0].ID != firstID {
		t.Errorf("link ID changed: got %s, want %s", all[0].ID, firstID)
	}
	if all[0].SourceDeviceID != d1ID {
		t.Errorf("SourceDeviceID = %s, want %s", all[0].SourceDeviceID, d1ID)
	}
	if all[0].TargetDeviceID != d2ID {
		t.Errorf("TargetDeviceID = %s, want %s", all[0].TargetDeviceID, d2ID)
	}
	if all[0].SourceIfName != "ether4" {
		t.Errorf("SourceIfName = %q, want %q", all[0].SourceIfName, "ether4")
	}
	if all[0].TargetIfName != "" {
		t.Errorf("TargetIfName = %q, want empty string", all[0].TargetIfName)
	}
}

func TestLinkRepo_Upsert_RetriesAfterBusyLock(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "busy-retry.db")
	dsn := fmt.Sprintf(
		"%s?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=50&_foreign_keys=on&_txlock=immediate",
		dbPath,
	)

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("opening primary db: %v", err)
	}
	ConfigureSQLiteDB(db)
	defer db.Close()

	lockerDB, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("opening locker db: %v", err)
	}
	ConfigureSQLiteDB(lockerDB)
	defer lockerDB.Close()

	if err := RunMigrations(db); err != nil {
		t.Fatalf("running migrations: %v", err)
	}

	deviceRepo := NewDeviceRepo(db, testKey, nil)
	linkRepo := NewLinkRepo(db, nil)
	d1ID, d2ID := createTestDevicePair(t, deviceRepo)

	lockTx, err := lockerDB.Begin()
	if err != nil {
		t.Fatalf("beginning lock tx: %v", err)
	}
	if _, err := lockTx.Exec(
		`INSERT OR REPLACE INTO settings (key, value, updated_at) VALUES ('busy-lock', '1', datetime('now'))`,
	); err != nil {
		t.Fatalf("acquiring write lock: %v", err)
	}

	resultCh := make(chan error, 1)
	go func() {
		_, upsertErr := linkRepo.Upsert(&domain.Link{
			SourceDeviceID:    d1ID,
			SourceIfName:      "ether9",
			TargetDeviceID:    d2ID,
			TargetIfName:      "ether9",
			DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
		})
		resultCh <- upsertErr
	}()

	time.Sleep(150 * time.Millisecond)
	if err := lockTx.Rollback(); err != nil {
		t.Fatalf("releasing write lock: %v", err)
	}

	select {
	case err := <-resultCh:
		if err != nil {
			t.Fatalf("Upsert should succeed after retry, got: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for upsert result")
	}

	all, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 link after busy retry, got %d", len(all))
	}
}

func TestSettingsRepo_GetSetGetAll(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSettingsRepo(db)

	// Defaults should be seeded by migrations
	all, err := repo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("Expected default settings to be seeded, got 0")
	}

	// Check a default value
	val, err := repo.Get(domain.SettingPollingInterval)
	if err != nil {
		t.Fatalf("Get polling_interval: %v", err)
	}
	if val != "60" {
		t.Errorf("Default polling_interval = %q, want %q", val, "60")
	}

	// Set a custom value
	if err := repo.Set(domain.SettingPrometheusURL, "http://prometheus:9090"); err != nil {
		t.Fatalf("Set prometheus_url: %v", err)
	}

	val2, err := repo.Get(domain.SettingPrometheusURL)
	if err != nil {
		t.Fatalf("Get prometheus_url: %v", err)
	}
	if val2 != "http://prometheus:9090" {
		t.Errorf("prometheus_url = %q, want %q", val2, "http://prometheus:9090")
	}

	// Set a new key
	if err := repo.Set("custom_key", "custom_value"); err != nil {
		t.Fatalf("Set custom_key: %v", err)
	}

	val3, err := repo.Get("custom_key")
	if err != nil {
		t.Fatalf("Get custom_key: %v", err)
	}
	if val3 != "custom_value" {
		t.Errorf("custom_key = %q, want %q", val3, "custom_value")
	}

	// Error for non-existent key
	_, err = repo.Get("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent key")
	}
}
