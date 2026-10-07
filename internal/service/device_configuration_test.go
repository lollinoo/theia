package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

type configurationTestRepo struct {
	*mockDeviceRepo
	fields domain.DeviceConfigurationFields
}

func (r *configurationTestRepo) GetDeviceForUpdate(id uuid.UUID) (*domain.Device, error) {
	return r.mockDeviceRepo.GetByID(id)
}
func (r *configurationTestRepo) UpdateConfiguration(_ *domain.Device, fields domain.DeviceConfigurationFields) error {
	r.fields = fields
	return nil
}
func (*configurationTestRepo) Update(*domain.Device) error {
	panic("configuration used aggregate write")
}
func (*configurationTestRepo) FindAddressConflict(string, domain.DeviceType, uuid.UUID) (*domain.Device, error) {
	panic("unchanged address was revalidated")
}

func TestConfigurationServiceSkipsUnchangedAddressValidation(t *testing.T) {
	repo := &configurationTestRepo{mockDeviceRepo: newMockDeviceRepo()}
	device := &domain.Device{ID: uuid.New(), IP: "192.0.2.1", Hostname: "before", DeviceType: domain.DeviceTypeRouter, Status: domain.DeviceStatusUp, MetricsSource: domain.MetricsSourceSNMP}
	if err := repo.Create(device); err != nil {
		t.Fatal(err)
	}
	svc := NewDeviceService(repo, newMockLinkRepo(), newMockSettingsRepo(), nil, nil)
	defer svc.Stop()
	hostname := "after"
	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{Hostname: &hostname}); err != nil {
		t.Fatal(err)
	}
	if !repo.fields.Hostname || repo.fields.Addresses || repo.fields.SNMPCredentials || repo.fields.Status {
		t.Fatalf("unexpected fields=%+v", repo.fields)
	}
}
