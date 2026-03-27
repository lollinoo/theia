package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/vendor"
)

// DeviceHandler provides HTTP handlers for device CRUD operations.
type DeviceHandler struct {
	svc            *service.DeviceService
	sshProfileRepo domain.SSHProfileRepository
	vendorRegistry *vendor.Registry
}

// NewDeviceHandler creates a new DeviceHandler.
func NewDeviceHandler(svc *service.DeviceService, sshProfileRepo domain.SSHProfileRepository, vendorRegistry *vendor.Registry) *DeviceHandler {
	return &DeviceHandler{svc: svc, sshProfileRepo: sshProfileRepo, vendorRegistry: vendorRegistry}
}

// --- JSON:API response types ---

type jsonAPIResource struct {
	Type          string                 `json:"type"`
	ID            string                 `json:"id"`
	Attributes    map[string]interface{} `json:"attributes"`
	Relationships map[string]interface{} `json:"relationships,omitempty"`
}

type jsonAPISingle struct {
	Data jsonAPIResource `json:"data"`
}

type jsonAPIList struct {
	Data []jsonAPIResource `json:"data"`
}

// --- Request types ---

type createDeviceRequest struct {
	IP                   string            `json:"ip"`
	Hostname             string            `json:"hostname"`
	SNMP                 snmpCredsRequest  `json:"snmp"`
	Tags                 map[string]string `json:"tags"`
	Vendor               string            `json:"vendor,omitempty"`
	MetricsSource        string            `json:"metrics_source,omitempty"`
	PrometheusLabelName  string            `json:"prometheus_label_name,omitempty"`
	PrometheusLabelValue string            `json:"prometheus_label_value,omitempty"`
	SSHProfileID         string            `json:"ssh_profile_id,omitempty"`
}

type snmpCredsRequest struct {
	Version   string `json:"version"`
	Community string `json:"community"`
	// v3 fields
	Username      string `json:"username"`
	AuthProtocol  string `json:"auth_protocol"`
	AuthPassword  string `json:"auth_password"`
	PrivProtocol  string `json:"priv_protocol"`
	PrivPassword  string `json:"priv_password"`
	SecurityLevel string `json:"security_level"`
}

type updateDeviceRequest struct {
	Hostname             *string            `json:"hostname,omitempty"`
	IP                   *string            `json:"ip,omitempty"`
	Tags                 *map[string]string `json:"tags,omitempty"`
	SNMP                 *snmpCredsRequest  `json:"snmp,omitempty"`
	Vendor               *string            `json:"vendor,omitempty"`
	MetricsSource        *string            `json:"metrics_source,omitempty"`
	PrometheusLabelName  *string            `json:"prometheus_label_name,omitempty"`
	PrometheusLabelValue *string            `json:"prometheus_label_value,omitempty"`
	SSHProfileID         *string            `json:"ssh_profile_id,omitempty"`
	AreaID               *string            `json:"area_id,omitempty"`
}

type batchAddRequest struct {
	Devices []createDeviceRequest `json:"devices"`
}

type batchAddResponse struct {
	BatchID string `json:"batch_id"`
	Status  string `json:"status"`
	Count   int    `json:"count"`
}

// HandleCreate handles POST /api/v1/devices
func (h *DeviceHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var req createDeviceRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	if req.IP == "" {
		writeError(w, http.StatusBadRequest, "ip is required")
		return
	}

	creds, err := parseSNMPCreds(req.SNMP)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	metricsSource := domain.MetricsSource(req.MetricsSource)
	prometheusLabelName := req.PrometheusLabelName
	prometheusLabelValue := req.PrometheusLabelValue

	var sshProfileID *uuid.UUID
	if req.SSHProfileID != "" {
		parsed, err := uuid.Parse(req.SSHProfileID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid ssh_profile_id")
			return
		}
		if _, err := h.sshProfileRepo.GetByID(parsed); err != nil {
			writeError(w, http.StatusBadRequest, "ssh profile not found")
			return
		}
		sshProfileID = &parsed
	}

	device, err := h.svc.AddDevice(r.Context(), req.IP, req.Hostname, creds, req.Tags, req.Vendor, metricsSource, prometheusLabelName, prometheusLabelValue, sshProfileID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(jsonAPISingle{Data: h.deviceToResource(device)})
}

// HandleList handles GET /api/v1/devices
func (h *DeviceHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	devices, err := h.svc.GetAllDevices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resources := make([]jsonAPIResource, 0, len(devices))
	for i := range devices {
		resources = append(resources, h.deviceToResource(&devices[i]))
	}

	json.NewEncoder(w).Encode(jsonAPIList{Data: resources})
}

// HandleGet handles GET /api/v1/devices/{id}
func (h *DeviceHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id, err := extractIDFromPath(r.URL.Path, "/api/v1/devices/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device ID")
		return
	}

	device, err := h.svc.GetDevice(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	json.NewEncoder(w).Encode(jsonAPISingle{Data: h.deviceToResource(device)})
}

// HandleUpdate handles PUT /api/v1/devices/{id}
func (h *DeviceHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := extractIDFromPath(r.URL.Path, "/api/v1/devices/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device ID")
		return
	}

	var req updateDeviceRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	update := service.DeviceUpdate{
		Hostname: req.Hostname,
		IP:       req.IP,
		Tags:     req.Tags,
	}

	if req.SNMP != nil {
		creds, err := parseSNMPCreds(*req.SNMP)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		update.SNMPCredentials = &creds
	}
	if req.Vendor != nil {
		update.Vendor = req.Vendor
	}
	if req.MetricsSource != nil {
		ms := domain.MetricsSource(*req.MetricsSource)
		update.MetricsSource = &ms
	}
	if req.PrometheusLabelName != nil {
		update.PrometheusLabelName = req.PrometheusLabelName
	}
	if req.PrometheusLabelValue != nil {
		update.PrometheusLabelValue = req.PrometheusLabelValue
	}
	if req.SSHProfileID != nil {
		if *req.SSHProfileID == "" {
			// Explicitly unassign
			update.SSHProfileID = new(*uuid.UUID)
			*update.SSHProfileID = nil
		} else {
			parsed, err := uuid.Parse(*req.SSHProfileID)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid ssh_profile_id")
				return
			}
			if _, err := h.sshProfileRepo.GetByID(parsed); err != nil {
				writeError(w, http.StatusBadRequest, "ssh profile not found")
				return
			}
			update.SSHProfileID = new(*uuid.UUID)
			*update.SSHProfileID = &parsed
		}
	}
	if req.AreaID != nil {
		if *req.AreaID == "" {
			// Explicitly unassign
			update.AreaID = new(*uuid.UUID)
			*update.AreaID = nil
		} else {
			parsed, err := uuid.Parse(*req.AreaID)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid area_id")
				return
			}
			update.AreaID = new(*uuid.UUID)
			*update.AreaID = &parsed
		}
	}

	if err := h.svc.UpdateDevice(r.Context(), id, update); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Return updated device
	device, err := h.svc.GetDevice(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	json.NewEncoder(w).Encode(jsonAPISingle{Data: h.deviceToResource(device)})
}

// HandleDelete handles DELETE /api/v1/devices/{id}
func (h *DeviceHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := extractIDFromPath(r.URL.Path, "/api/v1/devices/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device ID")
		return
	}

	if err := h.svc.DeleteDevice(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleProbe handles POST /api/v1/devices/{id}/probe
func (h *DeviceHandler) HandleProbe(w http.ResponseWriter, r *http.Request) {
	// Strip trailing /probe to get the ID
	path := strings.TrimSuffix(r.URL.Path, "/probe")
	id, err := extractIDFromPath(path, "/api/v1/devices/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device ID")
		return
	}

	if err := h.svc.ProbeDevice(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "probing"})
}

// HandleTestSNMP handles POST /api/v1/devices/{id}/snmp-test
func (h *DeviceHandler) HandleTestSNMP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/snmp-test")
	id, err := extractIDFromPath(path, "/api/v1/devices/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device ID")
		return
	}

	result, err := h.svc.TestSNMP(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	json.NewEncoder(w).Encode(result)
}

// HandleBatchAdd handles POST /api/v1/devices/batch
func (h *DeviceHandler) HandleBatchAdd(w http.ResponseWriter, r *http.Request) {
	var req batchAddRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	if len(req.Devices) == 0 {
		writeError(w, http.StatusBadRequest, "devices array is required and must not be empty")
		return
	}

	batchID := uuid.New().String()

	// Add each device asynchronously
	for _, d := range req.Devices {
		creds, err := parseSNMPCreds(d.SNMP)
		if err != nil {
			// Skip devices with bad credentials, continue with others
			continue
		}
		ms := domain.MetricsSource(d.MetricsSource)
		var batchSSHProfileID *uuid.UUID
		if d.SSHProfileID != "" {
			parsed, parseErr := uuid.Parse(d.SSHProfileID)
			if parseErr != nil {
				continue
			}
			if _, lookupErr := h.sshProfileRepo.GetByID(parsed); lookupErr != nil {
				continue
			}
			batchSSHProfileID = &parsed
		}
		_, _ = h.svc.AddDevice(r.Context(), d.IP, d.Hostname, creds, d.Tags, d.Vendor, ms, d.PrometheusLabelName, d.PrometheusLabelValue, batchSSHProfileID)
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(batchAddResponse{
		BatchID: batchID,
		Status:  "processing",
		Count:   len(req.Devices),
	})
}

// --- Helpers ---

func (h *DeviceHandler) deviceToResource(d *domain.Device) jsonAPIResource {
	attrs := map[string]interface{}{
		"hostname":               d.Hostname,
		"ip":                     d.IP,
		"device_type":            string(d.DeviceType),
		"status":                 string(d.Status),
		"sys_name":               d.SysName,
		"sys_descr":              d.SysDescr,
		"sys_object_id":          d.SysObjectID,
		"hardware_model":         d.HardwareModel,
		"vendor":                 d.Vendor,
		"managed":                d.Managed,
		"tags":                   d.Tags,
		"metrics_source":         string(d.MetricsSource),
		"prometheus_label_name":  d.PrometheusLabelName,
		"prometheus_label_value": d.PrometheusLabelValue,
		"created_at":             d.CreatedAt,
		"updated_at":             d.UpdatedAt,
	}

	if d.SSHProfileID != nil {
		attrs["ssh_profile_id"] = d.SSHProfileID.String()
	}
	if d.AreaID != nil {
		attrs["area_id"] = d.AreaID.String()
	}

	attrs["backup_supported"] = h.vendorRegistry.ResolveBackupConfig(d.Vendor).Supported

	// Include interfaces as a relationship
	var ifaceData []map[string]interface{}
	for _, iface := range d.Interfaces {
		ifaceData = append(ifaceData, map[string]interface{}{
			"id":           iface.ID.String(),
			"if_index":     iface.IfIndex,
			"if_name":      iface.IfName,
			"if_descr":     iface.IfDescr,
			"speed":        iface.Speed,
			"admin_status": iface.AdminStatus,
			"oper_status":  iface.OperStatus,
		})
	}

	var relationships map[string]interface{}
	if len(ifaceData) > 0 {
		relationships = map[string]interface{}{
			"interfaces": map[string]interface{}{
				"data": ifaceData,
			},
		}
	}

	return jsonAPIResource{
		Type:          "device",
		ID:            d.ID.String(),
		Attributes:    attrs,
		Relationships: relationships,
	}
}

func parseSNMPCreds(req snmpCredsRequest) (domain.SNMPCredentials, error) {
	creds := domain.SNMPCredentials{}

	switch req.Version {
	case "2c", "":
		creds.Version = domain.SNMPVersionV2c
		if req.Community == "" {
			req.Community = "public" // default
		}
		creds.V2c = &domain.SNMPv2cCredentials{Community: req.Community}
	case "3":
		creds.Version = domain.SNMPVersionV3
		creds.V3 = &domain.SNMPv3Credentials{
			Username:      req.Username,
			AuthProtocol:  req.AuthProtocol,
			AuthPassword:  req.AuthPassword,
			PrivProtocol:  req.PrivProtocol,
			PrivPassword:  req.PrivPassword,
			SecurityLevel: req.SecurityLevel,
		}
	default:
		return creds, &invalidFieldError{field: "snmp.version", value: req.Version}
	}

	return creds, nil
}

type invalidFieldError struct {
	field string
	value string
}

func (e *invalidFieldError) Error() string {
	return "invalid value for " + e.field + ": " + e.value
}

func extractIDFromPath(path, prefix string) (uuid.UUID, error) {
	idStr := strings.TrimPrefix(path, prefix)
	// Remove any trailing path segments
	if idx := strings.Index(idStr, "/"); idx >= 0 {
		idStr = idStr[:idx]
	}
	return uuid.Parse(idStr)
}

func writeError(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// decodeJSON reads and unmarshals JSON from the request body.
// Returns false and writes an error response if decoding fails.
// Detects MaxBytesError and returns 413 instead of 400.
func decodeJSON(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return false
		}
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}
