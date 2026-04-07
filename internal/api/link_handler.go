package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
	"github.com/google/uuid"
)

// LinkHandler provides HTTP handlers for link CRUD operations and the
// per-device interfaces endpoint.
type LinkHandler struct {
	linkRepo      domain.LinkRepository
	deviceService *service.DeviceService
}

// NewLinkHandler creates a new LinkHandler.
func NewLinkHandler(linkRepo domain.LinkRepository, deviceService *service.DeviceService) *LinkHandler {
	return &LinkHandler{linkRepo: linkRepo, deviceService: deviceService}
}

// --- Request types ---

type createLinkRequest struct {
	SourceDeviceID string `json:"source_device_id"`
	SourceIfName   string `json:"source_if_name"`
	TargetDeviceID string `json:"target_device_id"`
	TargetIfName   string `json:"target_if_name"`
}

type updateLinkRequest struct {
	SourceIfName string `json:"source_if_name"`
	TargetIfName string `json:"target_if_name"`
}

// --- Response types ---

type interfaceResponse struct {
	IfName      string `json:"if_name"`
	IfDescr     string `json:"if_descr"`
	Speed       int64  `json:"speed"`
	OperStatus  string `json:"oper_status"`
	AdminStatus string `json:"admin_status"`
	InUse       bool   `json:"in_use"`
}

// HandleList handles GET /api/v1/links
func (h *LinkHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	links, err := h.linkRepo.GetAll()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	if links == nil {
		links = []domain.Link{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"data": links})
}

// HandleCreate handles POST /api/v1/links
func (h *LinkHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var req createLinkRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	srcID, err := uuid.Parse(req.SourceDeviceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid source_device_id")
		return
	}
	tgtID, err := uuid.Parse(req.TargetDeviceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid target_device_id")
		return
	}
	// Fetch both devices (validates existence and gets DeviceType)
	srcDevice, err := h.deviceService.GetDevice(r.Context(), srcID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "source device not found")
		return
	}
	tgtDevice, err := h.deviceService.GetDevice(r.Context(), tgtID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "target device not found")
		return
	}

	// Per D-13: Reject both-virtual links
	srcIsVirtual := srcDevice.DeviceType == domain.DeviceTypeVirtual
	tgtIsVirtual := tgtDevice.DeviceType == domain.DeviceTypeVirtual
	if srcIsVirtual && tgtIsVirtual {
		writeError(w, http.StatusBadRequest, "at least one device must be non-virtual")
		return
	}

	// Per D-12: Allow empty if_name for the virtual side only
	if req.SourceIfName == "" && !srcIsVirtual {
		writeError(w, http.StatusBadRequest, "source_if_name is required")
		return
	}
	if req.TargetIfName == "" && !tgtIsVirtual {
		writeError(w, http.StatusBadRequest, "target_if_name is required")
		return
	}

	if len(req.SourceIfName) > 255 {
		writeError(w, http.StatusBadRequest, "source_if_name too long (max 255 characters)")
		return
	}
	if len(req.TargetIfName) > 255 {
		writeError(w, http.StatusBadRequest, "target_if_name too long (max 255 characters)")
		return
	}

	link := &domain.Link{
		ID:                uuid.New(),
		SourceDeviceID:    srcID,
		SourceIfName:      req.SourceIfName,
		TargetDeviceID:    tgtID,
		TargetIfName:      req.TargetIfName,
		DiscoveryProtocol: domain.DiscoveryProtocolManual,
	}

	if err := h.linkRepo.Create(link); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": link})
}

// HandleUpdate handles PUT /api/v1/links/:id
func (h *LinkHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := extractIDFromPath(r.URL.Path, "/api/v1/links/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid link ID")
		return
	}

	var req updateLinkRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	link, err := h.linkRepo.GetByID(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	if req.SourceIfName != "" {
		if len(req.SourceIfName) > 255 {
			writeError(w, http.StatusBadRequest, "source_if_name too long (max 255 characters)")
			return
		}
		link.SourceIfName = req.SourceIfName
	}
	if req.TargetIfName != "" {
		if len(req.TargetIfName) > 255 {
			writeError(w, http.StatusBadRequest, "target_if_name too long (max 255 characters)")
			return
		}
		link.TargetIfName = req.TargetIfName
	}

	if err := h.linkRepo.Update(link); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"data": link})
}

// HandleDelete handles DELETE /api/v1/links/:id
func (h *LinkHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := extractIDFromPath(r.URL.Path, "/api/v1/links/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid link ID")
		return
	}

	if err := h.linkRepo.Delete(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetInterfaces handles GET /api/v1/devices/:id/interfaces
// Returns a filtered and sorted list of interfaces with in_use annotation.
func (h *LinkHandler) HandleGetInterfaces(w http.ResponseWriter, r *http.Request) {
	// Strip /interfaces suffix to extract device ID
	path := strings.TrimSuffix(r.URL.Path, "/interfaces")
	id, err := extractIDFromPath(path, "/api/v1/devices/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device ID")
		return
	}

	device, err := h.deviceService.GetDevice(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	// Build set of in-use interface names for this device
	inUse := map[string]bool{}
	links, err := h.linkRepo.GetByDeviceID(id)
	if err == nil {
		for _, lnk := range links {
			if lnk.SourceDeviceID == id {
				inUse[lnk.SourceIfName] = true
			}
			if lnk.TargetDeviceID == id {
				inUse[lnk.TargetIfName] = true
			}
		}
	}

	// Filter and build response
	var result []interfaceResponse
	for _, iface := range device.Interfaces {
		name := iface.IfName
		// Exclude loopback, Null, and empty interfaces
		if name == "" {
			continue
		}
		if strings.HasPrefix(name, "lo") ||
			strings.HasPrefix(name, "Null") ||
			strings.EqualFold(name, "null") {
			continue
		}

		result = append(result, interfaceResponse{
			IfName:      iface.IfName,
			IfDescr:     iface.IfDescr,
			Speed:       iface.Speed,
			OperStatus:  iface.OperStatus,
			AdminStatus: iface.AdminStatus,
			InUse:       inUse[iface.IfName],
		})
	}

	// Sort: "up" interfaces first, then alphabetically by if_name
	sort.Slice(result, func(i, j int) bool {
		iUp := result[i].OperStatus == "up"
		jUp := result[j].OperStatus == "up"
		if iUp != jUp {
			return iUp
		}
		return result[i].IfName < result[j].IfName
	})

	if result == nil {
		result = []interfaceResponse{}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"data": result})
}
