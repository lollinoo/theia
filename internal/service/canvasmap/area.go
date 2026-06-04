package canvasmap

import (
	"errors"
	"strings"

	"github.com/lollinoo/theia/internal/domain"
)

var ErrAreaNameRequired = errors.New("name is required")
var ErrAreaNameTooLong = errors.New("area name too long (max 100 characters)")
var ErrInvalidAreaColor = errors.New("invalid color format (must be #RRGGBB)")

func AreaMembershipFromInput(
	name string,
	description string,
	color string,
) (domain.CanvasMapAreaMembership, error) {
	normalizedName := strings.TrimSpace(name)
	if normalizedName == "" {
		return domain.CanvasMapAreaMembership{}, ErrAreaNameRequired
	}
	if len(normalizedName) > 100 {
		return domain.CanvasMapAreaMembership{}, ErrAreaNameTooLong
	}

	normalizedColor := strings.TrimSpace(color)
	if normalizedColor == "" {
		normalizedColor = "#00E676"
	}
	if !strings.HasPrefix(normalizedColor, "#") || len(normalizedColor) != 7 {
		return domain.CanvasMapAreaMembership{}, ErrInvalidAreaColor
	}

	return domain.CanvasMapAreaMembership{
		Name:        normalizedName,
		Description: strings.TrimSpace(description),
		Color:       normalizedColor,
	}, nil
}
