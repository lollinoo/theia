package domain

import (
	"slices"
	"testing"
)

func TestNormalizeProbePortsPreservesOrder(t *testing.T) {
	got, err := NormalizeProbePorts([]int{22, 8291, 443})
	if err != nil {
		t.Fatalf("NormalizeProbePorts returned error: %v", err)
	}

	want := []int{22, 8291, 443}
	if !slices.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestParseProbePortsCSVRejectsInvalidPorts(t *testing.T) {
	tests := []string{"abc", "0", "-1", "65536", "22,,443"}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			if got, err := ParseProbePortsCSV(input); err == nil {
				t.Fatalf("expected error for %q, got ports %v", input, got)
			}
		})
	}
}

func TestProbePortFormattingAndInheritance(t *testing.T) {
	parsed, err := ParseProbePortsCSV(" 22, 8291,443 ")
	if err != nil {
		t.Fatalf("ParseProbePortsCSV returned error: %v", err)
	}
	if got, want := FormatProbePortsCSV(parsed), "22,8291,443"; got != want {
		t.Fatalf("expected formatted ports %q, got %q", want, got)
	}

	tests := []struct {
		name         string
		addressPorts []int
		devicePorts  []int
		globalPorts  []int
		want         []int
	}{
		{
			name:         "address override wins",
			addressPorts: []int{443},
			devicePorts:  []int{8291},
			globalPorts:  []int{22},
			want:         []int{443},
		},
		{
			name:        "device override wins over global",
			devicePorts: []int{8291},
			globalPorts: []int{22},
			want:        []int{8291},
		},
		{
			name:        "global fallback wins when overrides empty",
			globalPorts: []int{22},
			want:        []int{22},
		},
		{
			name:         "invalid address falls through to device",
			addressPorts: []int{0},
			devicePorts:  []int{8291},
			globalPorts:  []int{22},
			want:         []int{8291},
		},
		{
			name: "default fallback when all empty",
			want: DefaultNetworkProbePorts,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveProbePorts(tt.addressPorts, tt.devicePorts, tt.globalPorts)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestCoerceNetworkProbePortsFallback(t *testing.T) {
	got := CoerceNetworkProbePortsCSV("bad-setting")
	if !slices.Equal(got, DefaultNetworkProbePorts) {
		t.Fatalf("expected default ports %v, got %v", DefaultNetworkProbePorts, got)
	}

	got[0] = 12345
	if DefaultNetworkProbePorts[0] == 12345 {
		t.Fatal("expected fallback to return a copy of default ports")
	}
}
