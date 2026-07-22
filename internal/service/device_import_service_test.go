package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/snmp"
)

const deviceImportTestSecret = "IMPORT_PROFILE_SECRET_MARKER"

func TestDeviceImportServicePreviewComputesExactDigestClassifiesInBulkAndHasNoSideEffects(t *testing.T) {
	h := newDeviceImportServiceHarness(t)
	h.store.existing = map[string]struct{}{"existing.example.net": {}}
	uploaded := []byte(`
- targets:
    - " Existing.Example.NET "
    - "bad host value"
    - "EXISTING.EXAMPLE.NET"
  labels:
    identity: "SHOULD_NOT_APPEAR"
    vendor: "` + deviceImportTestSecret + `"
- labels: {anything: ignored}
- targets: nope
- targets: ["ready.example.net"]
  targets: ["ignored.example.net"]
- targets: ["ready.example.net:9116"]
`)
	expectedDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(uploaded))

	preview, err := h.importer.Preview(context.Background(), h.request(DeviceImportModePrometheus, uploaded))
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if preview.FileDigest != expectedDigest {
		t.Fatalf("FileDigest = %q, want %q", preview.FileDigest, expectedDigest)
	}
	if preview.Configuration.MetricsMode != DeviceImportModePrometheus || preview.Configuration.MapID != h.mapID {
		t.Fatalf("configuration = %#v", preview.Configuration)
	}
	wantStatuses := []DeviceImportTargetStatus{
		DeviceImportTargetStatusSkippedExisting,
		DeviceImportTargetStatusInvalid,
		DeviceImportTargetStatusSkippedDuplicateInFile,
		DeviceImportTargetStatusReady,
	}
	if len(preview.Targets) != len(wantStatuses) {
		t.Fatalf("target count = %d, want %d: %#v", len(preview.Targets), len(wantStatuses), preview.Targets)
	}
	for i, want := range wantStatuses {
		if preview.Targets[i].Status != want {
			t.Fatalf("target[%d] status = %q, want %q", i, preview.Targets[i].Status, want)
		}
	}
	if preview.Targets[0].Target != "Existing.Example.NET" || preview.Targets[0].Address != "existing.example.net" {
		t.Fatalf("first target = %#v", preview.Targets[0])
	}
	if preview.Targets[3].Target != "ready.example.net:9116" || preview.Targets[3].Address != "ready.example.net" {
		t.Fatalf("ready target = %#v", preview.Targets[3])
	}
	if got := preview.Summary; got.Total != 4 || got.Ready != 1 || got.Invalid != 1 ||
		got.InvalidGroups != 3 || got.SkippedExisting != 1 || got.SkippedDuplicateInFile != 1 {
		t.Fatalf("summary = %#v", got)
	}
	if len(preview.Diagnostics) != 3 {
		t.Fatalf("diagnostic count = %d, want 3: %#v", len(preview.Diagnostics), preview.Diagnostics)
	}
	if h.store.existingCalls != 1 {
		t.Fatalf("ExistingCanonicalAddresses calls = %d, want 1", h.store.existingCalls)
	}
	if got := strings.Join(h.store.existingInputs[0], ","); got != "existing.example.net,ready.example.net" {
		t.Fatalf("bulk existing input = %q", got)
	}
	if h.store.createCalls != 0 || h.store.publishCalls != 0 || h.reprobeCalls != 0 || len(h.audit.logs) != 0 {
		t.Fatalf("preview side effects: create=%d publish=%d reprobe=%d audit=%d",
			h.store.createCalls, h.store.publishCalls, h.reprobeCalls, len(h.audit.logs))
	}
	select {
	case <-h.topologyNotify:
		t.Fatal("preview emitted topology notification")
	default:
	}
	assertJSONOmitsDeviceImportSecrets(t, preview)
}

func TestDeviceImportServicePreviewResolvesSNMPProfileWithoutExposingOrUsingCredentials(t *testing.T) {
	h := newDeviceImportServiceHarness(t)
	uploaded := []byte("- targets: [\"router.example.net:161\"]\n  labels:\n    identity: " + deviceImportTestSecret + "\n")
	request := h.request(DeviceImportModeSNMP, uploaded)
	request.SNMPProfileID = &h.profileID

	preview, err := h.importer.Preview(context.Background(), request)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if len(preview.Targets) != 1 || preview.Targets[0].Status != DeviceImportTargetStatusReady {
		t.Fatalf("preview targets = %#v, want one ready target", preview.Targets)
	}
	if h.profiles.getCalls != 1 {
		t.Fatalf("profile lookups = %d, want 1", h.profiles.getCalls)
	}
	if h.store.createCalls != 0 || h.store.publishCalls != 0 || h.reprobeCalls != 0 {
		t.Fatalf("preview side effects: create=%d publish=%d reprobe=%d", h.store.createCalls, h.store.publishCalls, h.reprobeCalls)
	}
	assertJSONOmitsDeviceImportSecrets(t, preview)
}

func TestDeviceImportServicePreviewConfigurationEncodesOptionalIDsAsNull(t *testing.T) {
	h := newDeviceImportServiceHarness(t)
	request := h.request(DeviceImportModePrometheus, []byte("- targets: [\"router.example.net\"]\n"))
	request.AreaID = nil

	preview, err := h.importer.Preview(context.Background(), request)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	encoded, err := json.Marshal(preview.Configuration)
	if err != nil {
		t.Fatalf("json.Marshal(configuration): %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("json.Unmarshal(configuration): %v", err)
	}
	for _, field := range []string{"snmp_profile_id", "area_id"} {
		value, present := fields[field]
		if !present || string(value) != "null" {
			t.Fatalf("configuration %s = %s (present=%t), want explicit null: %s", field, value, present, encoded)
		}
	}
}

func TestDeviceImportServiceCommitBuildsApprovedDeviceFieldsForEveryMode(t *testing.T) {
	tests := []struct {
		name             string
		mode             DeviceImportMode
		target           string
		wantAddress      string
		wantSource       domain.MetricsSource
		wantSelector     string
		wantCredentials  bool
		wantReprobeCalls int
	}{
		{
			name:         "prometheus",
			mode:         DeviceImportModePrometheus,
			target:       " Router.Example.NET:9116 ",
			wantAddress:  "router.example.net",
			wantSource:   domain.MetricsSourcePrometheus,
			wantSelector: "Router.Example.NET:9116",
		},
		{
			name:             "prometheus snmp fallback",
			mode:             DeviceImportModePrometheusFallback,
			target:           " Router.Example.NET:161 ",
			wantAddress:      "router.example.net",
			wantSource:       domain.MetricsSourcePrometheusSNMPFallback,
			wantSelector:     "Router.Example.NET:161",
			wantCredentials:  true,
			wantReprobeCalls: 1,
		},
		{
			name:             "pure snmp ignores identity and selector",
			mode:             DeviceImportModeSNMP,
			target:           " Router.Example.NET:161 ",
			wantAddress:      "router.example.net",
			wantSource:       domain.MetricsSourceSNMP,
			wantCredentials:  true,
			wantReprobeCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newDeviceImportServiceHarness(t)
			uploaded := []byte("- targets: [\"" + tt.target + "\"]\n  labels:\n    identity: YAML_IDENTITY_MUST_BE_IGNORED\n    vendor: mikrotik\n")
			request := h.request(tt.mode, uploaded)
			request.ExpectedFileDigest = deviceImportDigest(uploaded)
			if tt.wantCredentials {
				request.SNMPProfileID = &h.profileID
			}

			result, err := h.importer.Commit(context.Background(), request)
			if err != nil {
				t.Fatalf("Commit() error = %v", err)
			}
			if len(h.store.created) != 1 {
				t.Fatalf("created device count = %d, want 1", len(h.store.created))
			}
			device := h.store.created[0]
			if device.IP != tt.wantAddress || device.Hostname != "" || device.DeviceType != domain.DeviceTypeUnknown ||
				device.Vendor != "default" || len(device.Tags) != 0 || len(device.AreaIDs) != 0 {
				t.Fatalf("created base fields = %#v", device)
			}
			if device.MetricsSource != tt.wantSource {
				t.Fatalf("MetricsSource = %q, want %q", device.MetricsSource, tt.wantSource)
			}
			if tt.mode == DeviceImportModeSNMP {
				if device.PrometheusLabelName != "" || device.PrometheusLabelValue != "" {
					t.Fatalf("pure SNMP selector = %q=%q, want empty", device.PrometheusLabelName, device.PrometheusLabelValue)
				}
			} else if device.PrometheusLabelName != "instance" || device.PrometheusLabelValue != tt.wantSelector {
				t.Fatalf("selector = %q=%q, want instance=%q", device.PrometheusLabelName, device.PrometheusLabelValue, tt.wantSelector)
			}
			if len(device.Addresses) != 1 || !device.Addresses[0].IsPrimary ||
				device.Addresses[0].Address != tt.wantAddress || device.Addresses[0].Role != domain.DeviceAddressRolePrimary {
				t.Fatalf("primary addresses = %#v", device.Addresses)
			}
			if tt.wantCredentials {
				if device.SNMPCredentials.V2c == nil || device.SNMPCredentials.V2c.Community != deviceImportTestSecret {
					t.Fatalf("copied credentials = %#v", device.SNMPCredentials)
				}
				if device.SNMPCredentials.V2c == h.profiles.profile.Credentials.V2c {
					t.Fatal("device credentials alias profile credentials")
				}
			} else if device.SNMPCredentials != (domain.SNMPCredentials{}) {
				t.Fatalf("Prometheus credentials = %#v, want empty", device.SNMPCredentials)
			}
			if h.reprobeCalls != tt.wantReprobeCalls {
				t.Fatalf("reprobe calls = %d, want %d", h.reprobeCalls, tt.wantReprobeCalls)
			}
			if result.Summary.Created != 1 || len(result.Results) != 1 || result.Results[0].Status != DeviceImportTargetStatusCreated || result.Results[0].DeviceID == nil {
				t.Fatalf("commit result = %#v", result)
			}
			if h.store.publishCalls != 1 || len(h.store.published) != 1 || len(h.store.published[0]) != 1 {
				t.Fatalf("publication = calls %d IDs %#v", h.store.publishCalls, h.store.published)
			}
			if len(h.store.placements) != 1 || h.store.placements[0].MapID != h.mapID ||
				h.store.placements[0].AreaID == nil || *h.store.placements[0].AreaID != h.areaID {
				t.Fatalf("map-local placement = %#v", h.store.placements)
			}
			select {
			case <-h.topologyNotify:
			default:
				t.Fatal("commit did not emit topology notification")
			}
			select {
			case <-h.topologyNotify:
				t.Fatal("commit emitted more than one topology notification")
			default:
			}
			assertJSONOmitsDeviceImportSecrets(t, result)
		})
	}
}

func TestDeviceImportServiceCommitDeepCopiesSNMPv3Credentials(t *testing.T) {
	h := newDeviceImportServiceHarness(t)
	profileCredentials := &domain.SNMPv3Credentials{
		Username:      "import-user",
		AuthProtocol:  "SHA",
		AuthPassword:  deviceImportTestSecret,
		PrivProtocol:  "AES",
		PrivPassword:  "private-" + deviceImportTestSecret,
		SecurityLevel: "authPriv",
	}
	wantCredentials := *profileCredentials
	h.profiles.profile.Credentials = domain.SNMPCredentials{
		Version: domain.SNMPVersionV3,
		V3:      profileCredentials,
	}
	var receivedCredentials *domain.SNMPv3Credentials
	h.store.createFunc = func(device *domain.Device, _ domain.DeviceImportPlacement) error {
		receivedCredentials = device.SNMPCredentials.V3
		return nil
	}
	uploaded := []byte("- targets: [\"router.example.net\"]\n")
	request := h.request(DeviceImportModeSNMP, uploaded)
	request.SNMPProfileID = &h.profileID
	request.ExpectedFileDigest = deviceImportDigest(uploaded)

	if _, err := h.importer.Commit(context.Background(), request); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if receivedCredentials == nil || *receivedCredentials != wantCredentials {
		t.Fatalf("created SNMPv3 credentials = %#v, want %#v", receivedCredentials, wantCredentials)
	}
	if receivedCredentials == profileCredentials {
		t.Fatal("created SNMPv3 credentials alias profile credentials")
	}
	profileCredentials.AuthPassword = "mutated"
	profileCredentials.PrivPassword = "mutated"
	if *receivedCredentials != wantCredentials {
		t.Fatalf("created SNMPv3 credentials changed with profile: %#v", receivedCredentials)
	}
}

func TestDeviceImportServiceRejectsInvalidConfigurationBeforeAnyWrite(t *testing.T) {
	validFile := []byte("- targets: [\"router.example.net\"]\n")
	tests := []struct {
		name   string
		mutate func(*deviceImportServiceHarness, *DeviceImportRequest)
	}{
		{
			name: "unsupported mode",
			mutate: func(_ *deviceImportServiceHarness, request *DeviceImportRequest) {
				request.MetricsMode = DeviceImportMode("unsupported")
			},
		},
		{
			name: "Prometheus rejects profile",
			mutate: func(h *deviceImportServiceHarness, request *DeviceImportRequest) {
				request.SNMPProfileID = &h.profileID
			},
		},
		{
			name: "fallback requires profile",
			mutate: func(_ *deviceImportServiceHarness, request *DeviceImportRequest) {
				request.MetricsMode = DeviceImportModePrometheusFallback
			},
		},
		{
			name: "pure SNMP requires profile",
			mutate: func(_ *deviceImportServiceHarness, request *DeviceImportRequest) {
				request.MetricsMode = DeviceImportModeSNMP
			},
		},
		{
			name: "map is required",
			mutate: func(_ *deviceImportServiceHarness, request *DeviceImportRequest) {
				request.MapID = uuid.Nil
			},
		},
		{
			name: "zero area is invalid",
			mutate: func(_ *deviceImportServiceHarness, request *DeviceImportRequest) {
				zero := uuid.Nil
				request.AreaID = &zero
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newDeviceImportServiceHarness(t)
			request := h.request(DeviceImportModePrometheus, validFile)
			tt.mutate(h, &request)
			_, err := h.importer.Preview(context.Background(), request)
			if !errors.Is(err, ErrDeviceImportInvalidConfiguration) {
				t.Fatalf("Preview() error = %v, want invalid configuration", err)
			}
			if h.store.createCalls != 0 || h.store.publishCalls != 0 || len(h.audit.logs) != 0 {
				t.Fatalf("invalid configuration side effects: create=%d publish=%d audit=%d",
					h.store.createCalls, h.store.publishCalls, len(h.audit.logs))
			}
		})
	}

	t.Run("selected area must belong to selected map", func(t *testing.T) {
		h := newDeviceImportServiceHarness(t)
		h.maps.membership.Areas = nil
		_, err := h.importer.Preview(context.Background(), h.request(DeviceImportModePrometheus, validFile))
		if !errors.Is(err, ErrDeviceImportInvalidConfiguration) {
			t.Fatalf("Preview() error = %v, want invalid configuration", err)
		}
		if h.store.createCalls != 0 {
			t.Fatalf("store writes = %d, want 0", h.store.createCalls)
		}
	})
}

func TestDeviceImportServiceCommitDetectsChangedConfigurationBeforeFirstWrite(t *testing.T) {
	validFile := []byte("- targets: [\"router.example.net\"]\n")
	tests := []struct {
		name   string
		mode   DeviceImportMode
		mutate func(*deviceImportServiceHarness)
	}{
		{name: "map disappeared", mode: DeviceImportModePrometheus, mutate: func(h *deviceImportServiceHarness) {
			h.maps.getErr = errors.New("canvas map not found")
		}},
		{name: "map membership disappeared", mode: DeviceImportModePrometheus, mutate: func(h *deviceImportServiceHarness) {
			h.maps.membershipErr = errors.New("canvas map not found: " + h.mapID.String())
		}},
		{name: "selected area disappeared", mode: DeviceImportModePrometheus, mutate: func(h *deviceImportServiceHarness) {
			h.maps.membership.Areas = nil
		}},
		{name: "selected profile disappeared", mode: DeviceImportModeSNMP, mutate: func(h *deviceImportServiceHarness) {
			h.profiles.getErr = errors.New("snmp profile not found")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newDeviceImportServiceHarness(t)
			tt.mutate(h)
			request := h.request(tt.mode, validFile)
			if tt.mode != DeviceImportModePrometheus {
				request.SNMPProfileID = &h.profileID
			}
			request.ExpectedFileDigest = deviceImportDigest(validFile)
			_, err := h.importer.Commit(context.Background(), request)
			if !errors.Is(err, ErrDeviceImportConfigurationChanged) {
				t.Fatalf("Commit() error = %v, want configuration changed", err)
			}
			if err != nil && strings.Contains(err.Error(), deviceImportTestSecret) {
				t.Fatalf("safe configuration error leaked secret: %v", err)
			}
			if h.store.createCalls != 0 || h.store.publishCalls != 0 || len(h.audit.logs) != 0 {
				t.Fatalf("changed configuration side effects: create=%d publish=%d audit=%d",
					h.store.createCalls, h.store.publishCalls, len(h.audit.logs))
			}
		})
	}
}

func TestDeviceImportServiceCommitRejectsDigestBeforeParsingOrRepositories(t *testing.T) {
	for _, expected := range []string{"", "sha256:deadbeef"} {
		t.Run(fmt.Sprintf("digest_%q", expected), func(t *testing.T) {
			h := newDeviceImportServiceHarness(t)
			h.maps.getErr = errors.New("map repository must not be called")
			malformed := []byte("- targets: [unterminated\n")
			request := h.request(DeviceImportModePrometheus, malformed)
			request.ExpectedFileDigest = expected

			result, err := h.importer.Commit(context.Background(), request)
			if !errors.Is(err, ErrDeviceImportDigestMismatch) {
				t.Fatalf("Commit() error = %v, want digest mismatch", err)
			}
			if result.FileDigest != deviceImportDigest(malformed) {
				t.Fatalf("result digest = %q, want exact uploaded digest", result.FileDigest)
			}
			if h.maps.getCalls != 0 || h.maps.membershipCalls != 0 || h.store.createCalls != 0 || h.store.publishCalls != 0 {
				t.Fatalf("digest mismatch dependency calls: map=%d membership=%d create=%d publish=%d",
					h.maps.getCalls, h.maps.membershipCalls, h.store.createCalls, h.store.publishCalls)
			}
		})
	}
}

func TestDeviceImportServiceCommitPreservesOrderAndContinuesTargetLocalFailures(t *testing.T) {
	h := newDeviceImportServiceHarness(t)
	logs := captureDeviceImportLogs(t)
	uploaded := []byte(`
- targets:
    - "created-one.example.net"
    - "CREATED-ONE.EXAMPLE.NET"
    - "existing.example.net"
    - "failed.example.net"
    - 123
    - "created-two.example.net"
  labels:
    identity: "YAML_IDENTITY_MUST_BE_IGNORED"
    secret: "` + deviceImportTestSecret + `"
`)
	h.store.createFunc = func(device *domain.Device, _ domain.DeviceImportPlacement) error {
		switch device.IP {
		case "existing.example.net":
			return domain.ErrDeviceImportAddressConflict
		case "failed.example.net":
			return errors.New("target database failure " + deviceImportTestSecret)
		default:
			return nil
		}
	}
	request := h.request(DeviceImportModePrometheus, uploaded)
	request.ExpectedFileDigest = deviceImportDigest(uploaded)

	result, err := h.importer.Commit(context.Background(), request)
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	wantStatuses := []DeviceImportTargetStatus{
		DeviceImportTargetStatusCreated,
		DeviceImportTargetStatusSkippedDuplicateInFile,
		DeviceImportTargetStatusSkippedExisting,
		DeviceImportTargetStatusFailed,
		DeviceImportTargetStatusInvalid,
		DeviceImportTargetStatusCreated,
	}
	if len(result.Results) != len(wantStatuses) {
		t.Fatalf("result count = %d, want %d", len(result.Results), len(wantStatuses))
	}
	for index, want := range wantStatuses {
		if result.Results[index].Status != want {
			t.Fatalf("result[%d] status = %q, want %q", index, result.Results[index].Status, want)
		}
	}
	if got := result.Summary; got.Total != 6 || got.Created != 2 || got.Skipped != 2 || got.Failed != 2 || got.NotProcessed != 0 {
		t.Fatalf("summary = %#v", got)
	}
	if result.Incomplete {
		t.Fatal("target-local failure marked commit incomplete")
	}
	if h.store.createCalls != 4 || len(h.store.created) != 2 {
		t.Fatalf("store calls=%d successful=%d, want 4/2", h.store.createCalls, len(h.store.created))
	}
	if h.store.publishCalls != 1 || len(h.store.published) != 1 || len(h.store.published[0]) != 2 {
		t.Fatalf("publication = calls %d IDs %#v", h.store.publishCalls, h.store.published)
	}
	if h.reprobeCalls != 0 {
		t.Fatalf("Prometheus reprobe calls = %d, want 0", h.reprobeCalls)
	}
	if len(h.audit.logs) != 1 {
		t.Fatalf("audit log count = %d, want 1", len(h.audit.logs))
	}
	assertDeviceImportAudit(t, h.audit.logs[0], request, result.Summary)
	if strings.Contains(logs.String(), deviceImportTestSecret) {
		t.Fatalf("logs leaked secret: %s", logs.String())
	}
	assertJSONOmitsDeviceImportSecrets(t, result)
}

func TestDeviceImportServiceCommitRechecksAddressThatAppearedAfterPreview(t *testing.T) {
	h := newDeviceImportServiceHarness(t)
	uploaded := []byte("- targets: [\"late-existing.example.net\"]\n")
	request := h.request(DeviceImportModePrometheus, uploaded)
	preview, err := h.importer.Preview(context.Background(), request)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if len(preview.Targets) != 1 || preview.Targets[0].Status != DeviceImportTargetStatusReady {
		t.Fatalf("preview targets = %#v, want ready", preview.Targets)
	}

	h.store.createFunc = func(*domain.Device, domain.DeviceImportPlacement) error {
		return domain.ErrDeviceImportAddressConflict
	}
	request.ExpectedFileDigest = preview.FileDigest
	result, err := h.importer.Commit(context.Background(), request)
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if len(result.Results) != 1 || result.Results[0].Status != DeviceImportTargetStatusSkippedExisting ||
		result.Summary.Skipped != 1 || result.Summary.Created != 0 {
		t.Fatalf("commit result = %#v, want skipped existing", result)
	}
	if h.store.existingCalls != 1 {
		t.Fatalf("bulk lookup calls = %d, want preview-only call", h.store.existingCalls)
	}
	if len(h.store.created) != 0 || len(h.store.placements) != 0 {
		t.Fatalf("existing target was persisted or mapped: devices=%d placements=%d", len(h.store.created), len(h.store.placements))
	}
	select {
	case <-h.topologyNotify:
		t.Fatal("all-skipped commit emitted topology notification")
	default:
	}
}

func TestDeviceImportServiceCommitReturnsPartialResultWithSystemicError(t *testing.T) {
	tests := []struct {
		name      string
		storeErr  error
		wantError error
	}{
		{name: "store unavailable", storeErr: domain.ErrDeviceImportStoreUnavailable, wantError: domain.ErrDeviceImportStoreUnavailable},
		{name: "destination changed", storeErr: domain.ErrDeviceImportDestinationChanged, wantError: ErrDeviceImportConfigurationChanged},
		{name: "context cancelled by store", storeErr: context.Canceled, wantError: context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newDeviceImportServiceHarness(t)
			uploaded := []byte("- targets: [\"created.example.net\", \"abort.example.net\", 123, \"CREATED.EXAMPLE.NET\"]\n")
			h.store.createFunc = func(device *domain.Device, _ domain.DeviceImportPlacement) error {
				if device.IP == "abort.example.net" {
					return tt.storeErr
				}
				return nil
			}
			request := h.request(DeviceImportModeSNMP, uploaded)
			request.SNMPProfileID = &h.profileID
			request.ExpectedFileDigest = deviceImportDigest(uploaded)

			result, err := h.importer.Commit(context.Background(), request)
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("Commit() error = %v, want %v", err, tt.wantError)
			}
			wantStatuses := []DeviceImportTargetStatus{
				DeviceImportTargetStatusCreated,
				DeviceImportTargetStatusNotProcessed,
				DeviceImportTargetStatusNotProcessed,
				DeviceImportTargetStatusNotProcessed,
			}
			if len(result.Results) != len(wantStatuses) {
				t.Fatalf("result count = %d, want %d", len(result.Results), len(wantStatuses))
			}
			for index, want := range wantStatuses {
				if result.Results[index].Status != want {
					t.Fatalf("result[%d] status = %q, want %q", index, result.Results[index].Status, want)
				}
			}
			if !result.Incomplete {
				t.Fatal("systemic abort did not set Incomplete")
			}
			if got := result.Summary; got.Total != 4 || got.Created != 1 || got.Failed != 0 || got.NotProcessed != 3 || got.Skipped != 0 {
				t.Fatalf("summary = %#v", got)
			}
			if h.store.createCalls != 2 || len(h.store.created) != 1 {
				t.Fatalf("store calls=%d successful=%d, want 2/1", h.store.createCalls, len(h.store.created))
			}
			if h.store.publishCalls != 1 || len(h.store.published[0]) != 1 {
				t.Fatalf("published = %#v", h.store.published)
			}
			if h.reprobeCalls != 1 || len(h.reprobeIDs) != 1 || h.reprobeIDs[0] != h.store.created[0].ID {
				t.Fatalf("reprobes = %d %#v", h.reprobeCalls, h.reprobeIDs)
			}
			if len(h.audit.logs) != 1 {
				t.Fatalf("audit logs = %d, want 1", len(h.audit.logs))
			}
			assertDeviceImportAudit(t, h.audit.logs[0], request, result.Summary)
		})
	}
}

func TestDeviceImportServiceCommitDetachesPostCommitWorkFromCanceledRequest(t *testing.T) {
	h := newDeviceImportServiceHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	h.store.createFunc = func(_ *domain.Device, _ domain.DeviceImportPlacement) error {
		cancel()
		return nil
	}
	uploaded := []byte("- targets: [\"created.example.net\", \"not-processed.example.net\"]\n")
	request := h.request(DeviceImportModeSNMP, uploaded)
	request.SNMPProfileID = &h.profileID
	request.ExpectedFileDigest = deviceImportDigest(uploaded)

	result, err := h.importer.Commit(ctx, request)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Commit() error = %v, want context canceled", err)
	}
	wantStatuses := []DeviceImportTargetStatus{
		DeviceImportTargetStatusCreated,
		DeviceImportTargetStatusNotProcessed,
	}
	for index, want := range wantStatuses {
		if result.Results[index].Status != want {
			t.Fatalf("result[%d] status = %q, want %q", index, result.Results[index].Status, want)
		}
	}
	if len(h.reprobeContextErrs) != 1 || h.reprobeContextErrs[0] != nil {
		t.Fatalf("reprobe context errors = %#v, want [nil]", h.reprobeContextErrs)
	}
	if len(h.audit.contextErrs) != 1 || h.audit.contextErrs[0] != nil {
		t.Fatalf("audit context errors = %#v, want [nil]", h.audit.contextErrs)
	}
}

func TestDeviceImportServiceValidationPreservesKnownSystemicErrors(t *testing.T) {
	validFile := []byte("- targets: [\"router.example.net\"]\n")
	tests := []struct {
		name      string
		mode      DeviceImportMode
		wantError error
		mutate    func(*deviceImportServiceHarness)
	}{
		{
			name:      "map store unavailable",
			mode:      DeviceImportModePrometheus,
			wantError: domain.ErrDeviceImportStoreUnavailable,
			mutate: func(h *deviceImportServiceHarness) {
				h.maps.getErr = fmt.Errorf("%s: %w", deviceImportTestSecret, domain.ErrDeviceImportStoreUnavailable)
			},
		},
		{
			name:      "unexpected map dependency failure",
			mode:      DeviceImportModePrometheus,
			wantError: domain.ErrDeviceImportStoreUnavailable,
			mutate: func(h *deviceImportServiceHarness) {
				h.maps.getErr = errors.New("unexpected map failure " + deviceImportTestSecret)
			},
		},
		{
			name:      "membership context cancelled",
			mode:      DeviceImportModePrometheus,
			wantError: context.Canceled,
			mutate: func(h *deviceImportServiceHarness) {
				h.maps.membershipErr = fmt.Errorf("%s: %w", deviceImportTestSecret, context.Canceled)
			},
		},
		{
			name:      "profile deadline exceeded",
			mode:      DeviceImportModeSNMP,
			wantError: context.DeadlineExceeded,
			mutate: func(h *deviceImportServiceHarness) {
				h.profiles.getErr = fmt.Errorf("%s: %w", deviceImportTestSecret, context.DeadlineExceeded)
			},
		},
	}

	for _, tt := range tests {
		for _, operation := range []string{"preview", "commit"} {
			t.Run(tt.name+" "+operation, func(t *testing.T) {
				h := newDeviceImportServiceHarness(t)
				logs := captureDeviceImportLogs(t)
				tt.mutate(h)
				request := h.request(tt.mode, validFile)
				if tt.mode != DeviceImportModePrometheus {
					request.SNMPProfileID = &h.profileID
				}
				var err error
				if operation == "preview" {
					_, err = h.importer.Preview(context.Background(), request)
				} else {
					request.ExpectedFileDigest = deviceImportDigest(validFile)
					_, err = h.importer.Commit(context.Background(), request)
				}
				if !errors.Is(err, tt.wantError) {
					t.Fatalf("%s error = %v, want %v", operation, err, tt.wantError)
				}
				if err != nil && strings.Contains(err.Error(), deviceImportTestSecret) {
					t.Fatalf("%s leaked dependency error text: %v", operation, err)
				}
				if strings.Contains(logs.String(), deviceImportTestSecret) {
					t.Fatalf("%s leaked dependency error text in logs: %s", operation, logs.String())
				}
				if h.store.createCalls != 0 || h.store.publishCalls != 0 || len(h.audit.logs) != 0 {
					t.Fatalf("%s systemic validation side effects: create=%d publish=%d audit=%d",
						operation, h.store.createCalls, h.store.publishCalls, len(h.audit.logs))
				}
			})
		}
	}
}

func TestDeviceImportServiceRequiresStoreDuringGlobalValidation(t *testing.T) {
	malformed := []byte("- targets: [unterminated\n")
	for _, operation := range []string{"preview", "commit"} {
		t.Run(operation, func(t *testing.T) {
			h := newDeviceImportServiceHarness(t)
			h.importer.store = nil
			request := h.request(DeviceImportModePrometheus, malformed)
			var err error
			if operation == "preview" {
				_, err = h.importer.Preview(context.Background(), request)
			} else {
				request.ExpectedFileDigest = deviceImportDigest(malformed)
				_, err = h.importer.Commit(context.Background(), request)
			}
			if !errors.Is(err, domain.ErrDeviceImportStoreUnavailable) {
				t.Fatalf("%s error = %v, want store unavailable", operation, err)
			}
			if h.maps.getCalls != 0 || h.store.createCalls != 0 || h.store.publishCalls != 0 || len(h.audit.logs) != 0 {
				t.Fatalf("%s nil-store dependency calls: map=%d create=%d publish=%d audit=%d",
					operation, h.maps.getCalls, h.store.createCalls, h.store.publishCalls, len(h.audit.logs))
			}
		})
	}
}

func TestDeviceImportServiceSanitizesGlobalParserAndLookupErrors(t *testing.T) {
	t.Run("preview bulk lookup", func(t *testing.T) {
		h := newDeviceImportServiceHarness(t)
		h.store.existingErr = errors.New("lookup failed " + deviceImportTestSecret)
		_, err := h.importer.Preview(
			context.Background(),
			h.request(DeviceImportModePrometheus, []byte("- targets: [\"router.example.net\"]\n")),
		)
		if !errors.Is(err, domain.ErrDeviceImportStoreUnavailable) {
			t.Fatalf("Preview() error = %v, want store unavailable", err)
		}
		if strings.Contains(err.Error(), deviceImportTestSecret) {
			t.Fatalf("Preview() leaked lookup error: %v", err)
		}
	})

	malformedWithMarker := []byte("- targets: [\"router.example.net\"]\n  labels: *" + deviceImportTestSecret + "\n")
	for _, operation := range []string{"preview", "commit"} {
		t.Run(operation+" parser", func(t *testing.T) {
			h := newDeviceImportServiceHarness(t)
			request := h.request(DeviceImportModePrometheus, malformedWithMarker)
			var err error
			if operation == "preview" {
				_, err = h.importer.Preview(context.Background(), request)
			} else {
				request.ExpectedFileDigest = deviceImportDigest(malformedWithMarker)
				_, err = h.importer.Commit(context.Background(), request)
			}
			if err == nil {
				t.Fatalf("%s parser error = nil", operation)
			}
			if !errors.Is(err, ErrDeviceImportInvalidFile) {
				t.Fatalf("%s parser error = %v, want invalid-file sentinel", operation, err)
			}
			if strings.Contains(err.Error(), deviceImportTestSecret) {
				t.Fatalf("%s leaked YAML marker: %v", operation, err)
			}
			if h.store.createCalls != 0 || h.store.publishCalls != 0 || len(h.audit.logs) != 0 {
				t.Fatalf("%s parser side effects: create=%d publish=%d audit=%d",
					operation, h.store.createCalls, h.store.publishCalls, len(h.audit.logs))
			}
		})
	}

	t.Run("limit sentinel remains typed", func(t *testing.T) {
		h := newDeviceImportServiceHarness(t)
		oversized := make([]byte, DeviceImportMaxFileBytes+1)
		_, err := h.importer.Preview(context.Background(), h.request(DeviceImportModePrometheus, oversized))
		if !errors.Is(err, ErrDeviceImportLimitExceeded) {
			t.Fatalf("Preview() error = %v, want limit sentinel", err)
		}
	})
}

func TestDeviceImportServicePostCommitFailuresDoNotChangeCreatedResultsOrLeakSecrets(t *testing.T) {
	h := newDeviceImportServiceHarness(t)
	logs := captureDeviceImportLogs(t)
	h.reprobeErr = errors.New("probe failed " + deviceImportTestSecret)
	h.audit.appendErr = errors.New("audit failed " + deviceImportTestSecret)
	uploaded := []byte("- targets: [\"created.example.net\"]\n")
	request := h.request(DeviceImportModePrometheusFallback, uploaded)
	request.SNMPProfileID = &h.profileID
	request.ExpectedFileDigest = deviceImportDigest(uploaded)

	result, err := h.importer.Commit(context.Background(), request)
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if result.Summary.Created != 1 || len(result.Results) != 1 || result.Results[0].Status != DeviceImportTargetStatusCreated {
		t.Fatalf("result changed after post-commit failure: %#v", result)
	}
	if h.reprobeCalls != 1 || len(h.audit.logs) != 1 {
		t.Fatalf("post-commit calls reprobe=%d audit=%d", h.reprobeCalls, len(h.audit.logs))
	}
	if strings.Contains(logs.String(), deviceImportTestSecret) {
		t.Fatalf("post-commit logs leaked secret: %s", logs.String())
	}
	assertJSONOmitsDeviceImportSecrets(t, result)
}

func assertDeviceImportAudit(
	t *testing.T,
	entry domain.AuditLog,
	request DeviceImportRequest,
	summary DeviceImportCommitSummary,
) {
	t.Helper()
	if entry.Action != "admin.device_import_completed" || entry.Resource != "device_import" || entry.ResourceID != request.MapID.String() {
		t.Fatalf("audit identity = %#v", entry)
	}
	if entry.ActorUserID == nil || *entry.ActorUserID != request.Actor.UserID || entry.IPAddress != request.Actor.IPAddress || entry.UserAgent != request.Actor.UserAgent {
		t.Fatalf("audit actor = %#v", entry)
	}
	if entry.CreatedAt.IsZero() {
		t.Fatal("audit CreatedAt is zero")
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal([]byte(entry.MetadataJSON), &metadata); err != nil {
		t.Fatalf("audit metadata JSON: %v", err)
	}
	wantKeys := map[string]bool{
		"actor_user_id": true,
		"file_digest":   true,
		"metrics_mode":  true,
		"map_id":        true,
		"area_id":       true,
		"summary":       true,
	}
	if request.SNMPProfileID != nil {
		wantKeys["snmp_profile_id"] = true
	}
	if len(metadata) != len(wantKeys) {
		t.Fatalf("audit metadata keys = %#v, want %#v", metadata, wantKeys)
	}
	for key := range metadata {
		if !wantKeys[key] {
			t.Fatalf("unexpected audit metadata key %q", key)
		}
	}
	var gotSummary DeviceImportCommitSummary
	if err := json.Unmarshal(metadata["summary"], &gotSummary); err != nil {
		t.Fatalf("audit summary: %v", err)
	}
	if gotSummary != summary {
		t.Fatalf("audit summary = %#v, want %#v", gotSummary, summary)
	}
	if strings.Contains(entry.MetadataJSON, deviceImportTestSecret) || strings.Contains(entry.MetadataJSON, "targets") || strings.Contains(entry.MetadataJSON, "labels") {
		t.Fatalf("audit metadata contains forbidden data: %s", entry.MetadataJSON)
	}
}

type deviceImportServiceHarness struct {
	importer           *DeviceImportService
	store              *fakeDeviceImportStore
	maps               *fakeDeviceImportMapRepository
	profiles           *fakeDeviceImportProfileRepository
	audit              *fakeDeviceImportAuditRepository
	mapID              uuid.UUID
	areaID             uuid.UUID
	profileID          uuid.UUID
	actorID            uuid.UUID
	topologyNotify     chan struct{}
	reprobeCalls       int
	reprobeIDs         []uuid.UUID
	reprobeContextErrs []error
	reprobeErr         error
}

func newDeviceImportServiceHarness(t *testing.T) *deviceImportServiceHarness {
	t.Helper()
	h := &deviceImportServiceHarness{
		store:          &fakeDeviceImportStore{},
		maps:           &fakeDeviceImportMapRepository{},
		profiles:       &fakeDeviceImportProfileRepository{},
		audit:          &fakeDeviceImportAuditRepository{},
		mapID:          uuid.New(),
		areaID:         uuid.New(),
		profileID:      uuid.New(),
		actorID:        uuid.New(),
		topologyNotify: make(chan struct{}, 4),
	}
	h.maps.canvasMap = domain.CanvasMap{ID: h.mapID, Name: "Import destination"}
	h.maps.membership = domain.CanvasMapMembership{Areas: []domain.CanvasMapAreaMembership{{AreaID: h.areaID, Name: "Rack A"}}}
	h.profiles.profile = &domain.SNMPProfile{
		ID:   h.profileID,
		Name: "Secret profile",
		Credentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: deviceImportTestSecret},
		},
	}
	devices := NewDeviceService(
		newMockDeviceRepo(),
		newMockLinkRepo(),
		newMockSettingsRepo(),
		func(string, domain.SNMPCredentials, domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
			t.Fatal("device import draft construction must not perform discovery")
			return nil, errors.New("unexpected discovery")
		},
		h.topologyNotify,
	)
	h.importer = NewDeviceImportService(h.store, devices, h.maps, h.profiles, h.audit)
	h.importer.reprobeDevice = func(ctx context.Context, id uuid.UUID) error {
		h.reprobeCalls++
		h.reprobeIDs = append(h.reprobeIDs, id)
		h.reprobeContextErrs = append(h.reprobeContextErrs, ctx.Err())
		return h.reprobeErr
	}
	return h
}

func (h *deviceImportServiceHarness) request(mode DeviceImportMode, fileBytes []byte) DeviceImportRequest {
	return DeviceImportRequest{
		FileBytes:   fileBytes,
		MetricsMode: mode,
		MapID:       h.mapID,
		AreaID:      &h.areaID,
		Actor: DeviceImportActor{
			UserID:    h.actorID,
			IPAddress: "192.0.2.44",
			UserAgent: "device-import-test",
		},
	}
}

func deviceImportDigest(fileBytes []byte) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256(fileBytes))
}

func assertJSONOmitsDeviceImportSecrets(t *testing.T, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal(%T): %v", value, err)
	}
	for _, forbidden := range []string{deviceImportTestSecret, "YAML_IDENTITY_MUST_BE_IGNORED", "mikrotik", "labels"} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("encoded %T contains forbidden marker %q: %s", value, forbidden, encoded)
		}
	}
}

type fakeDeviceImportStore struct {
	existing       map[string]struct{}
	existingErr    error
	existingCalls  int
	existingInputs [][]string
	createCalls    int
	createFunc     func(*domain.Device, domain.DeviceImportPlacement) error
	created        []*domain.Device
	placements     []domain.DeviceImportPlacement
	publishCalls   int
	published      [][]uuid.UUID
}

func (s *fakeDeviceImportStore) ExistingCanonicalAddresses(_ context.Context, addresses []string) (map[string]struct{}, error) {
	s.existingCalls++
	s.existingInputs = append(s.existingInputs, append([]string(nil), addresses...))
	if s.existingErr != nil {
		return nil, s.existingErr
	}
	result := make(map[string]struct{}, len(s.existing))
	for address := range s.existing {
		result[address] = struct{}{}
	}
	return result, nil
}

func (s *fakeDeviceImportStore) CreateDeviceInMap(_ context.Context, device *domain.Device, placement domain.DeviceImportPlacement) error {
	s.createCalls++
	if s.createFunc != nil {
		if err := s.createFunc(device, placement); err != nil {
			return err
		}
	}
	copy := cloneDeviceImportTestDevice(device)
	s.created = append(s.created, copy)
	s.placements = append(s.placements, placement)
	return nil
}

func (s *fakeDeviceImportStore) PublishCreatedDevices(ids []uuid.UUID) {
	s.publishCalls++
	s.published = append(s.published, append([]uuid.UUID(nil), ids...))
}

func cloneDeviceImportTestDevice(device *domain.Device) *domain.Device {
	copy := *device
	copy.Tags = make(map[string]string, len(device.Tags))
	for key, value := range device.Tags {
		copy.Tags[key] = value
	}
	copy.AreaIDs = append([]uuid.UUID(nil), device.AreaIDs...)
	copy.Addresses = append([]domain.DeviceAddress(nil), device.Addresses...)
	if device.SNMPCredentials.V2c != nil {
		v2c := *device.SNMPCredentials.V2c
		copy.SNMPCredentials.V2c = &v2c
	}
	if device.SNMPCredentials.V3 != nil {
		v3 := *device.SNMPCredentials.V3
		copy.SNMPCredentials.V3 = &v3
	}
	return &copy
}

type fakeDeviceImportMapRepository struct {
	canvasMap       domain.CanvasMap
	membership      domain.CanvasMapMembership
	getErr          error
	membershipErr   error
	getCalls        int
	membershipCalls int
}

func (r *fakeDeviceImportMapRepository) Create(domain.CanvasMapCreate) (domain.CanvasMap, error) {
	return domain.CanvasMap{}, errors.New("not implemented")
}
func (r *fakeDeviceImportMapRepository) GetByID(uuid.UUID) (domain.CanvasMap, error) {
	r.getCalls++
	return r.canvasMap, r.getErr
}
func (r *fakeDeviceImportMapRepository) GetDefault() (domain.CanvasMap, error) {
	return domain.CanvasMap{}, errors.New("not implemented")
}
func (r *fakeDeviceImportMapRepository) List() ([]domain.CanvasMap, error) {
	return nil, errors.New("not implemented")
}
func (r *fakeDeviceImportMapRepository) Update(uuid.UUID, domain.CanvasMapUpdate) (domain.CanvasMap, error) {
	return domain.CanvasMap{}, errors.New("not implemented")
}
func (r *fakeDeviceImportMapRepository) SetPrimary(uuid.UUID) (domain.CanvasMap, error) {
	return domain.CanvasMap{}, errors.New("not implemented")
}
func (r *fakeDeviceImportMapRepository) Delete(uuid.UUID) error { return errors.New("not implemented") }
func (r *fakeDeviceImportMapRepository) Duplicate(uuid.UUID, string) (domain.CanvasMap, error) {
	return domain.CanvasMap{}, errors.New("not implemented")
}
func (r *fakeDeviceImportMapRepository) GetMembership(uuid.UUID) (domain.CanvasMapMembership, error) {
	r.membershipCalls++
	return r.membership, r.membershipErr
}
func (r *fakeDeviceImportMapRepository) ReplaceMembership(uuid.UUID, domain.CanvasMapMembership) error {
	return errors.New("not implemented")
}
func (r *fakeDeviceImportMapRepository) UpdateDeviceVisualColor(uuid.UUID, uuid.UUID, *string) error {
	return errors.New("not implemented")
}
func (r *fakeDeviceImportMapRepository) RemoveDevice(uuid.UUID, uuid.UUID) error {
	return errors.New("not implemented")
}
func (r *fakeDeviceImportMapRepository) RemoveLink(uuid.UUID, uuid.UUID) error {
	return errors.New("not implemented")
}

type fakeDeviceImportProfileRepository struct {
	profile  *domain.SNMPProfile
	getErr   error
	getCalls int
}

func (r *fakeDeviceImportProfileRepository) Create(*domain.SNMPProfile) error {
	return errors.New("not implemented")
}
func (r *fakeDeviceImportProfileRepository) GetByID(uuid.UUID) (*domain.SNMPProfile, error) {
	r.getCalls++
	return r.profile, r.getErr
}
func (r *fakeDeviceImportProfileRepository) GetAll() ([]domain.SNMPProfile, error) {
	return nil, errors.New("not implemented")
}
func (r *fakeDeviceImportProfileRepository) Update(*domain.SNMPProfile) error {
	return errors.New("not implemented")
}
func (r *fakeDeviceImportProfileRepository) Delete(uuid.UUID) error {
	return errors.New("not implemented")
}

type fakeDeviceImportAuditRepository struct {
	logs        []domain.AuditLog
	contextErrs []error
	appendErr   error
}

func (r *fakeDeviceImportAuditRepository) AppendAuditLog(ctx context.Context, entry *domain.AuditLog) error {
	r.contextErrs = append(r.contextErrs, ctx.Err())
	if entry != nil {
		r.logs = append(r.logs, *entry)
	}
	return r.appendErr
}
func (r *fakeDeviceImportAuditRepository) ListAuditLogs(context.Context, domain.AuditLogFilter) ([]domain.AuditLog, error) {
	return nil, errors.New("not implemented")
}
func (r *fakeDeviceImportAuditRepository) DashboardStats(context.Context) (*domain.AdminDashboardStats, error) {
	return nil, errors.New("not implemented")
}

func captureDeviceImportLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })
	return &output
}

var _ domain.DeviceImportStore = (*fakeDeviceImportStore)(nil)
var _ domain.CanvasMapRepository = (*fakeDeviceImportMapRepository)(nil)
var _ domain.SNMPProfileRepository = (*fakeDeviceImportProfileRepository)(nil)
var _ domain.AuditLogRepository = (*fakeDeviceImportAuditRepository)(nil)
