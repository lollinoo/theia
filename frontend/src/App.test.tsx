import { render, screen, waitFor } from '@testing-library/react';
import { useEffect } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import App from './App';
import type { Area, Device, Link } from './types/api';
import type { SnapshotPayload } from './types/metrics';

const fetchAreasMock = vi.fn<() => Promise<Area[]>>();
const useWebSocketMock = vi.fn();

vi.mock('./api/client', () => ({
  fetchAreas: () => fetchAreasMock(),
}));

vi.mock('./hooks/useWebSocket', () => ({
  useWebSocket: (...args: unknown[]) => useWebSocketMock(...args),
}));

vi.mock('./contexts/ThemeContext', () => ({
  ThemeProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock('@xyflow/react', () => ({
  ReactFlowProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock('./components/Watermark', () => ({
  Watermark: () => <div data-testid="watermark" />,
}));

vi.mock('./components/NavigationPill', () => ({
  default: ({ onViewChange }: { onViewChange: (view: 'hub' | 'canvas' | 'dashboard') => void }) => (
    <div>
      <button type="button" onClick={() => onViewChange('hub')}>
        Hub
      </button>
      <button type="button" onClick={() => onViewChange('dashboard')}>
        Dashboard
      </button>
    </div>
  ),
}));

vi.mock('./components/Canvas', () => ({
  default: ({
    onDevicesChange,
    onLinksChange,
  }: {
    onDevicesChange: (devices: Device[]) => void;
    onLinksChange: (links: Link[]) => void;
  }) => {
    useEffect(() => {
      onDevicesChange([
        {
          id: 'dev-1',
          hostname: 'router-01',
          ip: '10.0.0.1',
          device_type: 'router',
          poll_class: 'standard',
          poll_interval_override: null,
          status: 'up',
          sys_name: 'router-01',
          sys_descr: 'RouterOS 7.15.1',
          hardware_model: 'RB5009',
          vendor: 'mikrotik',
          managed: true,
          interfaces: [],
          area_ids: ['area-1'],
          backup_supported: true,
          metrics_source: 'snmp',
          prometheus_label_name: 'instance',
          prometheus_label_value: '10.0.0.1:9100',
        },
      ]);
      onLinksChange([
        {
          id: 'link-1',
          source_device_id: 'dev-1',
          source_if_name: 'ether1',
          target_device_id: 'dev-2',
          target_if_name: 'ether2',
          discovery_protocol: 'lldp',
          source_if_speed: 1,
          source_if_oper_status: 'up',
          target_if_speed: 1,
          target_if_oper_status: 'up',
        },
      ]);
    }, [onDevicesChange, onLinksChange]);

    return <div data-testid="canvas" />;
  },
}));

vi.mock('./components/AreaHub', () => ({
  default: ({
    devices,
    links,
    snapshot,
  }: { devices: Device[]; links: Link[]; snapshot?: SnapshotPayload | null }) => (
    <div data-testid="area-hub">
      <span>{`devices:${devices.length}`}</span>
      <span>{`links:${links.length}`}</span>
      <span>{`snapshot:${String(snapshot)}`}</span>
    </div>
  ),
}));

vi.mock('./components/Dashboard', () => ({
  Dashboard: ({ devices, snapshot }: { devices: Device[]; snapshot: SnapshotPayload | null }) => (
    <div data-testid="dashboard">
      <span>{`devices:${devices.length}`}</span>
      <span>{`status:${snapshot?.device_statuses['dev-1'] ?? 'none'}`}</span>
    </div>
  ),
}));

function mockArea(overrides: Partial<Area> = {}): Area {
  return {
    id: 'area-1',
    name: 'Backbone',
    description: 'Core',
    color: '#00E676',
    device_count: 1,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

describe('App', () => {
  beforeEach(() => {
    fetchAreasMock.mockReset();
    useWebSocketMock.mockReset();
    fetchAreasMock.mockResolvedValue([mockArea()]);
    useWebSocketMock.mockReturnValue({
      snapshot: {
        device_metrics: {},
        link_metrics: {},
        device_statuses: { 'dev-1': 'down' },
      } satisfies SnapshotPayload,
      alerts: [],
      reconnecting: false,
      prometheusStatus: null,
    });
  });

  it('wires canvas devices and links into AreaHub and snapshot into Dashboard only', async () => {
    render(<App />);

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());

    screen.getByRole('button', { name: 'Hub' }).click();
    expect(await screen.findByTestId('area-hub')).toHaveTextContent('devices:1');
    expect(screen.getByTestId('area-hub')).toHaveTextContent('links:1');
    expect(screen.getByTestId('area-hub')).toHaveTextContent('snapshot:undefined');

    screen.getByRole('button', { name: 'Dashboard' }).click();
    expect(await screen.findByTestId('dashboard')).toHaveTextContent('devices:1');
    expect(screen.getByTestId('dashboard')).toHaveTextContent('status:down');
  });
});
