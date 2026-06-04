package api

import (
	"context"
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
	"github.com/lollinoo/theia/internal/service/canvasmap"
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

type canvasMapPatchDeviceRequest struct {
	VisualColor nullableCanvasMapString `json:"visual_color"`
}

type canvasMapAreaRepository interface {
	ListAreas(uuid.UUID) ([]domain.AreaWithCount, error)
	CreateArea(uuid.UUID, domain.CanvasMapAreaMembership) (domain.AreaWithCount, error)
	UpdateArea(uuid.UUID, uuid.UUID, domain.CanvasMapAreaMembership) (domain.AreaWithCount, error)
	DeleteArea(uuid.UUID, uuid.UUID) error
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
	sourceMapID, ok := h.validateSourceMapID(w, req.SourceMapID)
	if !ok {
		return
	}
	sourceAreaID, ok := h.validateCreateSourceAreaID(w, req.SourceAreaID, sourceMapID)
	if !ok {
		return
	}
	if !h.requireTopologyDeps(w) {
		return
	}
	materializationFilter := canvasmap.MaterializationFilter(req.Filter, sourceAreaID)
	persistedSourceAreaID := sourceAreaID
	if sourceMapID != nil {
		persistedSourceAreaID = nil
	}

	canvasMap, err := h.mapRepo.Create(domain.CanvasMapCreate{
		Name:         req.Name,
		Description:  req.Description,
		SourceAreaID: persistedSourceAreaID,
		Filter:       materializationFilter,
	})
	if err != nil {
		h.writeMapRepoMutationError(w, err)
		return
	}
	if canvasmap.ShouldCreateEmptyMembership(req.Filter, sourceAreaID) {
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
		if !h.copyDefaultCanvasMapPositionsForMaterializedMembership(w, canvasMap.ID) {
			return
		}
	}
	if err := h.isolateCanvasMapVirtualDevices(r.Context(), canvasMap.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to isolate canvas map virtual devices", err)
		return
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
		if !h.replaceMaterializedMembership(w, r, updated.ID, canvasmap.ProjectionFilterForMap(updated)) {
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
	if err := h.isolateCanvasMapVirtualDevices(r.Context(), duplicate.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to isolate duplicated virtual devices", err)
		return
	}
	duplicate, err = h.mapRepo.GetByID(duplicate.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load duplicated canvas map", err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": mapToResponse(duplicate)})
}

func (h *CanvasMapHandler) HandleSetPrimary(w http.ResponseWriter, r *http.Request) {
	if !h.requireMapRepos(w) {
		return
	}

	canvasMap, ok := h.loadMapFromRequest(w, r)
	if !ok {
		return
	}

	updated, err := h.mapRepo.SetPrimary(canvasMap.ID)
	if err != nil {
		if isCanvasMapNotFoundError(err) {
			writeError(w, http.StatusNotFound, "canvas map not found")
			return
		}
		h.writeMapRepoMutationError(w, err)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"data": mapToResponse(updated)})
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
	adder, ok := h.mapRepo.(interface {
		AddDeviceMembership(uuid.UUID, domain.CanvasMapDeviceMembership, []uuid.UUID, []domain.CanvasMapAreaMembership) error
	})
	if !ok {
		writeError(w, http.StatusNotImplemented, "canvas map incremental membership unavailable")
		return
	}
	for _, member := range membership.Devices {
		if member.DeviceID == deviceID {
			if includeConnectedLinks {
				links, err := h.linkRepo.GetByDeviceID(deviceID)
				if err != nil {
					writeError(w, http.StatusInternalServerError, "failed to list canvas map device links", err)
					return
				}
				linkIDs := canvasmap.ConnectedBaseLinkIDs(deviceID, membership, links)
				missingLinkIDs := canvasMapMissingLinkIDs(membership.LinkIDs, linkIDs)
				if len(missingLinkIDs) > 0 {
					if err := adder.AddDeviceMembership(canvasMap.ID, member, linkIDs, membership.Areas); err != nil {
						h.writeMapRepoMutationError(w, err)
						return
					}
					updated, err := h.mapRepo.GetByID(canvasMap.ID)
					if err != nil {
						writeError(w, http.StatusInternalServerError, "failed to load updated canvas map", err)
						return
					}
					json.NewEncoder(w).Encode(map[string]interface{}{"data": mapToResponse(updated)})
					return
				}
			}
			writeError(w, http.StatusConflict, "device already exists in this map")
			return
		}
	}
	if address := normalizeCanvasMapDeviceAddress(device.IP); address != "" {
		memberDevices, err := h.deviceService.GetDevicesByIDs(r.Context(), canvasmap.MembershipDeviceIDs(membership.Devices))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list canvas map devices", err)
			return
		}
		for _, memberDevice := range memberDevices {
			if memberDevice.ID == deviceID {
				continue
			}
			if normalizeCanvasMapDeviceAddress(memberDevice.IP) == address {
				writeError(w, http.StatusConflict, duplicateCanvasMapDeviceAddressMessage(device.IP))
				return
			}
		}
	}

	linkIDs := []uuid.UUID{}
	if includeConnectedLinks {
		links, err := h.linkRepo.GetByDeviceID(deviceID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list canvas map device links", err)
			return
		}
		linkIDs = canvasmap.ConnectedBaseLinkIDs(deviceID, membership, links)
	}
	areas, err := h.canvasMapAreaMembershipsForDevice(device)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load canvas map device areas", err)
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

func canvasMapMissingLinkIDs(existing []uuid.UUID, candidates []uuid.UUID) []uuid.UUID {
	if len(candidates) == 0 {
		return []uuid.UUID{}
	}
	known := make(map[uuid.UUID]struct{}, len(existing))
	for _, id := range existing {
		known[id] = struct{}{}
	}
	missing := make([]uuid.UUID, 0, len(candidates))
	for _, id := range candidates {
		if _, ok := known[id]; ok {
			continue
		}
		missing = append(missing, id)
	}
	return missing
}

func (h *CanvasMapHandler) HandlePatchDevice(w http.ResponseWriter, r *http.Request) {
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

	var req canvasMapPatchDeviceRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !req.VisualColor.Present {
		writeError(w, http.StatusBadRequest, "visual_color is required")
		return
	}
	visualColor, err := canvasmap.NormalizeVisualColor(req.VisualColor.Value)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	device, err := h.deviceService.GetDevice(r.Context(), deviceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "canvas map device not found")
		return
	}
	if device.DeviceType != domain.DeviceTypeVirtual {
		writeError(w, http.StatusBadRequest, "visual_color is only supported for virtual devices")
		return
	}

	updater, ok := h.mapRepo.(interface {
		UpdateDeviceVisualColor(uuid.UUID, uuid.UUID, *string) error
	})
	if !ok {
		writeError(w, http.StatusNotImplemented, "canvas map device visual color unavailable")
		return
	}
	if err := updater.UpdateDeviceVisualColor(canvasMap.ID, deviceID, visualColor); err != nil {
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

func (h *CanvasMapHandler) HandleListAreas(w http.ResponseWriter, r *http.Request) {
	if !h.requireMapRepos(w) {
		return
	}

	canvasMap, ok := h.loadMapFromRequest(w, r)
	if !ok {
		return
	}
	areaRepo, ok := h.mapAreaRepo(w)
	if !ok {
		return
	}

	areas, err := areaRepo.ListAreas(canvasMap.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list canvas map areas", err)
		return
	}

	response := make([]areaResponse, 0, len(areas))
	for i := range areas {
		response = append(response, areaToResponse(&areas[i].Area, areas[i].DeviceCount))
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"data": response})
}

func (h *CanvasMapHandler) HandleCreateArea(w http.ResponseWriter, r *http.Request) {
	if !h.requireMapRepos(w) {
		return
	}

	canvasMap, ok := h.loadMapFromRequest(w, r)
	if !ok {
		return
	}
	areaRepo, ok := h.mapAreaRepo(w)
	if !ok {
		return
	}

	var req areaRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	area, ok := canvasMapAreaMembershipFromRequest(w, req)
	if !ok {
		return
	}

	created, err := areaRepo.CreateArea(canvasMap.ID, area)
	if err != nil {
		h.writeCanvasMapAreaMutationError(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": areaToResponse(&created.Area, created.DeviceCount)})
}

func (h *CanvasMapHandler) HandleUpdateArea(w http.ResponseWriter, r *http.Request) {
	if !h.requireMapRepos(w) {
		return
	}

	canvasMap, ok := h.loadMapFromRequest(w, r)
	if !ok {
		return
	}
	areaRepo, ok := h.mapAreaRepo(w)
	if !ok {
		return
	}
	areaID, ok := parseCanvasMapAreaActionID(w, r)
	if !ok {
		return
	}

	var req areaRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	area, ok := canvasMapAreaMembershipFromRequest(w, req)
	if !ok {
		return
	}

	updated, err := areaRepo.UpdateArea(canvasMap.ID, areaID, area)
	if err != nil {
		h.writeCanvasMapAreaMutationError(w, err)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"data": areaToResponse(&updated.Area, updated.DeviceCount)})
}

func (h *CanvasMapHandler) HandleDeleteArea(w http.ResponseWriter, r *http.Request) {
	if !h.requireMapRepos(w) {
		return
	}

	canvasMap, ok := h.loadMapFromRequest(w, r)
	if !ok {
		return
	}
	areaRepo, ok := h.mapAreaRepo(w)
	if !ok {
		return
	}
	areaID, ok := parseCanvasMapAreaActionID(w, r)
	if !ok {
		return
	}

	if err := areaRepo.DeleteArea(canvasMap.ID, areaID); err != nil {
		h.writeCanvasMapAreaMutationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	if err := h.isolateCanvasMapVirtualDevices(r.Context(), canvasMap.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to isolate canvas map virtual devices", err)
		return canvasTopologyResponse{}, false
	}
	canvasMap, err := h.mapRepo.GetByID(canvasMap.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load canvas map", err)
		return canvasTopologyResponse{}, false
	}

	positions, err := h.mapPositionRepo.GetAllForMap(canvasMap.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list canvas map positions", err)
		return canvasTopologyResponse{}, false
	}
	var projection canvasmap.TopologyProjection
	var areaMembership []domain.AreaWithCount
	var deviceMembership []domain.CanvasMapDeviceMembership
	if canvasMap.MembershipMaterialized {
		membership, err := h.mapRepo.GetMembership(canvasMap.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load canvas map membership", err)
			return canvasTopologyResponse{}, false
		}
		devices, err := h.deviceService.GetDevicesByIDs(r.Context(), canvasmap.MembershipDeviceIDs(membership.Devices))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list canvas map devices", err)
			return canvasTopologyResponse{}, false
		}
		links, err := loadCanvasMapLinksByIDs(h.linkRepo, membership.LinkIDs)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list canvas map links", err)
			return canvasTopologyResponse{}, false
		}
		deviceMembership = membership.Devices
		projection = canvasmap.ProjectTopologyForMembership(devices, links, membership)
		areaMembership = canvasmap.AreaMembershipToAreas(membership.Areas, projection.Devices)
	} else {
		projection = canvasmap.TopologyProjection{}
		areaMembership = []domain.AreaWithCount{}
	}
	displayDevices := append([]domain.Device{}, projection.Devices...)
	displayDevices = append(displayDevices, projection.GhostDevices...)
	projectedPositions := canvasmap.FilterPositionsForDevices(positions, displayDevices)

	response := h.canvasTopology.buildResponse(displayDevices, projection.Links, projectedPositions, areaMembership)
	applyCanvasMapDeviceVisualColors(response.Devices, deviceMembership)
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

func (h *CanvasMapHandler) validateCreateSourceAreaID(
	w http.ResponseWriter,
	raw *string,
	sourceMapID *uuid.UUID,
) (*uuid.UUID, bool) {
	if raw == nil {
		return nil, true
	}
	if sourceMapID == nil {
		return h.validateSourceAreaID(w, raw)
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

	membership, err := h.mapRepo.GetMembership(*sourceMapID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load source map membership", err)
		return nil, false
	}
	for _, area := range membership.Areas {
		if area.AreaID == areaID {
			return &areaID, true
		}
	}

	writeError(w, http.StatusBadRequest, "unknown source_area_id")
	return nil, false
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

	membership := canvasmap.MaterializeMembership(devices, links, areas, filter)
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
	devices, err := h.deviceService.GetDevicesByIDs(r.Context(), canvasmap.MembershipDeviceIDs(sourceMembership.Devices))
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
		areaSnapshots = canvasmap.AreasWithCountToMembership(areas)
	}

	membership := canvasmap.MaterializeMembershipFromSourceMap(devices, links, sourceMembership, areaSnapshots, filter)
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
	positions := canvasmap.FilterPositionsForMemberDevices(sourcePositions, devices)
	if len(positions) == 0 {
		return nil
	}
	return h.mapPositionRepo.SaveAllForMap(mapID, positions)
}

func (h *CanvasMapHandler) copyDefaultCanvasMapPositionsForMaterializedMembership(
	w http.ResponseWriter,
	mapID uuid.UUID,
) bool {
	defaultMap, err := h.mapRepo.GetDefault()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load default canvas map", err)
		return false
	}
	if defaultMap.ID == mapID {
		return true
	}
	membership, err := h.mapRepo.GetMembership(mapID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load materialized canvas map membership", err)
		return false
	}
	sourcePositions, err := h.mapPositionRepo.GetAllForMap(defaultMap.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load default canvas map positions", err)
		return false
	}
	if len(sourcePositions) == 0 && h.legacyPositionRepo != nil {
		sourcePositions, err = h.legacyPositionRepo.GetAll()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load legacy canvas positions", err)
			return false
		}
	}
	positions := canvasmap.FilterPositionsForMemberDevices(sourcePositions, membership.Devices)
	if len(positions) == 0 {
		return true
	}
	if err := h.mapPositionRepo.SaveAllForMap(mapID, positions); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to copy default canvas map positions", err)
		return false
	}
	return true
}

func (h *CanvasMapHandler) isolateCanvasMapVirtualDevices(ctx context.Context, mapID uuid.UUID) error {
	membership, err := h.mapRepo.GetMembership(mapID)
	if err != nil {
		return fmt.Errorf("loading canvas map membership: %w", err)
	}
	if len(membership.Devices) == 0 {
		return nil
	}
	if h.deviceService == nil || h.linkRepo == nil {
		return fmt.Errorf("canvas map virtual device isolation dependencies unavailable")
	}

	devices, err := h.deviceService.GetDevicesByIDs(ctx, canvasmap.MembershipDeviceIDs(membership.Devices))
	if err != nil {
		return fmt.Errorf("loading canvas map devices: %w", err)
	}
	deviceByID := make(map[uuid.UUID]domain.Device, len(devices))
	for _, device := range devices {
		deviceByID[device.ID] = device
	}
	virtualMemberIDs := make(map[uuid.UUID]struct{})
	for _, member := range membership.Devices {
		device, ok := deviceByID[member.DeviceID]
		if !ok {
			return fmt.Errorf("canvas map member device %s not found", member.DeviceID)
		}
		if device.DeviceType == domain.DeviceTypeVirtual {
			virtualMemberIDs[member.DeviceID] = struct{}{}
		}
	}
	if len(virtualMemberIDs) == 0 {
		return nil
	}
	sharedVirtualIDs, err := h.sharedCanvasMapDeviceIDs(mapID, virtualMemberIDs)
	if err != nil {
		return err
	}
	if len(sharedVirtualIDs) == 0 {
		return nil
	}

	clonedDeviceIDs := make(map[uuid.UUID]uuid.UUID)
	nextMembership := domain.CanvasMapMembership{
		Devices: make([]domain.CanvasMapDeviceMembership, 0, len(membership.Devices)),
		Areas:   append([]domain.CanvasMapAreaMembership(nil), membership.Areas...),
	}
	for _, member := range membership.Devices {
		device, ok := deviceByID[member.DeviceID]
		if !ok {
			return fmt.Errorf("canvas map member device %s not found", member.DeviceID)
		}

		nextMember := domain.CanvasMapDeviceMembership{
			DeviceID:    member.DeviceID,
			Role:        member.Role,
			AreaIDs:     append([]uuid.UUID(nil), member.AreaIDs...),
			VisualColor: cloneOptionalString(member.VisualColor),
		}
		if _, shared := sharedVirtualIDs[member.DeviceID]; shared && device.DeviceType == domain.DeviceTypeVirtual {
			clone, err := h.cloneCanvasMapVirtualDevice(ctx, device)
			if err != nil {
				return err
			}
			clonedDeviceIDs[member.DeviceID] = clone.ID
			nextMember.DeviceID = clone.ID
		}
		nextMembership.Devices = append(nextMembership.Devices, nextMember)
	}
	if len(clonedDeviceIDs) == 0 {
		return nil
	}

	links, err := loadCanvasMapLinksByIDs(h.linkRepo, membership.LinkIDs)
	if err != nil {
		return fmt.Errorf("loading canvas map links: %w", err)
	}
	nextMembership.LinkIDs = make([]uuid.UUID, 0, len(links))
	for _, link := range links {
		nextLinkID, err := h.cloneCanvasMapLinkForVirtualDevices(link, clonedDeviceIDs)
		if err != nil {
			return err
		}
		nextMembership.LinkIDs = append(nextMembership.LinkIDs, nextLinkID)
	}

	positions, err := h.mapPositionRepo.GetAllForMap(mapID)
	if err != nil {
		return fmt.Errorf("loading canvas map positions: %w", err)
	}
	nextPositions := remapCanvasMapPositionsForDeviceClones(
		positions,
		clonedDeviceIDs,
		nextMembership.Devices,
	)

	if err := h.mapRepo.ReplaceMembership(mapID, nextMembership); err != nil {
		return fmt.Errorf("replacing canvas map membership with cloned virtual devices: %w", err)
	}
	if len(nextPositions) > 0 {
		if err := h.mapPositionRepo.SaveAllForMap(mapID, nextPositions); err != nil {
			return fmt.Errorf("saving cloned virtual device positions: %w", err)
		}
	}
	return nil
}

func (h *CanvasMapHandler) sharedCanvasMapDeviceIDs(
	mapID uuid.UUID,
	deviceIDs map[uuid.UUID]struct{},
) (map[uuid.UUID]struct{}, error) {
	canvasMaps, err := h.mapRepo.List()
	if err != nil {
		return nil, fmt.Errorf("listing canvas maps for virtual isolation: %w", err)
	}
	shared := make(map[uuid.UUID]struct{})
	for _, canvasMap := range canvasMaps {
		if canvasMap.ID == mapID {
			continue
		}
		membership, err := h.mapRepo.GetMembership(canvasMap.ID)
		if err != nil {
			return nil, fmt.Errorf("loading canvas map %s membership for virtual isolation: %w", canvasMap.ID, err)
		}
		for _, member := range membership.Devices {
			if _, ok := deviceIDs[member.DeviceID]; ok {
				shared[member.DeviceID] = struct{}{}
			}
		}
	}
	return shared, nil
}

func (h *CanvasMapHandler) cloneCanvasMapVirtualDevice(ctx context.Context, device domain.Device) (*domain.Device, error) {
	notes := cloneOptionalString(device.Notes)
	clone, err := h.deviceService.AddDevice(
		ctx,
		device.IP,
		device.Hostname,
		domain.DeviceTypeVirtual,
		domain.SNMPCredentials{},
		cloneStringMap(device.Tags),
		device.Vendor,
		domain.MetricsSourceNone,
		device.PrometheusLabelName,
		device.PrometheusLabelValue,
		device.TopologyDiscoveryMode,
		nil,
		notes,
	)
	if err != nil {
		return nil, fmt.Errorf("cloning virtual device %s: %w", device.ID, err)
	}

	var sourceOverride *int
	if device.PollIntervalOverride != nil {
		value := *device.PollIntervalOverride
		sourceOverride = &value
	}
	update := service.DeviceUpdate{PollIntervalOverride: &sourceOverride}
	if device.PollingEnabled != nil {
		value := *device.PollingEnabled
		update.PollingEnabled = &value
	}
	if err := h.deviceService.UpdateDevice(ctx, clone.ID, update); err != nil {
		return nil, fmt.Errorf("updating cloned virtual device %s: %w", clone.ID, err)
	}

	reloaded, err := h.deviceService.GetDevice(ctx, clone.ID)
	if err != nil {
		return nil, fmt.Errorf("loading cloned virtual device %s: %w", clone.ID, err)
	}
	return reloaded, nil
}

func (h *CanvasMapHandler) cloneCanvasMapLinkForVirtualDevices(
	link domain.Link,
	clonedDeviceIDs map[uuid.UUID]uuid.UUID,
) (uuid.UUID, error) {
	sourceID := link.SourceDeviceID
	targetID := link.TargetDeviceID
	cloned := false
	if cloneID, ok := clonedDeviceIDs[sourceID]; ok {
		sourceID = cloneID
		cloned = true
	}
	if cloneID, ok := clonedDeviceIDs[targetID]; ok {
		targetID = cloneID
		cloned = true
	}
	if !cloned {
		return link.ID, nil
	}

	nextLink := &domain.Link{
		SourceDeviceID:    sourceID,
		SourceIfName:      link.SourceIfName,
		TargetDeviceID:    targetID,
		TargetIfName:      link.TargetIfName,
		DiscoveryProtocol: link.DiscoveryProtocol,
	}
	if err := h.linkRepo.Create(nextLink); err != nil {
		return uuid.Nil, fmt.Errorf("cloning canvas map link %s: %w", link.ID, err)
	}
	return nextLink.ID, nil
}

func remapCanvasMapPositionsForDeviceClones(
	positions []domain.DevicePosition,
	clonedDeviceIDs map[uuid.UUID]uuid.UUID,
	members []domain.CanvasMapDeviceMembership,
) []domain.DevicePosition {
	memberIDs := make(map[uuid.UUID]struct{}, len(members))
	for _, member := range members {
		memberIDs[member.DeviceID] = struct{}{}
	}

	nextPositions := make([]domain.DevicePosition, 0, len(positions))
	for _, position := range positions {
		if cloneID, ok := clonedDeviceIDs[position.DeviceID]; ok {
			position.DeviceID = cloneID
		}
		if _, ok := memberIDs[position.DeviceID]; ok {
			nextPositions = append(nextPositions, position)
		}
	}
	return nextPositions
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
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

func (h *CanvasMapHandler) mapAreaRepo(w http.ResponseWriter) (canvasMapAreaRepository, bool) {
	areaRepo, ok := h.mapRepo.(canvasMapAreaRepository)
	if !ok {
		writeError(w, http.StatusNotImplemented, "canvas map area repository unavailable")
		return nil, false
	}
	return areaRepo, true
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

func (h *CanvasMapHandler) writeCanvasMapAreaMutationError(w http.ResponseWriter, err error) {
	switch {
	case isCanvasMapNotFoundError(err), isAreaNotFoundError(err):
		writeError(w, http.StatusNotFound, err.Error())
	case isCanvasMapConflictError(err):
		writeError(w, http.StatusConflict, "an area with that name already exists")
	case isCanvasMapValidationError(err):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "failed to mutate canvas map area", err)
	}
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

func parseCanvasMapAreaActionID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	_, action, ok := parseCanvasMapRoute(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return uuid.Nil, false
	}
	rawAreaID, ok := strings.CutPrefix(action, "areas/")
	if !ok || rawAreaID == "" || strings.Contains(rawAreaID, "/") {
		writeError(w, http.StatusNotFound, "not found")
		return uuid.Nil, false
	}
	areaID, err := uuid.Parse(rawAreaID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid area ID")
		return uuid.Nil, false
	}
	return areaID, true
}

func canvasMapAreaMembershipFromRequest(
	w http.ResponseWriter,
	req areaRequest,
) (domain.CanvasMapAreaMembership, bool) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return domain.CanvasMapAreaMembership{}, false
	}
	if len(name) > 100 {
		writeError(w, http.StatusBadRequest, "area name too long (max 100 characters)")
		return domain.CanvasMapAreaMembership{}, false
	}

	color := strings.TrimSpace(req.Color)
	if color == "" {
		color = "#00E676"
	}
	if !strings.HasPrefix(color, "#") || len(color) != 7 {
		writeError(w, http.StatusBadRequest, "invalid color format (must be #RRGGBB)")
		return domain.CanvasMapAreaMembership{}, false
	}

	return domain.CanvasMapAreaMembership{
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Color:       color,
	}, true
}

func applyCanvasMapDeviceVisualColors(
	devices []jsonAPIResource,
	membership []domain.CanvasMapDeviceMembership,
) {
	if len(devices) == 0 || len(membership) == 0 {
		return
	}
	visualColors := canvasmap.VisualColorsByDeviceID(membership)
	if len(visualColors) == 0 {
		return
	}
	for i := range devices {
		deviceID, err := uuid.Parse(devices[i].ID)
		if err != nil {
			continue
		}
		color, ok := visualColors[deviceID]
		if !ok {
			continue
		}
		if devices[i].Attributes == nil {
			devices[i].Attributes = map[string]interface{}{}
		}
		devices[i].Attributes["map_visual_color"] = color
	}
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

func normalizeCanvasMapDeviceAddress(address string) string {
	return strings.ToLower(strings.TrimSpace(address))
}

func duplicateCanvasMapDeviceAddressMessage(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return "a device with that address already exists in this map"
	}
	return fmt.Sprintf("a device with IP/host %q already exists in this map", address)
}

func isCanvasMapConflictError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") ||
		strings.Contains(message, "duplicate key") ||
		strings.Contains(message, "constraint failed") ||
		strings.Contains(message, "already exists")
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
