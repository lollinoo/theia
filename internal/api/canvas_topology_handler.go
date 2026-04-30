package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/vendor"
)

// CanvasTopologyHandler serves the canvas read model used for initial loads and
// structural refreshes. Runtime metrics remain owned by the WebSocket stream.
type CanvasTopologyHandler struct {
	deviceService  *service.DeviceService
	linkRepo       domain.LinkRepository
	positionRepo   domain.PositionRepository
	areaRepo       domain.AreaRepository
	vendorRegistry *vendor.Registry
}

func NewCanvasTopologyHandler(
	deviceService *service.DeviceService,
	linkRepo domain.LinkRepository,
	positionRepo domain.PositionRepository,
	areaRepo domain.AreaRepository,
	vendorRegistry *vendor.Registry,
) *CanvasTopologyHandler {
	return &CanvasTopologyHandler{
		deviceService:  deviceService,
		linkRepo:       linkRepo,
		positionRepo:   positionRepo,
		areaRepo:       areaRepo,
		vendorRegistry: vendorRegistry,
	}
}

type canvasTopologyResponse struct {
	SchemaVersion   int                          `json:"schema_version"`
	TopologyVersion string                       `json:"topology_version"`
	RuntimeVersion  string                       `json:"runtime_version,omitempty"`
	GeneratedAt     string                       `json:"generated_at"`
	Devices         []jsonAPIResource            `json:"devices"`
	Links           []enrichedLinkResponse       `json:"links"`
	Positions       map[string]canvasPosition    `json:"positions"`
	Areas           []areaResponse               `json:"areas"`
	Capabilities    canvasTopologyCapabilities   `json:"capabilities"`
	Settings        canvasTopologyCanvasSettings `json:"settings"`
}

type canvasPosition struct {
	DeviceID  string  `json:"device_id"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Pinned    bool    `json:"pinned"`
	UpdatedAt string  `json:"updated_at,omitempty"`
}

type canvasTopologyCapabilities struct {
	SupportsTopologyDelta    bool `json:"supports_topology_delta"`
	SupportsPositionRevision bool `json:"supports_position_revision"`
	SupportsAreaFiltering    bool `json:"supports_area_filtering"`
}

type canvasTopologyCanvasSettings struct {
	Layout canvasTopologyLayoutSettings `json:"layout"`
}

type canvasTopologyLayoutSettings struct {
	Version int `json:"version"`
}

type canvasTopologyVersionInput struct {
	Devices      []jsonAPIResource            `json:"devices"`
	Links        []enrichedLinkResponse       `json:"links"`
	Positions    map[string]canvasPosition    `json:"positions"`
	Areas        []areaResponse               `json:"areas"`
	Capabilities canvasTopologyCapabilities   `json:"capabilities"`
	Settings     canvasTopologyCanvasSettings `json:"settings"`
}

// HandleGet handles GET /api/v1/topology/canvas.
func (h *CanvasTopologyHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	devices, err := h.deviceService.GetAllDevices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list devices", err)
		return
	}
	links, err := h.linkRepo.GetAll()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list links", err)
		return
	}
	positions, err := h.positionRepo.GetAll()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list positions", err)
		return
	}
	areas, err := h.areaRepo.GetAllWithDeviceCount()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list areas", err)
		return
	}

	response := h.buildResponse(devices, links, positions, areas)
	etag := `"` + response.TopologyVersion + `"`
	w.Header().Set("ETag", etag)
	if requestETagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	json.NewEncoder(w).Encode(response)
}

func (h *CanvasTopologyHandler) buildResponse(
	devices []domain.Device,
	links []domain.Link,
	positions []domain.DevicePosition,
	areas []domain.AreaWithCount,
) canvasTopologyResponse {
	sort.Slice(devices, func(i, j int) bool {
		return devices[i].ID.String() < devices[j].ID.String()
	})
	sort.Slice(links, func(i, j int) bool {
		return links[i].ID.String() < links[j].ID.String()
	})
	sort.Slice(areas, func(i, j int) bool {
		return areas[i].ID.String() < areas[j].ID.String()
	})
	sort.Slice(positions, func(i, j int) bool {
		return positions[i].DeviceID.String() < positions[j].DeviceID.String()
	})

	deviceHandler := NewDeviceHandler(h.deviceService, nil, h.vendorRegistry)
	deviceResources := make([]jsonAPIResource, 0, len(devices))
	for i := range devices {
		deviceResources = append(deviceResources, deviceHandler.deviceToResource(&devices[i]))
	}

	positionMap := make(map[string]canvasPosition, len(positions))
	for _, position := range positions {
		deviceID := position.DeviceID.String()
		updatedAt := ""
		if !position.UpdatedAt.IsZero() {
			updatedAt = position.UpdatedAt.Format(time.RFC3339)
		}
		positionMap[deviceID] = canvasPosition{
			DeviceID:  deviceID,
			X:         position.X,
			Y:         position.Y,
			Pinned:    position.Pinned,
			UpdatedAt: updatedAt,
		}
	}

	areaResponses := make([]areaResponse, 0, len(areas))
	for i := range areas {
		areaResponses = append(areaResponses, areaToResponse(&areas[i].Area, areas[i].DeviceCount))
	}

	capabilities := canvasTopologyCapabilities{
		SupportsTopologyDelta:    false,
		SupportsPositionRevision: false,
		SupportsAreaFiltering:    true,
	}
	settings := canvasTopologyCanvasSettings{
		Layout: canvasTopologyLayoutSettings{Version: 1},
	}
	enrichedLinks := buildEnrichedLinkResponses(links, devices)

	versionInput := canvasTopologyVersionInput{
		Devices:      deviceResources,
		Links:        enrichedLinks,
		Positions:    positionMap,
		Areas:        areaResponses,
		Capabilities: capabilities,
		Settings:     settings,
	}

	return canvasTopologyResponse{
		SchemaVersion:   1,
		TopologyVersion: buildCanvasTopologyVersion(versionInput),
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		Devices:         deviceResources,
		Links:           enrichedLinks,
		Positions:       positionMap,
		Areas:           areaResponses,
		Capabilities:    capabilities,
		Settings:        settings,
	}
}

func buildCanvasTopologyVersion(input canvasTopologyVersionInput) string {
	data, err := json.Marshal(input)
	if err != nil {
		return "topo-unversioned"
	}
	sum := sha256.Sum256(data)
	return "topo-" + hex.EncodeToString(sum[:])[:16]
}

func requestETagMatches(headerValue string, etag string) bool {
	for _, candidate := range strings.Split(headerValue, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || candidate == etag {
			return true
		}
	}
	return false
}

type linkInterfaceInfo struct {
	speed      int64
	operStatus string
}

func buildEnrichedLinkResponses(
	links []domain.Link,
	devices []domain.Device,
) []enrichedLinkResponse {
	deviceIfMap := make(map[string]map[string]linkInterfaceInfo, len(devices))
	for i := range devices {
		device := &devices[i]
		ifMap := make(map[string]linkInterfaceInfo, len(device.Interfaces))
		for _, iface := range device.Interfaces {
			ifMap[iface.IfName] = linkInterfaceInfo{
				speed:      iface.Speed,
				operStatus: iface.OperStatus,
			}
		}
		deviceIfMap[device.ID.String()] = ifMap
	}

	enriched := make([]enrichedLinkResponse, 0, len(links))
	for _, link := range links {
		response := enrichedLinkResponse{
			ID:                link.ID.String(),
			SourceDeviceID:    link.SourceDeviceID.String(),
			SourceIfName:      link.SourceIfName,
			TargetDeviceID:    link.TargetDeviceID.String(),
			TargetIfName:      link.TargetIfName,
			DiscoveryProtocol: string(link.DiscoveryProtocol),
		}
		if ifMap, ok := deviceIfMap[response.SourceDeviceID]; ok {
			if info, ok := ifMap[link.SourceIfName]; ok {
				response.SourceIfSpeed = info.speed
				response.SourceIfOperStatus = info.operStatus
			}
		}
		if ifMap, ok := deviceIfMap[response.TargetDeviceID]; ok {
			if info, ok := ifMap[link.TargetIfName]; ok {
				response.TargetIfSpeed = info.speed
				response.TargetIfOperStatus = info.operStatus
			}
		}
		enriched = append(enriched, response)
	}

	return enriched
}
