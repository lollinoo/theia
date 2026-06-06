package canvasmap

// This file defines visual canvas-map service behavior and topology ownership rules.

import (
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// ErrInvalidVisualColor stores shared err invalid visual color state for the canvas-map orchestration.
var ErrInvalidVisualColor = errors.New("invalid visual_color format (must be #RRGGBB)")

// ErrVisualColorRequiresVirtualDevice stores shared err visual color requires virtual device state for the canvas-map orchestration.
var ErrVisualColorRequiresVirtualDevice = errors.New("visual_color is only supported for virtual devices")

// NormalizeVisualColor trims, uppercases, validates, or clears a map-local visual color.
func NormalizeVisualColor(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	color := strings.TrimSpace(*raw)
	if color == "" {
		return nil, nil
	}
	if !isHexRGBColor(color) {
		return nil, ErrInvalidVisualColor
	}
	normalized := strings.ToUpper(color)
	return &normalized, nil
}

// VisualColorsByDeviceID indexes map-local visual color metadata by device ID for topology projection.
func VisualColorsByDeviceID(
	membership []domain.CanvasMapDeviceMembership,
) map[uuid.UUID]string {
	visualColors := make(map[uuid.UUID]string, len(membership))
	for _, member := range membership {
		if member.VisualColor == nil {
			continue
		}
		visualColors[member.DeviceID] = *member.VisualColor
	}
	return visualColors
}

// ValidateVisualColorDevice enforces that visual color overrides apply only to virtual devices.
func ValidateVisualColorDevice(device domain.Device) error {
	if device.DeviceType != domain.DeviceTypeVirtual {
		return ErrVisualColorRequiresVirtualDevice
	}
	return nil
}

// isHexRGBColor checks the strict #RRGGBB format accepted by saved-map visual metadata.
func isHexRGBColor(color string) bool {
	if len(color) != 7 || color[0] != '#' {
		return false
	}
	for _, r := range color[1:] {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}
