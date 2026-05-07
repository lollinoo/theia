package domain

import (
	"strings"
	"testing"

	"github.com/google/uuid"
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

func TestValidateCanvasMapNameCountsRunes(t *testing.T) {
	exactlyMax := strings.Repeat("界", CanvasMapNameMaxLength)
	if err := ValidateCanvasMapName(exactlyMax); err != nil {
		t.Fatalf("ValidateCanvasMapName returned error for max rune count name: %v", err)
	}

	if err := ValidateCanvasMapName(exactlyMax + "界"); err == nil {
		t.Fatal("ValidateCanvasMapName returned nil for name over max rune count")
	}
}

func TestValidateCanvasMapNameRejectsPaddedRawLengthOverMax(t *testing.T) {
	name := " " + strings.Repeat("a", CanvasMapNameMaxLength) + " "

	if err := ValidateCanvasMapName(name); err == nil {
		t.Fatal("ValidateCanvasMapName returned nil for padded name with raw length over max")
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

func TestValidateCanvasMapDescriptionCountsRunes(t *testing.T) {
	exactlyMax := strings.Repeat("界", CanvasMapDescriptionMaxLength)
	if err := ValidateCanvasMapDescription(exactlyMax); err != nil {
		t.Fatalf("ValidateCanvasMapDescription returned error for max rune count description: %v", err)
	}

	if err := ValidateCanvasMapDescription(exactlyMax + "界"); err == nil {
		t.Fatal("ValidateCanvasMapDescription returned nil for description over max rune count")
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

func TestCanonicalCanvasMapFilterJSONSortsAndDeduplicatesDeviceIDs(t *testing.T) {
	firstID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	secondID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	first, err := CanonicalCanvasMapFilterJSON(CanvasMapFilter{
		DeviceIDs: []uuid.UUID{secondID, firstID, secondID},
	})
	if err != nil {
		t.Fatalf("CanonicalCanvasMapFilterJSON returned error for first filter: %v", err)
	}

	second, err := CanonicalCanvasMapFilterJSON(CanvasMapFilter{
		DeviceIDs: []uuid.UUID{firstID, secondID, firstID},
	})
	if err != nil {
		t.Fatalf("CanonicalCanvasMapFilterJSON returned error for second filter: %v", err)
	}

	want := `{"device_ids":["00000000-0000-0000-0000-000000000001","00000000-0000-0000-0000-000000000002"],"include_cross_area_links":false,"include_ghost_devices":false}`
	if first != second {
		t.Fatalf("CanonicalCanvasMapFilterJSON results differ for equivalent filters:\nfirst:  %s\nsecond: %s", first, second)
	}
	if first != want {
		t.Fatalf("CanonicalCanvasMapFilterJSON = %s; want %s", first, want)
	}
}

func TestCanonicalCanvasMapFilterJSONDoesNotMutateDeviceIDs(t *testing.T) {
	firstID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	secondID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	filter := CanvasMapFilter{
		DeviceIDs: []uuid.UUID{secondID, firstID, secondID},
	}

	if _, err := CanonicalCanvasMapFilterJSON(filter); err != nil {
		t.Fatalf("CanonicalCanvasMapFilterJSON returned error: %v", err)
	}

	want := []uuid.UUID{secondID, firstID, secondID}
	assertUUIDSliceEqual(t, filter.DeviceIDs, want)
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

func assertUUIDSliceEqual(t *testing.T, got, want []uuid.UUID) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("slice length = %d; want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("slice[%d] = %s; want %s", i, got[i], want[i])
		}
	}
}
