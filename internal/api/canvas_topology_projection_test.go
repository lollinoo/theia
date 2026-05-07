package api

import (
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestProjectCanvasTopologyForMapUsesAreaAndGhosts(t *testing.T) {
	areaID := uuid.New()
	localID := uuid.New()
	remoteID := uuid.New()
	devices := []domain.Device{
		{ID: localID, Hostname: "local", AreaIDs: []uuid.UUID{areaID}, Tags: map[string]string{}},
		{ID: remoteID, Hostname: "remote", AreaIDs: []uuid.UUID{}, Tags: map[string]string{}},
	}
	links := []domain.Link{{
		ID:             uuid.New(),
		SourceDeviceID: localID,
		TargetDeviceID: remoteID,
	}}

	result := projectCanvasTopologyForMap(devices, links, domain.CanvasMapFilter{
		AreaID:                &areaID,
		IncludeCrossAreaLinks: true,
		IncludeGhostDevices:   true,
	})

	if len(result.Devices) != 1 || result.Devices[0].ID != localID {
		t.Fatalf("expected only local base device, got %#v", result.Devices)
	}
	if len(result.Links) != 1 {
		t.Fatalf("expected cross-area link, got %#v", result.Links)
	}
	if len(result.GhostDevices) != 1 || result.GhostDevices[0].ID != remoteID {
		t.Fatalf("expected remote ghost device, got %#v", result.GhostDevices)
	}
}

func TestProjectCanvasTopologyForMapDeviceIDsTakePrecedenceOverArea(t *testing.T) {
	areaID := uuid.New()
	selectedID := uuid.New()
	areaDeviceID := uuid.New()
	devices := []domain.Device{
		{ID: selectedID, Hostname: "selected", AreaIDs: []uuid.UUID{}, Tags: map[string]string{}},
		{ID: areaDeviceID, Hostname: "area", AreaIDs: []uuid.UUID{areaID}, Tags: map[string]string{}},
	}

	result := projectCanvasTopologyForMap(devices, nil, domain.CanvasMapFilter{
		AreaID:    &areaID,
		DeviceIDs: []uuid.UUID{selectedID},
	})

	if len(result.Devices) != 1 || result.Devices[0].ID != selectedID {
		t.Fatalf("expected explicit device id to win over area, got %#v", result.Devices)
	}
}

func TestProjectCanvasTopologyForMapTagsNarrowBaseDevices(t *testing.T) {
	coreID := uuid.New()
	accessID := uuid.New()
	devices := []domain.Device{
		{ID: coreID, Hostname: "core", Tags: map[string]string{"role": "core"}},
		{ID: accessID, Hostname: "access", Tags: map[string]string{"role": "access"}},
	}

	result := projectCanvasTopologyForMap(devices, nil, domain.CanvasMapFilter{
		Tags: map[string]string{"role": "core"},
	})

	if len(result.Devices) != 1 || result.Devices[0].ID != coreID {
		t.Fatalf("expected only core device after tag filter, got %#v", result.Devices)
	}
}

func TestProjectCanvasTopologyForMapCrossAreaLinksFalseRequiresBothEndpointsBase(t *testing.T) {
	areaID := uuid.New()
	localID := uuid.New()
	peerID := uuid.New()
	remoteID := uuid.New()
	devices := []domain.Device{
		{ID: localID, Hostname: "local", AreaIDs: []uuid.UUID{areaID}, Tags: map[string]string{}},
		{ID: peerID, Hostname: "peer", AreaIDs: []uuid.UUID{areaID}, Tags: map[string]string{}},
		{ID: remoteID, Hostname: "remote", AreaIDs: []uuid.UUID{}, Tags: map[string]string{}},
	}
	localPeerLink := domain.Link{
		ID:             uuid.New(),
		SourceDeviceID: localID,
		TargetDeviceID: peerID,
	}
	localRemoteLink := domain.Link{
		ID:             uuid.New(),
		SourceDeviceID: localID,
		TargetDeviceID: remoteID,
	}

	result := projectCanvasTopologyForMap(devices, []domain.Link{localPeerLink, localRemoteLink}, domain.CanvasMapFilter{
		AreaID:                &areaID,
		IncludeCrossAreaLinks: false,
		IncludeGhostDevices:   true,
	})

	if len(result.Links) != 1 || result.Links[0].ID != localPeerLink.ID {
		t.Fatalf("expected only link whose endpoints are both base, got %#v", result.Links)
	}
	if len(result.GhostDevices) != 0 {
		t.Fatalf("expected no ghosts for excluded cross-area link, got %#v", result.GhostDevices)
	}
}

func TestProjectCanvasTopologyForMapCrossAreaLinksDoNotRequireGhostDevices(t *testing.T) {
	areaID := uuid.New()
	localID := uuid.New()
	remoteID := uuid.New()
	devices := []domain.Device{
		{ID: localID, Hostname: "local", AreaIDs: []uuid.UUID{areaID}, Tags: map[string]string{}},
		{ID: remoteID, Hostname: "remote", AreaIDs: []uuid.UUID{}, Tags: map[string]string{}},
	}
	links := []domain.Link{{
		ID:             uuid.New(),
		SourceDeviceID: localID,
		TargetDeviceID: remoteID,
	}}

	result := projectCanvasTopologyForMap(devices, links, domain.CanvasMapFilter{
		AreaID:                &areaID,
		IncludeCrossAreaLinks: true,
		IncludeGhostDevices:   false,
	})

	if len(result.Links) != 1 {
		t.Fatalf("expected cross-area link when ghosts are disabled, got %#v", result.Links)
	}
	if len(result.GhostDevices) != 0 {
		t.Fatalf("expected no ghost devices when disabled, got %#v", result.GhostDevices)
	}
}
