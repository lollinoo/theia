/**
 * Exercises link edge render component behavior so refactors preserve the documented contract.
 */
import { act, fireEvent, render, screen } from '@testing-library/react';
import type { CSSProperties, ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import LinkEdge from './LinkEdge';
import { LinkLabelLayer } from './LinkLabelLayer';
import { clearLinkLabelRegistry } from './linkLabelRegistry';

const flowState = vi.hoisted(() => ({
  internalNodes: {} as Record<string, unknown>,
}));

vi.mock('@xyflow/react', () => ({
  BaseEdge: ({ id, path, style }: { id: string; path: string; style?: CSSProperties }) => (
    <path data-testid={id} d={path} style={style} />
  ),
  EdgeLabelRenderer: ({ children }: { children: ReactNode }) => <>{children}</>,
  getBezierPath: () => ['M0 0 C0 0 10 10 10 10', 48, 24],
  useInternalNode: (id: string) => flowState.internalNodes[id],
}));

function mockInternalNode(
  id: string,
  x: number,
  y: number,
  width = 100,
  height = 60,
  data: Record<string, unknown> = {},
) {
  return {
    id,
    data,
    measured: { width, height },
    internals: { positionAbsolute: { x, y } },
  };
}

function EdgeFixture({
  overrides = {},
  dataOverrides = {},
}: {
  overrides?: Record<string, unknown>;
  dataOverrides?: Record<string, unknown>;
}) {
  return (
    <>
      <svg>
        <LinkEdge
          {...({
            id: 'edge-1',
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
              link: {
                source_device_id: 'dev-1',
                target_device_id: 'dev-2',
              },
              bandwidthLabel: '1 Gbps',
              speedLabel: 'SPD 1 Gbps',
              negotiationState: 'matched',
              speedMismatch: false,
              sourceDeviceStatus: 'up',
              targetDeviceStatus: 'up',
              sourceIfStatus: 'up',
              targetIfStatus: 'up',
              ...dataOverrides,
            },
            ...overrides,
          } as never)}
        />
      </svg>
      <LinkLabelLayer />
    </>
  );
}

function renderEdge(
  overrides: Record<string, unknown> = {},
  dataOverrides: Record<string, unknown> = {},
) {
  return render(<EdgeFixture overrides={overrides} dataOverrides={dataOverrides} />);
}

describe('LinkEdge render', () => {
  beforeEach(() => {
    flowState.internalNodes = {
      'dev-1': mockInternalNode('dev-1', 0, 0),
      'dev-2': mockInternalNode('dev-2', 300, 0),
    };
  });

  afterEach(() => {
    act(() => {
      clearLinkLabelRegistry();
    });
  });

  it('shows stacked rate and throughput badges without a standalone AUTO pill', () => {
    renderEdge({}, { throughputLabel: 'TX: 500M / RX: 300M' });

    expect(screen.getByText('1 Gbps')).toBeInTheDocument();
    expect(screen.getByText('TX: 500M / RX: 300M')).toBeInTheDocument();
    expect(screen.queryByText('SPD 1 Gbps')).not.toBeInTheDocument();
    expect(screen.queryByText('AUTO')).not.toBeInTheDocument();
    expect(screen.getByTestId('edge-1')).toHaveStyle({
      stroke: 'var(--color-status-up)',
      strokeOpacity: '0.72',
    });
  });

  it('keeps the transparent pointer hit target out of the button accessibility tree', () => {
    const { container } = renderEdge({}, { onContextMenu: vi.fn() });
    const hitTarget = container.querySelector('path.cursor-pointer');

    expect(hitTarget).not.toBeNull();
    expect(hitTarget).not.toHaveAttribute('role', 'button');
    expect(hitTarget).not.toHaveAttribute('tabindex');
  });

  it('renders from live rounded node borders and updates when an endpoint moves', () => {
    const { rerender } = renderEdge();
    const firstPath = screen.getByTestId('edge-1').getAttribute('d');

    expect(firstPath).toMatch(/^M 100,30 C /);
    expect(firstPath).toMatch(/ 300,30$/);
    expect(firstPath).not.toBe('M0 0 C0 0 10 10 10 10');

    flowState.internalNodes['dev-2'] = mockInternalNode('dev-2', 420, 120);
    rerender(<EdgeFixture overrides={{ selected: true }} />);

    expect(screen.getByTestId('edge-1').getAttribute('d')).not.toBe(firstPath);
  });

  it('keeps the existing self-loop geometry and context menu behavior', () => {
    const onContextMenu = vi.fn();
    const { container } = renderEdge(
      {
        id: 'edge-loop',
        source: 'dev-1',
        target: 'dev-1',
        sourceX: 236,
        sourceY: 120,
        targetX: 76,
        targetY: 120,
      },
      { onContextMenu },
    );

    expect(screen.getByTestId('edge-loop').getAttribute('d')).toMatch(/^M 236,120 C /);
    fireEvent.contextMenu(container.querySelector('path.cursor-pointer') as SVGPathElement);
    expect(onContextMenu).toHaveBeenCalledWith(expect.anything(), 'edge-loop');
  });

  it('keeps warning mismatches amber instead of green', () => {
    renderEdge(
      { id: 'edge-2' },
      {
        bandwidthLabel: '100 Mbps',
        speedLabel: 'SPD 1 Gbps',
        negotiationState: 'mismatch',
        speedMismatch: true,
      },
    );

    expect(screen.getByTestId('edge-2-badge-rate-warning')).toHaveTextContent('!');
    expect(screen.getByTestId('edge-2')).toHaveStyle({
      stroke: 'var(--color-edge-warning)',
      strokeOpacity: '0.96',
    });
  });

  it('keeps critical operational links prominent without adding a halo layer', () => {
    renderEdge({ id: 'edge-critical' }, { alertStatus: 'down' });

    expect(screen.getByTestId('edge-critical')).toHaveStyle({
      stroke: 'var(--color-edge-critical)',
      strokeOpacity: '0.96',
    });
    expect(screen.queryByTestId('edge-critical-halo')).not.toBeInTheDocument();
  });

  it('does not hide base telemetry when an edge is visually muted', () => {
    renderEdge(
      { id: 'edge-3' },
      {
        emphasis: 'muted',
        throughputLabel: 'TX: 500M / RX: 300M',
        negotiationState: 'partial',
      },
    );

    expect(screen.getByText('1 Gbps')).toBeInTheDocument();
    expect(screen.getByText('TX: 500M / RX: 300M')).toBeInTheDocument();
  });

  it('renders virtual-to-physical up rate badges with the up tone', () => {
    renderEdge(
      { id: 'edge-4' },
      {
        sourceIsVirtual: true,
        targetIsVirtual: false,
        negotiationState: 'not_applicable',
        targetIfStatus: 'up',
      },
    );

    expect(screen.getByTestId('edge-4-badge-rate')).toHaveClass('border-status-up/35');
  });

  it('renders thicker active strokes while keeping TX/RX telemetry visible on the edge', () => {
    renderEdge({ id: 'edge-thick', selected: true }, { throughputLabel: 'TX: 500M / RX: 300M' });

    expect(screen.getByText('1 Gbps')).toBeInTheDocument();
    expect(screen.getByText('TX: 500M / RX: 300M')).toBeInTheDocument();
    expect(screen.getByTestId('edge-thick')).toHaveStyle({ strokeWidth: '10.75' });
  });

  it('uses the canvas readability scale variable for zoom-resilient telemetry badge pills', () => {
    renderEdge({ id: 'edge-readable' }, { throughputLabel: 'TX: 500M / RX: 300M' });

    expect(screen.getByTestId('edge-readable-badge-stack').style.transform).toContain(
      'scale(var(--theia-link-badge-readability-scale, 1))',
    );

    for (const badgeKey of ['rate', 'throughput']) {
      expect(screen.getByTestId(`edge-readable-badge-${badgeKey}`)).toHaveClass(
        'min-h-7',
        'px-2.5',
        'py-1.5',
        'text-[11px]',
        'leading-none',
      );
    }
  });

  it('keeps telemetry badges visible during canvas interaction without paint-heavy badge chrome', () => {
    renderEdge(
      { id: 'edge-interactive' },
      { interactionMode: 'interactive', throughputLabel: 'TX: 500M / RX: 300M' },
    );

    expect(screen.getByTestId('edge-interactive-badge-stack')).toHaveClass(
      'topology-render-contained',
      'transition-none',
    );
    expect(screen.getByText('1 Gbps')).toBeInTheDocument();
    expect(screen.getByText('TX: 500M / RX: 300M')).toBeInTheDocument();
    expect(screen.getByTestId('edge-interactive-badge-rate')).not.toHaveClass('shadow-pill');
    expect(screen.getByTestId('edge-interactive')).toHaveStyle({ transition: 'none' });
  });
});
