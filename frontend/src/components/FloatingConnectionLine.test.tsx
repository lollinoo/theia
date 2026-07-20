/**
 * Exercises the in-progress floating connection preview.
 */
import { render } from '@testing-library/react';
import type { ConnectionLineComponentProps, InternalNode } from '@xyflow/react';
import { describe, expect, it } from 'vitest';
import type { DeviceNode } from './DeviceCard';
import { FloatingConnectionLine } from './FloatingConnectionLine';

function mockInternalNode(
  id: string,
  x: number,
  y: number,
  data: Partial<DeviceNode['data']> = {},
): InternalNode<DeviceNode> {
  return {
    id,
    data,
    measured: { width: 100, height: 60 },
    internals: { positionAbsolute: { x, y } },
  } as InternalNode<DeviceNode>;
}

function connectionProps(
  overrides: Partial<ConnectionLineComponentProps<DeviceNode>> = {},
): ConnectionLineComponentProps<DeviceNode> {
  return {
    fromNode: mockInternalNode('source', 0, 0),
    fromX: 100,
    fromY: 30,
    toX: 300,
    toY: 30,
    toNode: null,
    connectionLineStyle: undefined,
    ...overrides,
  } as ConnectionLineComponentProps<DeviceNode>;
}

describe('FloatingConnectionLine', () => {
  it('floats from the source border to a pointer-only target with default styling', () => {
    const { container } = render(
      <svg>
        <FloatingConnectionLine {...connectionProps()} />
      </svg>,
    );
    const paths = container.querySelectorAll('path');
    const path = paths[0];

    expect(paths).toHaveLength(1);
    expect(path).toHaveAttribute('d', expect.stringMatching(/^M 100,30 C .* 300,30$/));
    expect(path).toHaveAttribute('pointer-events', 'none');
    expect(path.style.fill).toBe('none');
    expect(path.style.stroke).toBe('var(--color-edge-default)');
    expect(path.style.strokeWidth).toBe('10');
  });

  it('floats onto a hovered target border and retains incoming line styles', () => {
    const { container } = render(
      <svg>
        <FloatingConnectionLine
          {...connectionProps({
            toX: 999,
            toY: 999,
            toNode: mockInternalNode('target', 300, 0, { isVirtual: true }),
            connectionLineStyle: { opacity: 0.45, strokeDasharray: '6 4' },
          })}
        />
      </svg>,
    );
    const path = container.querySelector('path');

    expect(path).toHaveAttribute('d', expect.stringMatching(/^M 100,30 C .* 300,30$/));
    expect(path).toHaveStyle({
      opacity: '0.45',
      stroke: 'var(--color-edge-default)',
      strokeDasharray: '6 4',
      strokeWidth: '10',
    });
  });
});
