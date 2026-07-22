package service

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseDeviceImportTargetsPreservesGroupAndTargetOrder(t *testing.T) {
	input := []byte(`
- targets:
    - 192.0.2.10
    - edge-a.example:9100
- targets: ["[2001:db8::10]:161", edge-b.example]
`)

	got, err := ParsePrometheusFileSD(input, DeviceImportModePrometheus)
	if err != nil {
		t.Fatalf("ParsePrometheusFileSD() error = %v", err)
	}
	want := []struct {
		group int
		item  int
		raw   string
	}{
		{group: 0, item: 0, raw: "192.0.2.10"},
		{group: 0, item: 1, raw: "edge-a.example:9100"},
		{group: 1, item: 0, raw: "[2001:db8::10]:161"},
		{group: 1, item: 1, raw: "edge-b.example"},
	}
	if len(got.Targets) != len(want) {
		t.Fatalf("target count = %d, want %d", len(got.Targets), len(want))
	}
	for i, expected := range want {
		target := got.Targets[i]
		if target.GroupIndex != expected.group || target.ItemIndex != expected.item || target.RawTarget != expected.raw {
			t.Errorf("target[%d] location/raw = (%d, %d, %q), want (%d, %d, %q)",
				i, target.GroupIndex, target.ItemIndex, target.RawTarget,
				expected.group, expected.item, expected.raw)
		}
	}
	if len(got.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", got.Diagnostics)
	}
}

func TestParseDeviceImportTargetsIgnoresAllLabels(t *testing.T) {
	input := []byte(`
- targets: ["10.0.9.246"]
  labels:
    identity: CORE_MUX
    vendor: mikrotik
    mapping: {nested: value}
    sequence: [one, two]
    scalar: ignored
    nothing: null
    anchor: &ignored {secret: value}
    alias: *ignored
    tagged: !private hidden
  identity: not-an-import-field
  vendor: not-an-import-field
  unknown:
    deeply:
      nested: ignored
`)

	got, err := ParsePrometheusFileSD(input, DeviceImportModePrometheus)
	if err != nil {
		t.Fatalf("ParsePrometheusFileSD() error = %v", err)
	}
	if len(got.Targets) != 1 {
		t.Fatalf("target count = %d, want 1", len(got.Targets))
	}
	if got.Targets[0].RawTarget != "10.0.9.246" {
		t.Errorf("RawTarget = %q, want %q", got.Targets[0].RawTarget, "10.0.9.246")
	}
	if got.Targets[0].CanonicalHost != "10.0.9.246" {
		t.Errorf("CanonicalHost = %q, want %q", got.Targets[0].CanonicalHost, "10.0.9.246")
	}

	for _, model := range []reflect.Type{
		reflect.TypeOf(ParsedDeviceImportFile{}),
		reflect.TypeOf(ParsedDeviceImportTarget{}),
		reflect.TypeOf(DeviceImportGroupDiagnostic{}),
	} {
		for _, forbidden := range []string{"Labels", "Identity", "Vendor"} {
			if _, exists := model.FieldByName(forbidden); exists {
				t.Errorf("%s unexpectedly exposes forbidden field %s", model.Name(), forbidden)
			}
		}
	}
}

func TestCanonicalImportTargetAcceptsAndCanonicalizesEndpoints(t *testing.T) {
	tests := []struct {
		name          string
		target        string
		wantRaw       string
		wantHost      string
		wantPort      uint16
		wantPortIsSet bool
	}{
		{name: "IPv4", target: "192.0.2.8", wantRaw: "192.0.2.8", wantHost: "192.0.2.8"},
		{name: "unbracketed IPv6", target: "2001:0DB8:0:0:0:0:0:8", wantRaw: "2001:0DB8:0:0:0:0:0:8", wantHost: "2001:db8::8"},
		{name: "bracketed IPv6 with port", target: "[2001:0DB8:0:0::9]:9100", wantRaw: "[2001:0DB8:0:0::9]:9100", wantHost: "2001:db8::9", wantPort: 9100, wantPortIsSet: true},
		{name: "DNS name", target: "Router-01.Example.COM", wantRaw: "Router-01.Example.COM", wantHost: "router-01.example.com"},
		{name: "IPv4 with port", target: "192.0.2.9:9200", wantRaw: "192.0.2.9:9200", wantHost: "192.0.2.9", wantPort: 9200, wantPortIsSet: true},
		{name: "surrounding whitespace", target: "  Router.Example.COM:9300\t", wantRaw: "Router.Example.COM:9300", wantHost: "router.example.com", wantPort: 9300, wantPortIsSet: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := []byte("- targets:\n  - \"" + strings.ReplaceAll(tt.target, "\"", "\\\"") + "\"\n")
			got, err := ParsePrometheusFileSD(input, DeviceImportModePrometheus)
			if err != nil {
				t.Fatalf("ParsePrometheusFileSD() error = %v", err)
			}
			if len(got.Targets) != 1 {
				t.Fatalf("target count = %d, want 1", len(got.Targets))
			}
			target := got.Targets[0]
			if target.ValidationError != "" {
				t.Fatalf("ValidationError = %q, want empty", target.ValidationError)
			}
			if target.RawTarget != tt.wantRaw {
				t.Errorf("RawTarget = %q, want %q", target.RawTarget, tt.wantRaw)
			}
			if target.CanonicalHost != tt.wantHost {
				t.Errorf("CanonicalHost = %q, want %q", target.CanonicalHost, tt.wantHost)
			}
			if tt.wantPortIsSet {
				if target.ExplicitPort == nil || *target.ExplicitPort != tt.wantPort {
					t.Errorf("ExplicitPort = %v, want %d", target.ExplicitPort, tt.wantPort)
				}
			} else if target.ExplicitPort != nil {
				t.Errorf("ExplicitPort = %d, want nil", *target.ExplicitPort)
			}
		})
	}
}

func TestCanonicalImportTargetMarksSameHostDuplicatesInOrder(t *testing.T) {
	input := []byte(`
- targets:
    - edge.example:9100
    - EDGE.EXAMPLE:9200
    - edge.example
`)

	got, err := ParsePrometheusFileSD(input, DeviceImportModePrometheus)
	if err != nil {
		t.Fatalf("ParsePrometheusFileSD() error = %v", err)
	}
	if len(got.Targets) != 3 {
		t.Fatalf("target count = %d, want 3", len(got.Targets))
	}
	if got.Targets[0].DuplicateOf != nil {
		t.Errorf("first target DuplicateOf = %#v, want nil", got.Targets[0].DuplicateOf)
	}
	for i := 1; i < len(got.Targets); i++ {
		want := DeviceImportTargetLocation{GroupIndex: 0, ItemIndex: 0}
		if got.Targets[i].DuplicateOf == nil || *got.Targets[i].DuplicateOf != want {
			t.Errorf("target[%d] DuplicateOf = %#v, want %#v", i, got.Targets[i].DuplicateOf, want)
		}
	}
}

func TestCanonicalImportTargetInvalidOccurrenceDoesNotOwnDuplicate(t *testing.T) {
	input := []byte(`
- targets:
    - edge.example:9100
    - EDGE.EXAMPLE:161
    - edge.example
`)

	got, err := ParsePrometheusFileSD(input, DeviceImportModeSNMP)
	if err != nil {
		t.Fatalf("ParsePrometheusFileSD() error = %v", err)
	}
	if len(got.Targets) != 3 {
		t.Fatalf("target count = %d, want 3", len(got.Targets))
	}
	if got.Targets[0].ValidationError == "" {
		t.Fatal("first target ValidationError is empty, want invalid SNMP port")
	}
	if got.Targets[0].DuplicateOf != nil {
		t.Errorf("invalid target DuplicateOf = %#v, want nil", got.Targets[0].DuplicateOf)
	}
	if got.Targets[1].ValidationError != "" || got.Targets[1].DuplicateOf != nil {
		t.Errorf("first valid target = %#v, want valid non-duplicate", got.Targets[1])
	}
	want := DeviceImportTargetLocation{GroupIndex: 0, ItemIndex: 1}
	if got.Targets[2].DuplicateOf == nil || *got.Targets[2].DuplicateOf != want {
		t.Errorf("last target DuplicateOf = %#v, want %#v", got.Targets[2].DuplicateOf, want)
	}
}

func TestCanonicalImportTargetEnforcesDirectSNMPPortRules(t *testing.T) {
	tests := []struct {
		name      string
		target    string
		wantValid bool
	}{
		{name: "omitted port", target: "router.example", wantValid: true},
		{name: "SNMP port", target: "router.example:161", wantValid: true},
		{name: "zero port", target: "router.example:0", wantValid: false},
		{name: "different port", target: "router.example:162", wantValid: false},
		{name: "high different port", target: "router.example:65535", wantValid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := []byte("- targets: [\"" + tt.target + "\"]\n")
			got, err := ParsePrometheusFileSD(input, DeviceImportModeSNMP)
			if err != nil {
				t.Fatalf("ParsePrometheusFileSD() error = %v", err)
			}
			isValid := got.Targets[0].ValidationError == ""
			if isValid != tt.wantValid {
				t.Errorf("target %q valid = %v, want %v (error %q)", tt.target, isValid, tt.wantValid, got.Targets[0].ValidationError)
			}
		})
	}
}

func TestCanonicalImportTargetEnforcesFallbackSNMPPortRules(t *testing.T) {
	tests := []struct {
		name      string
		target    string
		wantValid bool
	}{
		{name: "omitted port", target: "router.example", wantValid: true},
		{name: "SNMP port", target: "router.example:161", wantValid: true},
		{name: "different port", target: "router.example:162", wantValid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := []byte("- targets: [\"" + tt.target + "\"]\n")
			got, err := ParsePrometheusFileSD(input, DeviceImportModePrometheusFallback)
			if err != nil {
				t.Fatalf("ParsePrometheusFileSD() error = %v", err)
			}
			isValid := got.Targets[0].ValidationError == ""
			if isValid != tt.wantValid {
				t.Errorf("fallback target %q valid = %v, want %v (error %q)",
					tt.target, isValid, tt.wantValid, got.Targets[0].ValidationError)
			}
		})
	}
}

func TestCanonicalImportTargetFallbackInvalidPortDoesNotOwnDuplicate(t *testing.T) {
	input := []byte(`
- targets:
    - edge.example:9100
    - EDGE.EXAMPLE:161
    - edge.example
`)

	got, err := ParsePrometheusFileSD(input, DeviceImportModePrometheusFallback)
	if err != nil {
		t.Fatalf("ParsePrometheusFileSD() error = %v", err)
	}
	if len(got.Targets) != 3 {
		t.Fatalf("target count = %d, want 3", len(got.Targets))
	}
	if got.Targets[0].ValidationError == "" {
		t.Fatal("first target ValidationError is empty, want invalid fallback SNMP port")
	}
	if got.Targets[0].DuplicateOf != nil {
		t.Errorf("invalid fallback target DuplicateOf = %#v, want nil", got.Targets[0].DuplicateOf)
	}
	if got.Targets[1].ValidationError != "" || got.Targets[1].DuplicateOf != nil {
		t.Errorf("first valid fallback target = %#v, want valid non-duplicate", got.Targets[1])
	}
	want := DeviceImportTargetLocation{GroupIndex: 0, ItemIndex: 1}
	if got.Targets[2].DuplicateOf == nil || *got.Targets[2].DuplicateOf != want {
		t.Errorf("last fallback target DuplicateOf = %#v, want %#v", got.Targets[2].DuplicateOf, want)
	}
}

func TestParseDeviceImportTargetsReportsInvalidItems(t *testing.T) {
	input := []byte(`
- targets:
    - 123
    - null
    - {host: router.example}
    - [router.example]
    - &endpoint anchor.example
    - *endpoint
    - !custom tagged.example
    - ""
    - "http://router.example:9100"
    - "router.example/path"
    - "user@router.example"
    - "[2001:db8::1"
    - "router.example:70000"
`)

	got, err := ParsePrometheusFileSD(input, DeviceImportModePrometheus)
	if err != nil {
		t.Fatalf("ParsePrometheusFileSD() error = %v", err)
	}
	if len(got.Targets) != 13 {
		t.Fatalf("target count = %d, want 13", len(got.Targets))
	}
	for i, target := range got.Targets {
		if i == 4 {
			if target.ValidationError != "" {
				t.Errorf("anchored string ValidationError = %q, want empty", target.ValidationError)
			}
			continue
		}
		if target.ValidationError == "" {
			t.Errorf("target[%d] = %#v, want item-level validation error", i, target)
		}
	}
	if got.Targets[0].RawTarget != "" || got.Targets[0].CanonicalHost != "" {
		t.Errorf("non-string target leaked into loggable fields: %#v", got.Targets[0])
	}
	if got.Targets[5].RawTarget != "" || got.Targets[6].RawTarget != "" {
		t.Errorf("alias/tagged target leaked into loggable fields: alias=%#v tagged=%#v", got.Targets[5], got.Targets[6])
	}
}

func TestParseDeviceImportTargetsReportsGroupDiagnosticsAndRetainsOtherGroups(t *testing.T) {
	input := []byte(`
- not-a-mapping
- labels: {ignored: value}
- targets: [discarded.example]
  targets: [also-discarded.example]
- targets: not-a-sequence
- targets: [retained.example]
`)

	got, err := ParsePrometheusFileSD(input, DeviceImportModePrometheus)
	if err != nil {
		t.Fatalf("ParsePrometheusFileSD() error = %v", err)
	}
	if len(got.Targets) != 1 || got.Targets[0].RawTarget != "retained.example" || got.Targets[0].GroupIndex != 4 {
		t.Fatalf("targets = %#v, want only retained target from group 4", got.Targets)
	}
	if len(got.Diagnostics) != 4 {
		t.Fatalf("diagnostic count = %d, want 4: %#v", len(got.Diagnostics), got.Diagnostics)
	}
	wantMessages := []string{"mapping", "missing", "exactly one", "sequence"}
	for i, diagnostic := range got.Diagnostics {
		if diagnostic.GroupIndex != i {
			t.Errorf("diagnostic[%d].GroupIndex = %d, want %d", i, diagnostic.GroupIndex, i)
		}
		if !strings.Contains(strings.ToLower(diagnostic.Message), wantMessages[i]) {
			t.Errorf("diagnostic[%d].Message = %q, want substring %q", i, diagnostic.Message, wantMessages[i])
		}
	}
}

func TestParseDeviceImportTargetsRejectsUnreadableDocuments(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty input", input: ""},
		{name: "invalid YAML", input: "- targets: [\n"},
		{name: "multiple documents", input: "- targets: [one.example]\n---\n- targets: [two.example]\n"},
		{name: "mapping root", input: "targets: [one.example]\n"},
		{name: "scalar root", input: "one.example\n"},
		{name: "empty root sequence", input: "[]\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePrometheusFileSD([]byte(tt.input), DeviceImportModePrometheus)
			if err == nil {
				t.Fatalf("ParsePrometheusFileSD() = %#v, nil error", got)
			}
		})
	}
}

func TestParseDeviceImportTargetsEnforcesFileLimit(t *testing.T) {
	input := []byte(strings.Repeat(" ", DeviceImportMaxFileBytes+1))
	_, err := ParsePrometheusFileSD(input, DeviceImportModePrometheus)
	if !errors.Is(err, ErrDeviceImportLimitExceeded) {
		t.Fatalf("ParsePrometheusFileSD() error = %v, want ErrDeviceImportLimitExceeded", err)
	}
}

func TestParseDeviceImportTargetsCountsEveryRawTargetTowardLimit(t *testing.T) {
	tests := []struct {
		name string
		item string
	}{
		{name: "malformed items", item: "null"},
		{name: "duplicate items", item: "duplicate.example"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input strings.Builder
			input.WriteString("- targets:\n")
			for i := 0; i < DeviceImportMaxTargets+1; i++ {
				input.WriteString("  - ")
				input.WriteString(tt.item)
				input.WriteByte('\n')
			}
			_, err := ParsePrometheusFileSD([]byte(input.String()), DeviceImportModePrometheus)
			if !errors.Is(err, ErrDeviceImportLimitExceeded) {
				t.Fatalf("ParsePrometheusFileSD() error = %v, want ErrDeviceImportLimitExceeded", err)
			}
		})
	}
}

func TestParseDeviceImportTargetsEnforcesPrometheusLabelValueLimitByMode(t *testing.T) {
	exactHost := strings.Join([]string{
		strings.Repeat("a", 63),
		strings.Repeat("b", 63),
		strings.Repeat("c", 63),
		strings.Repeat("d", 59),
	}, ".")
	longHost := strings.Join([]string{
		strings.Repeat("a", 63),
		strings.Repeat("b", 63),
		strings.Repeat("c", 63),
		strings.Repeat("d", 61),
	}, ".")
	exactTarget := exactHost + ":161"
	longTarget := longHost + ":161"
	if len(exactTarget) != 255 || len(longTarget) != 257 {
		t.Fatalf("test fixture lengths = (%d, %d), want (255, 257)", len(exactTarget), len(longTarget))
	}

	tests := []struct {
		name      string
		mode      DeviceImportMode
		target    string
		wantValid bool
	}{
		{name: "Prometheus exact limit", mode: DeviceImportModePrometheus, target: exactTarget, wantValid: true},
		{name: "Prometheus over limit", mode: DeviceImportModePrometheus, target: longTarget, wantValid: false},
		{name: "fallback exact limit", mode: DeviceImportModePrometheusFallback, target: exactTarget, wantValid: true},
		{name: "fallback over limit", mode: DeviceImportModePrometheusFallback, target: longTarget, wantValid: false},
		{name: "direct SNMP over Prometheus limit", mode: DeviceImportModeSNMP, target: longTarget, wantValid: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := []byte("- targets: [\"" + tt.target + "\"]\n")
			got, err := ParsePrometheusFileSD(input, tt.mode)
			if err != nil {
				t.Fatalf("ParsePrometheusFileSD() error = %v", err)
			}
			isValid := got.Targets[0].ValidationError == ""
			if isValid != tt.wantValid {
				t.Errorf("target length %d in mode %q valid = %v, want %v (error %q)",
					len(tt.target), tt.mode, isValid, tt.wantValid, got.Targets[0].ValidationError)
			}
		})
	}
}
