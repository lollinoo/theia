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

func TestProjectCanvasTopologyForMapEmptyStringTagRequiresKeyPresence(t *testing.T) {
	emptyRackID := uuid.New()
	missingRackID := uuid.New()
	nilTagsID := uuid.New()
	devices := []domain.Device{
		{ID: emptyRackID, Hostname: "empty-rack", Tags: map[string]string{"rack": ""}},
		{ID: missingRackID, Hostname: "missing-rack", Tags: map[string]string{"role": "access"}},
		{ID: nilTagsID, Hostname: "nil-tags", Tags: nil},
	}

	result := projectCanvasTopologyForMap(devices, nil, domain.CanvasMapFilter{
		Tags: map[string]string{"rack": ""},
	})

	if len(result.Devices) != 1 || result.Devices[0].ID != emptyRackID {
		t.Fatalf("expected only device with explicit empty rack tag, got %#v", result.Devices)
	}
}

func TestProjectCanvasTopologyForMapNilTagsDoNotMatchRequiredTags(t *testing.T) {
	nilTagsID := uuid.New()
	emptyTagID := uuid.New()
	devices := []domain.Device{
		{ID: nilTagsID, Hostname: "nil-tags", Tags: nil},
		{ID: emptyTagID, Hostname: "empty-tag", Tags: map[string]string{"role": ""}},
	}

	result := projectCanvasTopologyForMap(devices, nil, domain.CanvasMapFilter{
		Tags: map[string]string{"role": ""},
	})

	if len(result.Devices) != 1 || result.Devices[0].ID != emptyTagID {
		t.Fatalf("expected nil tags not to match required empty tag, got %#v", result.Devices)
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

func TestProjectCanvasTopologyForMapDuplicateLinksKeepLinksAndDedupeGhosts(t *testing.T) {
	areaID := uuid.New()
	localID := uuid.New()
	remoteID := uuid.New()
	devices := []domain.Device{
		{ID: localID, Hostname: "local", AreaIDs: []uuid.UUID{areaID}, Tags: map[string]string{}},
		{ID: remoteID, Hostname: "remote", AreaIDs: []uuid.UUID{}, Tags: map[string]string{}},
	}
	firstLink := domain.Link{
		ID:             uuid.New(),
		SourceDeviceID: localID,
		TargetDeviceID: remoteID,
	}
	duplicateEndpointLink := domain.Link{
		ID:             uuid.New(),
		SourceDeviceID: localID,
		TargetDeviceID: remoteID,
	}

	result := projectCanvasTopologyForMap(devices, []domain.Link{firstLink, duplicateEndpointLink}, domain.CanvasMapFilter{
		AreaID:                &areaID,
		IncludeCrossAreaLinks: true,
		IncludeGhostDevices:   true,
	})

	if len(result.Links) != 2 || result.Links[0].ID != firstLink.ID || result.Links[1].ID != duplicateEndpointLink.ID {
		t.Fatalf("expected duplicate links to remain in link order, got %#v", result.Links)
	}
	if len(result.GhostDevices) != 1 || result.GhostDevices[0].ID != remoteID {
		t.Fatalf("expected one deduplicated remote ghost device, got %#v", result.GhostDevices)
	}
}

func TestProjectCanvasTopologyForMapUnknownExplicitDeviceIDsAreIgnored(t *testing.T) {
	knownID := uuid.New()
	unknownID := uuid.New()
	devices := []domain.Device{
		{ID: knownID, Hostname: "known", Tags: map[string]string{}},
	}

	result := projectCanvasTopologyForMap(devices, nil, domain.CanvasMapFilter{
		DeviceIDs: []uuid.UUID{unknownID},
	})

	if len(result.Devices) != 0 {
		t.Fatalf("expected unknown explicit device id not to create base devices, got %#v", result.Devices)
	}
}

func TestProjectCanvasTopologyForMapCrossAreaLinkWithUnknownEndpointIsExcluded(t *testing.T) {
	areaID := uuid.New()
	localID := uuid.New()
	unknownID := uuid.New()
	devices := []domain.Device{
		{ID: localID, Hostname: "local", AreaIDs: []uuid.UUID{areaID}, Tags: map[string]string{}},
	}
	unknownEndpointLink := domain.Link{
		ID:             uuid.New(),
		SourceDeviceID: localID,
		TargetDeviceID: unknownID,
	}

	result := projectCanvasTopologyForMap(devices, []domain.Link{unknownEndpointLink}, domain.CanvasMapFilter{
		AreaID:                &areaID,
		IncludeCrossAreaLinks: true,
		IncludeGhostDevices:   true,
	})

	if len(result.Links) != 0 {
		t.Fatalf("expected unknown endpoint link to be excluded, got %#v", result.Links)
	}
	if len(result.GhostDevices) != 0 {
		t.Fatalf("expected unknown endpoint not to create ghosts, got %#v", result.GhostDevices)
	}
}

func TestProjectCanvasTopologyForMapPreservesOutputOrder(t *testing.T) {
	areaID := uuid.New()
	baseFirstID := uuid.New()
	ghostFirstID := uuid.New()
	baseSecondID := uuid.New()
	ghostSecondID := uuid.New()
	devices := []domain.Device{
		{ID: baseFirstID, Hostname: "base-first", AreaIDs: []uuid.UUID{areaID}, Tags: map[string]string{}},
		{ID: ghostFirstID, Hostname: "ghost-first", AreaIDs: []uuid.UUID{}, Tags: map[string]string{}},
		{ID: baseSecondID, Hostname: "base-second", AreaIDs: []uuid.UUID{areaID}, Tags: map[string]string{}},
		{ID: ghostSecondID, Hostname: "ghost-second", AreaIDs: []uuid.UUID{}, Tags: map[string]string{}},
	}
	firstLink := domain.Link{
		ID:             uuid.New(),
		SourceDeviceID: baseSecondID,
		TargetDeviceID: ghostSecondID,
	}
	secondLink := domain.Link{
		ID:             uuid.New(),
		SourceDeviceID: baseFirstID,
		TargetDeviceID: ghostFirstID,
	}

	result := projectCanvasTopologyForMap(devices, []domain.Link{firstLink, secondLink}, domain.CanvasMapFilter{
		AreaID:                &areaID,
		IncludeCrossAreaLinks: true,
		IncludeGhostDevices:   true,
	})

	if len(result.Devices) != 2 || result.Devices[0].ID != baseFirstID || result.Devices[1].ID != baseSecondID {
		t.Fatalf("expected base devices to preserve device input order, got %#v", result.Devices)
	}
	if len(result.Links) != 2 || result.Links[0].ID != firstLink.ID || result.Links[1].ID != secondLink.ID {
		t.Fatalf("expected links to preserve link input order, got %#v", result.Links)
	}
	if len(result.GhostDevices) != 2 || result.GhostDevices[0].ID != ghostFirstID || result.GhostDevices[1].ID != ghostSecondID {
		t.Fatalf("expected ghost devices to preserve device input order, got %#v", result.GhostDevices)
	}
}
