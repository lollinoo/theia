package domain

import (
	"strings"
	"testing"
)

func TestValidateCanvasMapName(t *testing.T) {
	if err := ValidateCanvasMapName(" Core map "); err != nil {
		t.Fatalf("ValidateCanvasMapName returned error for valid name: %v", err)
	}

	if err := ValidateCanvasMapName("   "); err == nil {
		t.Fatal("ValidateCanvasMapName returned nil for blank name")
	}

	if err := ValidateCanvasMapName(strings.Repeat("a", CanvasMapNameMaxLength+1)); err == nil {
		t.Fatal("ValidateCanvasMapName returned nil for too-long name")
	}
}

func TestValidateCanvasMapDescription(t *testing.T) {
	if err := ValidateCanvasMapDescription(strings.Repeat("a", CanvasMapDescriptionMaxLength)); err != nil {
		t.Fatalf("ValidateCanvasMapDescription returned error for max length description: %v", err)
	}

	if err := ValidateCanvasMapDescription(strings.Repeat("a", CanvasMapDescriptionMaxLength+1)); err == nil {
		t.Fatal("ValidateCanvasMapDescription returned nil for too-long description")
	}
}

func TestCanonicalCanvasMapFilterJSONDefaultsEmptyCollections(t *testing.T) {
	got, err := CanonicalCanvasMapFilterJSON(CanvasMapFilter{})
	if err != nil {
		t.Fatalf("CanonicalCanvasMapFilterJSON returned error: %v", err)
	}

	want := `{"include_cross_area_links":false,"include_ghost_devices":false}`
	if got != want {
		t.Fatalf("CanonicalCanvasMapFilterJSON = %s; want %s", got, want)
	}
}

func TestParseCanvasMapFilterDefaultsEmptyCollections(t *testing.T) {
	got, err := ParseCanvasMapFilter("")
	if err != nil {
		t.Fatalf("ParseCanvasMapFilter returned error for blank raw filter: %v", err)
	}

	if got.DeviceIDs == nil {
		t.Fatal("ParseCanvasMapFilter returned nil DeviceIDs for blank raw filter")
	}
	if got.Tags == nil {
		t.Fatal("ParseCanvasMapFilter returned nil Tags for blank raw filter")
	}
}

func TestParseCanvasMapFilterRejectsInvalidJSON(t *testing.T) {
	if _, err := ParseCanvasMapFilter("{"); err == nil {
		t.Fatal("ParseCanvasMapFilter returned nil for invalid JSON")
	}
}
