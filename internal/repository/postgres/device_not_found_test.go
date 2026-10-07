package postgres

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestDeviceLookupDistinguishesMissingDeviceFromDatabaseFailure(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)
	id := uuid.New()
	device, err := repo.GetByID(id)
	if device != nil || !errors.Is(err, domain.ErrDeviceNotFound) {
		t.Fatalf("missing lookup: device=%v error=%v", device, err)
	}
	if err.Error() != "device not found: "+id.String() {
		t.Fatalf("existing error text changed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	device, err = repo.GetByID(id)
	if device != nil || err == nil || errors.Is(err, domain.ErrDeviceNotFound) {
		t.Fatalf("database failure mistaken for a missing device: device=%v error=%v", device, err)
	}
}
