package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
)

const deviceImportHandlerSecret = "DEVICE_IMPORT_HANDLER_SECRET"

type fakeDeviceImportProvider struct {
	previewResult  service.DeviceImportPreview
	previewErr     error
	previewCalls   int
	previewRequest service.DeviceImportRequest
	commitResult   service.DeviceImportCommitResult
	commitErr      error
	commitCalls    int
	commitRequest  service.DeviceImportRequest
}

func (f *fakeDeviceImportProvider) Preview(
	_ context.Context,
	request service.DeviceImportRequest,
) (service.DeviceImportPreview, error) {
	f.previewCalls++
	f.previewRequest = request
	return f.previewResult, f.previewErr
}

func (f *fakeDeviceImportProvider) Commit(
	_ context.Context,
	request service.DeviceImportRequest,
) (service.DeviceImportCommitResult, error) {
	f.commitCalls++
	f.commitRequest = request
	return f.commitResult, f.commitErr
}

type recordingDeviceImportAuth struct {
	*fakeAPIAuthProvider
	checked []string
}

func (a *recordingDeviceImportAuth) RequirePermission(
	user *service.AuthenticatedUser,
	permission string,
) error {
	a.checked = append(a.checked, permission)
	return a.fakeAPIAuthProvider.RequirePermission(user, permission)
}

type deviceImportMultipartPart struct {
	name      string
	value     []byte
	fileName  string
	forceText bool
}

func TestDeviceImportHandlerPreviewMapsBoundedMultipartRequest(t *testing.T) {
	provider := &fakeDeviceImportProvider{previewResult: service.DeviceImportPreview{
		FileDigest: "sha256:preview",
		Summary:    service.DeviceImportPreviewSummary{Total: 1, Ready: 1},
	}}
	mapID := uuid.New()
	areaID := uuid.New()
	uploaded := []byte("- targets: [\"10.0.9.246\"]\n  labels:\n    identity: \"" + deviceImportHandlerSecret + "\"\n")
	request := newDeviceImportMultipartRequest(t, http.MethodPost, "/api/v1/admin/device-imports/preview", []deviceImportMultipartPart{
		{name: "file", value: uploaded, fileName: "targets.yml"},
		{name: "metrics_mode", value: []byte(service.DeviceImportModePrometheus)},
		{name: "map_id", value: []byte(mapID.String())},
		{name: "area_id", value: []byte(areaID.String())},
	})
	request.Header.Set("X-Forwarded-For", "192.0.2.20, 198.51.100.10")
	request.Header.Set("User-Agent", "device-import-test")
	user, auth := authorizeDeviceImportRequest(request)

	response := httptest.NewRecorder()
	NewDeviceImportHandler(provider, auth).HandlePreview(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if provider.previewCalls != 1 || provider.commitCalls != 0 {
		t.Fatalf("provider calls preview=%d commit=%d", provider.previewCalls, provider.commitCalls)
	}
	got := provider.previewRequest
	if !bytes.Equal(got.FileBytes, uploaded) || got.MetricsMode != service.DeviceImportModePrometheus ||
		got.SNMPProfileID != nil || got.MapID != mapID || got.AreaID == nil || *got.AreaID != areaID ||
		got.ExpectedFileDigest != "" {
		t.Fatalf("preview request = %#v", got)
	}
	if got.Actor.UserID != user.User.User.ID || got.Actor.IPAddress != "192.0.2.20" || got.Actor.UserAgent != "device-import-test" {
		t.Fatalf("actor = %#v", got.Actor)
	}
	if strings.Contains(response.Body.String(), deviceImportHandlerSecret) || strings.Contains(response.Body.String(), "labels") {
		t.Fatalf("response leaked ignored YAML data: %s", response.Body.String())
	}
	var body service.DeviceImportPreview
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if body.FileDigest != provider.previewResult.FileDigest || body.Summary.Ready != 1 {
		t.Fatalf("preview response = %#v", body)
	}
}

func TestDeviceImportHandlerCommitMapsProfileDigestAndActor(t *testing.T) {
	provider := &fakeDeviceImportProvider{commitResult: service.DeviceImportCommitResult{
		FileDigest: "sha256:commit",
		Summary:    service.DeviceImportCommitSummary{Total: 1, Created: 1},
	}}
	mapID := uuid.New()
	profileID := uuid.New()
	uploaded := []byte("- targets: [\"10.0.9.246:161\"]\n")
	request := newDeviceImportMultipartRequest(t, http.MethodPost, "/api/v1/admin/device-imports/commit", []deviceImportMultipartPart{
		{name: "file", value: uploaded, fileName: "targets.yml"},
		{name: "metrics_mode", value: []byte(service.DeviceImportModeSNMP)},
		{name: "snmp_profile_id", value: []byte(profileID.String())},
		{name: "map_id", value: []byte(mapID.String())},
		{name: "expected_file_digest", value: []byte("sha256:expected")},
	})
	request.RemoteAddr = "198.51.100.22:4567"
	user, auth := authorizeDeviceImportRequest(request, domain.PermissionCredentialsRead)

	response := httptest.NewRecorder()
	NewDeviceImportHandler(provider, auth).HandleCommit(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if provider.commitCalls != 1 || provider.previewCalls != 0 {
		t.Fatalf("provider calls preview=%d commit=%d", provider.previewCalls, provider.commitCalls)
	}
	got := provider.commitRequest
	if !bytes.Equal(got.FileBytes, uploaded) || got.MetricsMode != service.DeviceImportModeSNMP ||
		got.SNMPProfileID == nil || *got.SNMPProfileID != profileID || got.MapID != mapID || got.AreaID != nil ||
		got.ExpectedFileDigest != "sha256:expected" {
		t.Fatalf("commit request = %#v", got)
	}
	if got.Actor.UserID != user.User.User.ID || got.Actor.IPAddress != "198.51.100.22:4567" {
		t.Fatalf("actor = %#v", got.Actor)
	}
	if !containsString(auth.checked, domain.PermissionCredentialsRead) || containsString(auth.checked, domain.PermissionCredentialsReveal) {
		t.Fatalf("checked permissions = %#v", auth.checked)
	}
}

func TestDeviceImportHandlerRejectsMalformedOrUnboundedMultipartBeforeProvider(t *testing.T) {
	mapID := uuid.New().String()
	validParts := []deviceImportMultipartPart{
		{name: "file", value: []byte("- targets: [\"router.example.net\"]\n"), fileName: "targets.yml"},
		{name: "metrics_mode", value: []byte(service.DeviceImportModePrometheus)},
		{name: "map_id", value: []byte(mapID)},
	}
	tests := []struct {
		name       string
		operation  string
		method     string
		parts      []deviceImportMultipartPart
		plainBody  string
		wantStatus int
	}{
		{name: "method", operation: "preview", method: http.MethodGet, parts: validParts, wantStatus: http.StatusMethodNotAllowed},
		{name: "malformed", operation: "preview", method: http.MethodPost, plainBody: "not multipart", wantStatus: http.StatusBadRequest},
		{name: "missing file", operation: "preview", method: http.MethodPost, parts: validParts[1:], wantStatus: http.StatusBadRequest},
		{name: "file sent as text field", operation: "preview", method: http.MethodPost, parts: replaceDeviceImportPartKind(validParts, "file", true, ""), wantStatus: http.StatusBadRequest},
		{name: "map sent as file field", operation: "preview", method: http.MethodPost, parts: replaceDeviceImportPartKind(validParts, "map_id", false, "map.txt"), wantStatus: http.StatusBadRequest},
		{name: "unknown field", operation: "preview", method: http.MethodPost, parts: appendParts(validParts, deviceImportMultipartPart{name: "labels", value: []byte(deviceImportHandlerSecret)}), wantStatus: http.StatusBadRequest},
		{name: "duplicate file", operation: "preview", method: http.MethodPost, parts: appendParts(validParts, validParts[0]), wantStatus: http.StatusBadRequest},
		{name: "duplicate mode", operation: "preview", method: http.MethodPost, parts: appendParts(validParts, validParts[1]), wantStatus: http.StatusBadRequest},
		{name: "invalid mode", operation: "preview", method: http.MethodPost, parts: replaceDeviceImportPart(validParts, "metrics_mode", []byte("other")), wantStatus: http.StatusBadRequest},
		{name: "invalid map", operation: "preview", method: http.MethodPost, parts: replaceDeviceImportPart(validParts, "map_id", []byte("not-a-uuid")), wantStatus: http.StatusBadRequest},
		{name: "missing mode", operation: "preview", method: http.MethodPost, parts: []deviceImportMultipartPart{validParts[0], validParts[2]}, wantStatus: http.StatusBadRequest},
		{name: "missing map", operation: "preview", method: http.MethodPost, parts: validParts[:2], wantStatus: http.StatusBadRequest},
		{name: "invalid area", operation: "preview", method: http.MethodPost, parts: appendParts(validParts, deviceImportMultipartPart{name: "area_id", value: []byte("not-a-uuid")}), wantStatus: http.StatusBadRequest},
		{name: "empty area", operation: "preview", method: http.MethodPost, parts: appendParts(validParts, deviceImportMultipartPart{name: "area_id", value: nil}), wantStatus: http.StatusBadRequest},
		{name: "invalid profile", operation: "preview", method: http.MethodPost, parts: appendParts(validParts, deviceImportMultipartPart{name: "snmp_profile_id", value: []byte("not-a-uuid")}), wantStatus: http.StatusBadRequest},
		{name: "empty profile", operation: "preview", method: http.MethodPost, parts: appendParts(validParts, deviceImportMultipartPart{name: "snmp_profile_id", value: nil}), wantStatus: http.StatusBadRequest},
		{name: "preview digest forbidden", operation: "preview", method: http.MethodPost, parts: appendParts(validParts, deviceImportMultipartPart{name: "expected_file_digest", value: []byte("sha256:nope")}), wantStatus: http.StatusBadRequest},
		{name: "commit digest required", operation: "commit", method: http.MethodPost, parts: validParts, wantStatus: http.StatusBadRequest},
		{name: "oversized file", operation: "preview", method: http.MethodPost, parts: replaceDeviceImportPart(validParts, "file", make([]byte, service.DeviceImportMaxFileBytes+1)), wantStatus: http.StatusRequestEntityTooLarge},
		{name: "oversized text", operation: "preview", method: http.MethodPost, parts: replaceDeviceImportPart(validParts, "map_id", bytes.Repeat([]byte("a"), deviceImportTextFieldMaxBytes+1)), wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &fakeDeviceImportProvider{}
			path := "/api/v1/admin/device-imports/" + tt.operation
			var request *http.Request
			if tt.plainBody != "" {
				request = httptest.NewRequest(tt.method, path, strings.NewReader(tt.plainBody))
				request.Header.Set("Content-Type", "text/plain")
			} else {
				request = newDeviceImportMultipartRequest(t, tt.method, path, tt.parts)
			}
			_, auth := authorizeDeviceImportRequest(request, domain.PermissionCredentialsRead)
			response := httptest.NewRecorder()
			handler := NewDeviceImportHandler(provider, auth)
			if tt.operation == "commit" {
				handler.HandleCommit(response, request)
			} else {
				handler.HandlePreview(response, request)
			}
			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, tt.wantStatus, response.Body.String())
			}
			if provider.previewCalls != 0 || provider.commitCalls != 0 {
				t.Fatalf("provider called preview=%d commit=%d", provider.previewCalls, provider.commitCalls)
			}
			if strings.Contains(response.Body.String(), deviceImportHandlerSecret) {
				t.Fatalf("response leaked multipart value: %s", response.Body.String())
			}
		})
	}
}

func TestDeviceImportHandlerRejectsTruncatedMultipartAsBadRequest(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	filePart, err := writer.CreateFormFile("file", "targets.yml")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := filePart.Write([]byte("- targets: [\"router.example.net\"]\n")); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writer.WriteField("metrics_mode", string(service.DeviceImportModePrometheus)); err != nil {
		t.Fatalf("WriteField(metrics_mode): %v", err)
	}
	if err := writer.WriteField("map_id", uuid.NewString()); err != nil {
		t.Fatalf("WriteField(map_id): %v", err)
	}
	// Deliberately omit writer.Close so the terminating boundary is absent.
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/device-imports/preview", bytes.NewReader(body.Bytes()))
	request.Header.Set("Content-Type", writer.FormDataContentType())
	provider := &fakeDeviceImportProvider{}
	_, auth := authorizeDeviceImportRequest(request)
	response := httptest.NewRecorder()

	NewDeviceImportHandler(provider, auth).HandlePreview(response, request)

	if response.Code != http.StatusBadRequest || provider.previewCalls != 0 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, provider.previewCalls, response.Body.String())
	}
}

func TestDeviceImportHandlerAcceptsExactTwoMiBFile(t *testing.T) {
	provider := &fakeDeviceImportProvider{}
	parts := []deviceImportMultipartPart{
		{name: "file", value: bytes.Repeat([]byte("a"), service.DeviceImportMaxFileBytes), fileName: "targets.yml"},
		{name: "metrics_mode", value: []byte(service.DeviceImportModePrometheus)},
		{name: "map_id", value: []byte(uuid.NewString())},
	}
	request := newDeviceImportMultipartRequest(t, http.MethodPost, "/api/v1/admin/device-imports/preview", parts)
	_, auth := authorizeDeviceImportRequest(request)
	response := httptest.NewRecorder()

	NewDeviceImportHandler(provider, auth).HandlePreview(response, request)

	if response.Code != http.StatusOK || provider.previewCalls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, provider.previewCalls, response.Body.String())
	}
	if len(provider.previewRequest.FileBytes) != service.DeviceImportMaxFileBytes {
		t.Fatalf("file bytes = %d, want %d", len(provider.previewRequest.FileBytes), service.DeviceImportMaxFileBytes)
	}
}

func TestDeviceImportHandlerEnforcesAllBaselineAndConditionalPermissions(t *testing.T) {
	baseline := []string{
		domain.PermissionAdminDashboard,
		domain.PermissionDevicesRead,
		domain.PermissionDevicesCreate,
		domain.PermissionTopologyRead,
		domain.PermissionTopologyUpdate,
	}
	validParts := []deviceImportMultipartPart{
		{name: "file", value: []byte("- targets: [\"router.example.net\"]\n"), fileName: "targets.yml"},
		{name: "metrics_mode", value: []byte(service.DeviceImportModePrometheus)},
		{name: "map_id", value: []byte(uuid.NewString())},
	}

	for _, missing := range baseline {
		t.Run("missing "+missing, func(t *testing.T) {
			permissions := withoutString(baseline, missing)
			provider := &fakeDeviceImportProvider{}
			request := newDeviceImportMultipartRequest(t, http.MethodPost, "/api/v1/admin/device-imports/preview", validParts)
			_, auth := authorizeDeviceImportRequestWithPermissions(request, permissions)
			response := httptest.NewRecorder()

			NewDeviceImportHandler(provider, auth).HandlePreview(response, request)

			if response.Code != http.StatusForbidden || provider.previewCalls != 0 {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, provider.previewCalls, response.Body.String())
			}
		})
	}

	t.Run("prometheus does not require credentials read", func(t *testing.T) {
		provider := &fakeDeviceImportProvider{}
		request := newDeviceImportMultipartRequest(t, http.MethodPost, "/api/v1/admin/device-imports/preview", validParts)
		_, auth := authorizeDeviceImportRequest(request)
		response := httptest.NewRecorder()
		NewDeviceImportHandler(provider, auth).HandlePreview(response, request)
		if response.Code != http.StatusOK || provider.previewCalls != 1 || containsString(auth.checked, domain.PermissionCredentialsRead) {
			t.Fatalf("status=%d calls=%d checked=%#v body=%s", response.Code, provider.previewCalls, auth.checked, response.Body.String())
		}
	})

	for _, mode := range []service.DeviceImportMode{service.DeviceImportModePrometheusFallback, service.DeviceImportModeSNMP} {
		t.Run(string(mode)+" requires credentials read", func(t *testing.T) {
			provider := &fakeDeviceImportProvider{}
			parts := replaceDeviceImportPart(validParts, "metrics_mode", []byte(mode))
			parts = appendParts(parts, deviceImportMultipartPart{name: "snmp_profile_id", value: []byte(uuid.NewString())})
			request := newDeviceImportMultipartRequest(t, http.MethodPost, "/api/v1/admin/device-imports/preview", parts)
			_, auth := authorizeDeviceImportRequest(request)
			response := httptest.NewRecorder()
			NewDeviceImportHandler(provider, auth).HandlePreview(response, request)
			if response.Code != http.StatusForbidden || provider.previewCalls != 0 || !containsString(auth.checked, domain.PermissionCredentialsRead) {
				t.Fatalf("status=%d calls=%d checked=%#v body=%s", response.Code, provider.previewCalls, auth.checked, response.Body.String())
			}
			if containsString(auth.checked, domain.PermissionCredentialsReveal) {
				t.Fatalf("credentials reveal was checked: %#v", auth.checked)
			}
		})
	}
}

func TestDeviceImportHandlerMapsSafeServiceErrors(t *testing.T) {
	tests := []struct {
		name       string
		operation  string
		err        error
		wantStatus int
		wantText   string
	}{
		{name: "invalid file", operation: "preview", err: fmt.Errorf("%w: %s", service.ErrDeviceImportInvalidFile, deviceImportHandlerSecret), wantStatus: http.StatusBadRequest, wantText: "invalid device import file"},
		{name: "invalid configuration", operation: "preview", err: fmt.Errorf("%w: unavailable", service.ErrDeviceImportInvalidConfiguration), wantStatus: http.StatusBadRequest, wantText: "invalid device import configuration"},
		{name: "limit", operation: "preview", err: fmt.Errorf("%w: too many", service.ErrDeviceImportLimitExceeded), wantStatus: http.StatusRequestEntityTooLarge, wantText: "device import limit exceeded"},
		{name: "digest mismatch", operation: "commit", err: service.ErrDeviceImportDigestMismatch, wantStatus: http.StatusConflict, wantText: "device import digest mismatch"},
		{name: "configuration changed", operation: "commit", err: service.ErrDeviceImportConfigurationChanged, wantStatus: http.StatusConflict, wantText: "device import configuration changed"},
		{name: "store unavailable", operation: "preview", err: fmt.Errorf("%w: %s", domain.ErrDeviceImportStoreUnavailable, deviceImportHandlerSecret), wantStatus: http.StatusServiceUnavailable, wantText: "device import store unavailable"},
		{name: "unknown", operation: "preview", err: errors.New("database " + deviceImportHandlerSecret), wantStatus: http.StatusInternalServerError, wantText: "internal error, ref:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &fakeDeviceImportProvider{}
			if tt.operation == "commit" {
				provider.commitErr = tt.err
			} else {
				provider.previewErr = tt.err
			}
			parts := []deviceImportMultipartPart{
				{name: "file", value: []byte("- targets: [\"router.example.net\"]\n"), fileName: "targets.yml"},
				{name: "metrics_mode", value: []byte(service.DeviceImportModePrometheus)},
				{name: "map_id", value: []byte(uuid.NewString())},
			}
			if tt.operation == "commit" {
				parts = appendParts(parts, deviceImportMultipartPart{name: "expected_file_digest", value: []byte("sha256:expected")})
			}
			request := newDeviceImportMultipartRequest(t, http.MethodPost, "/api/v1/admin/device-imports/"+tt.operation, parts)
			_, auth := authorizeDeviceImportRequest(request)
			response := httptest.NewRecorder()
			var logs bytes.Buffer
			previousWriter := log.Writer()
			log.SetOutput(&logs)
			t.Cleanup(func() { log.SetOutput(previousWriter) })
			handler := NewDeviceImportHandler(provider, auth)
			if tt.operation == "commit" {
				handler.HandleCommit(response, request)
			} else {
				handler.HandlePreview(response, request)
			}
			if response.Code != tt.wantStatus || !strings.Contains(response.Body.String(), tt.wantText) {
				t.Fatalf("status=%d want=%d body=%s", response.Code, tt.wantStatus, response.Body.String())
			}
			if strings.Contains(response.Body.String(), deviceImportHandlerSecret) || strings.Contains(logs.String(), deviceImportHandlerSecret) {
				t.Fatalf("service error leaked: body=%s logs=%s", response.Body.String(), logs.String())
			}
		})
	}
}

func TestDeviceImportHandlerRetainsPartialCommitResultOnSystemicError(t *testing.T) {
	createdID := uuid.New()
	provider := &fakeDeviceImportProvider{
		commitResult: service.DeviceImportCommitResult{
			FileDigest: "sha256:partial",
			Summary: service.DeviceImportCommitSummary{
				Total:        2,
				Created:      1,
				NotProcessed: 1,
			},
			Results: []service.DeviceImportResult{
				{Target: "created.example.net", Address: "created.example.net", Status: service.DeviceImportTargetStatusCreated, DeviceID: &createdID},
				{Target: "pending.example.net", Address: "pending.example.net", Status: service.DeviceImportTargetStatusNotProcessed},
			},
			Incomplete: true,
		},
		commitErr: domain.ErrDeviceImportStoreUnavailable,
	}
	parts := []deviceImportMultipartPart{
		{name: "file", value: []byte("- targets: [\"created.example.net\", \"pending.example.net\"]\n"), fileName: "targets.yml"},
		{name: "metrics_mode", value: []byte(service.DeviceImportModePrometheus)},
		{name: "map_id", value: []byte(uuid.NewString())},
		{name: "expected_file_digest", value: []byte("sha256:expected")},
	}
	request := newDeviceImportMultipartRequest(t, http.MethodPost, "/api/v1/admin/device-imports/commit", parts)
	_, auth := authorizeDeviceImportRequest(request)
	response := httptest.NewRecorder()

	NewDeviceImportHandler(provider, auth).HandleCommit(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", response.Code, response.Body.String())
	}
	var body struct {
		service.DeviceImportCommitResult
		Error string `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode partial response: %v", err)
	}
	if body.Error != "device import store unavailable" || !body.Incomplete || len(body.Results) != 2 ||
		body.Results[0].Status != service.DeviceImportTargetStatusCreated ||
		body.Results[1].Status != service.DeviceImportTargetStatusNotProcessed {
		t.Fatalf("partial response = %#v", body)
	}
}

func TestRouterDeviceImportRequiresSessionAndCSRF(t *testing.T) {
	parts := []deviceImportMultipartPart{
		{name: "file", value: []byte("- targets: [\"router.example.net\"]\n"), fileName: "targets.yml"},
		{name: "metrics_mode", value: []byte(service.DeviceImportModePrometheus)},
		{name: "map_id", value: []byte(uuid.NewString())},
	}

	t.Run("session", func(t *testing.T) {
		auth := newFakeAPIAuthProvider()
		provider := &fakeDeviceImportProvider{}
		router := newDeviceImportAuthTestRouter(auth, provider)
		request := newDeviceImportMultipartRequest(t, http.MethodPost, "/api/v1/admin/device-imports/preview", parts)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized || provider.previewCalls != 0 {
			t.Fatalf("status=%d calls=%d body=%s", response.Code, provider.previewCalls, response.Body.String())
		}
	})

	t.Run("csrf", func(t *testing.T) {
		auth := newFakeAPIAuthProvider()
		auth.setSession(testSessionToken, testCSRFToken, testAPIUser("admin", false, deviceImportTestPermissions()...))
		provider := &fakeDeviceImportProvider{}
		router := newDeviceImportAuthTestRouter(auth, provider)
		request := newDeviceImportMultipartRequest(t, http.MethodPost, "/api/v1/admin/device-imports/preview", parts)
		addSessionCookie(request, testSessionToken)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden || provider.previewCalls != 0 || !strings.Contains(response.Body.String(), "csrf_required") {
			t.Fatalf("status=%d calls=%d body=%s", response.Code, provider.previewCalls, response.Body.String())
		}
	})

	t.Run("authorized", func(t *testing.T) {
		auth := newFakeAPIAuthProvider()
		auth.setSession(testSessionToken, testCSRFToken, testAPIUser("admin", false, deviceImportTestPermissions()...))
		provider := &fakeDeviceImportProvider{}
		router := newDeviceImportAuthTestRouter(auth, provider)
		request := newDeviceImportMultipartRequest(t, http.MethodPost, "/api/v1/admin/device-imports/preview", parts)
		addSessionCookie(request, testSessionToken)
		addCSRFCookieAndHeader(request, testCSRFToken)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK || provider.previewCalls != 1 {
			t.Fatalf("status=%d calls=%d body=%s", response.Code, provider.previewCalls, response.Body.String())
		}
	})
}

func TestRouterDeviceImportUploadProfileBoundsWholeMultipartEnvelope(t *testing.T) {
	auth := newFakeAPIAuthProvider()
	auth.setSession(testSessionToken, testCSRFToken, testAPIUser("admin", false, deviceImportTestPermissions()...))

	t.Run("exact file accepted", func(t *testing.T) {
		provider := &fakeDeviceImportProvider{}
		router := newDeviceImportAuthTestRouter(auth, provider)
		request := newDeviceImportMultipartRequest(t, http.MethodPost, "/api/v1/admin/device-imports/preview", []deviceImportMultipartPart{
			{name: "file", value: bytes.Repeat([]byte("a"), service.DeviceImportMaxFileBytes), fileName: "targets.yml"},
			{name: "metrics_mode", value: []byte(service.DeviceImportModePrometheus)},
			{name: "map_id", value: []byte(uuid.NewString())},
		})
		addSessionCookie(request, testSessionToken)
		addCSRFCookieAndHeader(request, testCSRFToken)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK || provider.previewCalls != 1 {
			t.Fatalf("status=%d calls=%d body=%s", response.Code, provider.previewCalls, response.Body.String())
		}
	})

	t.Run("envelope rejected", func(t *testing.T) {
		provider := &fakeDeviceImportProvider{}
		router := newDeviceImportAuthTestRouter(auth, provider)
		request := newDeviceImportMultipartRequest(t, http.MethodPost, "/api/v1/admin/device-imports/preview", []deviceImportMultipartPart{
			{name: "file", value: bytes.Repeat([]byte("a"), service.DeviceImportMaxFileBytes), fileName: strings.Repeat("f", int(deviceImportMultipartEnvelopeOverheadBytes)+1024)},
			{name: "metrics_mode", value: []byte(service.DeviceImportModePrometheus)},
			{name: "map_id", value: []byte(uuid.NewString())},
		})
		addSessionCookie(request, testSessionToken)
		addCSRFCookieAndHeader(request, testCSRFToken)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusRequestEntityTooLarge || provider.previewCalls != 0 {
			t.Fatalf("status=%d calls=%d body=%s", response.Code, provider.previewCalls, response.Body.String())
		}
	})
}

func newDeviceImportMultipartRequest(
	t *testing.T,
	method string,
	path string,
	parts []deviceImportMultipartPart,
) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range parts {
		if !part.forceText && (part.fileName != "" || part.name == "file") {
			filePart, err := writer.CreateFormFile(part.name, part.fileName)
			if err != nil {
				t.Fatalf("CreateFormFile(%s): %v", part.name, err)
			}
			if _, err := filePart.Write(part.value); err != nil {
				t.Fatalf("write file part %s: %v", part.name, err)
			}
			continue
		}
		if err := writer.WriteField(part.name, string(part.value)); err != nil {
			t.Fatalf("WriteField(%s): %v", part.name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("multipart close: %v", err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(body.Bytes()))
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func newDeviceImportAuthTestRouter(auth *fakeAPIAuthProvider, provider deviceImportProvider) http.Handler {
	return NewRouter(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		"",
		nil,
		nil,
		withAuthProvider(auth),
		withDeviceImportProvider(provider),
	)
}

func deviceImportTestPermissions() []string {
	return []string{
		domain.PermissionAdminDashboard,
		domain.PermissionDevicesRead,
		domain.PermissionDevicesCreate,
		domain.PermissionTopologyRead,
		domain.PermissionTopologyUpdate,
	}
}

func authorizeDeviceImportRequest(
	request *http.Request,
	extraPermissions ...string,
) (*service.AuthenticatedUser, *recordingDeviceImportAuth) {
	permissions := []string{
		domain.PermissionAdminDashboard,
		domain.PermissionDevicesRead,
		domain.PermissionDevicesCreate,
		domain.PermissionTopologyRead,
		domain.PermissionTopologyUpdate,
	}
	permissions = append(permissions, extraPermissions...)
	return authorizeDeviceImportRequestWithPermissions(request, permissions)
}

func authorizeDeviceImportRequestWithPermissions(
	request *http.Request,
	permissions []string,
) (*service.AuthenticatedUser, *recordingDeviceImportAuth) {
	user := testAPIUser("device-import-admin", false, permissions...)
	auth := &recordingDeviceImportAuth{fakeAPIAuthProvider: newFakeAPIAuthProvider()}
	*request = *request.WithContext(withAuthenticatedUser(request.Context(), user))
	return user, auth
}

func appendParts(parts []deviceImportMultipartPart, values ...deviceImportMultipartPart) []deviceImportMultipartPart {
	result := append([]deviceImportMultipartPart(nil), parts...)
	return append(result, values...)
}

func replaceDeviceImportPart(
	parts []deviceImportMultipartPart,
	name string,
	value []byte,
) []deviceImportMultipartPart {
	result := append([]deviceImportMultipartPart(nil), parts...)
	for index := range result {
		if result[index].name == name {
			result[index].value = value
			return result
		}
	}
	return append(result, deviceImportMultipartPart{name: name, value: value})
}

func replaceDeviceImportPartKind(
	parts []deviceImportMultipartPart,
	name string,
	forceText bool,
	fileName string,
) []deviceImportMultipartPart {
	result := append([]deviceImportMultipartPart(nil), parts...)
	for index := range result {
		if result[index].name == name {
			result[index].forceText = forceText
			result[index].fileName = fileName
			return result
		}
	}
	return result
}

func withoutString(values []string, omitted string) []string {
	result := make([]string, 0, len(values)-1)
	for _, value := range values {
		if value != omitted {
			result = append(result, value)
		}
	}
	return result
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
