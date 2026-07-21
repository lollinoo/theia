package postgres

// This file defines map-local link route persistence and membership enforcement.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// CanvasMapLinkRouteRepo implements domain.CanvasMapLinkRouteRepository using PostgreSQL SQL.
type CanvasMapLinkRouteRepo struct {
	db *DB
}

var _ domain.CanvasMapLinkRouteRepository = (*CanvasMapLinkRouteRepo)(nil)

// NewCanvasMapLinkRouteRepo creates a new PostgreSQL-backed map-local link route repository.
func NewCanvasMapLinkRouteRepo(db *sql.DB) *CanvasMapLinkRouteRepo {
	return &CanvasMapLinkRouteRepo{db: wrapDB(db)}
}

// GetAllForMap retrieves validated link routes ordered by canonical link ID.
func (r *CanvasMapLinkRouteRepo) GetAllForMap(ctx context.Context, mapID uuid.UUID) ([]domain.CanvasMapLinkRoute, error) {
	if mapID == uuid.Nil {
		return nil, fmt.Errorf("canvas map id is required")
	}

	rows, err := r.db.QueryContext(
		ctx,
		`SELECT link_id, route_version, waypoints_json, updated_at
		 FROM canvas_map_link_routes
		 WHERE map_id = ?
		 ORDER BY link_id`,
		mapID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("querying canvas map link routes for %s: %w", mapID, err)
	}
	defer rows.Close()

	routes := make([]domain.CanvasMapLinkRoute, 0)
	for rows.Next() {
		route, err := scanCanvasMapLinkRoute(rows)
		if err != nil {
			return nil, fmt.Errorf("reading canvas map link route for map %s: %w", mapID, err)
		}
		routes = append(routes, route)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating canvas map link routes for %s: %w", mapID, err)
	}
	return routes, nil
}

// UpsertForMap creates or replaces one route after verifying that its link belongs to the map.
func (r *CanvasMapLinkRouteRepo) UpsertForMap(
	ctx context.Context,
	mapID uuid.UUID,
	route domain.CanvasMapLinkRoute,
) (domain.CanvasMapLinkRoute, error) {
	if mapID == uuid.Nil {
		return domain.CanvasMapLinkRoute{}, fmt.Errorf("canvas map id is required")
	}
	if err := domain.ValidateCanvasMapLinkRoute(route); err != nil {
		return domain.CanvasMapLinkRoute{}, fmt.Errorf("validating canvas map link route: %w", err)
	}

	waypointsJSON, err := json.Marshal(route.Waypoints)
	if err != nil {
		return domain.CanvasMapLinkRoute{}, fmt.Errorf("encoding canvas map link route waypoints: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.CanvasMapLinkRoute{}, fmt.Errorf("starting canvas map link route transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := ensureCanvasMapExistsContext(ctx, tx, mapID); err != nil {
		return domain.CanvasMapLinkRoute{}, err
	}
	if err := lockCanvasMapLinkMembership(ctx, tx, mapID, route.LinkID); err != nil {
		return domain.CanvasMapLinkRoute{}, err
	}

	route.UpdatedAt = time.Now().UTC()
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO canvas_map_link_routes
			(map_id, link_id, route_version, waypoints_json, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(map_id, link_id) DO UPDATE SET
			route_version = excluded.route_version,
			waypoints_json = excluded.waypoints_json,
			updated_at = excluded.updated_at`,
		mapID.String(),
		route.LinkID.String(),
		route.Version,
		string(waypointsJSON),
		route.UpdatedAt,
	); err != nil {
		return domain.CanvasMapLinkRoute{}, fmt.Errorf("upserting canvas map link route for %s: %w", route.LinkID, err)
	}
	if err := tx.Commit(); err != nil {
		return domain.CanvasMapLinkRoute{}, fmt.Errorf("committing canvas map link route for %s: %w", route.LinkID, err)
	}
	return route, nil
}

// DeleteForMap removes one map-local route without changing canonical link membership.
func (r *CanvasMapLinkRouteRepo) DeleteForMap(ctx context.Context, mapID uuid.UUID, linkID uuid.UUID) error {
	if mapID == uuid.Nil {
		return fmt.Errorf("canvas map id is required")
	}
	if linkID == uuid.Nil {
		return fmt.Errorf("link id is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting canvas map link route delete transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := ensureCanvasMapExistsContext(ctx, tx, mapID); err != nil {
		return err
	}
	if err := lockCanvasMapLinkMembership(ctx, tx, mapID, linkID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(
		ctx,
		`DELETE FROM canvas_map_link_routes WHERE map_id = ? AND link_id = ?`,
		mapID.String(),
		linkID.String(),
	); err != nil {
		return fmt.Errorf("deleting canvas map link route for %s: %w", linkID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing canvas map link route delete for %s: %w", linkID, err)
	}
	return nil
}

func lockCanvasMapLinkMembership(ctx context.Context, tx *Tx, mapID uuid.UUID, linkID uuid.UUID) error {
	var membershipMarker int
	if err := tx.QueryRowContext(
		ctx,
		`SELECT 1
		 FROM canvas_map_links
		 WHERE map_id = ? AND link_id = ?
		 FOR KEY SHARE`,
		mapID.String(),
		linkID.String(),
	).Scan(&membershipMarker); err == sql.ErrNoRows {
		return fmt.Errorf(
			"link %s on canvas map %s: %w",
			linkID,
			mapID,
			domain.ErrCanvasMapLinkRouteNotMember,
		)
	} else if err != nil {
		return fmt.Errorf("checking canvas map link route membership for %s: %w", linkID, err)
	}
	return nil
}

func scanCanvasMapLinkRoute(scanner rowScanner) (domain.CanvasMapLinkRoute, error) {
	var route domain.CanvasMapLinkRoute
	var linkIDRaw, waypointsJSON string
	if err := scanner.Scan(&linkIDRaw, &route.Version, &waypointsJSON, &route.UpdatedAt); err != nil {
		return domain.CanvasMapLinkRoute{}, fmt.Errorf("scanning canvas map link route: %w", err)
	}

	linkID, err := uuid.Parse(linkIDRaw)
	if err != nil {
		return domain.CanvasMapLinkRoute{}, fmt.Errorf("parsing canvas map link route id %q: %w", linkIDRaw, err)
	}
	route.LinkID = linkID
	if err := json.Unmarshal([]byte(waypointsJSON), &route.Waypoints); err != nil {
		return domain.CanvasMapLinkRoute{}, fmt.Errorf("decoding canvas map link route waypoints for %s: %w", linkID, err)
	}
	if err := domain.ValidateCanvasMapLinkRoute(route); err != nil {
		return domain.CanvasMapLinkRoute{}, fmt.Errorf("validating stored canvas map link route for %s: %w", linkID, err)
	}
	return route, nil
}
