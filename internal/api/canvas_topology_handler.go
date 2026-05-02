package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/logging"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/vendor"
	"github.com/lollinoo/theia/internal/ws"
)

// CanvasTopologyHandler serves the canvas read model used for initial loads and
// structural refreshes. Runtime metrics remain owned by the WebSocket stream.
type CanvasTopologyHandler struct {
	deviceService       *service.DeviceService
	linkRepo            domain.LinkRepository
	positionRepo        domain.PositionRepository
	areaRepo            domain.AreaRepository
	vendorRegistry      *vendor.Registry
	runtimeSnapshotFunc func() (*ws.SnapshotPayload, uint64)
}

func NewCanvasTopologyHandler(
	deviceService *service.DeviceService,
	linkRepo domain.LinkRepository,
	positionRepo domain.PositionRepository,
	areaRepo domain.AreaRepository,
	vendorRegistry *vendor.Registry,
	runtimeSnapshotFunc ...func() (*ws.SnapshotPayload, uint64),
) *CanvasTopologyHandler {
	var snapshotFunc func() (*ws.SnapshotPayload, uint64)
	if len(runtimeSnapshotFunc) > 0 {
		snapshotFunc = runtimeSnapshotFunc[0]
	}
	return &CanvasTopologyHandler{
		deviceService:       deviceService,
		linkRepo:            linkRepo,
		positionRepo:        positionRepo,
		areaRepo:            areaRepo,
		vendorRegistry:      vendorRegistry,
		runtimeSnapshotFunc: snapshotFunc,
	}
}

type canvasTopologyResponse struct {
	SchemaVersion   int                          `json:"schema_version"`
	TopologyVersion string                       `json:"topology_version"`
	RuntimeVersion  *uint64                      `json:"runtime_version,omitempty"`
	RuntimeIdentity string                       `json:"runtime_identity,omitempty"`
	RuntimeSnapshot *ws.SnapshotPayload          `json:"runtime_snapshot,omitempty"`
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
	Devices      []canvasTopologyVersionDevice            `json:"devices"`
	Links        []canvasTopologyVersionLink              `json:"links"`
	Positions    map[string]canvasTopologyVersionPosition `json:"positions"`
	Areas        []canvasTopologyVersionArea              `json:"areas"`
	Capabilities canvasTopologyCapabilities               `json:"capabilities"`
	Settings     canvasTopologyCanvasSettings             `json:"settings"`
}

type canvasTopologyVersionDevice struct {
	Type       string                 `json:"type"`
	ID         string                 `json:"id"`
	Attributes map[string]interface{} `json:"attributes"`
}

type canvasTopologyVersionLink struct {
	ID                string `json:"id"`
	SourceDeviceID    string `json:"source_device_id"`
	SourceIfName      string `json:"source_if_name"`
	TargetDeviceID    string `json:"target_device_id"`
	TargetIfName      string `json:"target_if_name"`
	DiscoveryProtocol string `json:"discovery_protocol"`
	SourceIfSpeed     int64  `json:"source_if_speed"`
	TargetIfSpeed     int64  `json:"target_if_speed"`
}

type canvasTopologyVersionPosition struct {
	DeviceID string  `json:"device_id"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Pinned   bool    `json:"pinned"`
}

type canvasTopologyVersionArea struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
	DeviceCount int    `json:"device_count"`
}

// HandleGet handles GET /api/v1/topology/canvas.
func (h *CanvasTopologyHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
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
		logCanvasTopologyResponse("/api/v1/topology/canvas", http.StatusNotModified, response, startedAt)
		return
	}

	logCanvasTopologyResponse("/api/v1/topology/canvas", http.StatusOK, response, startedAt)
	json.NewEncoder(w).Encode(response)
}

// HandleGetCanvas handles GET /api/v1/canvas and returns the complete canvas
// bootstrap: structural read model plus the current runtime base used by the
// WebSocket delta stream.
func (h *CanvasTopologyHandler) HandleGetCanvas(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
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
	if h.runtimeSnapshotFunc != nil {
		runtimeSnapshot, runtimeVersion := h.runtimeSnapshotFunc()
		response.RuntimeVersion = &runtimeVersion
		response.RuntimeSnapshot = ws.CloneSnapshot(runtimeSnapshot)
		response.RuntimeIdentity = ws.RuntimeIdentityForSnapshot(runtimeSnapshot)
	}

	w.Header().Set("Cache-Control", "no-store")
	logCanvasTopologyResponse("/api/v1/canvas", http.StatusOK, response, startedAt)
	json.NewEncoder(w).Encode(response)
}

func logCanvasTopologyResponse(endpoint string, status int, response canvasTopologyResponse, startedAt time.Time) {
	runtimeVersion := "-"
	if response.RuntimeVersion != nil {
		runtimeVersion = strconv.FormatUint(*response.RuntimeVersion, 10)
	}
	runtimeDevices := 0
	runtimeLinks := 0
	if response.RuntimeSnapshot != nil {
		runtimeDevices = len(response.RuntimeSnapshot.Devices)
		runtimeLinks = len(response.RuntimeSnapshot.Links)
	}
	logging.Debugf(
		"canvas response endpoint=%s status=%d schema_version=%d topology_version=%s runtime_version=%s devices=%d links=%d positions=%d areas=%d runtime_devices=%d runtime_links=%d duration_ms=%d",
		endpoint,
		status,
		response.SchemaVersion,
		response.TopologyVersion,
		runtimeVersion,
		len(response.Devices),
		len(response.Links),
		len(response.Positions),
		len(response.Areas),
		runtimeDevices,
		runtimeLinks,
		time.Since(startedAt).Milliseconds(),
	)
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

	versionInput := buildCanvasTopologyVersionInput(deviceResources, enrichedLinks, positionMap, areaResponses, capabilities, settings)

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

func buildCanvasTopologyVersionInput(
	devices []jsonAPIResource,
	links []enrichedLinkResponse,
	positions map[string]canvasPosition,
	areas []areaResponse,
	capabilities canvasTopologyCapabilities,
	settings canvasTopologyCanvasSettings,
) canvasTopologyVersionInput {
	versionDevices := make([]canvasTopologyVersionDevice, 0, len(devices))
	for _, device := range devices {
		versionDevices = append(versionDevices, canvasTopologyVersionDevice{
			Type:       device.Type,
			ID:         device.ID,
			Attributes: stableCanvasTopologyDeviceAttributes(device.Attributes),
		})
	}

	versionLinks := make([]canvasTopologyVersionLink, 0, len(links))
	for _, link := range links {
		versionLinks = append(versionLinks, canvasTopologyVersionLink{
			ID:                link.ID,
			SourceDeviceID:    link.SourceDeviceID,
			SourceIfName:      link.SourceIfName,
			TargetDeviceID:    link.TargetDeviceID,
			TargetIfName:      link.TargetIfName,
			DiscoveryProtocol: link.DiscoveryProtocol,
			SourceIfSpeed:     link.SourceIfSpeed,
			TargetIfSpeed:     link.TargetIfSpeed,
		})
	}

	versionPositions := make(map[string]canvasTopologyVersionPosition, len(positions))
	for key, position := range positions {
		versionPositions[key] = canvasTopologyVersionPosition{
			DeviceID: position.DeviceID,
			X:        position.X,
			Y:        position.Y,
			Pinned:   position.Pinned,
		}
	}

	versionAreas := make([]canvasTopologyVersionArea, 0, len(areas))
	for _, area := range areas {
		versionAreas = append(versionAreas, canvasTopologyVersionArea{
			ID:          area.ID,
			Name:        area.Name,
			Description: area.Description,
			Color:       area.Color,
			DeviceCount: area.DeviceCount,
		})
	}

	return canvasTopologyVersionInput{
		Devices:      versionDevices,
		Links:        versionLinks,
		Positions:    versionPositions,
		Areas:        versionAreas,
		Capabilities: capabilities,
		Settings:     settings,
	}
}

func stableCanvasTopologyDeviceAttributes(attributes map[string]interface{}) map[string]interface{} {
	stableKeys := []string{
		"hostname",
		"ip",
		"notes",
		"device_type",
		"poll_class",
		"polling_enabled",
		"poll_interval_override",
		"sys_name",
		"sys_descr",
		"sys_object_id",
		"hardware_model",
		"os_version",
		"vendor",
		"managed",
		"tags",
		"metrics_source",
		"prometheus_label_name",
		"prometheus_label_value",
		"topology_discovery_mode",
		"effective_topology_discovery_mode",
		"area_ids",
		"backup_supported",
	}

	stable := make(map[string]interface{}, len(stableKeys))
	for _, key := range stableKeys {
		value, ok := attributes[key]
		if !ok {
			continue
		}
		if key == "area_ids" {
			if areaIDs, ok := value.([]string); ok {
				cloned := append([]string(nil), areaIDs...)
				sort.Strings(cloned)
				stable[key] = cloned
				continue
			}
		}
		stable[key] = value
	}
	return stable
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
