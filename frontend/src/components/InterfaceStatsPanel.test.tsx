import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';

import { DeviceInterfaceStatsPanel, InterfaceStatsPanel } from './InterfaceStatsPanel';
import type {
  DeviceInterfacePanelModel,
  LinkInterfacePanelModel,
} from './panelModels';

function mockDeviceModel(overrides: Partial<DeviceInterfacePanelModel> = {}): DeviceInterfacePanelModel {
  return {
    deviceId: 'dev-1',
    deviceLabel: 'router-01',
    loadingInterfaces: false,
    sections: [
      {
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
    ],
    ...overrides,
  };
}

function mockLinkModel(overrides: Partial<LinkInterfacePanelModel> = {}): LinkInterfacePanelModel {
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

describe('InterfaceStatsPanel', () => {
  it('renders a device unreachable message path from the adapted device model', () => {
    render(
      <DeviceInterfaceStatsPanel
        model={mockDeviceModel({
          sections: [{
            ...mockDeviceModel().sections[0],
            metricsUnavailableMessage: 'Device unreachable',
            statusLabel: 'down',
            statusTone: 'down',
            txLabel: '--',
            rxLabel: '--',
            utilizationPct: null,
            utilizationColor: 'var(--color-status-unknown)',
          }],
        })}
      />,
    );

    expect(screen.getByText('Device unreachable')).toBeInTheDocument();
  });

  it('renders a Prometheus unavailable message path from the adapted link model', () => {
    render(
      <InterfaceStatsPanel
        model={mockLinkModel({
          source: {
            ...mockLinkModel().source,
            metricsUnavailableMessage: 'Prometheus unavailable',
            txLabel: '--',
            rxLabel: '--',
            utilizationPct: null,
            utilizationColor: 'var(--color-status-unknown)',
          },
        })}
      />,
    );

    expect(screen.getByText('Prometheus unavailable')).toBeInTheDocument();
  });

  it('renders TX, RX, and utilization from the adapted runtime model', () => {
    render(<InterfaceStatsPanel model={mockLinkModel()} />);

    expect(screen.getByText('2 Kbps')).toBeInTheDocument();
    expect(screen.getByText('3 Kbps')).toBeInTheDocument();
    expect(screen.getByText('42%')).toBeInTheDocument();
  });

  it('renders the device no-interfaces state from the adapted model', () => {
    render(
      <DeviceInterfaceStatsPanel
        model={mockDeviceModel({ sections: [] })}
      />,
    );

    expect(screen.getByText('No interfaces discovered for this device.')).toBeInTheDocument();
  });

  it('renders a loading placeholder while device interfaces are loading', () => {
    render(
      <DeviceInterfaceStatsPanel
        model={mockDeviceModel({ loadingInterfaces: true, sections: [] })}
      />,
    );

    expect(screen.getByText('Loading interface details...')).toBeInTheDocument();
  });

  it('renders unknown interface status with neutral styling', () => {
    render(
      <DeviceInterfaceStatsPanel
        model={mockDeviceModel({
          sections: [{
            ...mockDeviceModel().sections[0],
            statusLabel: 'unknown',
            statusTone: 'neutral' as never,
          }],
        })}
      />,
    );

    expect(screen.getByText('unknown')).toHaveClass('text-on-bg-secondary');
  });
});
