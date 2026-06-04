import { act, render, screen } from '@testing-library/react';
import type { CSSProperties, ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import LinkEdge from './LinkEdge';
import { LinkLabelLayer } from './LinkLabelLayer';
import { clearLinkLabelRegistry } from './linkLabelRegistry';

vi.mock('@xyflow/react', () => ({
  BaseEdge: ({
    id,
    style,
  }: {
    id: string;
    style?: CSSProperties;
  }) => <path data-testid={id} style={style} />,
  EdgeLabelRenderer: ({ children }: { children: ReactNode }) => <>{children}</>,
  getBezierPath: () => ['M0 0 C0 0 10 10 10 10', 48, 24],
}));

function renderEdge(
  overrides: Record<string, unknown> = {},
  dataOverrides: Record<string, unknown> = {},
) {
  return render(
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
    </>,
  );
}

describe('LinkEdge render', () => {
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
    expect(screen.getByTestId('edge-1')).toHaveStyle({ stroke: 'var(--color-status-up)' });
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
    expect(screen.getByTestId('edge-2')).toHaveStyle({ stroke: 'var(--color-edge-warning)' });
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
