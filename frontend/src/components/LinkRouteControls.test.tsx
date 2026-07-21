/**
 * Exercises accessible waypoint controls independently from edge geometry and persistence.
 */
import { act, fireEvent, render, screen } from '@testing-library/react';
import { type ReactNode, useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import type { LinkRoute } from '../types/api';
import { LinkRouteControls, type LinkRouteControlsProps } from './LinkRouteControls';
import { nudgeRouteWaypoint, removeRouteWaypoint } from './linkRouteEditing';

vi.mock('@xyflow/react', () => ({
  EdgeLabelRenderer: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

const ROUTE: LinkRoute = {
  version: 1,
  waypoints: [
    { x: 20, y: 30 },
    { x: 80, y: 90 },
  ],
};

function renderControls(
  overrides: Partial<LinkRouteControlsProps> = {},
  onParentPointerDown?: () => void,
) {
  const props: LinkRouteControlsProps = {
    edgeId: 'edge-1',
    route: ROUTE,
    selected: true,
    editable: true,
    selectedWaypointIndex: null,
    onWaypointFocus: vi.fn(),
    onWaypointPointerDown: vi.fn(),
    onWaypointPointerMove: vi.fn(),
    onWaypointPointerUp: vi.fn(),
    onWaypointPointerCancel: vi.fn(),
    onWaypointNudge: vi.fn(),
    onWaypointRemove: vi.fn(),
    ...overrides,
  };
  return {
    ...render(
      <div onPointerDown={onParentPointerDown}>
        <LinkRouteControls {...props} />
      </div>,
    ),
    props,
  };
}

describe('LinkRouteControls', () => {
  it('renders one accessible 24-pixel hit target per waypoint', () => {
    renderControls({ selectedWaypointIndex: 1 });

    const first = screen.getByRole('button', {
      name: 'Move waypoint 1 for link edge-1',
    });
    const second = screen.getByRole('button', {
      name: 'Move waypoint 2 for link edge-1',
    });

    expect(first).toHaveClass('h-6', 'w-6', 'nodrag', 'nopan', 'pointer-events-auto');
    expect(first).toHaveStyle({
      transform: 'translate(-50%, -50%) translate(20px, 30px)',
    });
    expect(first.firstElementChild).toHaveClass('h-2.5', 'w-2.5');
    expect(first).toHaveAttribute('aria-pressed', 'false');
    expect(second).toHaveAttribute('aria-pressed', 'true');
  });

  it('reports waypoint indices for pointer gestures and stops their propagation', () => {
    const parentPointerDown = vi.fn();
    const callbacks = {
      onWaypointPointerDown: vi.fn(),
      onWaypointPointerMove: vi.fn(),
      onWaypointPointerUp: vi.fn(),
      onWaypointPointerCancel: vi.fn(),
    };
    const { props } = renderControls({ ...callbacks }, parentPointerDown);
    const second = screen.getByRole('button', {
      name: 'Move waypoint 2 for link edge-1',
    });
    act(() => {
      fireEvent.pointerDown(second, { pointerId: 7, clientX: 80, clientY: 90 });
      fireEvent.pointerMove(second, { pointerId: 7, clientX: 82, clientY: 92 });
      fireEvent.pointerUp(second, { pointerId: 7, clientX: 82, clientY: 92 });
      fireEvent.pointerCancel(second, { pointerId: 7 });
    });

    expect(props.onWaypointFocus).toHaveBeenCalledWith(1);
    expect(callbacks.onWaypointPointerDown).toHaveBeenCalledWith(expect.anything(), 1);
    expect(callbacks.onWaypointPointerMove).toHaveBeenCalledWith(expect.anything(), 1);
    expect(callbacks.onWaypointPointerUp).toHaveBeenCalledWith(expect.anything(), 1);
    expect(callbacks.onWaypointPointerCancel).toHaveBeenCalledWith(expect.anything(), 1);
    expect(parentPointerDown).not.toHaveBeenCalled();
  });

  it('maps arrow keys to exact small and large nudges while retaining focus', () => {
    const onWaypointNudge = vi.fn();

    function Harness() {
      const [route, setRoute] = useState(ROUTE);
      return (
        <LinkRouteControls
          edgeId="edge-1"
          route={route}
          selected
          editable
          selectedWaypointIndex={0}
          onWaypointFocus={vi.fn()}
          onWaypointPointerDown={vi.fn()}
          onWaypointPointerMove={vi.fn()}
          onWaypointPointerUp={vi.fn()}
          onWaypointPointerCancel={vi.fn()}
          onWaypointNudge={(index, dx, dy) => {
            onWaypointNudge(index, dx, dy);
            setRoute((current) => nudgeRouteWaypoint(current, index, dx, dy));
          }}
          onWaypointRemove={vi.fn()}
        />
      );
    }

    render(<Harness />);
    const first = screen.getByRole('button', {
      name: 'Move waypoint 1 for link edge-1',
    });

    act(() => {
      first.focus();
      fireEvent.keyDown(first, { key: 'ArrowRight' });
    });
    expect(onWaypointNudge).toHaveBeenLastCalledWith(0, 1, 0);
    expect(first).toHaveStyle({
      transform: 'translate(-50%, -50%) translate(21px, 30px)',
    });
    expect(first).toHaveFocus();

    act(() => {
      fireEvent.keyDown(first, { key: 'ArrowDown', shiftKey: true });
    });
    expect(onWaypointNudge).toHaveBeenLastCalledWith(0, 0, 10);
    expect(first).toHaveStyle({
      transform: 'translate(-50%, -50%) translate(21px, 40px)',
    });
    expect(first).toHaveFocus();
  });

  it('removes the focused waypoint for Delete and Backspace', () => {
    const onWaypointRemove = vi.fn();

    function Harness() {
      const [route, setRoute] = useState(ROUTE);
      return (
        <LinkRouteControls
          edgeId="edge-1"
          route={route}
          selected
          editable
          selectedWaypointIndex={0}
          onWaypointFocus={vi.fn()}
          onWaypointPointerDown={vi.fn()}
          onWaypointPointerMove={vi.fn()}
          onWaypointPointerUp={vi.fn()}
          onWaypointPointerCancel={vi.fn()}
          onWaypointNudge={vi.fn()}
          onWaypointRemove={(index) => {
            onWaypointRemove(index);
            setRoute((current) => removeRouteWaypoint(current, index) ?? current);
          }}
        />
      );
    }

    render(<Harness />);
    const first = screen.getByRole('button', {
      name: 'Move waypoint 1 for link edge-1',
    });

    act(() => {
      fireEvent.keyDown(first, { key: 'Delete' });
    });
    expect(onWaypointRemove).toHaveBeenCalledWith(0);
    expect(screen.getAllByRole('button')).toHaveLength(1);

    act(() => {
      fireEvent.keyDown(screen.getByRole('button'), { key: 'Backspace' });
    });
    expect(onWaypointRemove).toHaveBeenLastCalledWith(0);
  });

  it('does not render controls unless the edge is both selected and editable', () => {
    const { props, rerender } = renderControls({ selected: false, editable: true });
    expect(screen.queryByRole('button')).not.toBeInTheDocument();

    rerender(<LinkRouteControls {...props} selected editable={false} />);
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });
});
