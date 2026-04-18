package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/vendor"
)

// DeviceHandler provides HTTP handlers for device CRUD operations.
type DeviceHandler struct {
	svc                   *service.DeviceService
	credentialProfileRepo domain.CredentialProfileRepository
	vendorRegistry        *vendor.Registry
}

// NewDeviceHandler creates a new DeviceHandler.
func NewDeviceHandler(svc *service.DeviceService, credentialProfileRepo domain.CredentialProfileRepository, vendorRegistry *vendor.Registry) *DeviceHandler {
	return &DeviceHandler{svc: svc, credentialProfileRepo: credentialProfileRepo, vendorRegistry: vendorRegistry}
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

// --- Validation allowlists ---

var validDeviceTypes = map[string]bool{
	"router": true, "switch": true, "access_point": true,
	"firewall": true, "unknown": true, "virtual": true, "": true,
}

var validMetricsSources = map[string]bool{
	"prometheus":               true,
	"snmp":                     true,
	"prometheus_snmp_fallback": true,
	"none":                     true,
	"":                         true,
}

var validTopologyDiscoveryModes = map[string]bool{
	"inherit":        true,
	"off":            true,
	"lldp":           true,
	"lldp_cdp":       true,
	"bootstrap_once": true,
	"":               true,
}

var validSNMPv3AuthProtocols = map[string]bool{
	"MD5": true, "SHA": true, "SHA-224": true,
	"SHA-256": true, "SHA-384": true, "SHA-512": true,
}

var validSNMPv3PrivProtocols = map[string]bool{
	"DES": true, "AES": true,
}

var validSNMPv3SecurityLevels = map[string]bool{
	"noAuthNoPriv": true, "authNoPriv": true, "authPriv": true,
}

// --- Request types ---

type createDeviceRequest struct {
	IP                    string            `json:"ip"`
	Hostname              string            `json:"hostname"`
	Notes                 *string           `json:"notes"`
	DeviceType            string            `json:"device_type,omitempty"`
	SNMP                  snmpCredsRequest  `json:"snmp"`
	Tags                  map[string]string `json:"tags"`
	Vendor                string            `json:"vendor,omitempty"`
	MetricsSource         string            `json:"metrics_source,omitempty"`
	PrometheusLabelName   string            `json:"prometheus_label_name,omitempty"`
	PrometheusLabelValue  string            `json:"prometheus_label_value,omitempty"`
	TopologyDiscoveryMode string            `json:"topology_discovery_mode,omitempty"`
	AreaIDs               []string          `json:"area_ids,omitempty"`
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

type optionalPollIntervalOverride struct {
	Set   bool
	Value *int
}

func (o *optionalPollIntervalOverride) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}

	var value int
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}

	o.Value = &value
	return nil
}

type optionalNullableString struct {
	Set   bool
	Value *string
}

func (o *optionalNullableString) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}

	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}

	o.Value = &value
	return nil
}

type updateDeviceRequest struct {
	Hostname              *string                      `json:"hostname,omitempty"`
	IP                    *string                      `json:"ip,omitempty"`
	Notes                 optionalNullableString       `json:"notes"`
	Tags                  *map[string]string           `json:"tags,omitempty"`
	SNMP                  *snmpCredsRequest            `json:"snmp,omitempty"`
	Vendor                *string                      `json:"vendor,omitempty"`
	MetricsSource         *string                      `json:"metrics_source,omitempty"`
	PrometheusLabelName   *string                      `json:"prometheus_label_name,omitempty"`
	PrometheusLabelValue  *string                      `json:"prometheus_label_value,omitempty"`
	TopologyDiscoveryMode *string                      `json:"topology_discovery_mode,omitempty"`
	PollIntervalOverride  optionalPollIntervalOverride `json:"poll_interval_override"`
	AreaIDs               *[]string                    `json:"area_ids,omitempty"`
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

	// Determine device type
	deviceType := domain.DeviceType(req.DeviceType)

	if deviceType == domain.DeviceTypeVirtual {
		// Per D-08: Virtual devices allow empty IP, skip SNMP validation
		// Per D-09: Require display_name and valid virtual_subtype
		if req.Tags == nil {
			req.Tags = make(map[string]string)
		}
		displayName := strings.TrimSpace(req.Tags["display_name"])
		if displayName == "" {
			writeError(w, http.StatusBadRequest, "tags.display_name is required for virtual devices")
			return
		}
		subtype := strings.TrimSpace(req.Tags["virtual_subtype"])
		validSubtypes := map[string]bool{"internet": true, "cloud": true, "server": true, "generic": true}
		if !validSubtypes[subtype] {
			writeError(w, http.StatusBadRequest, "tags.virtual_subtype must be one of: internet, cloud, server, generic")
			return
		}

		// Parse optional area IDs (same logic as regular devices)
		var areaIDs []uuid.UUID
		for _, idStr := range req.AreaIDs {
			if idStr == "" {
				continue
			}
			parsed, err := uuid.Parse(idStr)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid area_id: "+idStr)
				return
			}
			areaIDs = append(areaIDs, parsed)
		}

		device, err := h.svc.AddDevice(r.Context(), req.IP, req.Hostname,
			domain.DeviceTypeVirtual,
			domain.SNMPCredentials{}, req.Tags, "", "", "", "", "", areaIDs, req.Notes)
		if err != nil {
			if isDeviceIPConflict(err) {
				writeError(w, http.StatusConflict, duplicateDeviceAddressMessage(req.IP))
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to create virtual device", err)
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(jsonAPISingle{Data: h.deviceToResource(device)})
		return
	}

	// Per D-10: Regular device creation retains "ip is required" validation
	if req.IP == "" {
		writeError(w, http.StatusBadRequest, "ip is required")
		return
	}
	if !isValidIPOrHostname(req.IP) {
		writeError(w, http.StatusBadRequest, "ip must be a valid IP address or hostname")
		return
	}
	if req.Hostname != "" {
		req.Hostname = strings.TrimSpace(req.Hostname)
		if len(req.Hostname) > 253 {
			writeError(w, http.StatusBadRequest, "hostname too long (max 253 characters)")
			return
		}
	}
	if !validDeviceTypes[req.DeviceType] {
		writeError(w, http.StatusBadRequest, "invalid device_type")
		return
	}
	if !validMetricsSources[req.MetricsSource] {
		writeError(w, http.StatusBadRequest, "invalid metrics_source")
		return
	}
	if !validTopologyDiscoveryModes[req.TopologyDiscoveryMode] {
		writeError(w, http.StatusBadRequest, "invalid topology_discovery_mode")
		return
	}
	if len(req.Vendor) > 255 {
		writeError(w, http.StatusBadRequest, "vendor too long (max 255 characters)")
		return
	}
	if len(req.PrometheusLabelName) > 255 {
		writeError(w, http.StatusBadRequest, "prometheus_label_name too long (max 255 characters)")
		return
	}
	if len(req.PrometheusLabelValue) > 255 {
		writeError(w, http.StatusBadRequest, "prometheus_label_value too long (max 255 characters)")
		return
	}
	for k, v := range req.Tags {
		if len(k) > 255 {
			writeError(w, http.StatusBadRequest, "tag key too long (max 255 characters)")
			return
		}
		if len(v) > 255 {
			writeError(w, http.StatusBadRequest, "tag value too long (max 255 characters)")
			return
		}
	}

	creds, err := parseSNMPCreds(req.SNMP)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	metricsSource := domain.MetricsSource(req.MetricsSource)
	prometheusLabelName := req.PrometheusLabelName
	prometheusLabelValue := req.PrometheusLabelValue

	var areaIDs []uuid.UUID
	for _, idStr := range req.AreaIDs {
		if idStr == "" {
			continue
		}
		parsed, err := uuid.Parse(idStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid area_id: "+idStr)
			return
		}
		areaIDs = append(areaIDs, parsed)
	}

	if deviceType == "" {
		deviceType = domain.DeviceTypeUnknown
	}
	device, err := h.svc.AddDevice(r.Context(), req.IP, req.Hostname,
		deviceType,
		creds, req.Tags, req.Vendor, metricsSource,
		prometheusLabelName, prometheusLabelValue, domain.TopologyDiscoveryMode(req.TopologyDiscoveryMode), areaIDs, req.Notes)
	if err != nil {
		if isDeviceIPConflict(err) {
			writeError(w, http.StatusConflict, duplicateDeviceAddressMessage(req.IP))
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create device", err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(jsonAPISingle{Data: h.deviceToResource(device)})
}

// HandleList handles GET /api/v1/devices
func (h *DeviceHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	devices, err := h.svc.GetAllDevices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list devices", err)
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
		writeError(w, http.StatusInternalServerError, "failed to get device", err)
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

	// Validate optional update fields
	if req.IP != nil && *req.IP != "" && !isValidIPOrHostname(*req.IP) {
		writeError(w, http.StatusBadRequest, "ip must be a valid IP address or hostname")
		return
	}
	if req.Hostname != nil {
		h := strings.TrimSpace(*req.Hostname)
		if len(h) > 253 {
			writeError(w, http.StatusBadRequest, "hostname too long (max 253 characters)")
			return
		}
		req.Hostname = &h
	}
	if req.MetricsSource != nil && !validMetricsSources[*req.MetricsSource] {
		writeError(w, http.StatusBadRequest, "invalid metrics_source")
		return
	}
	if req.TopologyDiscoveryMode != nil && !validTopologyDiscoveryModes[*req.TopologyDiscoveryMode] {
		writeError(w, http.StatusBadRequest, "invalid topology_discovery_mode")
		return
	}
	if req.Vendor != nil && len(*req.Vendor) > 255 {
		writeError(w, http.StatusBadRequest, "vendor too long (max 255 characters)")
		return
	}
	if req.PrometheusLabelName != nil && len(*req.PrometheusLabelName) > 255 {
		writeError(w, http.StatusBadRequest, "prometheus_label_name too long (max 255 characters)")
		return
	}
	if req.PrometheusLabelValue != nil && len(*req.PrometheusLabelValue) > 255 {
		writeError(w, http.StatusBadRequest, "prometheus_label_value too long (max 255 characters)")
		return
	}
	if req.Tags != nil {
		for k, v := range *req.Tags {
			if len(k) > 255 {
				writeError(w, http.StatusBadRequest, "tag key too long (max 255 characters)")
				return
			}
			if len(v) > 255 {
				writeError(w, http.StatusBadRequest, "tag value too long (max 255 characters)")
				return
			}
		}
	}
	if req.PollIntervalOverride.Set && req.PollIntervalOverride.Value != nil {
		if *req.PollIntervalOverride.Value < 5 || *req.PollIntervalOverride.Value > 3600 {
			writeError(w, http.StatusBadRequest, "poll_interval_override must be between 5 and 3600 seconds")
			return
		}
	}

	update := service.DeviceUpdate{
		Hostname: req.Hostname,
		IP:       req.IP,
		Tags:     req.Tags,
	}
	if req.Notes.Set {
		update.Notes = &req.Notes.Value
	}
	if req.PollIntervalOverride.Set {
		update.PollIntervalOverride = &req.PollIntervalOverride.Value
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
	if req.TopologyDiscoveryMode != nil {
		mode := domain.TopologyDiscoveryMode(*req.TopologyDiscoveryMode)
		update.TopologyDiscoveryMode = &mode
	}
	if req.AreaIDs != nil {
		var parsedIDs []uuid.UUID
		for _, idStr := range *req.AreaIDs {
			if idStr == "" {
				continue
			}
			parsed, err := uuid.Parse(idStr)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid area_id: "+idStr)
				return
			}
			parsedIDs = append(parsedIDs, parsed)
		}
		update.AreaIDs = &parsedIDs
	}

	if err := h.svc.UpdateDevice(r.Context(), id, update); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if isDeviceIPConflict(err) {
			writeError(w, http.StatusConflict, duplicateDeviceAddressMessage(derefString(req.IP)))
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update device", err)
		return
	}

	// Return updated device
	device, err := h.svc.GetDevice(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get device", err)
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
		writeError(w, http.StatusInternalServerError, "failed to delete device", err)
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
		writeError(w, http.StatusInternalServerError, "failed to probe device", err)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "probing"})
}

// HandleRunTopologyDiscovery handles POST /api/v1/devices/{id}/topology-discovery.
func (h *DeviceHandler) HandleRunTopologyDiscovery(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/topology-discovery")
	id, err := extractIDFromPath(path, "/api/v1/devices/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device ID")
		return
	}

	if err := h.svc.RunTopologyDiscoveryNow(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if strings.Contains(err.Error(), "requires") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to run topology discovery", err)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "topology_discovery_started"})
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
		writeError(w, http.StatusInternalServerError, "failed to test SNMP", err)
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

	type batchAddFailure struct {
		IP     string `json:"ip"`
		Reason string `json:"reason"`
	}
	var failures []batchAddFailure

	// Add each device
	for _, d := range req.Devices {
		if d.IP != "" && !isValidIPOrHostname(d.IP) {
			failures = append(failures, batchAddFailure{IP: d.IP, Reason: "invalid IP address or hostname"})
			continue
		}
		creds, err := parseSNMPCreds(d.SNMP)
		if err != nil {
			failures = append(failures, batchAddFailure{IP: d.IP, Reason: err.Error()})
			continue
		}
		ms := domain.MetricsSource(d.MetricsSource)
		batchDeviceType := domain.DeviceType(d.DeviceType)
		if batchDeviceType == "" {
			batchDeviceType = domain.DeviceTypeUnknown
		}
		if _, err := h.svc.AddDevice(r.Context(), d.IP, d.Hostname,
			batchDeviceType,
			creds, d.Tags, d.Vendor, ms,
			d.PrometheusLabelName, d.PrometheusLabelValue, domain.TopologyDiscoveryMode(d.TopologyDiscoveryMode), nil, d.Notes); err != nil {
			failures = append(failures, batchAddFailure{IP: d.IP, Reason: err.Error()})
		}
	}

	if failures == nil {
		failures = []batchAddFailure{}
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"batch_id": batchID,
		"status":   "processing",
		"count":    len(req.Devices),
		"failures": failures,
	})
}

// --- Helpers ---

func (h *DeviceHandler) deviceToResource(d *domain.Device) jsonAPIResource {
	attrs := map[string]interface{}{
		"hostname":                          d.Hostname,
		"ip":                                d.IP,
		"notes":                             d.Notes,
		"device_type":                       string(d.DeviceType),
		"poll_class":                        string(d.PollClass),
		"poll_interval_override":            d.PollIntervalOverride,
		"status":                            string(d.Status),
		"sys_name":                          d.SysName,
		"sys_descr":                         d.SysDescr,
		"sys_object_id":                     d.SysObjectID,
		"hardware_model":                    d.HardwareModel,
		"os_version":                        d.OSVersion,
		"vendor":                            d.Vendor,
		"managed":                           d.Managed,
		"tags":                              d.Tags,
		"metrics_source":                    string(d.MetricsSource),
		"prometheus_label_name":             d.PrometheusLabelName,
		"prometheus_label_value":            d.PrometheusLabelValue,
		"topology_discovery_mode":           string(d.TopologyDiscoveryMode),
		"effective_topology_discovery_mode": string(d.EffectiveTopologyDiscoveryMode),
		"topology_bootstrap_state":          string(d.TopologyBootstrapState),
		"last_topology_discovery_at":        d.LastTopologyDiscoveryAt,
		"last_topology_discovery_result":    d.LastTopologyDiscoveryResult,
		"created_at":                        d.CreatedAt,
		"updated_at":                        d.UpdatedAt,
	}

	areaIDStrs := make([]string, 0, len(d.AreaIDs))
	for _, aid := range d.AreaIDs {
		areaIDStrs = append(areaIDStrs, aid.String())
	}
	attrs["area_ids"] = areaIDStrs

	attrs["backup_supported"] = h.vendorRegistry.ResolveBackupConfig(d.Vendor).Supported

	return jsonAPIResource{
		Type:          "device",
		ID:            d.ID.String(),
		Attributes:    attrs,
		Relationships: nil,
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
		if req.AuthProtocol != "" && !validSNMPv3AuthProtocols[req.AuthProtocol] {
			return domain.SNMPCredentials{}, fmt.Errorf("invalid auth_protocol: must be one of MD5, SHA, SHA-224, SHA-256, SHA-384, SHA-512")
		}
		if req.PrivProtocol != "" && !validSNMPv3PrivProtocols[req.PrivProtocol] {
			return domain.SNMPCredentials{}, fmt.Errorf("invalid priv_protocol: must be one of DES, AES")
		}
		if req.SecurityLevel != "" && !validSNMPv3SecurityLevels[req.SecurityLevel] {
			return domain.SNMPCredentials{}, fmt.Errorf("invalid security_level: must be one of noAuthNoPriv, authNoPriv, authPriv")
		}
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

func writeError(w http.ResponseWriter, code int, message string, internalErr ...error) {
	if code == http.StatusInternalServerError {
		ref := uuid.New().String()[:8]
		if len(internalErr) > 0 && internalErr[0] != nil {
			log.Printf("internal error ref=%s: %v", ref, internalErr[0])
		}
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("internal error, ref: %s", ref),
		})
		return
	}
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func isDeviceIPConflict(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "idx_devices_ip") ||
		(strings.Contains(message, "duplicate key value violates unique constraint") && strings.Contains(message, "devices")) ||
		strings.Contains(message, "unique constraint failed: devices.ip")
}

func duplicateDeviceAddressMessage(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return "a device with that address already exists"
	}
	return fmt.Sprintf("a device with IP/host %q already exists", address)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// isValidIPOrHostname returns true if s is a valid IPv4/IPv6 address or RFC 1123 hostname.
func isValidIPOrHostname(s string) bool {
	if net.ParseIP(s) != nil {
		return true
	}
	return isValidHostname(s)
}

// isValidHostname validates s as a hostname (labels 1-63 chars, total <= 253).
// Each label must contain at least one letter and may only contain alphanumeric
// characters and hyphens (hyphens not at start or end of a label). This rejects
// purely numeric labels (e.g. "12345") which are not valid hostnames.
func isValidHostname(s string) bool {
	if len(s) == 0 || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		hasLetter := false
		for i, c := range label {
			switch {
			case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
				hasLetter = true
			case c >= '0' && c <= '9':
				// digits allowed but not exclusively
			case c == '-' && i > 0 && i < len(label)-1:
				// hyphen allowed in the middle only
			default:
				return false
			}
		}
		if !hasLetter {
			return false
		}
	}
	return true
}

// sanitizeFilename strips characters that could enable HTTP response splitting.
func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer(
		"\r", "", "\n", "", "\"", "", ";", "", "\t", "",
	)
	return replacer.Replace(name)
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
