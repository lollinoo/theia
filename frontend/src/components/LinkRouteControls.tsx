/**
 * Renders accessible HTML waypoint handles above React Flow's SVG edge layer.
 */
import { EdgeLabelRenderer } from '@xyflow/react';
import { type KeyboardEvent, type PointerEvent, useLayoutEffect, useRef } from 'react';
import type { LinkRoute } from '../types/api';
import { LINK_ROUTE_KEYBOARD_LARGE_STEP, LINK_ROUTE_KEYBOARD_STEP } from './linkRouteEditing';

/** Props used to coordinate route controls with one selected LinkEdge. */
export interface LinkRouteControlsProps {
  edgeId: string;
  route: LinkRoute | null;
  selected: boolean;
  editable: boolean;
  selectedWaypointIndex: number | null;
  onWaypointFocus: (waypointIndex: number) => void;
  onWaypointPointerDown: (event: PointerEvent<HTMLButtonElement>, waypointIndex: number) => void;
  onWaypointPointerMove: (event: PointerEvent<HTMLButtonElement>, waypointIndex: number) => void;
  onWaypointPointerUp: (event: PointerEvent<HTMLButtonElement>, waypointIndex: number) => void;
  onWaypointPointerCancel: (event: PointerEvent<HTMLButtonElement>, waypointIndex: number) => void;
  onWaypointNudge: (waypointIndex: number, dx: number, dy: number) => void;
  onWaypointRemove: (waypointIndex: number) => void;
}

/** Renders one 24-pixel pointer and keyboard target for every active waypoint. */
export function LinkRouteControls({
  edgeId,
  route,
  selected,
  editable,
  selectedWaypointIndex,
  onWaypointFocus,
  onWaypointPointerDown,
  onWaypointPointerMove,
  onWaypointPointerUp,
  onWaypointPointerCancel,
  onWaypointNudge,
  onWaypointRemove,
}: LinkRouteControlsProps) {
  const buttonRefs = useRef<Array<HTMLButtonElement | null>>([]);

  useLayoutEffect(() => {
    if (selected && editable && selectedWaypointIndex !== null) {
      buttonRefs.current[selectedWaypointIndex]?.focus();
    }
  }, [editable, route?.waypoints.length, selected, selectedWaypointIndex]);

  if (!selected || !editable || !route) {
    return null;
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLButtonElement>, waypointIndex: number) => {
    const step = event.shiftKey ? LINK_ROUTE_KEYBOARD_LARGE_STEP : LINK_ROUTE_KEYBOARD_STEP;
    const delta =
      event.key === 'ArrowLeft'
        ? { dx: -step, dy: 0 }
        : event.key === 'ArrowRight'
          ? { dx: step, dy: 0 }
          : event.key === 'ArrowUp'
            ? { dx: 0, dy: -step }
            : event.key === 'ArrowDown'
              ? { dx: 0, dy: step }
              : null;

    if (delta) {
      event.preventDefault();
      event.stopPropagation();
      onWaypointNudge(waypointIndex, delta.dx, delta.dy);
      return;
    }

    if (event.key === 'Delete' || event.key === 'Backspace') {
      event.preventDefault();
      event.stopPropagation();
      onWaypointRemove(waypointIndex);
    }
  };

  return (
    <EdgeLabelRenderer>
      {route.waypoints.map((point, index) => {
        return (
          <button
            // biome-ignore lint/suspicious/noArrayIndexKey: Route position is the waypoint identity; coordinate keys would remount and lose focus while dragging.
            key={index}
            ref={(element) => {
              buttonRefs.current[index] = element;
            }}
            type="button"
            className="nodrag nopan pointer-events-auto absolute h-6 w-6 -translate-x-1/2 -translate-y-1/2 rounded-full flex items-center justify-center focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring"
            style={{
              // Tailwind 4 emits translate utilities as an independent property; neutralize it
              // so the required inline transform centers the hit target exactly once.
              translate: 'none',
              transform: `translate(-50%, -50%) translate(${point.x}px, ${point.y}px)`,
            }}
            aria-label={`Move waypoint ${index + 1} for link ${edgeId}`}
            aria-pressed={selectedWaypointIndex === index}
            data-testid={`link-route-waypoint-${edgeId}-${index}`}
            onFocus={() => onWaypointFocus(index)}
            onClick={(event) => event.stopPropagation()}
            onDoubleClick={(event) => event.stopPropagation()}
            onPointerDown={(event) => {
              event.stopPropagation();
              event.currentTarget.focus();
              onWaypointFocus(index);
              onWaypointPointerDown(event, index);
            }}
            onPointerMove={(event) => {
              event.stopPropagation();
              onWaypointPointerMove(event, index);
            }}
            onPointerUp={(event) => {
              event.stopPropagation();
              onWaypointPointerUp(event, index);
            }}
            onPointerCancel={(event) => {
              event.stopPropagation();
              onWaypointPointerCancel(event, index);
            }}
            onKeyDown={(event) => handleKeyDown(event, index)}
          >
            <span
              aria-hidden="true"
              className="pointer-events-none block h-2.5 w-2.5 rounded-full border border-on-primary/60 bg-primary shadow-sm"
            />
          </button>
        );
      })}
    </EdgeLabelRenderer>
  );
}
