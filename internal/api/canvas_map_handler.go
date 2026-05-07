package api

import (
	"encoding/json"
	"fmt"
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
	Filter       domain.CanvasMapFilter `json:"filter"`
}

type canvasMapPatchRequest struct {
	Name         *string                 `json:"name"`
	Description  *string                 `json:"description"`
	SourceAreaID *string                 `json:"source_area_id"`
	Filter       *domain.CanvasMapFilter `json:"filter"`
}

type canvasMapDuplicateRequest struct {
	Name string `json:"name"`
}

func (h *CanvasMapHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	if !h.requireMapRepos(w) {
		return
	}
	if !h.requireTopologyDeps(w) {
		return
	}

	maps, err := h.mapRepo.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list canvas maps", err)
		return
	}
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

	responses := make([]canvasMapResponse, 0, len(maps))
	for _, canvasMap := range maps {
		filter := projectionFilterForCanvasMap(canvasMap)
		projection := projectCanvasTopologyForMap(devices, links, filter)
		positions, err := h.mapPositionRepo.GetAllForMap(canvasMap.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list canvas map positions", err)
			return
		}

		response := mapToResponse(canvasMap)
		response.DeviceCount = len(projection.Devices)
		response.LinkCount = len(projection.Links)
		response.PositionCount = len(positions)
		responses = append(responses, response)
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

	canvasMap, err := h.mapRepo.Create(domain.CanvasMapCreate{
		Name:         req.Name,
		Description:  req.Description,
		SourceAreaID: sourceAreaID,
		Filter:       req.Filter,
	})
	if err != nil {
		h.writeMapRepoMutationError(w, err)
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
	if req.SourceAreaID != nil {
		sourceAreaID, ok := h.validateSourceAreaID(w, req.SourceAreaID)
		if !ok {
			return
		}
		canvasMap.SourceAreaID = sourceAreaID
	}

	updated, err := h.mapRepo.Update(canvasMap.ID, domain.CanvasMapUpdate{
		Name:            req.Name,
		Description:     req.Description,
		SourceAreaID:    canvasMap.SourceAreaID,
		SourceAreaIDSet: req.SourceAreaID != nil,
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

	devices, err := h.deviceService.GetAllDevices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list devices", err)
		return canvasTopologyResponse{}, false
	}
	links, err := h.linkRepo.GetAll()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list links", err)
		return canvasTopologyResponse{}, false
	}
	positions, err := h.mapPositionRepo.GetAllForMap(canvasMap.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list canvas map positions", err)
		return canvasTopologyResponse{}, false
	}
	areas, err := h.areaRepo.GetAllWithDeviceCount()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list areas", err)
		return canvasTopologyResponse{}, false
	}

	projection := projectCanvasTopologyForMap(devices, links, projectionFilterForCanvasMap(canvasMap))
	response := h.canvasTopology.buildResponse(projection.Devices, projection.Links, positions, areas)
	mapResponse := mapToResponse(canvasMap)
	mapResponse.DeviceCount = len(projection.Devices)
	mapResponse.LinkCount = len(projection.Links)
	mapResponse.PositionCount = len(positions)
	response.Map = &mapResponse
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
		strings.Contains(message, "canvas map filter")
}
