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

function normalizedCompositePath(path: string, reverse: boolean) {
  const tokens = path.match(/[MLC]|-?(?:\d+\.?\d*|\.\d+)(?:e[+-]?\d+)?/gi);
  if (
    tokens?.length !== 16 ||
    tokens[0] !== 'M' ||
    tokens[3] !== 'L' ||
    tokens[6] !== 'C' ||
    tokens[13] !== 'L'
  ) {
    throw new Error(`Expected an M-L-C-L composite path, received: ${path}`);
  }
  const point = (index: number) => [Number(tokens[index]), Number(tokens[index + 1])];
  const points = [point(1), point(4), point(7), point(9), point(11), point(14)];
  return reverse ? [points[5], points[4], points[3], points[2], points[1], points[0]] : points;
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
    expect(path).toHaveAttribute('d', expect.stringMatching(/^M 100,30 L .* C .* L 300,30$/));
    expect(path).toHaveAttribute('pointer-events', 'none');
    expect(path.style.fill).toBe('none');
    expect(path.style.stroke).toBe('var(--color-edge-default)');
    expect(path.style.strokeWidth).toBe('10');
    expect(container.querySelector('button')).not.toBeInTheDocument();
    expect(container.querySelector('[data-testid^="link-route-"]')).not.toBeInTheDocument();
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

    expect(path).toHaveAttribute('d', expect.stringMatching(/^M 100,30 L .* C .* L 300,30$/));
    expect(path).toHaveStyle({
      opacity: '0.45',
      stroke: 'var(--color-edge-default)',
      strokeDasharray: '6 4',
      strokeWidth: '10',
    });
  });

  it('uses hovered endpoint ids to keep preview orientation stable in either direction', () => {
    const forward = render(
      <svg>
        <FloatingConnectionLine
          {...connectionProps({
            fromNode: mockInternalNode('device-a', 0, 0),
            toNode: mockInternalNode('device-b', 300, 0),
          })}
        />
      </svg>,
    );
    const forwardPath = forward.container.querySelector('path')?.getAttribute('d');
    forward.unmount();

    const reverse = render(
      <svg>
        <FloatingConnectionLine
          {...connectionProps({
            fromNode: mockInternalNode('device-b', 300, 0),
            fromX: 300,
            fromY: 30,
            toNode: mockInternalNode('device-a', 0, 0),
            toX: 100,
            toY: 30,
          })}
        />
      </svg>,
    );
    const reversePath = reverse.container.querySelector('path')?.getAttribute('d');

    expect(forwardPath).toBeDefined();
    expect(reversePath).toBeDefined();
    expect(normalizedCompositePath(reversePath as string, true)).toEqual(
      normalizedCompositePath(forwardPath as string, false),
    );
  });
});
