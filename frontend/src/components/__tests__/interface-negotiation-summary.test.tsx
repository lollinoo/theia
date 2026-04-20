import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';

import { InterfaceStatsPanel } from '../InterfaceStatsPanel';
import type { LinkInterfacePanelModel } from '../panelModels';

function mockModel(overrides: Partial<LinkInterfacePanelModel> = {}): LinkInterfacePanelModel {
  return {
    linkId: 'link-1',
    negotiation: {
      sourceLabel: '1 Gbps',
      targetLabel: '1 Gbps',
      summaryLabel: 'Matched at 1 Gbps',
      detailLabel: 'Both interfaces report the same negotiated speed.',
      tone: 'matched',
    },
    source: {
      deviceLabel: 'router-01',
      ifName: 'ether1',
      interfaceDescription: 'Uplink',
      speedLabel: '1 Gbps',
      statusLabel: 'up',
      statusTone: 'up',
      availabilityReason: null,
      metricsUnavailableMessage: null,
      txLabel: '2 Kbps',
      rxLabel: '3 Kbps',
      utilizationPct: 42,
      utilizationColor: 'var(--color-status-up)',
    },
    target: {
      deviceLabel: 'switch-01',
      ifName: 'ether2',
      interfaceDescription: 'Downlink',
      speedLabel: '1 Gbps',
      statusLabel: 'up',
      statusTone: 'up',
      availabilityReason: null,
      metricsUnavailableMessage: null,
      txLabel: '4 Kbps',
      rxLabel: '5 Kbps',
      utilizationPct: 75,
      utilizationColor: 'var(--color-status-probing)',
    },
    ...overrides,
  };
}

describe('InterfaceStatsPanel autonegotiation summary', () => {
  it('renders a dedicated autonegotiation summary card', () => {
    render(<InterfaceStatsPanel model={mockModel()} />);
    const summaryCard = screen.getByText('Matched at 1 Gbps').closest('div.rounded-xl');

    expect(screen.getByText('Autonegotiation')).toBeInTheDocument();
    expect(screen.getByText('Matched at 1 Gbps')).toBeInTheDocument();
    expect(screen.getByText('Both interfaces report the same negotiated speed.')).toBeInTheDocument();
    expect(summaryCard).toHaveClass('border-status-up/30');
  });

  it('renders mismatch, partial, and unknown negotiation variants', () => {
    const { rerender } = render(
      <InterfaceStatsPanel
        model={mockModel({
          negotiation: {
            sourceLabel: '1 Gbps',
            targetLabel: '100 Mbps',
            summaryLabel: '1 Gbps vs 100 Mbps',
            detailLabel: 'The two ends report different negotiated speeds.',
            tone: 'mismatch',
          },
        })}
      />,
    );
    let summaryCard = screen.getByText('1 Gbps vs 100 Mbps').closest('div.rounded-xl');

    expect(screen.getByText('1 Gbps vs 100 Mbps')).toBeInTheDocument();
    expect(screen.getByText('The two ends report different negotiated speeds.')).toBeInTheDocument();
    expect(summaryCard).toHaveClass('border-status-probing/30');

    rerender(
      <InterfaceStatsPanel
        model={mockModel({
          negotiation: {
            sourceLabel: '1 Gbps',
            targetLabel: 'Unknown',
            summaryLabel: '1 Gbps',
            detailLabel: 'Only one side exposed a negotiated speed.',
            tone: 'partial',
          },
        })}
      />,
    );
    expect(screen.getByText('Only one side exposed a negotiated speed.')).toBeInTheDocument();
    summaryCard = screen.getByText('Only one side exposed a negotiated speed.').closest('div.rounded-xl');
    expect(summaryCard).toHaveClass('border-status-probing/30');

    rerender(
      <InterfaceStatsPanel
        model={mockModel({
          negotiation: {
            sourceLabel: 'Unknown',
            targetLabel: 'Unknown',
            summaryLabel: 'Autonegotiation',
            detailLabel: 'Waiting for interface speed data from one or both ends.',
            tone: 'unknown',
          },
        })}
      />,
    );
    summaryCard = screen.getByText('Waiting for interface speed data from one or both ends.').closest('div.rounded-xl');

    expect(screen.getByText('Waiting for interface speed data from one or both ends.')).toBeInTheDocument();
    expect(summaryCard).toHaveClass('border-outline-subtle');
  });
});
