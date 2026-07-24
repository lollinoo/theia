package domain

// This file defines map-local link route contracts and validation invariants.

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
)

const (
	CanvasMapLinkRouteVersion      = 1
	CanvasMapLinkRouteMaxWaypoints = 16
)

// ErrCanvasMapLinkRouteNotMember reports that a route targets a link outside the saved map.
var ErrCanvasMapLinkRouteNotMember = errors.New("link is not a member of the canvas map")

// CanvasPoint is one map-local waypoint in canvas coordinates.
type CanvasPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// CanvasMapLinkRoute stores the ordered map-local waypoints for one canonical link.
type CanvasMapLinkRoute struct {
	LinkID    uuid.UUID     `json:"link_id"`
	Version   int           `json:"version"`
	Waypoints []CanvasPoint `json:"waypoints"`
	UpdatedAt time.Time     `json:"updated_at,omitempty"`
}

// CanvasMapLinkRouteRepository persists map-local link routes independently of canonical links.
type CanvasMapLinkRouteRepository interface {
	GetAllForMap(ctx context.Context, mapID uuid.UUID) ([]CanvasMapLinkRoute, error)
	UpsertForMap(ctx context.Context, mapID uuid.UUID, route CanvasMapLinkRoute) (CanvasMapLinkRoute, error)
	DeleteForMap(ctx context.Context, mapID uuid.UUID, linkID uuid.UUID) error
}

// ValidateCanvasMapLinkRoute enforces the supported route shape and finite coordinates.
func ValidateCanvasMapLinkRoute(route CanvasMapLinkRoute) error {
	if route.LinkID == uuid.Nil {
		return fmt.Errorf("link_id is required")
	}
	if route.Version != CanvasMapLinkRouteVersion {
		return fmt.Errorf("version must be %d", CanvasMapLinkRouteVersion)
	}
	if len(route.Waypoints) < 1 || len(route.Waypoints) > CanvasMapLinkRouteMaxWaypoints {
		return fmt.Errorf("waypoints must contain between 1 and %d points", CanvasMapLinkRouteMaxWaypoints)
	}
	for i, waypoint := range route.Waypoints {
		if math.IsNaN(waypoint.X) || math.IsInf(waypoint.X, 0) {
			return fmt.Errorf("waypoints[%d].x must be finite", i)
		}
		if math.IsNaN(waypoint.Y) || math.IsInf(waypoint.Y, 0) {
			return fmt.Errorf("waypoints[%d].y must be finite", i)
		}
	}
	return nil
}
