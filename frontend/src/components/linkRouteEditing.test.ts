/**
 * Exercises immutable link-route editing helpers used by pointer and keyboard controls.
 */
import { describe, expect, it } from 'vitest';
import type { LinkRoute } from '../types/api';
import {
  insertRouteWaypoint,
  moveRouteWaypoint,
  nudgeRouteWaypoint,
  removeRouteWaypoint,
} from './linkRouteEditing';

function routeWith(...waypoints: LinkRoute['waypoints']): LinkRoute {
  return { version: 1, waypoints };
}

describe('link route editing', () => {
  it('inserts a waypoint immutably at the requested route position', () => {
    const first = { x: 10, y: 20 };
    const second = { x: 50, y: 60 };
    const inserted = { x: 30, y: 40 };
    const route = routeWith(first, second);

    const next = insertRouteWaypoint(route, 1, inserted);

    expect(next).toEqual({
      version: 1,
      waypoints: [first, { x: 30, y: 40 }, second],
    });
    expect(next).not.toBe(route);
    expect(next?.waypoints).not.toBe(route.waypoints);
    expect(next?.waypoints[0]).toBe(first);
    expect(next?.waypoints[1]).not.toBe(inserted);
    expect(next?.waypoints[2]).toBe(second);
    expect(route.waypoints).toEqual([first, second]);
  });

  it('creates a version-one route when inserting into an automatic link', () => {
    expect(insertRouteWaypoint(null, 0, { x: 12, y: -8 })).toEqual({
      version: 1,
      waypoints: [{ x: 12, y: -8 }],
    });
  });

  it('refuses a seventeenth waypoint without changing route identity', () => {
    const route = routeWith(
      ...Array.from({ length: 16 }, (_, index) => ({ x: index, y: index * 2 })),
    );

    expect(insertRouteWaypoint(route, 8, { x: 99, y: 100 })).toBe(route);
  });

  it('moves only the requested waypoint and preserves stable ordering', () => {
    const first = { x: 10, y: 20 };
    const middle = { x: 30, y: 40 };
    const last = { x: 50, y: 60 };
    const point = { x: 33, y: 44 };
    const route = routeWith(first, middle, last);

    const next = moveRouteWaypoint(route, 1, point);

    expect(next).toEqual({ version: 1, waypoints: [first, point, last] });
    expect(next).not.toBe(route);
    expect(next.waypoints).not.toBe(route.waypoints);
    expect(next.waypoints[0]).toBe(first);
    expect(next.waypoints[1]).not.toBe(middle);
    expect(next.waypoints[1]).not.toBe(point);
    expect(next.waypoints[2]).toBe(last);
    expect(route.waypoints[1]).toBe(middle);
  });

  it('removes one waypoint immutably and returns null after removing the final waypoint', () => {
    const first = { x: 10, y: 20 };
    const middle = { x: 30, y: 40 };
    const last = { x: 50, y: 60 };
    const route = routeWith(first, middle, last);

    const next = removeRouteWaypoint(route, 1);

    expect(next).toEqual({ version: 1, waypoints: [first, last] });
    expect(next).not.toBe(route);
    expect(next?.waypoints).not.toBe(route.waypoints);
    expect(next?.waypoints[0]).toBe(first);
    expect(next?.waypoints[1]).toBe(last);
    expect(removeRouteWaypoint(routeWith(first), 0)).toBeNull();
  });

  it('nudges one waypoint by exact canvas-unit deltas', () => {
    const first = { x: 10, y: 20 };
    const second = { x: 50, y: 60 };
    const route = routeWith(first, second);

    const next = nudgeRouteWaypoint(route, 0, -1, 10);

    expect(next).toEqual({
      version: 1,
      waypoints: [{ x: 9, y: 30 }, second],
    });
    expect(next.waypoints[0]).not.toBe(first);
    expect(next.waypoints[1]).toBe(second);
    expect(route.waypoints[0]).toBe(first);
  });
});
