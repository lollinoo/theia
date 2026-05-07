import { act, render, screen, waitFor } from '@testing-library/react';
import { useEffect } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import App from './App';
import type { Area, CanvasMap, Device, Link } from './types/api';
import type { SnapshotPayload } from './types/metrics';

const fetchAreasMock = vi.fn<() => Promise<Area[]>>();
const fetchCanvasMapsMock = vi.fn<() => Promise<CanvasMap[]>>();
const useWebSocketMock = vi.fn();
const watermarkPropsMock = vi.hoisted(() => vi.fn());

vi.mock('./api/client', () => ({
  fetchAreas: () => fetchAreasMock(),
  fetchCanvasMaps: () => fetchCanvasMapsMock(),
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
  Watermark: (props: { hidden?: boolean }) => {
    watermarkPropsMock(props);
    return <div data-testid="watermark" data-hidden={String(props.hidden ?? false)} />;
  },
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
    onInteractionActiveChange,
  }: {
    onDevicesChange: (devices: Device[]) => void;
    onLinksChange: (links: Link[]) => void;
    onInteractionActiveChange: (active: boolean) => void;
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

    return (
      <div data-testid="canvas">
        <button type="button" onClick={() => onInteractionActiveChange(true)}>
          Start interaction
        </button>
        <button type="button" onClick={() => onInteractionActiveChange(false)}>
          End interaction
        </button>
      </div>
    );
  },
}));

vi.mock('./components/topology-hub/TopologyHub', () => ({
  default: ({
    devices,
    links,
    snapshot,
    maps,
    mapsLoading,
    mapsError,
    savedMapsEnabled,
  }: {
    devices: Device[];
    links: Link[];
    snapshot: SnapshotPayload | null;
    maps: CanvasMap[];
    mapsLoading: boolean;
    mapsError: string | null;
    savedMapsEnabled: boolean;
  }) => (
    <div data-testid="topology-hub">
      <span>{`devices:${devices.length}`}</span>
      <span>{`links:${links.length}`}</span>
      <span>{`snapshot:${snapshot?.devices['dev-1']?.status ?? 'none'}`}</span>
      <span>{`maps:${maps.length}`}</span>
      <span>{`loading:${String(mapsLoading)}`}</span>
      <span>{`error:${mapsError ?? 'none'}`}</span>
      <span>{`savedMapsEnabled:${String(savedMapsEnabled)}`}</span>
    </div>
  ),
}));

vi.mock('./components/Dashboard', () => ({
  Dashboard: ({ devices, snapshot }: { devices: Device[]; snapshot: SnapshotPayload | null }) => (
    <div data-testid="dashboard">
      <span>{`devices:${devices.length}`}</span>
      <span>{`status:${snapshot?.devices['dev-1']?.status ?? 'none'}`}</span>
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
    fetchCanvasMapsMock.mockReset();
    useWebSocketMock.mockReset();
    watermarkPropsMock.mockClear();
    fetchAreasMock.mockResolvedValue([mockArea()]);
    fetchCanvasMapsMock.mockResolvedValue([]);
    useWebSocketMock.mockReturnValue({
      snapshot: {
        devices: { 'dev-1': { status: 'down' } },
        links: {},
      } as unknown as SnapshotPayload,
      alerts: [],
      reconnecting: false,
      prometheusStatus: null,
    });
  });

  it('wires canvas devices links and snapshot into TopologyHub and Dashboard', async () => {
    render(<App />);

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());

    screen.getByRole('button', { name: 'Hub' }).click();
    expect(await screen.findByTestId('topology-hub')).toHaveTextContent('devices:1');
    expect(screen.getByTestId('topology-hub')).toHaveTextContent('links:1');
    expect(screen.getByTestId('topology-hub')).toHaveTextContent('snapshot:down');
    expect(screen.getByTestId('topology-hub')).toHaveTextContent('maps:0');
    expect(screen.getByTestId('topology-hub')).toHaveTextContent('savedMapsEnabled:false');
    expect(fetchCanvasMapsMock).not.toHaveBeenCalled();

    screen.getByRole('button', { name: 'Dashboard' }).click();
    expect(await screen.findByTestId('dashboard')).toHaveTextContent('devices:1');
    expect(screen.getByTestId('dashboard')).toHaveTextContent('status:down');
  });

  it('keeps websocket runtime updates paused briefly after canvas interaction ends', async () => {
    render(<App />);

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());
    expect(useWebSocketMock).toHaveBeenLastCalledWith(
      '/api/v1/ws',
      null,
      expect.objectContaining({ runtimeUpdatesPaused: false }),
    );

    vi.useFakeTimers();
    try {
      act(() => {
        screen.getByRole('button', { name: 'Start interaction' }).click();
      });
      expect(useWebSocketMock).toHaveBeenLastCalledWith(
        '/api/v1/ws',
        null,
        expect.objectContaining({ runtimeUpdatesPaused: true }),
      );

      act(() => {
        screen.getByRole('button', { name: 'End interaction' }).click();
      });
      expect(useWebSocketMock).toHaveBeenLastCalledWith(
        '/api/v1/ws',
        null,
        expect.objectContaining({ runtimeUpdatesPaused: true }),
      );

      act(() => {
        vi.advanceTimersByTime(1499);
      });
      expect(useWebSocketMock).toHaveBeenLastCalledWith(
        '/api/v1/ws',
        null,
        expect.objectContaining({ runtimeUpdatesPaused: true }),
      );

      act(() => {
        vi.advanceTimersByTime(1);
      });
      expect(useWebSocketMock).toHaveBeenLastCalledWith(
        '/api/v1/ws',
        null,
        expect.objectContaining({ runtimeUpdatesPaused: false }),
      );
    } finally {
      vi.useRealTimers();
    }
  });

  it('keeps the canvas watermark visible while canvas interaction pauses runtime updates', async () => {
    render(<App />);

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());

    act(() => {
      screen.getByRole('button', { name: 'Start interaction' }).click();
    });

    expect(screen.getByTestId('watermark')).toHaveAttribute('data-hidden', 'false');
    expect(watermarkPropsMock.mock.lastCall?.[0]).not.toHaveProperty('hidden');
  });

  it('anchors the canvas watermark inside the canvas viewport wrapper', async () => {
    render(<App />);

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());

    const canvasViewport = screen.getByTestId('watermark').parentElement;
    expect(canvasViewport?.className).toContain('relative');
    expect(canvasViewport?.className).toContain('h-full');
  });
});
