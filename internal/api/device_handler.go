package api

// This file defines device handler HTTP handler behavior and request/response boundaries.

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
	"github.com/lollinoo/theia/internal/service/canvasmap"
	"github.com/lollinoo/theia/internal/vendor"
)

// DeviceHandler provides HTTP handlers for device CRUD operations.
type DeviceHandler struct {
	svc                   *service.DeviceService
	credentialProfileRepo domain.CredentialProfileRepository
	vendorRegistry        *vendor.Registry
	canvasMapRepo         domain.CanvasMapRepository
	areaRepo              domain.AreaRepository
	linkRepo              domain.LinkRepository
}

// DeviceHandlerOption configures optional collaborators for device handlers.
type DeviceHandlerOption func(*DeviceHandler)

// WithPrimaryCanvasMapMembership enables default map membership for API-created devices.
func WithPrimaryCanvasMapMembership(
	mapRepo domain.CanvasMapRepository,
	areaRepo domain.AreaRepository,
	linkRepo domain.LinkRepository,
) DeviceHandlerOption {
	return func(h *DeviceHandler) {
		h.canvasMapRepo = mapRepo
		h.areaRepo = areaRepo
		h.linkRepo = linkRepo
	}
}

// NewDeviceHandler creates a new DeviceHandler.
func NewDeviceHandler(svc *service.DeviceService, credentialProfileRepo domain.CredentialProfileRepository, vendorRegistry *vendor.Registry, options ...DeviceHandlerOption) *DeviceHandler {
	handler := &DeviceHandler{svc: svc, credentialProfileRepo: credentialProfileRepo, vendorRegistry: vendorRegistry}
	for _, option := range options {
		if option != nil {
			option(handler)
		}
	}
	return handler
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
	IP                       string                 `json:"ip"`
	Addresses                []deviceAddressRequest `json:"addresses,omitempty"`
	Hostname                 string                 `json:"hostname"`
	Notes                    *string                `json:"notes"`
	DeviceType               string                 `json:"device_type,omitempty"`
	SNMP                     snmpCredsRequest       `json:"snmp"`
	Tags                     map[string]string      `json:"tags"`
	Vendor                   string                 `json:"vendor,omitempty"`
	MetricsSource            string                 `json:"metrics_source,omitempty"`
	PrometheusLabelName      string                 `json:"prometheus_label_name,omitempty"`
	PrometheusLabelValue     string                 `json:"prometheus_label_value,omitempty"`
	TopologyDiscoveryMode    string                 `json:"topology_discovery_mode,omitempty"`
	AreaIDs                  []string               `json:"area_ids,omitempty"`
	SkipPrimaryMapMembership bool                   `json:"skip_primary_map_membership,omitempty"`
}

type deviceAddressRequest struct {
	Address   string `json:"address"`
	Label     string `json:"label,omitempty"`
	Role      string `json:"role,omitempty"`
	IsPrimary *bool  `json:"is_primary,omitempty"`
	Priority  *int   `json:"priority,omitempty"`
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
	Addresses             *[]deviceAddressRequest      `json:"addresses,omitempty"`
	Notes                 optionalNullableString       `json:"notes"`
	Tags                  *map[string]string           `json:"tags,omitempty"`
	SNMP                  *snmpCredsRequest            `json:"snmp,omitempty"`
	Vendor                *string                      `json:"vendor,omitempty"`
	MetricsSource         *string                      `json:"metrics_source,omitempty"`
	PrometheusLabelName   *string                      `json:"prometheus_label_name,omitempty"`
	PrometheusLabelValue  *string                      `json:"prometheus_label_value,omitempty"`
	TopologyDiscoveryMode *string                      `json:"topology_discovery_mode,omitempty"`
	PollingEnabled        *bool                        `json:"polling_enabled,omitempty"`
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

type validatedCreateDeviceRequest struct {
	IP                       string
	Addresses                []domain.DeviceAddress
	Hostname                 string
	DeviceType               domain.DeviceType
	SNMPCredentials          domain.SNMPCredentials
	Tags                     map[string]string
	Vendor                   string
	MetricsSource            domain.MetricsSource
	PrometheusLabelName      string
	PrometheusLabelValue     string
	TopologyDiscoveryMode    domain.TopologyDiscoveryMode
	AreaIDs                  []uuid.UUID
	Notes                    *string
	Virtual                  bool
	SkipPrimaryMapMembership bool
}

// HandleCreate handles POST /api/v1/devices
func (h *DeviceHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var req createDeviceRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	validated, err := validateCreateDeviceRequest(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	device, err := h.svc.AddDeviceWithAddresses(r.Context(), validated.IP, validated.Hostname,
		validated.DeviceType,
		validated.SNMPCredentials, validated.Tags, validated.Vendor, validated.MetricsSource,
		validated.PrometheusLabelName, validated.PrometheusLabelValue, validated.TopologyDiscoveryMode, validated.AreaIDs, validated.Addresses, validated.Notes)
	if err != nil {
		if isDeviceIPConflict(err) {
			writeError(w, http.StatusConflict, duplicateDeviceAddressMessage(validated.IP))
			return
		}
		if validated.Virtual {
			writeError(w, http.StatusInternalServerError, "failed to create virtual device", err)
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create device", err)
		return
	}
	if !validated.SkipPrimaryMapMembership {
		if err := h.addDeviceToPrimaryCanvasMap(device); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to add device to primary map", err)
			return
		}
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

// HandleListOrphans handles GET /api/v1/devices/orphans.
func (h *DeviceHandler) HandleListOrphans(w http.ResponseWriter, r *http.Request) {
	devices, err := h.svc.GetOrphanDevices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list orphan devices", err)
		return
	}

	resources := make([]jsonAPIResource, 0, len(devices))
	for i := range devices {
		resources = append(resources, h.deviceToResource(&devices[i]))
	}

	json.NewEncoder(w).Encode(jsonAPIList{Data: resources})
}

func (h *DeviceHandler) addDeviceToPrimaryCanvasMap(device *domain.Device) error {
	if h.canvasMapRepo == nil || device == nil {
		return nil
	}
	adder, ok := h.canvasMapRepo.(interface {
		AddDeviceMembership(uuid.UUID, domain.CanvasMapDeviceMembership, []uuid.UUID, []domain.CanvasMapAreaMembership) error
	})
	if !ok {
		return nil
	}
	primaryMap, err := h.canvasMapRepo.GetDefault()
	if err != nil {
		return fmt.Errorf("loading primary canvas map: %w", err)
	}
	areas, areaIDs, err := h.canvasMapAreaMembershipsForDevice(device)
	if err != nil {
		return err
	}
	linkIDs := []uuid.UUID{}
	if h.linkRepo != nil {
		membership, err := h.canvasMapRepo.GetMembership(primaryMap.ID)
		if err != nil {
			return fmt.Errorf("loading primary canvas map membership: %w", err)
		}
		links, err := h.linkRepo.GetByDeviceID(device.ID)
		if err != nil {
			return fmt.Errorf("loading primary canvas map connected links: %w", err)
		}
		linkIDs = canvasmap.ConnectedBaseLinkIDs(device.ID, membership, links)
	}
	return adder.AddDeviceMembership(
		primaryMap.ID,
		domain.CanvasMapDeviceMembership{
			DeviceID: device.ID,
			Role:     domain.CanvasMapDeviceRoleBase,
			AreaIDs:  areaIDs,
		},
		linkIDs,
		areas,
	)
}

func (h *DeviceHandler) canvasMapAreaMembershipsForDevice(device *domain.Device) ([]domain.CanvasMapAreaMembership, []uuid.UUID, error) {
	if h.areaRepo == nil || device == nil || len(device.AreaIDs) == 0 {
		return []domain.CanvasMapAreaMembership{}, []uuid.UUID{}, nil
	}
	areas := make([]domain.CanvasMapAreaMembership, 0, len(device.AreaIDs))
	areaIDs := make([]uuid.UUID, 0, len(device.AreaIDs))
	for _, areaID := range device.AreaIDs {
		area, err := h.areaRepo.GetByID(areaID)
		if err != nil {
			return nil, nil, fmt.Errorf("loading device area %s for primary canvas map: %w", areaID, err)
		}
		areas = append(areas, domain.CanvasMapAreaMembership{
			AreaID:      area.ID,
			Name:        area.Name,
			Description: area.Description,
			Color:       area.Color,
		})
		areaIDs = append(areaIDs, area.ID)
	}
	return areas, areaIDs, nil
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
	var addressesUpdate *[]domain.DeviceAddress
	if req.Addresses != nil {
		addresses, _, err := validateDeviceAddressRequests(derefString(req.IP), *req.Addresses, true)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		addressesUpdate = &addresses
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
		Hostname:  req.Hostname,
		IP:        req.IP,
		Addresses: addressesUpdate,
		Tags:      req.Tags,
	}
	if req.Notes.Set {
		update.Notes = &req.Notes.Value
	}
	if req.PollIntervalOverride.Set {
		update.PollIntervalOverride = &req.PollIntervalOverride.Value
	}
	if req.PollingEnabled != nil {
		update.PollingEnabled = req.PollingEnabled
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

func validateCreateDeviceRequest(req createDeviceRequest) (validatedCreateDeviceRequest, error) {
	deviceType := domain.DeviceType(req.DeviceType)
	addressesProvided := req.Addresses != nil

	if deviceType == domain.DeviceTypeVirtual {
		if req.Tags == nil {
			req.Tags = make(map[string]string)
		}
		displayName := strings.TrimSpace(req.Tags["display_name"])
		if displayName == "" {
			return validatedCreateDeviceRequest{}, errors.New("tags.display_name is required for virtual devices")
		}
		subtype := strings.TrimSpace(req.Tags["virtual_subtype"])
		validSubtypes := map[string]bool{"internet": true, "cloud": true, "server": true, "generic": true}
		if !validSubtypes[subtype] {
			return validatedCreateDeviceRequest{}, errors.New("tags.virtual_subtype must be one of: internet, cloud, server, generic")
		}

		areaIDs, err := parseCreateAreaIDs(req.AreaIDs)
		if err != nil {
			return validatedCreateDeviceRequest{}, err
		}

		return validatedCreateDeviceRequest{
			IP:                       req.IP,
			Hostname:                 req.Hostname,
			DeviceType:               domain.DeviceTypeVirtual,
			SNMPCredentials:          domain.SNMPCredentials{},
			Tags:                     req.Tags,
			MetricsSource:            domain.MetricsSourceNone,
			AreaIDs:                  areaIDs,
			Notes:                    req.Notes,
			Virtual:                  true,
			SkipPrimaryMapMembership: req.SkipPrimaryMapMembership,
		}, nil
	}

	addresses, derivedIP, err := validateDeviceAddressRequests(req.IP, req.Addresses, false)
	if err != nil {
		return validatedCreateDeviceRequest{}, err
	}
	req.IP = derivedIP

	if req.IP == "" && !addressesProvided {
		return validatedCreateDeviceRequest{}, errors.New("ip is required")
	}
	if req.IP == "" {
		return validatedCreateDeviceRequest{}, errors.New("ip is required")
	}
	if !isValidIPOrHostname(req.IP) {
		return validatedCreateDeviceRequest{}, errors.New("ip must be a valid IP address or hostname")
	}
	if req.Hostname != "" {
		req.Hostname = strings.TrimSpace(req.Hostname)
		if len(req.Hostname) > 253 {
			return validatedCreateDeviceRequest{}, errors.New("hostname too long (max 253 characters)")
		}
	}
	if !validDeviceTypes[req.DeviceType] {
		return validatedCreateDeviceRequest{}, errors.New("invalid device_type")
	}
	if !validMetricsSources[req.MetricsSource] {
		return validatedCreateDeviceRequest{}, errors.New("invalid metrics_source")
	}
	if !validTopologyDiscoveryModes[req.TopologyDiscoveryMode] {
		return validatedCreateDeviceRequest{}, errors.New("invalid topology_discovery_mode")
	}
	if len(req.Vendor) > 255 {
		return validatedCreateDeviceRequest{}, errors.New("vendor too long (max 255 characters)")
	}
	if len(req.PrometheusLabelName) > 255 {
		return validatedCreateDeviceRequest{}, errors.New("prometheus_label_name too long (max 255 characters)")
	}
	if len(req.PrometheusLabelValue) > 255 {
		return validatedCreateDeviceRequest{}, errors.New("prometheus_label_value too long (max 255 characters)")
	}
	for k, v := range req.Tags {
		if len(k) > 255 {
			return validatedCreateDeviceRequest{}, errors.New("tag key too long (max 255 characters)")
		}
		if len(v) > 255 {
			return validatedCreateDeviceRequest{}, errors.New("tag value too long (max 255 characters)")
		}
	}

	creds, err := parseSNMPCreds(req.SNMP)
	if err != nil {
		return validatedCreateDeviceRequest{}, err
	}

	areaIDs, err := parseCreateAreaIDs(req.AreaIDs)
	if err != nil {
		return validatedCreateDeviceRequest{}, err
	}

	if deviceType == "" {
		deviceType = domain.DeviceTypeUnknown
	}

	return validatedCreateDeviceRequest{
		IP:                       req.IP,
		Addresses:                addresses,
		Hostname:                 req.Hostname,
		DeviceType:               deviceType,
		SNMPCredentials:          creds,
		Tags:                     req.Tags,
		Vendor:                   req.Vendor,
		MetricsSource:            domain.MetricsSource(req.MetricsSource),
		PrometheusLabelName:      req.PrometheusLabelName,
		PrometheusLabelValue:     req.PrometheusLabelValue,
		TopologyDiscoveryMode:    domain.TopologyDiscoveryMode(req.TopologyDiscoveryMode),
		AreaIDs:                  areaIDs,
		Notes:                    req.Notes,
		SkipPrimaryMapMembership: req.SkipPrimaryMapMembership,
	}, nil
}

func validateDeviceAddressRequests(ip string, raw []deviceAddressRequest, allowEmpty bool) ([]domain.DeviceAddress, string, error) {
	trimmedIP := strings.TrimSpace(ip)
	if len(raw) == 0 {
		return nil, trimmedIP, nil
	}

	addresses := make([]domain.DeviceAddress, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	primaryCount := 0
	for _, item := range raw {
		address := strings.TrimSpace(item.Address)
		if address == "" {
			if allowEmpty {
				continue
			}
			return nil, "", errors.New("address is required")
		}
		if !isValidIPOrHostname(address) {
			return nil, "", errors.New("address must be a valid IP address or hostname")
		}
		normalized := domain.NormalizeDeviceAddressValue(address)
		if _, exists := seen[normalized]; exists {
			return nil, "", fmt.Errorf("duplicate address: %s", address)
		}
		seen[normalized] = struct{}{}

		role, err := parseDeviceAddressRole(item.Role)
		if err != nil {
			return nil, "", err
		}
		isPrimary := role == domain.DeviceAddressRolePrimary
		if item.IsPrimary != nil {
			isPrimary = *item.IsPrimary
		}
		if isPrimary {
			primaryCount++
		}
		priority := 100
		if item.Priority != nil {
			priority = *item.Priority
		}
		addresses = append(addresses, domain.DeviceAddress{
			Address:   address,
			Label:     strings.TrimSpace(item.Label),
			Role:      role,
			IsPrimary: isPrimary,
			Priority:  priority,
		})
	}
	if len(addresses) == 0 {
		return addresses, trimmedIP, nil
	}
	if primaryCount > 1 {
		return nil, "", errors.New("multiple primary addresses are not allowed")
	}

	temp := domain.Device{IP: trimmedIP, Addresses: addresses}
	domain.NormalizeDeviceAddresses(&temp)
	return temp.Addresses, temp.IP, nil
}

func parseDeviceAddressRole(value string) (domain.DeviceAddressRole, error) {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return domain.DeviceAddressRoleOther, nil
	}
	role := domain.NormalizeDeviceAddressRole(domain.DeviceAddressRole(trimmed))
	if string(role) != trimmed {
		return "", fmt.Errorf("invalid address role: %s", value)
	}
	return role, nil
}

func parseCreateAreaIDs(rawIDs []string) ([]uuid.UUID, error) {
	var areaIDs []uuid.UUID
	for _, idStr := range rawIDs {
		if idStr == "" {
			continue
		}
		parsed, err := uuid.Parse(idStr)
		if err != nil {
			return nil, fmt.Errorf("invalid area_id: %s", idStr)
		}
		areaIDs = append(areaIDs, parsed)
	}
	return areaIDs, nil
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
		validated, err := validateCreateDeviceRequest(d)
		if err != nil {
			failures = append(failures, batchAddFailure{IP: d.IP, Reason: err.Error()})
			continue
		}
		device, err := h.svc.AddDevice(r.Context(), validated.IP, validated.Hostname,
			validated.DeviceType,
			validated.SNMPCredentials, validated.Tags, validated.Vendor, validated.MetricsSource,
			validated.PrometheusLabelName, validated.PrometheusLabelValue, validated.TopologyDiscoveryMode, validated.AreaIDs, validated.Notes)
		if err != nil {
			failures = append(failures, batchAddFailure{IP: d.IP, Reason: err.Error()})
			continue
		}
		if !validated.SkipPrimaryMapMembership {
			if err := h.addDeviceToPrimaryCanvasMap(device); err != nil {
				failures = append(failures, batchAddFailure{IP: d.IP, Reason: err.Error()})
			}
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
		"polling_enabled":                   domain.DevicePollingEnabled(*d),
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
	attrs["addresses"] = deviceAddressesToResponse(d.Addresses)

	attrs["backup_supported"] = h.vendorRegistry.ResolveBackupConfig(d.Vendor).Supported

	return jsonAPIResource{
		Type:          "device",
		ID:            d.ID.String(),
		Attributes:    attrs,
		Relationships: nil,
	}
}

func deviceAddressesToResponse(addresses []domain.DeviceAddress) []map[string]interface{} {
	response := make([]map[string]interface{}, 0, len(addresses))
	for _, address := range addresses {
		if strings.TrimSpace(address.Address) == "" {
			continue
		}
		response = append(response, map[string]interface{}{
			"id":         address.ID.String(),
			"device_id":  address.DeviceID.String(),
			"address":    address.Address,
			"label":      address.Label,
			"role":       string(address.Role),
			"is_primary": address.IsPrimary,
			"priority":   address.Priority,
			"created_at": address.CreatedAt,
			"updated_at": address.UpdatedAt,
		})
	}
	return response
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
		strings.Contains(message, "device_addresses") ||
		strings.Contains(message, "devices_ip_physical_virtual_excl") ||
		strings.Contains(message, "exclusion constraint") ||
		(strings.Contains(message, "duplicate key value violates unique constraint") && strings.Contains(message, "devices")) ||
		strings.Contains(message, "unique constraint failed: devices.ip") ||
		strings.Contains(message, "device ip conflict") ||
		strings.Contains(message, "device address conflict")
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
