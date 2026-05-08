package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/ws"
)

type CanvasMapHandler struct {
	mapRepo             domain.CanvasMapRepository
	mapPositionRepo     domain.CanvasMapPositionRepository
	legacyPositionRepo  domain.PositionRepository
	canvasTopology      *CanvasTopologyHandler
	deviceService       *service.DeviceService
	linkRepo            domain.LinkRepository
	areaRepo            domain.AreaRepository
	runtimeSnapshotFunc func() (*ws.SnapshotPayload, uint64)
}

func NewCanvasMapHandler(
	mapRepo domain.CanvasMapRepository,
	mapPositionRepo domain.CanvasMapPositionRepository,
	legacyPositionRepo domain.PositionRepository,
	canvasTopology *CanvasTopologyHandler,
	deviceService *service.DeviceService,
	linkRepo domain.LinkRepository,
	areaRepo domain.AreaRepository,
	runtimeSnapshotFunc func() (*ws.SnapshotPayload, uint64),
) *CanvasMapHandler {
	return &CanvasMapHandler{
		mapRepo:             mapRepo,
		mapPositionRepo:     mapPositionRepo,
		legacyPositionRepo:  legacyPositionRepo,
		canvasTopology:      canvasTopology,
		deviceService:       deviceService,
		linkRepo:            linkRepo,
		areaRepo:            areaRepo,
		runtimeSnapshotFunc: runtimeSnapshotFunc,
	}
}

type canvasMapCreateRequest struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	SourceAreaID *string                `json:"source_area_id"`
	SourceMapID  *string                `json:"source_map_id"`
	Filter       domain.CanvasMapFilter `json:"filter"`
}

type canvasMapPatchRequest struct {
	Name         *string                 `json:"name"`
	Description  *string                 `json:"description"`
	SourceAreaID nullableCanvasMapString `json:"source_area_id"`
	Filter       *domain.CanvasMapFilter `json:"filter"`
}

type nullableCanvasMapString struct {
	Present bool
	Value   *string
}

func (v *nullableCanvasMapString) UnmarshalJSON(data []byte) error {
	v.Present = true
	if strings.TrimSpace(string(data)) == "null" {
		v.Value = nil
		return nil
	}

	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	v.Value = &value
	return nil
}

type canvasMapDuplicateRequest struct {
	Name string `json:"name"`
}

type canvasMapAddDeviceRequest struct {
	IncludeConnectedLinks *bool `json:"include_connected_links"`
}

type canvasMapUpdateDeviceAreasRequest struct {
	DeviceIDs []string `json:"device_ids"`
	AreaIDs   []string `json:"area_ids"`
}

func (h *CanvasMapHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	if !h.requireMapRepos(w) {
		return
	}

	maps, err := h.mapRepo.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list canvas maps", err)
		return
	}

	responses := make([]canvasMapResponse, 0, len(maps))
	for _, canvasMap := range maps {
		responses = append(responses, mapToResponse(canvasMap))
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"data": responses})
}

func (h *CanvasMapHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if !h.requireMapRepos(w) {
		return
	}

	var req canvasMapCreateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := domain.ValidateCanvasMapName(req.Name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := domain.ValidateCanvasMapDescription(req.Description); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sourceAreaID, ok := h.validateSourceAreaID(w, req.SourceAreaID)
	if !ok {
		return
	}
	sourceMapID, ok := h.validateSourceMapID(w, req.SourceMapID)
	if !ok {
		return
	}
	if !h.requireTopologyDeps(w) {
		return
	}
	materializationFilter := canvasMapMaterializationFilter(req.Filter, sourceAreaID)

	canvasMap, err := h.mapRepo.Create(domain.CanvasMapCreate{
		Name:         req.Name,
		Description:  req.Description,
		SourceAreaID: sourceAreaID,
		Filter:       materializationFilter,
	})
	if err != nil {
		h.writeMapRepoMutationError(w, err)
		return
	}
	if shouldCreateEmptyCanvasMapMembership(req.Filter, sourceAreaID) {
		if err := h.mapRepo.ReplaceMembership(canvasMap.ID, domain.CanvasMapMembership{}); err != nil {
			h.writeMapRepoMutationError(w, err)
			return
		}
	} else if sourceMapID != nil {
		if !h.replaceMaterializedMembershipFromSourceMap(w, r, canvasMap.ID, *sourceMapID, materializationFilter) {
			return
		}
	} else {
		if !h.replaceMaterializedMembership(w, r, canvasMap.ID, materializationFilter) {
			return
		}
	}
	canvasMap, err = h.mapRepo.GetByID(canvasMap.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load materialized canvas map", err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": mapToResponse(canvasMap)})
}

func (h *CanvasMapHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	if !h.requireMapRepos(w) {
		return
	}

	canvasMap, ok := h.loadMapFromRequest(w, r)
	if !ok {
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"data": mapToResponse(canvasMap)})
}

func (h *CanvasMapHandler) HandlePatch(w http.ResponseWriter, r *http.Request) {
	if !h.requireMapRepos(w) {
		return
	}

	canvasMap, ok := h.loadMapFromRequest(w, r)
	if !ok {
		return
	}

	var req canvasMapPatchRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name != nil {
		if err := domain.ValidateCanvasMapName(*req.Name); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if req.Description != nil {
		if err := domain.ValidateCanvasMapDescription(*req.Description); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if req.SourceAreaID.Present {
		if req.SourceAreaID.Value == nil {
			canvasMap.SourceAreaID = nil
		} else {
			sourceAreaID, ok := h.validateSourceAreaID(w, req.SourceAreaID.Value)
			if !ok {
				return
			}
			canvasMap.SourceAreaID = sourceAreaID
		}
	}

	updated, err := h.mapRepo.Update(canvasMap.ID, domain.CanvasMapUpdate{
		Name:            req.Name,
		Description:     req.Description,
		SourceAreaID:    canvasMap.SourceAreaID,
		SourceAreaIDSet: req.SourceAreaID.Present,
		Filter:          req.Filter,
	})
	if err != nil {
		if isCanvasMapNotFoundError(err) {
			writeError(w, http.StatusNotFound, "canvas map not found")
			return
		}
		h.writeMapRepoMutationError(w, err)
		return
	}
	if req.Filter != nil || req.SourceAreaID.Present {
		if !h.requireTopologyDeps(w) {
			return
		}
		if !h.replaceMaterializedMembership(w, r, updated.ID, projectionFilterForCanvasMap(updated)) {
			return
		}
		updated, err = h.mapRepo.GetByID(updated.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load materialized canvas map", err)
			return
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"data": mapToResponse(updated)})
}

func (h *CanvasMapHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if !h.requireMapRepos(w) {
		return
	}

	canvasMap, ok := h.loadMapFromRequest(w, r)
	if !ok {
		return
	}
	if canvasMap.IsDefault {
		writeError(w, http.StatusConflict, "cannot delete default canvas map")
		return
	}

	if err := h.mapRepo.Delete(canvasMap.ID); err != nil {
		if isCanvasMapNotFoundError(err) {
			writeError(w, http.StatusNotFound, "canvas map not found")
			return
		}
		h.writeMapRepoMutationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *CanvasMapHandler) HandleDuplicate(w http.ResponseWriter, r *http.Request) {
	if !h.requireMapRepos(w) {
		return
	}

	canvasMap, ok := h.loadMapFromRequest(w, r)
	if !ok {
		return
	}

	var req canvasMapDuplicateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := domain.ValidateCanvasMapName(req.Name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	duplicate, err := h.mapRepo.Duplicate(canvasMap.ID, req.Name)
	if err != nil {
		if isCanvasMapNotFoundError(err) {
			writeError(w, http.StatusNotFound, "canvas map not found")
			return
		}
		h.writeMapRepoMutationError(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": mapToResponse(duplicate)})
}

func (h *CanvasMapHandler) HandleRemoveDevice(w http.ResponseWriter, r *http.Request) {
	if !h.requireMapRepos(w) {
		return
	}

	canvasMap, ok := h.loadMapFromRequest(w, r)
	if !ok {
		return
	}
	_, action, ok := parseCanvasMapRoute(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "canvas map not found")
		return
	}
	deviceID, ok := parseCanvasMapDeviceAction(action)
	if !ok {
		writeError(w, http.StatusNotFound, "canvas map device not found")
		return
	}

	if err := h.mapRepo.RemoveDevice(canvasMap.ID, deviceID); err != nil {
		if isCanvasMapNotFoundError(err) {
			writeError(w, http.StatusNotFound, "canvas map not found")
			return
		}
		h.writeMapRepoMutationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *CanvasMapHandler) HandleAddDevice(w http.ResponseWriter, r *http.Request) {
	if !h.requireMapRepos(w) {
		return
	}
	if !h.requireTopologyDeps(w) {
		return
	}

	canvasMap, ok := h.loadMapFromRequest(w, r)
	if !ok {
		return
	}
	_, action, ok := parseCanvasMapRoute(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "canvas map not found")
		return
	}
	deviceID, ok := parseCanvasMapDeviceAction(action)
	if !ok {
		writeError(w, http.StatusNotFound, "canvas map device not found")
		return
	}

	req := canvasMapAddDeviceRequest{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	includeConnectedLinks := true
	if req.IncludeConnectedLinks != nil {
		includeConnectedLinks = *req.IncludeConnectedLinks
	}

	device, err := h.deviceService.GetDevice(r.Context(), deviceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "canvas map device not found")
		return
	}
	membership, err := h.mapRepo.GetMembership(canvasMap.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load canvas map membership", err)
		return
	}

	linkIDs := []uuid.UUID{}
	if includeConnectedLinks {
		links, err := h.linkRepo.GetByDeviceID(deviceID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list canvas map device links", err)
			return
		}
		linkIDs = canvasMapConnectedBaseLinkIDs(deviceID, membership, links)
	}
	areas, err := h.canvasMapAreaMembershipsForDevice(device)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load canvas map device areas", err)
		return
	}

	adder, ok := h.mapRepo.(interface {
		AddDeviceMembership(uuid.UUID, domain.CanvasMapDeviceMembership, []uuid.UUID, []domain.CanvasMapAreaMembership) error
	})
	if !ok {
		writeError(w, http.StatusNotImplemented, "canvas map incremental membership unavailable")
		return
	}
	if err := adder.AddDeviceMembership(
		canvasMap.ID,
		domain.CanvasMapDeviceMembership{
			DeviceID: deviceID,
			Role:     domain.CanvasMapDeviceRoleBase,
			AreaIDs:  append([]uuid.UUID(nil), device.AreaIDs...),
		},
		linkIDs,
		areas,
	); err != nil {
		h.writeMapRepoMutationError(w, err)
		return
	}

	updated, err := h.mapRepo.GetByID(canvasMap.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load updated canvas map", err)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"data": mapToResponse(updated)})
}

func (h *CanvasMapHandler) HandleUpdateDeviceAreas(w http.ResponseWriter, r *http.Request) {
	if !h.requireMapRepos(w) {
		return
	}

	canvasMap, ok := h.loadMapFromRequest(w, r)
	if !ok {
		return
	}

	var req canvasMapUpdateDeviceAreasRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	deviceIDs, ok := parseCanvasMapRequestUUIDs(w, req.DeviceIDs, "device_id")
	if !ok {
		return
	}
	areaIDs, ok := parseCanvasMapRequestUUIDs(w, req.AreaIDs, "area_id")
	if !ok {
		return
	}

	updater, ok := h.mapRepo.(interface {
		UpdateDeviceAreaMemberships(uuid.UUID, []uuid.UUID, []uuid.UUID) error
	})
	if !ok {
		writeError(w, http.StatusNotImplemented, "canvas map device area membership unavailable")
		return
	}
	if err := updater.UpdateDeviceAreaMemberships(canvasMap.ID, deviceIDs, areaIDs); err != nil {
		h.writeMapRepoMutationError(w, err)
		return
	}

	updated, err := h.mapRepo.GetByID(canvasMap.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load updated canvas map", err)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"data": mapToResponse(updated)})
}

func (h *CanvasMapHandler) HandleTopology(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	response, ok := h.buildMapTopologyResponse(w, r)
	if !ok {
		return
	}

	etag := `"` + response.TopologyVersion + `"`
	w.Header().Set("ETag", etag)
	if requestETagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		logCanvasTopologyResponse("/api/v1/canvas/maps/{id}/topology", http.StatusNotModified, response, startedAt)
		return
	}

	logCanvasTopologyResponse("/api/v1/canvas/maps/{id}/topology", http.StatusOK, response, startedAt)
	json.NewEncoder(w).Encode(response)
}

func (h *CanvasMapHandler) HandleBootstrap(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	response, ok := h.buildMapTopologyResponse(w, r)
	if !ok {
		return
	}

	if h.runtimeSnapshotFunc != nil {
		runtimeSnapshot, runtimeVersion := h.runtimeSnapshotFunc()
		response.RuntimeVersion = &runtimeVersion
		response.RuntimeSnapshot = ws.CloneSnapshot(runtimeSnapshot)
		response.RuntimeIdentity = ws.RuntimeIdentityForSnapshot(runtimeSnapshot)
	}

	w.Header().Set("Cache-Control", "no-store")
	logCanvasTopologyResponse("/api/v1/canvas/maps/{id}/bootstrap", http.StatusOK, response, startedAt)
	json.NewEncoder(w).Encode(response)
}

func (h *CanvasMapHandler) HandleListPositions(w http.ResponseWriter, r *http.Request) {
	if !h.requireMapRepos(w) {
		return
	}

	canvasMap, ok := h.loadMapFromRequest(w, r)
	if !ok {
		return
	}
	positions, err := h.mapPositionRepo.GetAllForMap(canvasMap.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list canvas map positions", err)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"data": positions})
}

func (h *CanvasMapHandler) HandleSavePositions(w http.ResponseWriter, r *http.Request) {
	if !h.requireMapRepos(w) {
		return
	}
	if h.deviceService == nil {
		writeError(w, http.StatusInternalServerError, "device service unavailable")
		return
	}

	canvasMap, ok := h.loadMapFromRequest(w, r)
	if !ok {
		return
	}

	var req bulkPositionsRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	knownDevices, err := h.deviceService.GetAllDevices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list devices", err)
		return
	}
	knownDeviceIDs := make(map[uuid.UUID]struct{}, len(knownDevices))
	for _, device := range knownDevices {
		knownDeviceIDs[device.ID] = struct{}{}
	}

	positions := make([]domain.DevicePosition, 0, len(req.Positions))
	for _, payload := range req.Positions {
		deviceID, err := uuid.Parse(payload.DeviceID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid device_id")
			return
		}
		if _, ok := knownDeviceIDs[deviceID]; !ok {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown device_id: %s", deviceID))
			return
		}
		if !isFiniteCoordinate(payload.X) || !isFiniteCoordinate(payload.Y) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid coordinate for device %s: NaN and Infinity are not allowed", deviceID))
			return
		}

		positions = append(positions, domain.DevicePosition{
			DeviceID: deviceID,
			X:        payload.X,
			Y:        payload.Y,
			Pinned:   payload.Pinned,
		})
	}

	if err := h.mapPositionRepo.SaveAllForMap(canvasMap.ID, positions); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save canvas map positions", err)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"count":  len(positions),
	})
}

func (h *CanvasMapHandler) buildMapTopologyResponse(w http.ResponseWriter, r *http.Request) (canvasTopologyResponse, bool) {
	if !h.requireMapRepos(w) {
		return canvasTopologyResponse{}, false
	}
	if !h.requireTopologyDeps(w) {
		return canvasTopologyResponse{}, false
	}

	canvasMap, ok := h.loadMapFromRequest(w, r)
	if !ok {
		return canvasTopologyResponse{}, false
	}

	positions, err := h.mapPositionRepo.GetAllForMap(canvasMap.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list canvas map positions", err)
		return canvasTopologyResponse{}, false
	}
	var projection canvasTopologyProjection
	var areaMembership []domain.AreaWithCount
	if canvasMap.MembershipMaterialized {
		membership, err := h.mapRepo.GetMembership(canvasMap.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load canvas map membership", err)
			return canvasTopologyResponse{}, false
		}
		devices, err := h.deviceService.GetDevicesByIDs(r.Context(), canvasMapMembershipDeviceIDs(membership.Devices))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list canvas map devices", err)
			return canvasTopologyResponse{}, false
		}
		links, err := loadCanvasMapLinksByIDs(h.linkRepo, membership.LinkIDs)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list canvas map links", err)
			return canvasTopologyResponse{}, false
		}
		projection = projectCanvasTopologyForMembership(devices, links, membership)
		areaMembership = canvasMapAreaMembershipToAreas(membership.Areas, projection.Devices)
	} else {
		projection = canvasTopologyProjection{}
		areaMembership = []domain.AreaWithCount{}
	}
	displayDevices := append([]domain.Device{}, projection.Devices...)
	displayDevices = append(displayDevices, projection.GhostDevices...)
	projectedPositions := filterPositionsForDevices(positions, displayDevices)

	response := h.canvasTopology.buildResponse(displayDevices, projection.Links, projectedPositions, areaMembership)
	mapResponse := mapToResponse(canvasMap)
	mapResponse.DeviceCount = len(projection.Devices)
	mapResponse.LinkCount = len(projection.Links)
	mapResponse.PositionCount = len(projectedPositions)
	response.Map = &mapResponse
	response.TopologyVersion = buildCanvasMapTopologyVersion(response)
	return response, true
}

func (h *CanvasMapHandler) loadMapFromRequest(w http.ResponseWriter, r *http.Request) (domain.CanvasMap, bool) {
	mapID, _, ok := parseCanvasMapRoute(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "canvas map not found")
		return domain.CanvasMap{}, false
	}
	canvasMap, err := h.mapRepo.GetByID(mapID)
	if err != nil {
		if isCanvasMapNotFoundError(err) {
			writeError(w, http.StatusNotFound, "canvas map not found")
			return domain.CanvasMap{}, false
		}
		writeError(w, http.StatusInternalServerError, "failed to load canvas map", err)
		return domain.CanvasMap{}, false
	}
	return canvasMap, true
}

func (h *CanvasMapHandler) validateSourceAreaID(w http.ResponseWriter, raw *string) (*uuid.UUID, bool) {
	if raw == nil {
		return nil, true
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		writeError(w, http.StatusBadRequest, "invalid source_area_id")
		return nil, false
	}
	areaID, err := uuid.Parse(trimmed)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid source_area_id")
		return nil, false
	}
	if h.areaRepo == nil {
		writeError(w, http.StatusInternalServerError, "area repository unavailable")
		return nil, false
	}
	if _, err := h.areaRepo.GetByID(areaID); err != nil {
		if isAreaNotFoundError(err) {
			writeError(w, http.StatusBadRequest, "unknown source_area_id")
			return nil, false
		}
		writeError(w, http.StatusInternalServerError, "failed to load source area", err)
		return nil, false
	}
	return &areaID, true
}

func (h *CanvasMapHandler) validateSourceMapID(w http.ResponseWriter, raw *string) (*uuid.UUID, bool) {
	if raw == nil {
		return nil, true
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		writeError(w, http.StatusBadRequest, "invalid source_map_id")
		return nil, false
	}
	mapID, err := uuid.Parse(trimmed)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid source_map_id")
		return nil, false
	}
	if _, err := h.mapRepo.GetByID(mapID); err != nil {
		if isCanvasMapNotFoundError(err) {
			writeError(w, http.StatusBadRequest, "unknown source_map_id")
			return nil, false
		}
		writeError(w, http.StatusInternalServerError, "failed to load source map", err)
		return nil, false
	}
	return &mapID, true
}

func (h *CanvasMapHandler) replaceMaterializedMembership(
	w http.ResponseWriter,
	r *http.Request,
	mapID uuid.UUID,
	filter domain.CanvasMapFilter,
) bool {
	devices, err := h.deviceService.GetAllDevices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list devices", err)
		return false
	}
	links, err := h.linkRepo.GetAll()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list links", err)
		return false
	}
	areas, err := h.areaRepo.GetAllWithDeviceCount()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list areas", err)
		return false
	}

	membership := materializeCanvasMapMembership(devices, links, areas, filter)
	if err := h.mapRepo.ReplaceMembership(mapID, membership); err != nil {
		h.writeMapRepoMutationError(w, err)
		return false
	}
	return true
}

func (h *CanvasMapHandler) replaceMaterializedMembershipFromSourceMap(
	w http.ResponseWriter,
	r *http.Request,
	mapID uuid.UUID,
	sourceMapID uuid.UUID,
	filter domain.CanvasMapFilter,
) bool {
	sourceMembership, err := h.mapRepo.GetMembership(sourceMapID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load source map membership", err)
		return false
	}
	devices, err := h.deviceService.GetDevicesByIDs(r.Context(), canvasMapMembershipDeviceIDs(sourceMembership.Devices))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list source map devices", err)
		return false
	}
	links, err := loadCanvasMapLinksByIDs(h.linkRepo, sourceMembership.LinkIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list source map links", err)
		return false
	}
	areaSnapshots := sourceMembership.Areas
	if len(areaSnapshots) == 0 {
		areas, err := h.areaRepo.GetAllWithDeviceCount()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list source map areas", err)
			return false
		}
		areaSnapshots = canvasMapAreasWithCountToMembership(areas)
	}

	membership := materializeCanvasMapMembershipFromSourceMap(devices, links, sourceMembership, areaSnapshots, filter)
	if err := h.mapRepo.ReplaceMembership(mapID, membership); err != nil {
		h.writeMapRepoMutationError(w, err)
		return false
	}
	if err := h.copyCanvasMapPositionsForMembership(mapID, sourceMapID, membership.Devices); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to copy source map positions", err)
		return false
	}
	return true
}

func (h *CanvasMapHandler) copyCanvasMapPositionsForMembership(
	mapID uuid.UUID,
	sourceMapID uuid.UUID,
	devices []domain.CanvasMapDeviceMembership,
) error {
	sourcePositions, err := h.mapPositionRepo.GetAllForMap(sourceMapID)
	if err != nil {
		return err
	}
	positions := filterPositionsForMemberDevices(sourcePositions, devices)
	if len(positions) == 0 {
		return nil
	}
	return h.mapPositionRepo.SaveAllForMap(mapID, positions)
}

func (h *CanvasMapHandler) requireMapRepos(w http.ResponseWriter) bool {
	if h.mapRepo == nil || h.mapPositionRepo == nil {
		writeError(w, http.StatusNotImplemented, "canvas map repository unavailable")
		return false
	}
	return true
}

func (h *CanvasMapHandler) requireTopologyDeps(w http.ResponseWriter) bool {
	if h.canvasTopology == nil || h.deviceService == nil || h.linkRepo == nil || h.areaRepo == nil {
		writeError(w, http.StatusInternalServerError, "canvas topology dependencies unavailable")
		return false
	}
	return true
}

func (h *CanvasMapHandler) writeMapRepoMutationError(w http.ResponseWriter, err error) {
	switch {
	case isCanvasMapConflictError(err):
		writeError(w, http.StatusConflict, "canvas map conflict")
	case isCanvasMapValidationError(err):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "failed to mutate canvas map", err)
	}
}

func projectionFilterForCanvasMap(canvasMap domain.CanvasMap) domain.CanvasMapFilter {
	filter, err := domain.ParseCanvasMapFilter(canvasMap.FilterJSON)
	if err != nil {
		filter, _ = domain.ParseCanvasMapFilter("")
	}
	if filter.AreaID == nil && canvasMap.SourceAreaID != nil {
		areaID := *canvasMap.SourceAreaID
		filter.AreaID = &areaID
	}
	return filter
}

func canvasMapMaterializationFilter(filter domain.CanvasMapFilter, sourceAreaID *uuid.UUID) domain.CanvasMapFilter {
	if filter.AreaID == nil && sourceAreaID != nil {
		areaID := *sourceAreaID
		filter.AreaID = &areaID
	}
	return filter
}

func shouldCreateEmptyCanvasMapMembership(filter domain.CanvasMapFilter, sourceAreaID *uuid.UUID) bool {
	return sourceAreaID == nil &&
		filter.AreaID == nil &&
		len(filter.DeviceIDs) == 0 &&
		len(filter.Tags) == 0
}

func materializeCanvasMapMembership(
	devices []domain.Device,
	links []domain.Link,
	areas []domain.AreaWithCount,
	filter domain.CanvasMapFilter,
) domain.CanvasMapMembership {
	projection := projectCanvasTopologyForMap(devices, links, filter)
	areaMemberships := canvasMapAreasForMembership(areas, projection.Devices, filter)
	includedAreaIDs := canvasMapAreaWithCountIDSet(areaMemberships)
	membership := domain.CanvasMapMembership{
		Devices: make([]domain.CanvasMapDeviceMembership, 0, len(projection.Devices)+len(projection.GhostDevices)),
		LinkIDs: make([]uuid.UUID, 0, len(projection.Links)),
		Areas:   make([]domain.CanvasMapAreaMembership, 0, len(areas)),
	}

	for _, device := range projection.Devices {
		membership.Devices = append(membership.Devices, domain.CanvasMapDeviceMembership{
			DeviceID: device.ID,
			Role:     domain.CanvasMapDeviceRoleBase,
			AreaIDs:  filterCanvasMapDeviceAreaIDs(device.AreaIDs, includedAreaIDs),
		})
	}
	for _, device := range projection.GhostDevices {
		membership.Devices = append(membership.Devices, domain.CanvasMapDeviceMembership{
			DeviceID: device.ID,
			Role:     domain.CanvasMapDeviceRoleGhost,
		})
	}
	for _, link := range projection.Links {
		membership.LinkIDs = append(membership.LinkIDs, link.ID)
	}
	for _, area := range areaMemberships {
		membership.Areas = append(membership.Areas, domain.CanvasMapAreaMembership{
			AreaID:      area.ID,
			Name:        area.Name,
			Description: area.Description,
			Color:       area.Color,
		})
	}

	return membership
}

func materializeCanvasMapMembershipFromSourceMap(
	devices []domain.Device,
	links []domain.Link,
	sourceMembership domain.CanvasMapMembership,
	areas []domain.CanvasMapAreaMembership,
	filter domain.CanvasMapFilter,
) domain.CanvasMapMembership {
	sourceProjection := projectCanvasTopologyForMembership(devices, links, sourceMembership)
	sourceDevices := append([]domain.Device{}, sourceProjection.Devices...)
	sourceDevices = append(sourceDevices, sourceProjection.GhostDevices...)
	projection := projectCanvasTopologyForMap(sourceDevices, sourceProjection.Links, filter)
	areaMemberships := canvasMapAreaSnapshotsForMembership(areas, projection.Devices, filter)
	includedAreaIDs := canvasMapAreaSnapshotIDSet(areaMemberships)
	membership := domain.CanvasMapMembership{
		Devices: make([]domain.CanvasMapDeviceMembership, 0, len(projection.Devices)+len(projection.GhostDevices)),
		LinkIDs: make([]uuid.UUID, 0, len(projection.Links)),
		Areas:   make([]domain.CanvasMapAreaMembership, 0, len(areas)),
	}

	for _, device := range projection.Devices {
		membership.Devices = append(membership.Devices, domain.CanvasMapDeviceMembership{
			DeviceID: device.ID,
			Role:     domain.CanvasMapDeviceRoleBase,
			AreaIDs:  filterCanvasMapDeviceAreaIDs(device.AreaIDs, includedAreaIDs),
		})
	}
	for _, device := range projection.GhostDevices {
		membership.Devices = append(membership.Devices, domain.CanvasMapDeviceMembership{
			DeviceID: device.ID,
			Role:     domain.CanvasMapDeviceRoleGhost,
		})
	}
	for _, link := range projection.Links {
		membership.LinkIDs = append(membership.LinkIDs, link.ID)
	}
	membership.Areas = append(membership.Areas, areaMemberships...)

	return membership
}

func canvasMapAreaWithCountIDSet(areas []domain.AreaWithCount) map[uuid.UUID]struct{} {
	ids := make(map[uuid.UUID]struct{}, len(areas))
	for _, area := range areas {
		ids[area.ID] = struct{}{}
	}
	return ids
}

func canvasMapAreaSnapshotIDSet(areas []domain.CanvasMapAreaMembership) map[uuid.UUID]struct{} {
	ids := make(map[uuid.UUID]struct{}, len(areas))
	for _, area := range areas {
		ids[area.AreaID] = struct{}{}
	}
	return ids
}

func filterCanvasMapDeviceAreaIDs(areaIDs []uuid.UUID, included map[uuid.UUID]struct{}) []uuid.UUID {
	if len(areaIDs) == 0 || len(included) == 0 {
		return []uuid.UUID{}
	}
	filtered := make([]uuid.UUID, 0, len(areaIDs))
	for _, areaID := range areaIDs {
		if _, ok := included[areaID]; ok {
			filtered = append(filtered, areaID)
		}
	}
	return filtered
}

func canvasMapAreasWithCountToMembership(areas []domain.AreaWithCount) []domain.CanvasMapAreaMembership {
	snapshots := make([]domain.CanvasMapAreaMembership, 0, len(areas))
	for _, area := range areas {
		snapshots = append(snapshots, domain.CanvasMapAreaMembership{
			AreaID:      area.ID,
			Name:        area.Name,
			Description: area.Description,
			Color:       area.Color,
		})
	}
	return snapshots
}

func canvasMapAreasForMembership(
	areas []domain.AreaWithCount,
	baseDevices []domain.Device,
	filter domain.CanvasMapFilter,
) []domain.AreaWithCount {
	includedAreaIDs := make(map[uuid.UUID]struct{})
	if filter.AreaID != nil {
		includedAreaIDs[*filter.AreaID] = struct{}{}
	} else {
		for _, device := range baseDevices {
			for _, areaID := range device.AreaIDs {
				includedAreaIDs[areaID] = struct{}{}
			}
		}
	}

	filtered := make([]domain.AreaWithCount, 0, len(includedAreaIDs))
	for _, area := range areas {
		if _, ok := includedAreaIDs[area.ID]; ok {
			filtered = append(filtered, area)
		}
	}
	return filtered
}

func canvasMapAreaSnapshotsForMembership(
	areas []domain.CanvasMapAreaMembership,
	baseDevices []domain.Device,
	filter domain.CanvasMapFilter,
) []domain.CanvasMapAreaMembership {
	includedAreaIDs := make(map[uuid.UUID]struct{})
	if filter.AreaID != nil {
		includedAreaIDs[*filter.AreaID] = struct{}{}
	} else {
		for _, device := range baseDevices {
			for _, areaID := range device.AreaIDs {
				includedAreaIDs[areaID] = struct{}{}
			}
		}
	}

	filtered := make([]domain.CanvasMapAreaMembership, 0, len(includedAreaIDs))
	for _, area := range areas {
		if _, ok := includedAreaIDs[area.AreaID]; ok {
			filtered = append(filtered, area)
		}
	}
	return filtered
}

func projectCanvasTopologyForMembership(
	devices []domain.Device,
	links []domain.Link,
	membership domain.CanvasMapMembership,
) canvasTopologyProjection {
	deviceRoles := make(map[uuid.UUID]domain.CanvasMapDeviceRole, len(membership.Devices))
	deviceAreaIDs := make(map[uuid.UUID][]uuid.UUID, len(membership.Devices))
	for _, device := range membership.Devices {
		deviceRoles[device.DeviceID] = device.Role
		deviceAreaIDs[device.DeviceID] = append([]uuid.UUID(nil), device.AreaIDs...)
	}

	linkIDs := make(map[uuid.UUID]struct{}, len(membership.LinkIDs))
	for _, linkID := range membership.LinkIDs {
		linkIDs[linkID] = struct{}{}
	}

	projection := canvasTopologyProjection{
		Devices:      []domain.Device{},
		Links:        []domain.Link{},
		GhostDevices: []domain.Device{},
	}
	for _, device := range devices {
		role, ok := deviceRoles[device.ID]
		if !ok {
			continue
		}
		device.AreaIDs = append([]uuid.UUID(nil), deviceAreaIDs[device.ID]...)
		if role == domain.CanvasMapDeviceRoleGhost {
			projection.GhostDevices = append(projection.GhostDevices, device)
			continue
		}
		projection.Devices = append(projection.Devices, device)
	}

	for _, link := range links {
		if _, ok := linkIDs[link.ID]; !ok {
			continue
		}
		if _, ok := deviceRoles[link.SourceDeviceID]; !ok {
			continue
		}
		if _, ok := deviceRoles[link.TargetDeviceID]; !ok {
			continue
		}
		projection.Links = append(projection.Links, link)
	}

	return projection
}

func canvasMapMembershipDeviceIDs(devices []domain.CanvasMapDeviceMembership) []uuid.UUID {
	if len(devices) == 0 {
		return []uuid.UUID{}
	}
	ids := make([]uuid.UUID, 0, len(devices))
	for _, device := range devices {
		ids = append(ids, device.DeviceID)
	}
	return ids
}

func loadCanvasMapLinksByIDs(repo domain.LinkRepository, ids []uuid.UUID) ([]domain.Link, error) {
	if len(ids) == 0 {
		return []domain.Link{}, nil
	}

	type linkBatchRepository interface {
		GetByIDs([]uuid.UUID) ([]domain.Link, error)
	}
	if batchRepo, ok := repo.(linkBatchRepository); ok {
		return batchRepo.GetByIDs(ids)
	}

	links, err := repo.GetAll()
	if err != nil {
		return nil, err
	}
	requested := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		requested[id] = struct{}{}
	}
	filtered := links[:0]
	for _, link := range links {
		if _, ok := requested[link.ID]; ok {
			filtered = append(filtered, link)
		}
	}
	return filtered, nil
}

func canvasMapConnectedBaseLinkIDs(deviceID uuid.UUID, membership domain.CanvasMapMembership, links []domain.Link) []uuid.UUID {
	baseDeviceIDs := make(map[uuid.UUID]struct{}, len(membership.Devices)+1)
	for _, member := range membership.Devices {
		if member.Role == domain.CanvasMapDeviceRoleBase {
			baseDeviceIDs[member.DeviceID] = struct{}{}
		}
	}
	baseDeviceIDs[deviceID] = struct{}{}

	seen := make(map[uuid.UUID]struct{}, len(links))
	linkIDs := make([]uuid.UUID, 0, len(links))
	for _, link := range links {
		if _, ok := baseDeviceIDs[link.SourceDeviceID]; !ok {
			continue
		}
		if _, ok := baseDeviceIDs[link.TargetDeviceID]; !ok {
			continue
		}
		if _, ok := seen[link.ID]; ok {
			continue
		}
		seen[link.ID] = struct{}{}
		linkIDs = append(linkIDs, link.ID)
	}
	return linkIDs
}

func (h *CanvasMapHandler) canvasMapAreaMembershipsForDevice(device *domain.Device) ([]domain.CanvasMapAreaMembership, error) {
	if device == nil || len(device.AreaIDs) == 0 {
		return []domain.CanvasMapAreaMembership{}, nil
	}
	areas := make([]domain.CanvasMapAreaMembership, 0, len(device.AreaIDs))
	for _, areaID := range device.AreaIDs {
		area, err := h.areaRepo.GetByID(areaID)
		if err != nil {
			return nil, err
		}
		areas = append(areas, domain.CanvasMapAreaMembership{
			AreaID:      area.ID,
			Name:        area.Name,
			Description: area.Description,
			Color:       area.Color,
		})
	}
	return areas, nil
}

func canvasMapAreaMembershipToAreas(
	areas []domain.CanvasMapAreaMembership,
	baseDevices []domain.Device,
) []domain.AreaWithCount {
	response := make([]domain.AreaWithCount, 0, len(areas))
	for _, area := range areas {
		response = append(response, domain.AreaWithCount{
			Area: domain.Area{
				ID:          area.AreaID,
				Name:        area.Name,
				Description: area.Description,
				Color:       area.Color,
			},
			DeviceCount: canvasMapAreaDeviceCount(area.AreaID, baseDevices),
		})
	}
	return response
}

func canvasMapAreaDeviceCount(areaID uuid.UUID, devices []domain.Device) int {
	count := 0
	for _, device := range devices {
		if deviceHasArea(device, areaID) {
			count++
		}
	}
	return count
}

func parseCanvasMapDeviceAction(action string) (uuid.UUID, bool) {
	rawDeviceID, ok := strings.CutPrefix(action, "devices/")
	if !ok || rawDeviceID == "" || strings.Contains(rawDeviceID, "/") {
		return uuid.Nil, false
	}
	deviceID, err := uuid.Parse(rawDeviceID)
	if err != nil {
		return uuid.Nil, false
	}
	return deviceID, true
}

func parseCanvasMapRequestUUIDs(w http.ResponseWriter, rawIDs []string, fieldName string) ([]uuid.UUID, bool) {
	ids := make([]uuid.UUID, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		trimmed := strings.TrimSpace(rawID)
		if trimmed == "" {
			writeError(w, http.StatusBadRequest, "invalid "+fieldName)
			return nil, false
		}
		id, err := uuid.Parse(trimmed)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid "+fieldName)
			return nil, false
		}
		ids = append(ids, id)
	}
	return ids, true
}

func filterPositionsForDevices(positions []domain.DevicePosition, devices []domain.Device) []domain.DevicePosition {
	if len(positions) == 0 || len(devices) == 0 {
		return []domain.DevicePosition{}
	}

	deviceIDs := make(map[uuid.UUID]struct{}, len(devices))
	for _, device := range devices {
		deviceIDs[device.ID] = struct{}{}
	}

	filtered := make([]domain.DevicePosition, 0, len(positions))
	for _, position := range positions {
		if _, ok := deviceIDs[position.DeviceID]; ok {
			filtered = append(filtered, position)
		}
	}
	return filtered
}

func filterPositionsForMemberDevices(
	positions []domain.DevicePosition,
	devices []domain.CanvasMapDeviceMembership,
) []domain.DevicePosition {
	if len(positions) == 0 || len(devices) == 0 {
		return []domain.DevicePosition{}
	}

	deviceIDs := make(map[uuid.UUID]struct{}, len(devices))
	for _, device := range devices {
		deviceIDs[device.DeviceID] = struct{}{}
	}

	filtered := make([]domain.DevicePosition, 0, len(positions))
	for _, position := range positions {
		if _, ok := deviceIDs[position.DeviceID]; ok {
			filtered = append(filtered, position)
		}
	}
	return filtered
}

func buildCanvasMapTopologyVersion(response canvasTopologyResponse) string {
	versionInput := buildCanvasTopologyVersionInput(
		response.Devices,
		response.Links,
		response.Positions,
		response.Areas,
		response.Capabilities,
		response.Settings,
	)
	payload := struct {
		Map      *canvasMapResponse         `json:"map"`
		Topology canvasTopologyVersionInput `json:"topology"`
	}{
		Map:      response.Map,
		Topology: versionInput,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "topo-unversioned"
	}
	sum := sha256.Sum256(data)
	return "topo-" + hex.EncodeToString(sum[:])[:16]
}

func isFiniteCoordinate(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func isCanvasMapNotFoundError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "canvas map not found")
}

func isAreaNotFoundError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "area not found")
}

func isCanvasMapConflictError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") ||
		strings.Contains(message, "duplicate key") ||
		strings.Contains(message, "constraint failed")
}

func isCanvasMapValidationError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "map name") ||
		strings.Contains(message, "map description") ||
		strings.Contains(message, "canvas map filter") ||
		strings.Contains(message, "device_id") ||
		strings.Contains(message, "area_id") ||
		strings.Contains(message, "not a member")
}
