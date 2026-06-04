package canvasmap

import (
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

var ErrInvalidVisualColor = errors.New("invalid visual_color format (must be #RRGGBB)")

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
