/**
 * Pure, bounded transformations for local link-route editing.
 */
import type { LinkRoute, LinkWaypoint } from '../types/api';

const LINK_ROUTE_MAX_WAYPOINTS = 16;

/** Minimum client-pixel movement that turns an edge press into a route drag. */
export const LINK_ROUTE_DRAG_THRESHOLD_PX = 4;
/** Canvas-unit distance applied by an unmodified arrow key. */
export const LINK_ROUTE_KEYBOARD_STEP = 1;
/** Canvas-unit distance applied by Shift plus an arrow key. */
export const LINK_ROUTE_KEYBOARD_LARGE_STEP = 10;

/** Inserts one copied waypoint while preserving references to existing points. */
export function insertRouteWaypoint(
  route: LinkRoute | null,
  insertIndex: number,
  point: LinkWaypoint,
): LinkRoute {
  if (route && route.waypoints.length >= LINK_ROUTE_MAX_WAYPOINTS) {
    return route;
  }

  const waypoints = route?.waypoints ?? [];
  const boundedIndex = Math.min(Math.max(0, insertIndex), waypoints.length);
  const nextWaypoints = waypoints.slice();
  nextWaypoints.splice(boundedIndex, 0, { x: point.x, y: point.y });
  return { version: 1, waypoints: nextWaypoints };
}

/** Replaces one waypoint with a copied point and leaves all other point references stable. */
export function moveRouteWaypoint(
  route: LinkRoute,
  waypointIndex: number,
  point: LinkWaypoint,
): LinkRoute {
  if (waypointIndex < 0 || waypointIndex >= route.waypoints.length) {
    return route;
  }

  const waypoints = route.waypoints.slice();
  waypoints[waypointIndex] = { x: point.x, y: point.y };
  return { version: 1, waypoints };
}

/** Removes one waypoint, normalizing an empty route back to automatic routing. */
export function removeRouteWaypoint(route: LinkRoute, waypointIndex: number): LinkRoute | null {
  if (waypointIndex < 0 || waypointIndex >= route.waypoints.length) {
    return route;
  }
  if (route.waypoints.length === 1) {
    return null;
  }

  const waypoints = route.waypoints.slice();
  waypoints.splice(waypointIndex, 1);
  return { version: 1, waypoints };
}

/** Moves one waypoint by an exact canvas-space delta. */
export function nudgeRouteWaypoint(
  route: LinkRoute,
  waypointIndex: number,
  dx: number,
  dy: number,
): LinkRoute {
  const waypoint = route.waypoints[waypointIndex];
  if (!waypoint) {
    return route;
  }
  return moveRouteWaypoint(route, waypointIndex, {
    x: waypoint.x + dx,
    y: waypoint.y + dy,
  });
}
