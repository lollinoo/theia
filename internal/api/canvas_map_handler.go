package api

// This file defines canvas map handler HTTP handler behavior and request/response boundaries.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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

// CanvasMapHandler represents canvas map handler data used by the HTTP boundary and route policy.
type CanvasMapHandler struct {
	mapRepo            domain.CanvasMapRepository
	mapPositionRepo    domain.CanvasMapPositionRepository
	legacyPositionRepo domain.PositionRepository
	canvasTopology     *CanvasTopologyHandler
	deviceService      *service.DeviceService
	linkRepo           domain.LinkRepository
	linkRouteRepo      domain.CanvasMapLinkRouteRepository
	areaRepo           domain.AreaRepository
	runtimeStateFunc   ws.RuntimeOverviewStateFunc
}

// NewCanvasMapHandler wires the HTTP adapter to saved-map repositories and topology collaborators.
func NewCanvasMapHandler(
	mapRepo domain.CanvasMapRepository,
	mapPositionRepo domain.CanvasMapPositionRepository,
	legacyPositionRepo domain.PositionRepository,
	canvasTopology *CanvasTopologyHandler,
	deviceService *service.DeviceService,
	linkRepo domain.LinkRepository,
	linkRouteRepo domain.CanvasMapLinkRouteRepository,
	areaRepo domain.AreaRepository,
	runtimeStateFunc ws.RuntimeOverviewStateFunc,
) *CanvasMapHandler {
	return &CanvasMapHandler{
		mapRepo:            mapRepo,
		mapPositionRepo:    mapPositionRepo,
		legacyPositionRepo: legacyPositionRepo,
		canvasTopology:     canvasTopology,
		deviceService:      deviceService,
		linkRepo:           linkRepo,
		linkRouteRepo:      linkRouteRepo,
		areaRepo:           areaRepo,
		runtimeStateFunc:   runtimeStateFunc,
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

// UnmarshalJSON records whether a nullable string field was present and whether it was null.
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

type canvasLinkRouteRequest struct {
	Version   int                              `json:"version"`
	Waypoints []canvasLinkRouteWaypointRequest `json:"waypoints"`
}

type canvasLinkRouteWaypointRequest struct {
	X *float64 `json:"x"`
	Y *float64 `json:"y"`
}

func (r canvasLinkRouteRequest) domainWaypoints() ([]domain.CanvasPoint, error) {
	waypoints := make([]domain.CanvasPoint, 0, len(r.Waypoints))
	for i, waypoint := range r.Waypoints {
		if waypoint.X == nil {
			return nil, fmt.Errorf("waypoints[%d].x is required", i)
		}
		if waypoint.Y == nil {
			return nil, fmt.Errorf("waypoints[%d].y is required", i)
		}
		waypoints = append(waypoints, domain.CanvasPoint{X: *waypoint.X, Y: *waypoint.Y})
	}
	return waypoints, nil
}

type canvasMapAreaRepository interface {
	ListAreas(uuid.UUID) ([]domain.AreaWithCount, error)
	CreateArea(uuid.UUID, domain.CanvasMapAreaMembership) (domain.AreaWithCount, error)
	UpdateArea(uuid.UUID, uuid.UUID, domain.CanvasMapAreaMembership) (domain.AreaWithCount, error)
	DeleteArea(uuid.UUID, uuid.UUID) error
}

// HandleList encodes all saved maps for the current operator.
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

// HandleCreate decodes saved-map creation input and delegates materialization rules to canvasmap services.
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
	createPlan := canvasmap.PlanCreate(req.Filter, sourceAreaID, sourceMapID)

	canvasMap, err := h.mapRepo.Create(domain.CanvasMapCreate{
		Name:         req.Name,
		Description:  req.Description,
		SourceAreaID: createPlan.PersistedSourceAreaID,
		Filter:       createPlan.Filter,
	})
	if err != nil {
		h.writeMapRepoMutationError(w, err)
		return
	}
	if createPlan.CreateEmptyMembership {
		if err := h.mapRepo.ReplaceMembership(canvasMap.ID, domain.CanvasMapMembership{}); err != nil {
			h.writeMapRepoMutationError(w, err)
			return
		}
	} else if createPlan.SourceMapID != nil {
		if !h.replaceMaterializedMembershipFromSourceMap(w, r, canvasMap.ID, *createPlan.SourceMapID, createPlan.Filter) {
			return
		}
	} else {
		if !h.replaceMaterializedMembership(w, r, canvasMap.ID, createPlan.Filter) {
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

// HandleGet parses the map path and returns one saved-map DTO.
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

// HandlePatch applies DTO field presence semantics and refreshes materialized membership when needed.
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

// HandleDelete rejects default-map deletion and maps repository delete errors to HTTP responses.
func (h *CanvasMapHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if !h.requireMapRepos(w) {
		return
	}

	canvasMap, ok := h.loadMapFromRequest(w, r)
	if !ok {
		return
	}
	if err := canvasmap.ValidateDelete(canvasMap); err != nil {
		writeError(w, http.StatusConflict, err.Error())
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

// HandleSaveLinkRoute validates and persists one map-local route for a canonical link.
func (h *CanvasMapHandler) HandleSaveLinkRoute(w http.ResponseWriter, r *http.Request) {
	if !h.requireLinkRouteRepos(w) {
		return
	}

	mapID, linkID, ok := parseCanvasMapLinkRouteIDs(w, r)
	if !ok {
		return
	}
	if !h.loadCanvasMapForLinkRoute(w, mapID) {
		return
	}
	link, err := h.linkRepo.GetByID(linkID)
	if err != nil {
		if isLinkNotFoundError(err) {
			writeError(w, http.StatusNotFound, "link not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load link", err)
		return
	}
	if link == nil {
		writeError(w, http.StatusNotFound, "link not found")
		return
	}

	var req canvasLinkRouteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	waypoints, err := req.domainWaypoints()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	route := domain.CanvasMapLinkRoute{
		LinkID:    linkID,
		Version:   req.Version,
		Waypoints: waypoints,
	}
	if err := domain.ValidateCanvasMapLinkRoute(route); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	saved, err := h.linkRouteRepo.UpsertForMap(r.Context(), mapID, route)
	if err != nil {
		h.writeCanvasMapLinkRouteMutationError(w, "save", err)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"data": canvasLinkRouteToResponse(saved)})
}

// HandleDeleteLinkRoute removes one map-local route and restores automatic routing.
func (h *CanvasMapHandler) HandleDeleteLinkRoute(w http.ResponseWriter, r *http.Request) {
	if !h.requireLinkRouteRepos(w) {
		return
	}

	mapID, linkID, ok := parseCanvasMapLinkRouteIDs(w, r)
	if !ok {
		return
	}
	if !h.loadCanvasMapForLinkRoute(w, mapID) {
		return
	}
	link, err := h.linkRepo.GetByID(linkID)
	if err != nil {
		if isLinkNotFoundError(err) {
			writeError(w, http.StatusNotFound, "link not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load link", err)
		return
	}
	if link == nil {
		writeError(w, http.StatusNotFound, "link not found")
		return
	}
	if err := h.linkRouteRepo.DeleteForMap(r.Context(), mapID, linkID); err != nil {
		h.writeCanvasMapLinkRouteMutationError(w, "delete", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleDuplicate clones a saved map through repository duplication while preserving HTTP error mapping.
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

// HandleSetPrimary promotes a map to primary and returns the updated map DTO.
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

// HandleRemoveDevice removes one map-local device membership parsed from the route action.
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

// HandleAddDevice adds map-local device membership and optionally connected links.
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

	adder, ok := h.mapRepo.(canvasmap.AddDeviceMembershipMapRepository)
	if !ok {
		writeError(w, http.StatusNotImplemented, "canvas map incremental membership unavailable")
		return
	}
	if err := canvasmap.AddDeviceToMaterializedMembership(r.Context(), canvasMap.ID, deviceID, includeConnectedLinks, canvasmap.AddDeviceMembershipDeps{
		Maps:    adder,
		Devices: h.deviceService,
		Links:   h.linkRepo,
		Areas:   h.areaRepo,
	}); err != nil {
		h.writeCanvasMapAddDeviceWorkflowError(w, err)
		return
	}

	updated, err := h.mapRepo.GetByID(canvasMap.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load updated canvas map", err)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"data": mapToResponse(updated)})
}

// HandlePatchDevice applies map-local device metadata updates such as visual_color.
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
	if err := canvasmap.ValidateVisualColorDevice(*device); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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

// HandleListAreas returns map-local areas with their saved-map member counts.
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

// HandleCreateArea decodes and persists one map-local area.
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

// HandleUpdateArea parses the map-local area route and persists area metadata changes.
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

// HandleDeleteArea deletes one map-local area and its saved-map device assignments.
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

// HandleUpdateDeviceAreas replaces area assignments for selected saved-map member devices.
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

// HandleTopology returns the saved-map topology response for React Flow.
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

// HandleBootstrap returns topology plus optional runtime bootstrap payload.
func (h *CanvasMapHandler) HandleBootstrap(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	response, ok := h.buildMapTopologyResponse(w, r)
	if !ok {
		return
	}

	if h.runtimeStateFunc != nil {
		applyRuntimeOverviewState(&response, h.runtimeStateFunc())
	}

	w.Header().Set("Cache-Control", "no-store")
	logCanvasTopologyResponse("/api/v1/canvas/maps/{id}/bootstrap", http.StatusOK, response, startedAt)
	json.NewEncoder(w).Encode(response)
}

// HandleListPositions encodes all saved positions for one map.
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

// HandleSavePositions validates finite coordinates and persists map-local positions.
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

// buildMapTopologyResponse keeps the HTTP response shape while canvasmap loads and projects saved-map topology.
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
	loaded, err := canvasmap.LoadTopology(r.Context(), canvasMap.ID, canvasmap.TopologyLoadDeps{
		Maps:      h.mapRepo,
		Positions: h.mapPositionRepo,
		Devices:   canvasMapVirtualIsolationDeviceService{service: h.deviceService},
		Links:     h.linkRepo,
	})
	if err != nil {
		h.writeCanvasMapTopologyLoadError(w, err)
		return canvasTopologyResponse{}, false
	}

	responsePlan := loaded.Plan
	var linkRoutes map[string]canvasLinkRouteResponse
	if h.linkRouteRepo != nil {
		routes, err := h.linkRouteRepo.GetAllForMap(r.Context(), canvasMap.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list canvas map link routes", err)
			return canvasTopologyResponse{}, false
		}
		loadedLinkIDs := make(map[uuid.UUID]struct{}, len(responsePlan.Links))
		for _, link := range responsePlan.Links {
			loadedLinkIDs[link.ID] = struct{}{}
		}
		for _, route := range routes {
			if _, ok := loadedLinkIDs[route.LinkID]; !ok {
				continue
			}
			if linkRoutes == nil {
				linkRoutes = make(map[string]canvasLinkRouteResponse)
			}
			linkRoutes[route.LinkID.String()] = canvasLinkRouteToResponse(route)
		}
	}

	canvasMap = loaded.Map
	response := h.canvasTopology.buildResponse(responsePlan.Devices, responsePlan.Links, responsePlan.Positions, responsePlan.Areas)
	applyCanvasMapDeviceVisualColors(response.Devices, responsePlan.VisualColors)
	mapResponse := mapToResponse(canvasMap)
	mapResponse.DeviceCount = responsePlan.DeviceCount
	mapResponse.LinkCount = responsePlan.LinkCount
	mapResponse.PositionCount = responsePlan.PositionCount
	response.Map = &mapResponse
	response.LinkRoutes = linkRoutes
	response.TopologyVersion = buildCanvasMapTopologyVersion(response)
	return response, true
}

// loadMapFromRequest parses the map ID from the path and loads the map or writes an HTTP error.
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

// validateSourceAreaID parses and verifies a source area identifier from a create or patch DTO.
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

// validateCreateSourceAreaID applies create-only source-area rules including source-map checks.
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

// validateSourceMapID parses and verifies a source saved-map identifier.
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

// replaceMaterializedMembership delegates current-topology materialization while preserving HTTP error mapping.
func (h *CanvasMapHandler) replaceMaterializedMembership(
	w http.ResponseWriter,
	r *http.Request,
	mapID uuid.UUID,
	filter domain.CanvasMapFilter,
) bool {
	if err := canvasmap.ReplaceMaterializedMembership(r.Context(), mapID, filter, canvasmap.MaterializationDeps{
		Maps:    h.mapRepo,
		Devices: h.deviceService,
		Links:   h.linkRepo,
		Areas:   h.areaRepo,
	}); err != nil {
		h.writeCanvasMapMaterializationError(w, err)
		return false
	}
	return true
}

// replaceMaterializedMembershipFromSourceMap delegates saved-map source materialization and keeps HTTP mapping local.
func (h *CanvasMapHandler) replaceMaterializedMembershipFromSourceMap(
	w http.ResponseWriter,
	r *http.Request,
	mapID uuid.UUID,
	sourceMapID uuid.UUID,
	filter domain.CanvasMapFilter,
) bool {
	if err := canvasmap.ReplaceMaterializedMembershipFromSourceMap(r.Context(), mapID, sourceMapID, filter, canvasmap.SourceMapMaterializationDeps{
		Maps:      h.mapRepo,
		Positions: h.mapPositionRepo,
		Devices:   h.deviceService,
		Links:     h.linkRepo,
		Areas:     h.areaRepo,
	}); err != nil {
		h.writeCanvasMapSourceMapMaterializationError(w, err)
		return false
	}
	return true
}

// copyDefaultCanvasMapPositionsForMaterializedMembership delegates default-position copy while preserving HTTP mapping.
func (h *CanvasMapHandler) copyDefaultCanvasMapPositionsForMaterializedMembership(
	w http.ResponseWriter,
	mapID uuid.UUID,
) bool {
	if err := canvasmap.CopyDefaultPositionsForMaterializedMembership(mapID, canvasmap.DefaultPositionCopyDeps{
		Maps:            h.mapRepo,
		Positions:       h.mapPositionRepo,
		LegacyPositions: h.legacyPositionRepo,
	}); err != nil {
		h.writeCanvasMapDefaultPositionCopyError(w, err)
		return false
	}
	return true
}

// isolateCanvasMapVirtualDevices delegates virtual-device isolation after membership materialization.
func (h *CanvasMapHandler) isolateCanvasMapVirtualDevices(ctx context.Context, mapID uuid.UUID) error {
	var deviceService canvasmap.VirtualIsolationDeviceService
	if h.deviceService != nil {
		deviceService = canvasMapVirtualIsolationDeviceService{service: h.deviceService}
	}
	return canvasmap.IsolateVirtualDevices(ctx, mapID, canvasmap.VirtualIsolationDeps{
		Maps:      h.mapRepo,
		Positions: h.mapPositionRepo,
		Devices:   deviceService,
		Links:     h.linkRepo,
	})
}

type canvasMapVirtualIsolationDeviceService struct {
	service *service.DeviceService
}

// GetDevicesByIDs adapts the full device service to the canvasmap isolation dependency.
func (s canvasMapVirtualIsolationDeviceService) GetDevicesByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.Device, error) {
	return s.service.GetTopologyDevicesByIDs(ctx, ids)
}

// AddDevice adapts virtual clone creation while preserving device-service defaults.
func (s canvasMapVirtualIsolationDeviceService) AddDevice(
	ctx context.Context,
	ip string,
	hostname string,
	deviceType domain.DeviceType,
	creds domain.SNMPCredentials,
	tags map[string]string,
	vendor string,
	metricsSource domain.MetricsSource,
	prometheusLabelName string,
	prometheusLabelValue string,
	topologyDiscoveryMode domain.TopologyDiscoveryMode,
	areaIDs []uuid.UUID,
	notes ...*string,
) (*domain.Device, error) {
	return s.service.AddDevice(
		ctx,
		ip,
		hostname,
		deviceType,
		creds,
		tags,
		vendor,
		metricsSource,
		prometheusLabelName,
		prometheusLabelValue,
		topologyDiscoveryMode,
		areaIDs,
		notes...,
	)
}

// UpdateClonedVirtualDevice adapts clone-only mutable field persistence.
func (s canvasMapVirtualIsolationDeviceService) UpdateClonedVirtualDevice(
	ctx context.Context,
	id uuid.UUID,
	update canvasmap.VirtualDeviceCloneUpdate,
) error {
	return s.service.UpdateDevice(ctx, id, service.DeviceUpdate{
		PollIntervalOverride: update.PollIntervalOverride,
		PollingEnabled:       update.PollingEnabled,
	})
}

// GetDevice adapts clone reloads for isolation workflows.
func (s canvasMapVirtualIsolationDeviceService) GetDevice(ctx context.Context, id uuid.UUID) (*domain.Device, error) {
	return s.service.GetDevice(ctx, id)
}

// requireMapRepos writes a service-unavailable response when saved-map repositories are missing.
func (h *CanvasMapHandler) requireMapRepos(w http.ResponseWriter) bool {
	if h.mapRepo == nil || h.mapPositionRepo == nil {
		writeError(w, http.StatusNotImplemented, "canvas map repository unavailable")
		return false
	}
	return true
}

// requireTopologyDeps writes an error when topology collaborators are missing.
func (h *CanvasMapHandler) requireTopologyDeps(w http.ResponseWriter) bool {
	if h.canvasTopology == nil || h.deviceService == nil || h.linkRepo == nil || h.areaRepo == nil {
		writeError(w, http.StatusInternalServerError, "canvas topology dependencies unavailable")
		return false
	}
	return true
}

// requireLinkRouteRepos verifies the persistence collaborators needed by route mutations.
func (h *CanvasMapHandler) requireLinkRouteRepos(w http.ResponseWriter) bool {
	if h.mapRepo == nil || h.linkRepo == nil || h.linkRouteRepo == nil {
		writeError(w, http.StatusInternalServerError, "canvas map link route dependencies unavailable")
		return false
	}
	return true
}

// loadCanvasMapForLinkRoute verifies that the route's owning saved map exists.
func (h *CanvasMapHandler) loadCanvasMapForLinkRoute(w http.ResponseWriter, mapID uuid.UUID) bool {
	if _, err := h.mapRepo.GetByID(mapID); err != nil {
		if isCanvasMapNotFoundError(err) {
			writeError(w, http.StatusNotFound, "canvas map not found")
			return false
		}
		writeError(w, http.StatusInternalServerError, "failed to load canvas map", err)
		return false
	}
	return true
}

// writeCanvasMapLinkRouteMutationError maps route membership and persistence failures to stable statuses.
func (h *CanvasMapHandler) writeCanvasMapLinkRouteMutationError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, domain.ErrCanvasMapLinkRouteNotMember):
		writeError(w, http.StatusBadRequest, domain.ErrCanvasMapLinkRouteNotMember.Error())
	case isCanvasMapNotFoundError(err):
		writeError(w, http.StatusNotFound, "canvas map not found")
	case isLinkNotFoundError(err):
		writeError(w, http.StatusNotFound, "link not found")
	default:
		writeError(w, http.StatusInternalServerError, "failed to "+operation+" canvas map link route", err)
	}
}

// mapAreaRepo narrows the map repository to area mutation support for area endpoints.
func (h *CanvasMapHandler) mapAreaRepo(w http.ResponseWriter) (canvasMapAreaRepository, bool) {
	areaRepo, ok := h.mapRepo.(canvasMapAreaRepository)
	if !ok {
		writeError(w, http.StatusNotImplemented, "canvas map area repository unavailable")
		return nil, false
	}
	return areaRepo, true
}

// writeMapRepoMutationError preserves saved-map mutation error status mapping.
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

// writeCanvasMapMaterializationError maps service-stage failures to the existing saved-map HTTP errors.
func (h *CanvasMapHandler) writeCanvasMapMaterializationError(w http.ResponseWriter, err error) {
	var materializationErr canvasmap.MaterializationError
	if !errors.As(err, &materializationErr) {
		writeError(w, http.StatusInternalServerError, "failed to materialize canvas map membership", err)
		return
	}

	switch materializationErr.Stage {
	case canvasmap.MaterializationStageDevices:
		writeError(w, http.StatusInternalServerError, "failed to list devices", materializationErr.Err)
	case canvasmap.MaterializationStageLinks:
		writeError(w, http.StatusInternalServerError, "failed to list links", materializationErr.Err)
	case canvasmap.MaterializationStageAreas:
		writeError(w, http.StatusInternalServerError, "failed to list areas", materializationErr.Err)
	case canvasmap.MaterializationStageReplace:
		h.writeMapRepoMutationError(w, materializationErr.Err)
	default:
		writeError(w, http.StatusInternalServerError, "failed to materialize canvas map membership", materializationErr.Err)
	}
}

// writeCanvasMapSourceMapMaterializationError maps source-map service stages to existing HTTP errors.
func (h *CanvasMapHandler) writeCanvasMapSourceMapMaterializationError(w http.ResponseWriter, err error) {
	var sourceErr canvasmap.SourceMapMaterializationError
	if !errors.As(err, &sourceErr) {
		writeError(w, http.StatusInternalServerError, "failed to materialize source map membership", err)
		return
	}

	switch sourceErr.Stage {
	case canvasmap.SourceMapMaterializationStageMembership:
		writeError(w, http.StatusInternalServerError, "failed to load source map membership", sourceErr.Err)
	case canvasmap.SourceMapMaterializationStageDevices:
		writeError(w, http.StatusInternalServerError, "failed to list source map devices", sourceErr.Err)
	case canvasmap.SourceMapMaterializationStageLinks:
		writeError(w, http.StatusInternalServerError, "failed to list source map links", sourceErr.Err)
	case canvasmap.SourceMapMaterializationStageAreas:
		writeError(w, http.StatusInternalServerError, "failed to list source map areas", sourceErr.Err)
	case canvasmap.SourceMapMaterializationStageReplace:
		h.writeMapRepoMutationError(w, sourceErr.Err)
	case canvasmap.SourceMapMaterializationStagePositions, canvasmap.SourceMapMaterializationStageSavePositions:
		writeError(w, http.StatusInternalServerError, "failed to copy source map positions", sourceErr.Err)
	default:
		writeError(w, http.StatusInternalServerError, "failed to materialize source map membership", sourceErr.Err)
	}
}

// writeCanvasMapDefaultPositionCopyError maps position-copy service stages to existing HTTP errors.
func (h *CanvasMapHandler) writeCanvasMapDefaultPositionCopyError(w http.ResponseWriter, err error) {
	var copyErr canvasmap.DefaultPositionCopyError
	if !errors.As(err, &copyErr) {
		writeError(w, http.StatusInternalServerError, "failed to copy default canvas map positions", err)
		return
	}

	switch copyErr.Stage {
	case canvasmap.DefaultPositionCopyStageDefaultMap:
		writeError(w, http.StatusInternalServerError, "failed to load default canvas map", copyErr.Err)
	case canvasmap.DefaultPositionCopyStageMembership:
		writeError(w, http.StatusInternalServerError, "failed to load materialized canvas map membership", copyErr.Err)
	case canvasmap.DefaultPositionCopyStagePositions:
		writeError(w, http.StatusInternalServerError, "failed to load default canvas map positions", copyErr.Err)
	case canvasmap.DefaultPositionCopyStageLegacyPositions:
		writeError(w, http.StatusInternalServerError, "failed to load legacy canvas positions", copyErr.Err)
	case canvasmap.DefaultPositionCopyStageSavePositions:
		writeError(w, http.StatusInternalServerError, "failed to copy default canvas map positions", copyErr.Err)
	default:
		writeError(w, http.StatusInternalServerError, "failed to copy default canvas map positions", copyErr.Err)
	}
}

// writeCanvasMapAddDeviceWorkflowError maps add-device service stages and plan errors to existing HTTP errors.
func (h *CanvasMapHandler) writeCanvasMapAddDeviceWorkflowError(w http.ResponseWriter, err error) {
	var workflowErr canvasmap.AddDeviceMembershipError
	if !errors.As(err, &workflowErr) {
		h.writeCanvasMapAddDevicePlanError(w, err)
		return
	}

	switch workflowErr.Stage {
	case canvasmap.AddDeviceMembershipStageDevice:
		writeError(w, http.StatusNotFound, "canvas map device not found")
	case canvasmap.AddDeviceMembershipStageMembership:
		writeError(w, http.StatusInternalServerError, "failed to load canvas map membership", workflowErr.Err)
	case canvasmap.AddDeviceMembershipStageMemberDevices:
		writeError(w, http.StatusInternalServerError, "failed to list canvas map devices", workflowErr.Err)
	case canvasmap.AddDeviceMembershipStageLinks:
		writeError(w, http.StatusInternalServerError, "failed to list canvas map device links", workflowErr.Err)
	case canvasmap.AddDeviceMembershipStageAreas:
		writeError(w, http.StatusInternalServerError, "failed to load canvas map device areas", workflowErr.Err)
	case canvasmap.AddDeviceMembershipStagePersist:
		h.writeMapRepoMutationError(w, workflowErr.Err)
	default:
		writeError(w, http.StatusInternalServerError, "failed to add canvas map device", workflowErr.Err)
	}
}

// writeCanvasMapAreaMutationError maps map-local area persistence errors to stable HTTP statuses.
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

// writeCanvasMapAddDevicePlanError maps pure add-device planning errors to HTTP responses.
func (h *CanvasMapHandler) writeCanvasMapAddDevicePlanError(w http.ResponseWriter, err error) {
	var duplicateAddress canvasmap.DuplicateDeviceAddressError
	switch {
	case errors.Is(err, canvasmap.ErrDeviceAlreadyInCanvasMap):
		writeError(w, http.StatusConflict, err.Error())
	case errors.As(err, &duplicateAddress):
		writeError(w, http.StatusConflict, duplicateAddress.Error())
	default:
		writeError(w, http.StatusInternalServerError, "failed to plan canvas map device membership", err)
	}
}

// writeCanvasMapTopologyLoadError maps service load stages back to the existing HTTP error messages.
func (h *CanvasMapHandler) writeCanvasMapTopologyLoadError(w http.ResponseWriter, err error) {
	var loadErr canvasmap.TopologyLoadError
	if !errors.As(err, &loadErr) {
		writeError(w, http.StatusInternalServerError, "failed to load canvas map topology", err)
		return
	}
	switch loadErr.Stage {
	case canvasmap.TopologyLoadStageIsolate:
		writeError(w, http.StatusInternalServerError, "failed to isolate canvas map virtual devices", err)
	case canvasmap.TopologyLoadStageMap:
		writeError(w, http.StatusInternalServerError, "failed to load canvas map", err)
	case canvasmap.TopologyLoadStagePositions:
		writeError(w, http.StatusInternalServerError, "failed to list canvas map positions", err)
	case canvasmap.TopologyLoadStageMembership:
		writeError(w, http.StatusInternalServerError, "failed to load canvas map membership", err)
	case canvasmap.TopologyLoadStageDevices:
		writeError(w, http.StatusInternalServerError, "failed to list canvas map devices", err)
	case canvasmap.TopologyLoadStageLinks:
		writeError(w, http.StatusInternalServerError, "failed to list canvas map links", err)
	default:
		writeError(w, http.StatusInternalServerError, "failed to load canvas map topology", err)
	}
}

// parseCanvasMapDeviceAction extracts the trailing device ID from map device actions.
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

// parseCanvasMapLinkRouteIDs extracts the owning map and canonical link identifiers from a route item path.
func parseCanvasMapLinkRouteIDs(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	mapID, action, ok := parseCanvasMapRoute(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "canvas map not found")
		return uuid.Nil, uuid.Nil, false
	}
	rawLinkID, ok := strings.CutPrefix(action, "link-routes/")
	if !ok || rawLinkID == "" || strings.Contains(rawLinkID, "/") {
		writeError(w, http.StatusNotFound, "canvas map link route not found")
		return uuid.Nil, uuid.Nil, false
	}
	linkID, err := uuid.Parse(rawLinkID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid link ID")
		return uuid.Nil, uuid.Nil, false
	}
	return mapID, linkID, true
}

// parseCanvasMapRequestUUIDs decodes UUID arrays from request DTOs and writes field-specific errors.
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

// parseCanvasMapAreaActionID extracts the map-local area ID from the current route action.
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

// canvasMapAreaMembershipFromRequest validates area DTO metadata through the canvasmap service helper.
func canvasMapAreaMembershipFromRequest(
	w http.ResponseWriter,
	req areaRequest,
) (domain.CanvasMapAreaMembership, bool) {
	area, err := canvasmap.AreaMembershipFromInput(req.Name, req.Description, req.Color)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return domain.CanvasMapAreaMembership{}, false
	}
	return area, true
}

// applyCanvasMapDeviceVisualColors overlays map-local visual metadata onto topology devices.
func applyCanvasMapDeviceVisualColors(
	devices []jsonAPIResource,
	visualColors map[uuid.UUID]string,
) {
	if len(devices) == 0 || len(visualColors) == 0 {
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

// buildCanvasMapTopologyVersion hashes the response content into a stable topology version token.
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
		Map        *canvasMapResponse                 `json:"map"`
		Topology   canvasTopologyVersionInput         `json:"topology"`
		LinkRoutes map[string]canvasLinkRouteResponse `json:"link_routes,omitempty"`
	}{
		Map:        response.Map,
		Topology:   versionInput,
		LinkRoutes: response.LinkRoutes,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "topo-unversioned"
	}
	sum := sha256.Sum256(data)
	return "topo-" + hex.EncodeToString(sum[:])[:16]
}

// isFiniteCoordinate rejects NaN and infinity in persisted canvas positions.
func isFiniteCoordinate(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

// isCanvasMapNotFoundError detects repository not-found errors without exposing storage details.
func isCanvasMapNotFoundError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "canvas map not found")
}

// isAreaNotFoundError detects area-not-found errors for map-local area endpoints.
func isAreaNotFoundError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "area not found")
}

// isLinkNotFoundError detects canonical link lookup failures without exposing storage details.
func isLinkNotFoundError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "link not found")
}

func canvasLinkRouteToResponse(route domain.CanvasMapLinkRoute) canvasLinkRouteResponse {
	return canvasLinkRouteResponse{
		Version:   route.Version,
		Waypoints: append([]domain.CanvasPoint(nil), route.Waypoints...),
		UpdatedAt: formatCanvasMapTimestamp(route.UpdatedAt),
	}
}

// isCanvasMapConflictError detects saved-map uniqueness and membership conflicts.
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

// isCanvasMapValidationError detects saved-map validation errors that should remain 400 responses.
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
