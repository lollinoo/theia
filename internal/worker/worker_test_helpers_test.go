package worker

import (
	"fmt"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/vendor"
	"github.com/lollinoo/theia/internal/ws"
)

func floatPtr(f float64) *float64 { return &f }

type mockWorkerDeviceRepo struct {
	devices     []domain.Device
	updateCalls int32
}

func (r *mockWorkerDeviceRepo) Create(_ *domain.Device) error { return nil }

func (r *mockWorkerDeviceRepo) GetByID(id uuid.UUID) (*domain.Device, error) {
	for _, d := range r.devices {
		if d.ID == id {
			cp := d
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("device not found: %s", id)
}

func (r *mockWorkerDeviceRepo) GetByIP(_ string) (*domain.Device, error) { return nil, nil }

func (r *mockWorkerDeviceRepo) GetByAddress(_ string) (*domain.Device, error) { return nil, nil }

func (r *mockWorkerDeviceRepo) GetDeviceAddresses(_ uuid.UUID) ([]domain.DeviceAddress, error) {
	return nil, nil
}

func (r *mockWorkerDeviceRepo) ReplaceDeviceAddresses(_ uuid.UUID, _ []domain.DeviceAddress) error {
	return nil
}

func (r *mockWorkerDeviceRepo) FindAddressConflict(_ string, _ domain.DeviceType, _ uuid.UUID) (*domain.Device, error) {
	return nil, nil
}

func (r *mockWorkerDeviceRepo) FindPhysicalVirtualIPConflict(_ string, _ domain.DeviceType, _ uuid.UUID) (*domain.Device, error) {
	return nil, nil
}

func (r *mockWorkerDeviceRepo) GetBySysName(_ string) (*domain.Device, error) { return nil, nil }

func (r *mockWorkerDeviceRepo) GetAll() ([]domain.Device, error) {
	result := make([]domain.Device, len(r.devices))
	copy(result, r.devices)
	return result, nil
}

func (r *mockWorkerDeviceRepo) Update(_ *domain.Device) error {
	atomic.AddInt32(&r.updateCalls, 1)
	return nil
}

func (r *mockWorkerDeviceRepo) Delete(_ uuid.UUID) error { return nil }

func (r *mockWorkerDeviceRepo) UpdateStatus(_ uuid.UUID, _ domain.DeviceStatus) error {
	atomic.AddInt32(&r.updateCalls, 1)
	return nil
}

type mockWorkerLinkRepo struct {
	links []domain.Link
}

func (r *mockWorkerLinkRepo) Create(_ *domain.Link) error { return nil }

func (r *mockWorkerLinkRepo) CreateManualIdempotent(link *domain.Link, _ bool) (*domain.Link, bool, error) {
	return link, true, nil
}

func (r *mockWorkerLinkRepo) GetByID(_ uuid.UUID) (*domain.Link, error) { return nil, nil }

func (r *mockWorkerLinkRepo) GetByDeviceID(_ uuid.UUID) ([]domain.Link, error) { return nil, nil }

func (r *mockWorkerLinkRepo) GetAll() ([]domain.Link, error) {
	result := make([]domain.Link, len(r.links))
	copy(result, r.links)
	return result, nil
}

func (r *mockWorkerLinkRepo) Update(_ *domain.Link) error { return nil }

func (r *mockWorkerLinkRepo) Delete(_ uuid.UUID) error { return nil }

func (r *mockWorkerLinkRepo) Upsert(_ *domain.Link) (bool, error) { return false, nil }

func buildEmptyVendorRegistry() *vendor.Registry {
	records := []vendor.DBVendorRecord{
		{
			Name: "default",
			ConfigJSON: `{
				"vendor": {"name": "default", "display_name": "Generic"},
				"detection": {},
				"backup": {"supported": false}
			}`,
		},
	}
	reg, err := vendor.LoadRegistryFromDB(records)
	if err != nil {
		panic(fmt.Sprintf("buildEmptyVendorRegistry: %v", err))
	}
	return reg
}

func drainBroadcastCh(hub *ws.Hub) [][]byte {
	var msgs [][]byte
	for {
		select {
		case msg := <-hub.BroadcastCh():
			msgs = append(msgs, msg)
		default:
			return msgs
		}
	}
}
