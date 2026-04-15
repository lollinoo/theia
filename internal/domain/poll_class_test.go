package domain

import (
	"encoding/json"
	"testing"
	"time"
)

// TestClassifyPollClass verifies every DeviceType maps to the correct PollClass
// per D-04, including empty-string and unknown-literal fallbacks.
func TestClassifyPollClass(t *testing.T) {
	tests := []struct {
		name string
		in   DeviceType
		want PollClass
	}{
		{name: "router → core", in: DeviceTypeRouter, want: PollClassCore},
		{name: "switch → core", in: DeviceTypeSwitch, want: PollClassCore},
		{name: "ap → standard", in: DeviceTypeAP, want: PollClassStandard},
		{name: "unknown → standard", in: DeviceTypeUnknown, want: PollClassStandard},
		{name: "virtual → low", in: DeviceTypeVirtual, want: PollClassLow},
		{name: "empty string → standard (D-04 fallback)", in: DeviceType(""), want: PollClassStandard},
		{name: "bogus literal → standard (unknown fallback)", in: DeviceType("bogus"), want: PollClassStandard},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyPollClass(tc.in)
			if got != tc.want {
				t.Errorf("ClassifyPollClass(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestPollClass_Interval verifies each PollClass returns the correct performance
// polling interval per D-07, and that an unknown PollClass falls back gracefully.
func TestPollClass_Interval(t *testing.T) {
	tests := []struct {
		name string
		in   PollClass
		want time.Duration
	}{
		{name: "core → 30s", in: PollClassCore, want: 30 * time.Second},
		{name: "standard → 60s", in: PollClassStandard, want: 60 * time.Second},
		{name: "low → 300s", in: PollClassLow, want: 300 * time.Second},
		{name: "garbage → standard interval fallback", in: PollClass("garbage"), want: PollClassStandardInterval},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.Interval()
			if got != tc.want {
				t.Errorf("PollClass(%q).Interval() = %v; want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestDevice_PollClassFields_JSONRoundTrip verifies that PollClass and
// PollIntervalOverride survive a marshal/unmarshal cycle, and that a nil
// override round-trips as nil.
func TestDevice_PollClassFields_JSONRoundTrip(t *testing.T) {
	t.Run("non-nil override", func(t *testing.T) {
		override := 15
		d := Device{
			PollClass:            PollClassCore,
			PollIntervalOverride: &override,
		}

		data, err := json.Marshal(d)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}

		var got Device
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}

		if got.PollClass != PollClassCore {
			t.Errorf("PollClass: got %q; want %q", got.PollClass, PollClassCore)
		}
		if got.PollIntervalOverride == nil {
			t.Fatal("PollIntervalOverride: got nil; want non-nil")
		}
		if *got.PollIntervalOverride != 15 {
			t.Errorf("*PollIntervalOverride: got %d; want 15", *got.PollIntervalOverride)
		}
	})

	t.Run("nil override", func(t *testing.T) {
		d := Device{
			PollClass:            PollClassStandard,
			PollIntervalOverride: nil,
		}

		data, err := json.Marshal(d)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}

		var got Device
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}

		if got.PollClass != PollClassStandard {
			t.Errorf("PollClass: got %q; want %q", got.PollClass, PollClassStandard)
		}
		if got.PollIntervalOverride != nil {
			t.Errorf("PollIntervalOverride: got %v; want nil", got.PollIntervalOverride)
		}
	})
}
