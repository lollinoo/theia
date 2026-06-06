package canvasmap

// This file defines area canvas-map service behavior and topology ownership rules.

import (
	"errors"
	"strings"

	"github.com/lollinoo/theia/internal/domain"
)

// ErrAreaNameRequired stores shared err area name required state for the canvas-map orchestration.
var ErrAreaNameRequired = errors.New("name is required")

// ErrAreaNameTooLong stores shared err area name too long state for the canvas-map orchestration.
var ErrAreaNameTooLong = errors.New("area name too long (max 100 characters)")

// ErrInvalidAreaColor stores shared err invalid area color state for the canvas-map orchestration.
var ErrInvalidAreaColor = errors.New("invalid color format (must be #RRGGBB)")

// AreaMembershipFromInput normalizes canvas-map area input while preserving legacy validation text.
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
