/**
 * Exercises node border handle behavior so canvas connections remain available
 * from every edge without rendering visible handle dots.
 */
import { render, screen } from '@testing-library/react';
import type { CSSProperties } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { NodeBorderHandles } from './NodeBorderHandles';

interface MockHandleProps {
  id?: string;
  type?: string;
  position?: string;
  isConnectable?: boolean;
  style?: CSSProperties;
}

vi.mock('@xyflow/react', () => ({
  Handle: ({ id, type, position, isConnectable, style }: MockHandleProps) => (
    <span
      data-testid={`border-handle-${id}`}
      data-handle-id={id}
      data-handle-type={type}
      data-handle-position={position}
      data-is-connectable={String(isConnectable)}
      style={style}
    />
  ),
  Position: {
    Top: 'top',
    Right: 'right',
    Bottom: 'bottom',
    Left: 'left',
  },
}));

describe('NodeBorderHandles', () => {
  it('renders exactly four connectable source handles with stable ids and positions', () => {
    render(<NodeBorderHandles isConnectable />);

    const handles = screen.getAllByTestId(/^border-handle-/);
    expect(handles).toHaveLength(4);
    expect(
      handles.map((handle) => ({
        id: handle.dataset.handleId,
        type: handle.dataset.handleType,
        position: handle.dataset.handlePosition,
      })),
    ).toEqual([
      { id: 'top', type: 'source', position: 'top' },
      { id: 'right', type: 'source', position: 'right' },
      { id: 'bottom', type: 'source', position: 'bottom' },
      { id: 'left', type: 'source', position: 'left' },
    ]);

    for (const handle of handles) {
      expect(handle).toHaveAttribute('data-is-connectable', 'true');
      expect(handle).toHaveStyle({ pointerEvents: 'auto' });
    }
  });

  it('uses transparent full-edge strips instead of visible handle dots', () => {
    render(<NodeBorderHandles isConnectable />);

    for (const id of ['top', 'bottom']) {
      const handle = screen.getByTestId(`border-handle-${id}`);
      expect(handle).toHaveStyle({
        width: '100%',
        height: '12px',
      });
      expect(handle.style.background).toBe('transparent');
      expect(handle.style.borderWidth).toBe('0px');
    }

    for (const id of ['left', 'right']) {
      const handle = screen.getByTestId(`border-handle-${id}`);
      expect(handle).toHaveStyle({
        width: '12px',
        height: '100%',
      });
      expect(handle.style.background).toBe('transparent');
      expect(handle.style.borderWidth).toBe('0px');
    }
  });

  it('stacks every border strip above the self-link badge layer', () => {
    render(<NodeBorderHandles isConnectable />);

    for (const handle of screen.getAllByTestId(/^border-handle-/)) {
      expect(handle.style.zIndex).toBe('30');
    }
  });

  it('removes pointer interaction from every disabled border strip', () => {
    render(<NodeBorderHandles isConnectable={false} />);

    for (const handle of screen.getAllByTestId(/^border-handle-/)) {
      expect(handle).toHaveAttribute('data-is-connectable', 'false');
      expect(handle).toHaveStyle({ pointerEvents: 'none' });
    }
  });
});
