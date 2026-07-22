package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
)

const (
	deviceImportMultipartEnvelopeOverheadBytes int64 = 64 << 10
	deviceImportTextFieldMaxBytes                    = 256
	deviceImportMaxRequestBytes                      = int64(service.DeviceImportMaxFileBytes) + deviceImportMultipartEnvelopeOverheadBytes
)

var deviceImportBaselinePermissions = []string{
	domain.PermissionAdminDashboard,
	domain.PermissionDevicesRead,
	domain.PermissionDevicesCreate,
	domain.PermissionTopologyRead,
	domain.PermissionTopologyUpdate,
}

type deviceImportProvider interface {
	Preview(context.Context, service.DeviceImportRequest) (service.DeviceImportPreview, error)
	Commit(context.Context, service.DeviceImportRequest) (service.DeviceImportCommitResult, error)
}

// DeviceImportHandler exposes one-time, non-retained Admin Area imports.
type DeviceImportHandler struct {
	importer deviceImportProvider
	auth     authProvider
}

// NewDeviceImportHandler creates the bounded preview and commit HTTP boundary.
func NewDeviceImportHandler(importer deviceImportProvider, auth authProvider) *DeviceImportHandler {
	return &DeviceImportHandler{importer: importer, auth: auth}
}

// ServeHTTP dispatches the two exact import routes through one router handler key.
func (h *DeviceImportHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/v1/admin/device-imports/preview":
		h.HandlePreview(w, r)
	case "/api/v1/admin/device-imports/commit":
		h.HandleCommit(w, r)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

// HandlePreview validates and previews a bounded multipart file-SD upload.
func (h *DeviceImportHandler) HandlePreview(w http.ResponseWriter, r *http.Request) {
	h.handle(w, r, false)
}

// HandleCommit replays the exact upload and returns complete or partial commit results.
func (h *DeviceImportHandler) HandleCommit(w http.ResponseWriter, r *http.Request) {
	h.handle(w, r, true)
}

func (h *DeviceImportHandler) handle(w http.ResponseWriter, r *http.Request, commit bool) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	actor, ok := h.authorizeBaseline(w, r)
	if !ok {
		return
	}
	// Keep the handler safe even if it is mounted without the dedicated router profile.
	r.Body = http.MaxBytesReader(w, r.Body, deviceImportMaxRequestBytes)
	request, err := decodeDeviceImportRequest(r, actor, commit)
	if err != nil {
		writeDeviceImportDecodeError(w, err)
		return
	}
	if deviceImportModeRequiresCredentialsRead(request.MetricsMode) &&
		!requirePermission(w, h.auth, actor, domain.PermissionCredentialsRead) {
		return
	}
	if h == nil || h.importer == nil {
		writeError(w, http.StatusServiceUnavailable, "device import service unavailable")
		return
	}

	if commit {
		result, commitErr := h.importer.Commit(r.Context(), request)
		if commitErr != nil {
			writeDeviceImportServiceError(w, commitErr, &result)
			return
		}
		json.NewEncoder(w).Encode(result)
		return
	}

	preview, previewErr := h.importer.Preview(r.Context(), request)
	if previewErr != nil {
		writeDeviceImportServiceError(w, previewErr, nil)
		return
	}
	json.NewEncoder(w).Encode(preview)
}

func (h *DeviceImportHandler) authorizeBaseline(
	w http.ResponseWriter,
	r *http.Request,
) (*service.AuthenticatedUser, bool) {
	actor, ok := AuthenticatedUserFromRequest(r)
	if !ok {
		writeAuthCodeError(w, http.StatusUnauthorized, "authentication_required", "authentication required")
		return nil, false
	}
	for _, permission := range deviceImportBaselinePermissions {
		if !requirePermission(w, h.auth, actor, permission) {
			return nil, false
		}
	}
	return actor, true
}

type decodedDeviceImportFields struct {
	fileBytes          []byte
	metricsMode        string
	snmpProfileID      string
	snmpProfileIDSet   bool
	mapID              string
	areaID             string
	areaIDSet          bool
	expectedFileDigest string
}

func decodeDeviceImportRequest(
	r *http.Request,
	actor *service.AuthenticatedUser,
	commit bool,
) (service.DeviceImportRequest, error) {
	var request service.DeviceImportRequest
	reader, err := r.MultipartReader()
	if err != nil {
		return request, newDeviceImportRequestError(http.StatusBadRequest, "invalid multipart form data")
	}
	fields, err := readDeviceImportMultipart(reader, commit)
	if err != nil {
		return request, err
	}
	if err := drainDeviceImportRequestBody(r.Body); err != nil {
		return request, err
	}
	mode, ok := parseDeviceImportMode(fields.metricsMode)
	if !ok {
		return request, newDeviceImportRequestError(http.StatusBadRequest, "unsupported metrics_mode")
	}
	mapID, err := uuid.Parse(fields.mapID)
	if err != nil || mapID == uuid.Nil {
		return request, newDeviceImportRequestError(http.StatusBadRequest, "invalid map_id")
	}

	var areaID *uuid.UUID
	if fields.areaIDSet {
		parsed, parseErr := uuid.Parse(fields.areaID)
		if parseErr != nil || parsed == uuid.Nil {
			return request, newDeviceImportRequestError(http.StatusBadRequest, "invalid area_id")
		}
		areaID = &parsed
	}
	var profileID *uuid.UUID
	if fields.snmpProfileIDSet {
		parsed, parseErr := uuid.Parse(fields.snmpProfileID)
		if parseErr != nil || parsed == uuid.Nil {
			return request, newDeviceImportRequestError(http.StatusBadRequest, "invalid snmp_profile_id")
		}
		profileID = &parsed
	}
	if commit && fields.expectedFileDigest == "" {
		return request, newDeviceImportRequestError(http.StatusBadRequest, "expected_file_digest is required")
	}

	request = service.DeviceImportRequest{
		FileBytes:          fields.fileBytes,
		MetricsMode:        mode,
		SNMPProfileID:      profileID,
		MapID:              mapID,
		AreaID:             areaID,
		ExpectedFileDigest: fields.expectedFileDigest,
		Actor: service.DeviceImportActor{
			IPAddress: clientIPAddress(r),
			UserAgent: r.UserAgent(),
		},
	}
	if actor != nil {
		request.Actor.UserID = actor.User.User.ID
	}
	return request, nil
}

func readDeviceImportMultipart(reader *multipart.Reader, commit bool) (decodedDeviceImportFields, error) {
	var fields decodedDeviceImportFields
	seen := make(map[string]struct{}, 6)
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if isRequestBodyTooLarge(err) {
				return fields, newDeviceImportRequestError(http.StatusRequestEntityTooLarge, "device import upload is too large")
			}
			return fields, newDeviceImportRequestError(http.StatusBadRequest, "invalid multipart form data")
		}
		name := part.FormName()
		if !deviceImportFieldAllowed(name, commit) {
			part.Close()
			return fields, newDeviceImportRequestError(http.StatusBadRequest, "unknown multipart field")
		}
		if _, duplicate := seen[name]; duplicate {
			part.Close()
			return fields, newDeviceImportRequestError(http.StatusBadRequest, "duplicate multipart field")
		}
		seen[name] = struct{}{}
		if name == "file" {
			if !deviceImportPartHasFilename(part) {
				part.Close()
				return fields, newDeviceImportRequestError(http.StatusBadRequest, "file must be a multipart file field")
			}
			fields.fileBytes, err = readDeviceImportFilePart(part)
		} else {
			if deviceImportPartHasFilename(part) {
				part.Close()
				return fields, newDeviceImportRequestError(http.StatusBadRequest, "invalid multipart text field")
			}
			var value string
			value, err = readDeviceImportTextPart(part)
			if err == nil {
				setDecodedDeviceImportField(&fields, name, value)
			}
		}
		closeErr := part.Close()
		if err != nil {
			return fields, err
		}
		if closeErr != nil {
			return fields, newDeviceImportRequestError(http.StatusBadRequest, "invalid multipart form data")
		}
	}
	if _, ok := seen["file"]; !ok {
		return fields, newDeviceImportRequestError(http.StatusBadRequest, "file is required")
	}
	if _, ok := seen["metrics_mode"]; !ok || fields.metricsMode == "" {
		return fields, newDeviceImportRequestError(http.StatusBadRequest, "metrics_mode is required")
	}
	if _, ok := seen["map_id"]; !ok || fields.mapID == "" {
		return fields, newDeviceImportRequestError(http.StatusBadRequest, "map_id is required")
	}
	return fields, nil
}

func deviceImportFieldAllowed(name string, commit bool) bool {
	switch name {
	case "file", "metrics_mode", "snmp_profile_id", "map_id", "area_id":
		return true
	case "expected_file_digest":
		return commit
	default:
		return false
	}
}

func deviceImportPartHasFilename(part *multipart.Part) bool {
	_, parameters, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
	if err != nil {
		return false
	}
	_, present := parameters["filename"]
	return present
}

func readDeviceImportFilePart(part io.Reader) ([]byte, error) {
	content, err := io.ReadAll(io.LimitReader(part, int64(service.DeviceImportMaxFileBytes)+1))
	if err != nil {
		if isRequestBodyTooLarge(err) {
			return nil, newDeviceImportRequestError(http.StatusRequestEntityTooLarge, "device import upload is too large")
		}
		return nil, newDeviceImportRequestError(http.StatusBadRequest, "invalid multipart form data")
	}
	if len(content) > service.DeviceImportMaxFileBytes {
		return nil, newDeviceImportRequestError(http.StatusRequestEntityTooLarge, "device import file is too large")
	}
	return content, nil
}

func readDeviceImportTextPart(part io.Reader) (string, error) {
	content, err := io.ReadAll(io.LimitReader(part, deviceImportTextFieldMaxBytes+1))
	if err != nil {
		if isRequestBodyTooLarge(err) {
			return "", newDeviceImportRequestError(http.StatusRequestEntityTooLarge, "device import upload is too large")
		}
		return "", newDeviceImportRequestError(http.StatusBadRequest, "invalid multipart form data")
	}
	if len(content) > deviceImportTextFieldMaxBytes {
		return "", newDeviceImportRequestError(http.StatusBadRequest, "multipart text field is too large")
	}
	return strings.TrimSpace(string(content)), nil
}

func drainDeviceImportRequestBody(body io.Reader) error {
	if _, err := io.Copy(io.Discard, body); err != nil {
		if isRequestBodyTooLarge(err) {
			return newDeviceImportRequestError(http.StatusRequestEntityTooLarge, "device import upload is too large")
		}
		return newDeviceImportRequestError(http.StatusBadRequest, "invalid multipart form data")
	}
	return nil
}

func setDecodedDeviceImportField(fields *decodedDeviceImportFields, name, value string) {
	switch name {
	case "metrics_mode":
		fields.metricsMode = value
	case "snmp_profile_id":
		fields.snmpProfileID = value
		fields.snmpProfileIDSet = true
	case "map_id":
		fields.mapID = value
	case "area_id":
		fields.areaID = value
		fields.areaIDSet = true
	case "expected_file_digest":
		fields.expectedFileDigest = value
	}
}

func parseDeviceImportMode(value string) (service.DeviceImportMode, bool) {
	mode := service.DeviceImportMode(value)
	switch mode {
	case service.DeviceImportModePrometheus,
		service.DeviceImportModePrometheusFallback,
		service.DeviceImportModeSNMP:
		return mode, true
	default:
		return "", false
	}
}

func deviceImportModeRequiresCredentialsRead(mode service.DeviceImportMode) bool {
	return mode == service.DeviceImportModePrometheusFallback || mode == service.DeviceImportModeSNMP
}

type deviceImportRequestError struct {
	status  int
	message string
}

func newDeviceImportRequestError(status int, message string) error {
	return &deviceImportRequestError{status: status, message: message}
}

func (e *deviceImportRequestError) Error() string {
	return e.message
}

func writeDeviceImportDecodeError(w http.ResponseWriter, err error) {
	var requestErr *deviceImportRequestError
	if errors.As(err, &requestErr) {
		writeError(w, requestErr.status, requestErr.message)
		return
	}
	writeDeviceImportInternalError(w, nil)
}

func writeDeviceImportServiceError(
	w http.ResponseWriter,
	err error,
	partial *service.DeviceImportCommitResult,
) {
	status, message := deviceImportServiceErrorStatus(err)
	if status == http.StatusInternalServerError {
		message = deviceImportInternalErrorMessage()
	}
	if partial != nil && (partial.Incomplete || len(partial.Results) > 0) {
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(struct {
			service.DeviceImportCommitResult
			Error string `json:"error"`
		}{DeviceImportCommitResult: *partial, Error: message})
		return
	}
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func deviceImportServiceErrorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, service.ErrDeviceImportInvalidFile):
		return http.StatusBadRequest, service.ErrDeviceImportInvalidFile.Error()
	case errors.Is(err, service.ErrDeviceImportInvalidConfiguration):
		return http.StatusBadRequest, service.ErrDeviceImportInvalidConfiguration.Error()
	case errors.Is(err, service.ErrDeviceImportLimitExceeded):
		return http.StatusRequestEntityTooLarge, service.ErrDeviceImportLimitExceeded.Error()
	case errors.Is(err, service.ErrDeviceImportDigestMismatch):
		return http.StatusConflict, service.ErrDeviceImportDigestMismatch.Error()
	case errors.Is(err, service.ErrDeviceImportConfigurationChanged):
		return http.StatusConflict, service.ErrDeviceImportConfigurationChanged.Error()
	case errors.Is(err, domain.ErrDeviceImportStoreUnavailable):
		return http.StatusServiceUnavailable, domain.ErrDeviceImportStoreUnavailable.Error()
	default:
		return http.StatusInternalServerError, ""
	}
}

func writeDeviceImportInternalError(w http.ResponseWriter, partial *service.DeviceImportCommitResult) {
	message := deviceImportInternalErrorMessage()
	if partial != nil && (partial.Incomplete || len(partial.Results) > 0) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(struct {
			service.DeviceImportCommitResult
			Error string `json:"error"`
		}{DeviceImportCommitResult: *partial, Error: message})
		return
	}
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func deviceImportInternalErrorMessage() string {
	ref := uuid.NewString()[:8]
	log.Printf("device import request failed ref=%s", ref)
	return fmt.Sprintf("internal error, ref: %s", ref)
}
