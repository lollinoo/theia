package service

import (
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"testing"
)

func TestUpdateDeviceStatusDoesNotLoadAggregate(t *testing.T) {
	repo := newMockDeviceRepo()
	id := uuid.New()
	device := &domain.Device{ID: id, Hostname: "edited", Status: domain.DeviceStatusDown}
	if err := repo.Create(device); err != nil {
		t.Fatal(err)
	}
	repo.failGetByID = true
	svc := NewDeviceService(repo, newMockLinkRepo(), newMockSettingsRepo(), nil, nil)
	if err := svc.updateDeviceStatus(id, domain.DeviceStatusUp); err != nil {
		t.Fatal(err)
	}
	if repo.getByIDCalls != 0 {
		t.Fatalf("loaded device aggregate %d times", repo.getByIDCalls)
	}
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.devices[id].Hostname != "edited" || repo.devices[id].Status != domain.DeviceStatusUp {
		t.Fatalf("unexpected device: %+v", repo.devices[id])
	}
}
