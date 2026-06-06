/**
 * Exercises device topology discovery section device configuration behavior so refactors preserve the documented contract.
 */
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ServerError, ValidationError } from '../../api/errors';
import type { Device, MetricsSource, TopologyDiscoveryMode } from '../../types/api';
import {
  formatTopologyBootstrapState,
  formatTopologyDiscoveryResult,
  formatTopologyDiscoveryTimestamp,
} from '../../utils/topologyDiscovery';
import { DeviceTopologyDiscoverySection } from './DeviceTopologyDiscoverySection';

vi.mock('../../api/client', () => ({
  fetchSettings: vi.fn().mockResolvedValue({}),
  runTopologyDiscovery: vi.fn().mockResolvedValue(undefined),
}));

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-1',
    hostname: 'router-01',
    ip: '10.0.0.1',
    notes: null,
    device_type: 'router',
    poll_class: 'core',
    poll_interval_override: null,
    polling_enabled: true,
    status: 'up',
    sys_name: 'router-01',
    sys_descr: 'RouterOS',
    hardware_model: 'RB4011',
    vendor: 'mikrotik',
    managed: true,
    interfaces: [],
    backup_supported: true,
    metrics_source: 'snmp',
    prometheus_label_name: 'instance',
    prometheus_label_value: '10.0.0.1:9100',
    topology_discovery_mode: 'inherit',
    effective_topology_discovery_mode: 'off',
    topology_bootstrap_state: 'idle',
    last_topology_discovery_at: null,
    last_topology_discovery_result: '',
    area_ids: [],
    ...overrides,
  };
}

function renderSection({
  device = mockDevice(),
  topologyDiscoveryMode = 'inherit',
  metricsMode = 'snmp',
  ip = device.ip,
  readOnly = false,
  resetKey = 'dev-1',
  isVirtual = false,
  onTopologyDiscoveryModeChange = vi.fn(),
}: {
  device?: Device;
  topologyDiscoveryMode?: TopologyDiscoveryMode;
  metricsMode?: MetricsSource;
  ip?: string;
  readOnly?: boolean;
  resetKey?: string;
  isVirtual?: boolean;
  onTopologyDiscoveryModeChange?: (mode: TopologyDiscoveryMode) => void;
} = {}) {
  return render(
    <DeviceTopologyDiscoverySection
      device={device}
      topologyDiscoveryMode={topologyDiscoveryMode}
      metricsMode={metricsMode}
      ip={ip}
      readOnly={readOnly}
      resetKey={resetKey}
      isVirtual={isVirtual}
      onTopologyDiscoveryModeChange={onTopologyDiscoveryModeChange}
    />,
  );
}

function controlledPromise<T = void>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve;
    reject = promiseReject;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('DeviceTopologyDiscoverySection', () => {
  it('renders nothing for virtual devices', () => {
    renderSection({ isVirtual: true });

    expect(screen.queryByText('Topology Discovery')).not.toBeInTheDocument();
  });

  it('renders mode controls and discovery status summaries', async () => {
    const { fetchSettings } = await import('../../api/client');
    (fetchSettings as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      topology_discovery_default_mode: 'off',
    });

    renderSection({
      device: mockDevice({
        effective_topology_discovery_mode: 'bootstrap_once',
        topology_bootstrap_state: 'followup_scheduled',
        last_topology_discovery_at: '2026-04-18T10:09:16Z',
        last_topology_discovery_result: 'ports_pending',
      }),
    });

    expect(screen.getByText('Topology Discovery')).toBeInTheDocument();
    expect(screen.getByText('Effective: Bootstrap once')).toBeInTheDocument();
    expect(screen.getByLabelText('Topology Discovery')).toHaveValue('inherit');
    expect(screen.getByText('Bootstrap State')).toBeInTheDocument();
    expect(
      screen.getByText(formatTopologyBootstrapState('followup_scheduled')),
    ).toBeInTheDocument();
    expect(screen.getByText('Last Discovery')).toBeInTheDocument();
    expect(
      screen.getByText(formatTopologyDiscoveryTimestamp('2026-04-18T10:09:16Z')),
    ).toBeInTheDocument();
    expect(screen.getByText('Last Result')).toBeInTheDocument();
    expect(screen.getByText(formatTopologyDiscoveryResult('ports_pending'))).toBeInTheDocument();
    expect(screen.getByText('Next Follow-up')).toBeInTheDocument();
    expect(
      screen.getByText('Automatic follow-up runs about 20s after last discovery.'),
    ).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByText('Use global default (Off)')).toBeInTheDocument();
    });
  });

  it('uses lldp/cdp as the global default label when settings fetch fails', async () => {
    const { fetchSettings } = await import('../../api/client');
    (fetchSettings as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error('offline'));

    renderSection();

    expect(await screen.findByText('Use global default (LLDP + CDP)')).toBeInTheDocument();
  });

  it('notifies the parent when topology discovery mode changes', () => {
    const onTopologyDiscoveryModeChange = vi.fn();
    renderSection({ onTopologyDiscoveryModeChange });

    fireEvent.change(screen.getByLabelText('Topology Discovery'), {
      target: { value: 'bootstrap_once' },
    });

    expect(onTopologyDiscoveryModeChange).toHaveBeenCalledWith('bootstrap_once');
  });

  it('runs manual discovery and resets feedback when resetKey changes', async () => {
    const { runTopologyDiscovery } = await import('../../api/client');
    const { rerender } = render(
      <DeviceTopologyDiscoverySection
        device={mockDevice()}
        topologyDiscoveryMode="off"
        metricsMode="snmp"
        ip="10.0.0.1"
        resetKey="dev-1"
        onTopologyDiscoveryModeChange={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByText('Run Topology Discovery Now'));

    await waitFor(() => {
      expect(runTopologyDiscovery).toHaveBeenCalledWith('dev-1');
    });
    expect(
      await screen.findByText(
        'Topology discovery started. Links and ports will refresh when the SNMP pass completes.',
      ),
    ).toBeInTheDocument();

    rerender(
      <DeviceTopologyDiscoverySection
        device={mockDevice({ hostname: 'router-02' })}
        topologyDiscoveryMode="off"
        metricsMode="snmp"
        ip="10.0.0.1"
        resetKey="dev-1-router-02"
        onTopologyDiscoveryModeChange={vi.fn()}
      />,
    );

    expect(
      screen.queryByText(
        'Topology discovery started. Links and ports will refresh when the SNMP pass completes.',
      ),
    ).not.toBeInTheDocument();
  });

  it('ignores stale manual discovery success after resetKey changes', async () => {
    const { runTopologyDiscovery } = await import('../../api/client');
    const discovery = controlledPromise();
    (runTopologyDiscovery as ReturnType<typeof vi.fn>).mockReturnValueOnce(discovery.promise);

    const { rerender } = renderSection({
      device: mockDevice(),
      topologyDiscoveryMode: 'off',
      resetKey: 'dev-1',
    });

    fireEvent.click(screen.getByText('Run Topology Discovery Now'));

    await waitFor(() => {
      expect(runTopologyDiscovery).toHaveBeenCalledWith('dev-1');
    });
    expect(screen.getByText('Topology discovery running...')).toBeDisabled();

    rerender(
      <DeviceTopologyDiscoverySection
        device={mockDevice()}
        topologyDiscoveryMode="off"
        metricsMode="snmp"
        ip="10.0.0.1"
        resetKey="dev-1-reset"
        onTopologyDiscoveryModeChange={vi.fn()}
      />,
    );

    expect(screen.getByText('Run Topology Discovery Now')).not.toBeDisabled();

    await act(async () => {
      discovery.resolve();
      await discovery.promise;
    });

    expect(
      screen.queryByText(
        'Topology discovery started. Links and ports will refresh when the SNMP pass completes.',
      ),
    ).not.toBeInTheDocument();
    expect(screen.getByText('Run Topology Discovery Now')).not.toBeDisabled();
  });

  it('ignores stale manual discovery errors after a virtual-hidden transition', async () => {
    const { runTopologyDiscovery } = await import('../../api/client');
    const discovery = controlledPromise();
    (runTopologyDiscovery as ReturnType<typeof vi.fn>).mockReturnValueOnce(discovery.promise);

    const { rerender } = renderSection({
      device: mockDevice(),
      topologyDiscoveryMode: 'off',
      resetKey: 'dev-1',
    });

    fireEvent.click(screen.getByText('Run Topology Discovery Now'));

    await waitFor(() => {
      expect(runTopologyDiscovery).toHaveBeenCalledWith('dev-1');
    });

    rerender(
      <DeviceTopologyDiscoverySection
        device={mockDevice()}
        topologyDiscoveryMode="off"
        metricsMode="snmp"
        ip="10.0.0.1"
        resetKey="dev-1"
        isVirtual
        onTopologyDiscoveryModeChange={vi.fn()}
      />,
    );

    await act(async () => {
      discovery.reject(new ValidationError('Topology discovery is stale'));
      await discovery.promise.catch(() => undefined);
    });

    rerender(
      <DeviceTopologyDiscoverySection
        device={mockDevice()}
        topologyDiscoveryMode="off"
        metricsMode="snmp"
        ip="10.0.0.1"
        resetKey="dev-1"
        onTopologyDiscoveryModeChange={vi.fn()}
      />,
    );

    expect(screen.queryByText('Topology discovery is stale')).not.toBeInTheDocument();
    expect(screen.getByText('Run Topology Discovery Now')).not.toBeDisabled();
  });

  it('shows typed and fallback manual discovery errors', async () => {
    const { runTopologyDiscovery } = await import('../../api/client');
    (runTopologyDiscovery as ReturnType<typeof vi.fn>)
      .mockRejectedValueOnce(new ValidationError('Topology discovery is disabled'))
      .mockRejectedValueOnce(new ServerError('internal error, ref: topo001', 'topo001'))
      .mockRejectedValueOnce('boom');

    renderSection();

    fireEvent.click(screen.getByText('Run Topology Discovery Now'));
    expect(await screen.findByText('Topology discovery is disabled')).toBeInTheDocument();

    fireEvent.click(screen.getByText('Run Topology Discovery Now'));
    expect(await screen.findByText('internal error, ref: topo001')).toBeInTheDocument();

    fireEvent.click(screen.getByText('Run Topology Discovery Now'));
    expect(await screen.findByText('Failed to start topology discovery.')).toBeInTheDocument();
  });

  it('disables manual discovery and renders matching explanatory text by state', () => {
    const { rerender } = renderSection({ metricsMode: 'prometheus' });

    expect(screen.getByText('Run Topology Discovery Now')).toBeDisabled();
    expect(
      screen.getByText(
        'Prometheus-only devices cannot run SNMP topology discovery until SNMP or fallback mode is enabled.',
      ),
    ).toBeInTheDocument();

    rerender(
      <DeviceTopologyDiscoverySection
        device={mockDevice()}
        topologyDiscoveryMode="inherit"
        metricsMode="snmp"
        ip="   "
        resetKey="blank-ip"
        onTopologyDiscoveryModeChange={vi.fn()}
      />,
    );

    expect(screen.getByText('Run Topology Discovery Now')).toBeDisabled();
    expect(screen.getByText('Topology discovery requires a device IP.')).toBeInTheDocument();

    rerender(
      <DeviceTopologyDiscoverySection
        device={mockDevice({ topology_bootstrap_state: 'pending' })}
        topologyDiscoveryMode="inherit"
        metricsMode="snmp"
        ip="10.0.0.1"
        resetKey="pending"
        onTopologyDiscoveryModeChange={vi.fn()}
      />,
    );

    expect(screen.getByText('Topology discovery running...')).toBeDisabled();
    expect(
      screen.getByText(
        'Bootstrap once opens a short discovery window, may queue one follow-up to fill missing ports, then returns the device to Off.',
      ),
    ).toBeInTheDocument();
  });
});
