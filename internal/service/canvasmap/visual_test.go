package canvasmap

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestNormalizeVisualColor(t *testing.T) {
	t.Run("nil and blank clear the color", func(t *testing.T) {
		if got, err := NormalizeVisualColor(nil); err != nil || got != nil {
			t.Fatalf("nil color = (%v, %v), want nil nil", got, err)
		}
		blank := "  "
		if got, err := NormalizeVisualColor(&blank); err != nil || got != nil {
			t.Fatalf("blank color = (%v, %v), want nil nil", got, err)
		}
	})

	t.Run("valid colors are trimmed and uppercased", func(t *testing.T) {
		raw := " #a1b2c3 "
		got, err := NormalizeVisualColor(&raw)
		if err != nil {
			t.Fatalf("normalize color: %v", err)
		}
		if got == nil || *got != "#A1B2C3" {
			t.Fatalf("normalized color = %#v, want #A1B2C3", got)
		}
	})

	t.Run("invalid colors fail with key API error text", func(t *testing.T) {
		raw := "red"
		if _, err := NormalizeVisualColor(&raw); err == nil || err.Error() != "invalid visual_color format (must be #RRGGBB)" {
			t.Fatalf("invalid color err = %v", err)
		}
	})
}

func TestVisualColorsByDeviceID(t *testing.T) {
	deviceWithColor := uuid.New()
	deviceWithoutColor := uuid.New()
	color := "#AABBCC"

	got := VisualColorsByDeviceID([]domain.CanvasMapDeviceMembership{
		{DeviceID: deviceWithColor, VisualColor: &color},
		{DeviceID: deviceWithoutColor},
	})

	if got[deviceWithColor] != color {
		t.Fatalf("color for device = %q, want %q", got[deviceWithColor], color)
	}
	if _, ok := got[deviceWithoutColor]; ok {
		t.Fatalf("unexpected color for device without visual color")
	}
}

// TestValidateVisualColorDeviceRequiresVirtualDevice preserves the visual-color target restriction.
func TestValidateVisualColorDeviceRequiresVirtualDevice(t *testing.T) {
	if err := ValidateVisualColorDevice(domain.Device{DeviceType: domain.DeviceTypeVirtual}); err != nil {
		t.Fatalf("ValidateVisualColorDevice() virtual error = %v", err)
	}

	err := ValidateVisualColorDevice(domain.Device{DeviceType: domain.DeviceTypeRouter})
	if !errors.Is(err, ErrVisualColorRequiresVirtualDevice) {
		t.Fatalf("ValidateVisualColorDevice() router error = %v, want ErrVisualColorRequiresVirtualDevice", err)
	}
	if got, want := err.Error(), "visual_color is only supported for virtual devices"; got != want {
		t.Fatalf("ValidateVisualColorDevice() error = %q, want %q", got, want)
	}
}
