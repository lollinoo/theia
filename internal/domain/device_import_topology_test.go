package domain

import "testing"

func TestNormalizeDeviceImportTopologyLayoutScopeDefaultsToPreserve(t *testing.T) {
	if got := NormalizeDeviceImportTopologyLayoutScope(""); got != DeviceImportTopologyLayoutScopePreserve {
		t.Fatalf("NormalizeDeviceImportTopologyLayoutScope(\"\") = %q, want %q", got, DeviceImportTopologyLayoutScopePreserve)
	}
}

func TestDeviceImportTopologyItemStateTerminal(t *testing.T) {
	tests := []struct {
		state DeviceImportTopologyItemState
		want  bool
	}{
		{state: DeviceImportTopologyItemStateQueued, want: false},
		{state: DeviceImportTopologyItemStateRunning, want: false},
		{state: DeviceImportTopologyItemStateSucceeded, want: true},
		{state: DeviceImportTopologyItemStateWarning, want: true},
		{state: DeviceImportTopologyItemStateFailed, want: true},
	}

	for _, test := range tests {
		if got := test.state.Terminal(); got != test.want {
			t.Fatalf("DeviceImportTopologyItemState(%q).Terminal() = %v, want %v", test.state, got, test.want)
		}
	}
}

func TestDeviceImportTopologyRunStateActive(t *testing.T) {
	tests := []struct {
		state DeviceImportTopologyRunState
		want  bool
	}{
		{state: DeviceImportTopologyRunStateImporting, want: true},
		{state: DeviceImportTopologyRunStateDiscovering, want: true},
		{state: DeviceImportTopologyRunStateReconciling, want: true},
		{state: DeviceImportTopologyRunStateFollowup, want: true},
		{state: DeviceImportTopologyRunStateReadyForLayout, want: true},
		{state: DeviceImportTopologyRunStateFailed, want: true},
		{state: DeviceImportTopologyRunStateCompleted, want: false},
		{state: DeviceImportTopologyRunStateSuperseded, want: false},
	}

	for _, test := range tests {
		if got := test.state.Active(); got != test.want {
			t.Fatalf("DeviceImportTopologyRunState(%q).Active() = %v, want %v", test.state, got, test.want)
		}
	}
}
