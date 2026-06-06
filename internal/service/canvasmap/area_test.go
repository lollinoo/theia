package canvasmap

// This file exercises area behavior so refactors preserve the documented contract.

import "testing"

// TestAreaMembershipFromInputTrimsFieldsAndDefaultsColor preserves map-local area normalization.
func TestAreaMembershipFromInputTrimsFieldsAndDefaultsColor(t *testing.T) {
	area, err := AreaMembershipFromInput(" Core ", " backbone ", " ")
	if err != nil {
		t.Fatalf("AreaMembershipFromInput() error = %v", err)
	}

	if area.Name != "Core" {
		t.Fatalf("area name = %q, want Core", area.Name)
	}
	if area.Description != "backbone" {
		t.Fatalf("area description = %q, want backbone", area.Description)
	}
	if area.Color != "#00E676" {
		t.Fatalf("area color = %q, want default #00E676", area.Color)
	}
}

// TestAreaMembershipFromInputPreservesLegacyColorValidation keeps the historical shallow color validation.
func TestAreaMembershipFromInputPreservesLegacyColorValidation(t *testing.T) {
	area, err := AreaMembershipFromInput("Core", "", " #GGGGGG ")
	if err != nil {
		t.Fatalf("AreaMembershipFromInput() legacy color error = %v", err)
	}
	if area.Color != "#GGGGGG" {
		t.Fatalf("area color = %q, want trimmed legacy color", area.Color)
	}
}

// TestAreaMembershipFromInputValidationErrors locks the public area validation messages.
func TestAreaMembershipFromInputValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		inputName   string
		inputColor  string
		wantMessage string
	}{
		{
			name:        "blank name",
			inputName:   " ",
			inputColor:  "#00E676",
			wantMessage: "name is required",
		},
		{
			name:        "long name",
			inputName:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			inputColor:  "#00E676",
			wantMessage: "area name too long (max 100 characters)",
		},
		{
			name:        "bad color",
			inputName:   "Core",
			inputColor:  "00E676",
			wantMessage: "invalid color format (must be #RRGGBB)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := AreaMembershipFromInput(tt.inputName, "", tt.inputColor)
			if err == nil {
				t.Fatal("AreaMembershipFromInput() error = nil, want validation error")
			}
			if got := err.Error(); got != tt.wantMessage {
				t.Fatalf("AreaMembershipFromInput() error = %q, want %q", got, tt.wantMessage)
			}
		})
	}
}
