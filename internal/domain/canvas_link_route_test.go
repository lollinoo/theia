package domain

// This file exercises map-local link route validation so persistence and APIs share one contract.

import (
	"math"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestValidateCanvasMapLinkRoute(t *testing.T) {
	valid := CanvasMapLinkRoute{
		LinkID:  uuid.New(),
		Version: CanvasMapLinkRouteVersion,
		Waypoints: []CanvasPoint{
			{X: 12.5, Y: -8},
		},
	}
	if err := ValidateCanvasMapLinkRoute(valid); err != nil {
		t.Fatalf("valid route rejected: %v", err)
	}

	tests := []struct {
		name      string
		route     CanvasMapLinkRoute
		wantField string
	}{
		{
			name: "nil link id",
			route: CanvasMapLinkRoute{
				Version:   CanvasMapLinkRouteVersion,
				Waypoints: []CanvasPoint{{X: 1, Y: 2}},
			},
			wantField: "link_id",
		},
		{
			name: "wrong version",
			route: CanvasMapLinkRoute{
				LinkID:    uuid.New(),
				Version:   CanvasMapLinkRouteVersion + 1,
				Waypoints: []CanvasPoint{{X: 1, Y: 2}},
			},
			wantField: "version",
		},
		{
			name: "zero waypoints",
			route: CanvasMapLinkRoute{
				LinkID:  uuid.New(),
				Version: CanvasMapLinkRouteVersion,
			},
			wantField: "waypoints",
		},
		{
			name: "too many waypoints",
			route: CanvasMapLinkRoute{
				LinkID:    uuid.New(),
				Version:   CanvasMapLinkRouteVersion,
				Waypoints: make([]CanvasPoint, CanvasMapLinkRouteMaxWaypoints+1),
			},
			wantField: "waypoints",
		},
		{
			name: "not a number",
			route: CanvasMapLinkRoute{
				LinkID:    uuid.New(),
				Version:   CanvasMapLinkRouteVersion,
				Waypoints: []CanvasPoint{{X: math.NaN(), Y: 0}},
			},
			wantField: "waypoints[0].x",
		},
		{
			name: "infinity",
			route: CanvasMapLinkRoute{
				LinkID:    uuid.New(),
				Version:   CanvasMapLinkRouteVersion,
				Waypoints: []CanvasPoint{{X: 0, Y: math.Inf(1)}},
			},
			wantField: "waypoints[0].y",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCanvasMapLinkRoute(tt.route)
			if err == nil {
				t.Fatalf("expected %s validation error", tt.wantField)
			}
			if !strings.Contains(err.Error(), tt.wantField) {
				t.Fatalf("validation error %q does not identify %s", err, tt.wantField)
			}
		})
	}
}
