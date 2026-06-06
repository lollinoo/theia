/**
 * Exercises link label layer component behavior so refactors preserve the documented contract.
 */
import { act, render, screen } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { LinkLabelLayer } from './LinkLabelLayer';
import {
  clearLinkLabelRegistry,
  registerLinkLabel,
  unregisterLinkLabel,
} from './linkLabelRegistry';

vi.mock('@xyflow/react', () => ({
  EdgeLabelRenderer: ({ children }: { children: ReactNode }) => (
    <div data-testid="central-edge-label-renderer">{children}</div>
  ),
}));

describe('LinkLabelLayer', () => {
  afterEach(() => {
    act(() => {
      clearLinkLabelRegistry();
    });
  });

  it('renders registered link telemetry badges through one centralized label renderer', () => {
    render(<LinkLabelLayer />);

    act(() => {
      registerLinkLabel({
        edgeId: 'edge-1',
        interactive: false,
        presentation: {
          anchor: { x: 48, y: 24 },
          opacity: 0.9,
          scale: 1,
          visibility: {
            zoomBand: 'medium',
            showRate: true,
            showThroughput: true,
          },
          semanticState: 'up',
          semanticPriority: 'normal',
          items: [
            {
              key: 'rate',
              text: '1 Gbps',
              title: 'Matched speed',
              className: 'border-status-up/35 text-status-up',
            },
            {
              key: 'throughput',
              text: 'TX: 500M / RX: 300M',
              className: 'border-status-up/35 text-status-up',
            },
          ],
        },
      });
    });

    expect(screen.getByTestId('central-edge-label-renderer')).toBeInTheDocument();
    expect(screen.getByTestId('edge-1-badge-stack')).toHaveClass('topology-render-contained');
    expect(screen.getByTestId('edge-1-badge-stack').style.transform).toContain(
      'translate(48px, 24px)',
    );
    expect(screen.getByText('1 Gbps')).toBeInTheDocument();
    expect(screen.getByText('TX: 500M / RX: 300M')).toBeInTheDocument();
  });

  it('keeps badges visible during interactions while disabling per-label transitions', () => {
    render(<LinkLabelLayer />);

    act(() => {
      registerLinkLabel({
        edgeId: 'edge-interactive',
        interactive: true,
        presentation: {
          anchor: { x: 12, y: 18 },
          opacity: 1,
          scale: 1,
          visibility: {
            zoomBand: 'low',
            showRate: true,
            showThroughput: false,
          },
          semanticState: 'warning',
          semanticPriority: 'alert',
          items: [
            {
              key: 'rate',
              text: '100 Mbps',
              className: 'border-warning/35 text-warning',
              warningIndicator: {
                text: '!',
                title: 'Speed mismatch',
                className: 'border-warning/45 bg-warning/12 text-warning',
              },
            },
          ],
        },
      });
    });

    expect(screen.getByTestId('edge-interactive-badge-stack')).toHaveClass('transition-none');
    expect(screen.getByTestId('edge-interactive-badge-rate')).toHaveClass(
      'topology-link-badge',
      'transition-none',
    );
    expect(screen.getByTestId('edge-interactive-badge-rate-warning')).toHaveTextContent('!');
  });

  it('removes labels when their edge unregisters', () => {
    render(<LinkLabelLayer />);

    act(() => {
      registerLinkLabel({
        edgeId: 'edge-removed',
        interactive: false,
        presentation: {
          anchor: { x: 0, y: 0 },
          opacity: 1,
          scale: 1,
          visibility: {
            zoomBand: 'medium',
            showRate: true,
            showThroughput: false,
          },
          semanticState: 'neutral',
          semanticPriority: 'normal',
          items: [
            {
              key: 'rate',
              text: '1 Gbps',
              className: 'border-outline text-on-bg-secondary',
            },
          ],
        },
      });
    });

    expect(screen.getByTestId('edge-removed-badge-stack')).toBeInTheDocument();

    act(() => {
      unregisterLinkLabel('edge-removed');
    });

    expect(screen.queryByTestId('edge-removed-badge-stack')).not.toBeInTheDocument();
  });

  it('renders semantic zoom gating attributes and hideable badge text spans', () => {
    render(<LinkLabelLayer />);

    act(() => {
      registerLinkLabel({
        edgeId: 'edge-semantic',
        interactive: false,
        presentation: {
          anchor: { x: 8, y: 16 },
          opacity: 1,
          scale: 1,
          visibility: {
            zoomBand: 'medium',
            showRate: true,
            showThroughput: false,
          },
          semanticState: 'critical',
          semanticPriority: 'alert',
          items: [
            {
              key: 'rate',
              text: '100 Mbps',
              className: 'border-status-down/35 text-status-down',
              warningIndicator: {
                text: '!',
                title: 'Endpoint down',
                className: 'border-warning/45 bg-warning/12 text-warning',
              },
            },
          ],
        },
      });
    });

    const stack = screen.getByTestId('edge-semantic-badge-stack');
    expect(stack).toHaveClass('topology-link-badge-stack');
    expect(stack).toHaveAttribute('data-link-edge-state', 'critical');
    expect(stack).toHaveAttribute('data-link-badge-priority', 'alert');
    expect(screen.getByTestId('edge-semantic-badge-rate-text')).toHaveTextContent('100 Mbps');
    expect(screen.getByTestId('edge-semantic-badge-rate-warning')).toHaveTextContent('!');
  });

  it('updates badge gating attributes when only semantic priority changes', () => {
    const presentation = {
      anchor: { x: 8, y: 16 },
      opacity: 0.5,
      scale: 1,
      visibility: {
        zoomBand: 'medium' as const,
        showRate: true,
        showThroughput: false,
      },
      semanticState: 'up' as const,
      semanticPriority: 'normal' as const,
      items: [
        {
          key: 'rate',
          text: '1 Gbps',
          className: 'border-status-up/35 text-status-up',
        },
      ],
    };

    render(<LinkLabelLayer />);

    act(() => {
      registerLinkLabel({
        edgeId: 'edge-priority',
        interactive: false,
        presentation,
      });
    });

    expect(screen.getByTestId('edge-priority-badge-stack')).toHaveAttribute(
      'data-link-badge-priority',
      'normal',
    );

    act(() => {
      registerLinkLabel({
        edgeId: 'edge-priority',
        interactive: false,
        presentation: {
          ...presentation,
          semanticPriority: 'active',
        },
      });
    });

    expect(screen.getByTestId('edge-priority-badge-stack')).toHaveAttribute(
      'data-link-badge-priority',
      'active',
    );
  });
});
