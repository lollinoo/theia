package api

import (
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

type canvasTopologyProjection struct {
	Devices      []domain.Device
	Links        []domain.Link
	GhostDevices []domain.Device
}

func projectCanvasTopologyForMap(
	devices []domain.Device,
	links []domain.Link,
	filter domain.CanvasMapFilter,
) canvasTopologyProjection {
	baseDeviceIDs := make(map[uuid.UUID]struct{}, len(devices))
	selectedDeviceIDs := make(map[uuid.UUID]struct{}, len(filter.DeviceIDs))
	for _, deviceID := range filter.DeviceIDs {
		selectedDeviceIDs[deviceID] = struct{}{}
	}

	projection := canvasTopologyProjection{
		Devices: make([]domain.Device, 0, len(devices)),
		Links:   make([]domain.Link, 0, len(links)),
	}
	for _, device := range devices {
		baseDevice := false
		switch {
		case len(selectedDeviceIDs) > 0:
			_, baseDevice = selectedDeviceIDs[device.ID]
		case filter.AreaID != nil:
			baseDevice = deviceHasArea(device, *filter.AreaID)
		default:
			baseDevice = true
		}
		if !baseDevice || !deviceMatchesTags(device, filter.Tags) {
			continue
		}

		projection.Devices = append(projection.Devices, device)
		baseDeviceIDs[device.ID] = struct{}{}
	}

	ghostDeviceIDs := make(map[uuid.UUID]struct{})
	for _, link := range links {
		_, sourceIsBase := baseDeviceIDs[link.SourceDeviceID]
		_, targetIsBase := baseDeviceIDs[link.TargetDeviceID]
		includeLink := sourceIsBase && targetIsBase
		if filter.IncludeCrossAreaLinks && (sourceIsBase || targetIsBase) {
			includeLink = true
		}
		if !includeLink {
			continue
		}

		projection.Links = append(projection.Links, link)
		if !filter.IncludeGhostDevices {
			continue
		}
		if sourceIsBase && !targetIsBase {
			ghostDeviceIDs[link.TargetDeviceID] = struct{}{}
		}
		if targetIsBase && !sourceIsBase {
			ghostDeviceIDs[link.SourceDeviceID] = struct{}{}
		}
	}

	if filter.IncludeGhostDevices && len(ghostDeviceIDs) > 0 {
		projection.GhostDevices = make([]domain.Device, 0, len(ghostDeviceIDs))
		for _, device := range devices {
			if _, isBase := baseDeviceIDs[device.ID]; isBase {
				continue
			}
			if _, isGhost := ghostDeviceIDs[device.ID]; isGhost {
				projection.GhostDevices = append(projection.GhostDevices, device)
			}
		}
	}

	return projection
}

func deviceHasArea(device domain.Device, areaID uuid.UUID) bool {
	for _, deviceAreaID := range device.AreaIDs {
		if deviceAreaID == areaID {
			return true
		}
	}
	return false
}

func deviceMatchesTags(device domain.Device, tags map[string]string) bool {
	for key, expected := range tags {
		if device.Tags[key] != expected {
			return false
		}
	}
	return true
}
