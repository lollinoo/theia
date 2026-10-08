package cache

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

type detailDeviceRepo struct {
	domain.DeviceRepository
	devices []domain.Device
}

func (r *detailDeviceRepo) GetAll() ([]domain.Device, error) { return r.devices, nil }

type detailLinkRepo struct {
	domain.LinkRepository
	links   []domain.Link
	changes chan domain.LinkChangeEvent
}

func (r *detailLinkRepo) GetAll() ([]domain.Link, error) { return r.links, nil }
func (r *detailLinkRepo) GetByID(id uuid.UUID) (*domain.Link, error) {
	for _, link := range r.links {
		if link.ID == id {
			return &link, nil
		}
	}
	return nil, errors.New("link not found")
}
func (r *detailLinkRepo) LinkChanges() <-chan domain.LinkChangeEvent { return r.changes }
func (r *detailLinkRepo) DrainLinkRepair() bool                      { return false }

func TestDeviceTopologyTracksLinkReorientationAndDeletion(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	deviceRepo := &detailDeviceRepo{devices: []domain.Device{{ID: a}, {ID: b}, {ID: c}}}
	link := domain.Link{ID: uuid.New(), SourceDeviceID: a, TargetDeviceID: b}
	linkRepo := &detailLinkRepo{links: []domain.Link{link}, changes: make(chan domain.LinkChangeEvent, 4)}
	cache := NewDeviceLinkCache(deviceRepo, linkRepo, nil)
	devices, links, err := cache.GetDeviceTopology(a)
	if err != nil || len(devices) != 2 || len(links) != 1 {
		t.Fatalf("initial scoped topology: devices=%d links=%d error=%v", len(devices), len(links), err)
	}
	delete(devices, b)
	links[0].SourceDeviceID = c
	link.TargetDeviceID = c
	linkRepo.links = []domain.Link{link}
	linkRepo.changes <- domain.LinkChangeEvent{Kind: domain.ChangeKindUpdated, LinkID: link.ID}
	devices, links, err = cache.GetDeviceTopology(a)
	if err != nil || len(devices) != 2 || len(links) != 1 || links[0].TargetDeviceID != c {
		t.Fatalf("reoriented topology: %#v %#v %v", devices, links, err)
	}
	_, links, err = cache.GetDeviceTopology(b)
	if err != nil || len(links) != 0 {
		t.Fatalf("old endpoint still indexed: %#v %v", links, err)
	}
	linkRepo.changes <- domain.LinkChangeEvent{Kind: domain.ChangeKindDeleted, LinkID: link.ID}
	_, links, err = cache.GetDeviceTopology(a)
	if err != nil || len(links) != 0 {
		t.Fatalf("deleted link still indexed: %#v %v", links, err)
	}
}

func BenchmarkDeviceTopologyFixedDegree(b *testing.B) {
	for _, size := range []int{100, 10000} {
		b.Run(map[int]string{100: "100_devices", 10000: "10000_devices"}[size], func(b *testing.B) {
			devices := make([]domain.Device, size)
			for i := range devices {
				devices[i].ID = uuid.New()
			}
			links := make([]domain.Link, size-1)
			for i := range links {
				links[i] = domain.Link{ID: uuid.New(), SourceDeviceID: devices[i].ID, TargetDeviceID: devices[i+1].ID}
			}
			cache := NewDeviceLinkCache(&detailDeviceRepo{devices: devices}, &detailLinkRepo{links: links, changes: make(chan domain.LinkChangeEvent)}, nil)
			if _, _, err := cache.GetDeviceTopology(devices[0].ID); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, _, err := cache.GetDeviceTopology(devices[0].ID); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
