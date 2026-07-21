/**
 * Exercises link edge zoom component behavior so refactors preserve the documented contract.
 */
import { act, render, screen } from '@testing-library/react';
import type { CSSProperties, ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import LinkEdge from './LinkEdge';
import { LinkLabelLayer } from './LinkLabelLayer';
import { clearLinkLabelRegistry } from './linkLabelRegistry';

vi.mock('@xyflow/react', () => ({
  BaseEdge: ({ id, style }: { id: string; style?: CSSProperties }) => (
    <path data-testid={id} style={style} />
  ),
  EdgeLabelRenderer: ({ children }: { children: ReactNode }) => <>{children}</>,
  getBezierPath: () => ['M0 0 C0 0 10 10 10 10', 48, 24],
  useInternalNode: () => undefined,
  useReactFlow: () => ({
    screenToFlowPosition: ({ x, y }: { x: number; y: number }) => ({ x, y }),
  }),
  useStore: () => {
    throw new Error('LinkEdge must not subscribe to the React Flow viewport store');
  },
}));

describe('LinkEdge zoom performance', () => {
  afterEach(() => {
    act(() => {
      clearLinkLabelRegistry();
    });
  });

  it('uses canvas CSS variables for badge readability without subscribing each edge to zoom', () => {
    render(
      <>
        <svg>
          <LinkEdge
            {...({
              id: 'edge-readable',
              source: 'dev-1',
              target: 'dev-2',
              sourceX: 0,
              sourceY: 0,
              targetX: 100,
              targetY: 100,
              sourcePosition: 'right',
              targetPosition: 'left',
              selected: false,
              data: {
                bandwidthLabel: '1 Gbps',
                throughputLabel: 'TX: 500M / RX: 300M',
                negotiationState: 'matched',
                sourceDeviceStatus: 'up',
                targetDeviceStatus: 'up',
                sourceIfStatus: 'up',
                targetIfStatus: 'up',
              },
            } as never)}
          />
        </svg>
        <LinkLabelLayer />
      </>,
    );

    expect(screen.getByTestId('edge-readable-badge-stack').style.transform).toContain(
      'var(--theia-link-badge-readability-scale, 1)',
    );
  });
});
