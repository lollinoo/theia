import type React from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import Canvas from './Canvas';
import type { Device } from '../types/api';
import {
  BRIDGE_HEALTH_TIMEOUT_MESSAGE,
  BRIDGE_LAUNCH_TIMEOUT_MESSAGE,
  BRIDGE_REQUEST_TIMEOUT_MS,
} from '../utils/bridgeRequests';

const testState = vi.hoisted(() => ({
  openDeviceMenu: null as null | ((event: unknown, deviceId: string) => void),
  bridgeRunningAfterCheck: true,
  bridgeErrorAfterCheck: null as string | null,
  devices: [
    {
      id: 'dev-1',
      hostname: 'router-01',
      ip: '10.0.0.1',
      device_type: 'router',
      poll_class: 'core',
      poll_interval_override: null,
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
      area_ids: [],
    },
  ] as Device[],
}));

const apiMocks = vi.hoisted(() => ({
  fetchDeviceCredentialProfiles: vi.fn(),
  fetchSettings: vi.fn(),
  fetchBridgeToken: vi.fn(),
}));

vi.mock('@xyflow/react', async () => {
  const ReactModule = await import('react');

  return {
    ConnectionMode: { Loose: 'loose' },
    SelectionMode: { Partial: 'partial' },
    Background: () => null,
    MiniMap: () => null,
    ReactFlow: ({ children }: { children: React.ReactNode }) => (
      <div>
        <button
          type="button"
          onClick={(event) => testState.openDeviceMenu?.(event, 'dev-1')}
        >
          Open device menu
        </button>
        {children}
      </div>
    ),
    applyEdgeChanges: (_changes: unknown, current: unknown) => current,
    useNodesState: () => [[], vi.fn(), vi.fn()],
    useReactFlow: () => ({
      fitView: vi.fn(),
      zoomIn: vi.fn(),
      zoomOut: vi.fn(),
      getNodes: () => [],
      setCenter: vi.fn(),
    }),
  };
});

vi.mock('./DeviceCard', () => ({
  default: () => null,
}));

vi.mock('./LinkEdge', () => ({
  default: () => null,
}));

vi.mock('./SearchOverlay', () => ({
  default: () => null,
}));

vi.mock('./ZoomControls', () => ({
  default: () => null,
}));

vi.mock('./SidePanel', () => ({
  SidePanel: ({ children, open }: { children: React.ReactNode; open: boolean }) =>
    open ? <div>{children}</div> : null,
}));

vi.mock('./ShortcutHelp', () => ({
  ShortcutHelp: () => null,
}));

vi.mock('./Toolbar', () => ({
  Toolbar: () => null,
}));

vi.mock('./canvas/CanvasPanels', () => ({
  CanvasPanels: () => null,
}));

vi.mock('./canvas/CanvasOverlays', () => ({
  CanvasOverlays: () => null,
}));

vi.mock('./canvas/detailSubscription', () => ({
  getCanvasDetailDeviceId: () => null,
}));

vi.mock('../hooks/useKeyboardShortcuts', () => ({
  useKeyboardShortcuts: () => undefined,
}));

vi.mock('../hooks/usePositions', () => ({
  usePositions: () => ({ savePositions: vi.fn() }),
}));

vi.mock('../contexts/ThemeContext', () => ({
  useTheme: () => ({ resolvedTheme: 'dark' as const }),
  adaptAreaColor: (color: string) => color,
}));

vi.mock('../hooks/useBridgeHealth', async () => {
  const ReactModule = await import('react');

  return {
    useBridgeHealth: () => {
      const [bridgeRunning, setBridgeRunning] = ReactModule.useState(false);
      const [bridgeChecked, setBridgeChecked] = ReactModule.useState(false);
      const [bridgeError, setBridgeError] = ReactModule.useState<string | null>(null);
      return {
        bridgeRunning,
        bridgeChecked,
        bridgeError,
        checkBridgeHealth: () => {
          setBridgeRunning(testState.bridgeRunningAfterCheck);
          setBridgeChecked(true);
          setBridgeError(testState.bridgeErrorAfterCheck);
        },
      };
    },
  };
});

vi.mock('./canvas/useCanvasMenus', async () => {
  const ReactModule = await import('react');

  return {
    useCanvasMenus: () => {
      const [deviceMenu, setDeviceMenu] = ReactModule.useState<null | { deviceId: string; x: number; y: number }>(null);
      const [edgeMenu, setEdgeMenu] = ReactModule.useState<null | { edgeID: string; x: number; y: number }>(null);
      const [panelContent, setPanelContent] = ReactModule.useState<null>(null);
      const [showShortcuts, setShowShortcuts] = ReactModule.useState(false);
      const [showSearch, setShowSearch] = ReactModule.useState(false);
      const [editMode, setEditMode] = ReactModule.useState(false);

      return {
        deviceMenu,
        setDeviceMenu,
        edgeMenu,
        setEdgeMenu,
        panelContent,
        setPanelContent,
        showShortcuts,
        setShowShortcuts,
        showSearch,
        setShowSearch,
        editMode,
        setEditMode,
        shortcuts: [],
        getPanelTitle: () => '',
      };
    },
  };
});

vi.mock('./canvas/useCanvasData', () => ({
  useCanvasData: ({ openDeviceMenu }: { openDeviceMenu: (event: unknown, deviceId: string) => void }) => {
    testState.openDeviceMenu = openDeviceMenu;
    return {
      devices: testState.devices,
      setDevices: vi.fn(),
      topologyLinks: [],
      loading: false,
      error: null,
      loadTopology: vi.fn().mockResolvedValue(undefined),
      grafanaUrlRef: { current: '' },
      deviceGrafanaUrlsRef: { current: new Map<string, string>() },
      refreshSettings: vi.fn(),
      prometheusAlertDismissed: false,
      setPrometheusAlertDismissed: vi.fn(),
      showRecoveryToast: false,
      setShowRecoveryToast: vi.fn(),
    };
  },
}));

vi.mock('./canvas/useAreaFilteredTopology', () => ({
  useAreaFilteredTopology: (devices: Device[]) => ({
    filteredDevices: devices,
    filteredLinks: [],
    ghostDevices: [],
  }),
}));

vi.mock('../api/client', () => apiMocks);

describe('Canvas WinBox gating', () => {
  beforeEach(() => {
    testState.openDeviceMenu = null;
    testState.bridgeRunningAfterCheck = true;
    testState.bridgeErrorAfterCheck = null;
    apiMocks.fetchDeviceCredentialProfiles.mockReset();
    apiMocks.fetchSettings.mockReset();
    apiMocks.fetchBridgeToken.mockReset();
    apiMocks.fetchSettings.mockResolvedValue({
      bridge_port: '1337',
      bridge_secret: 'secret',
    });
    vi.stubGlobal('fetch', vi.fn());
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('refreshes a stale false WinBox cache when reopening the same device menu', async () => {
    apiMocks.fetchDeviceCredentialProfiles
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([
        { profile_id: 'p1', name: 'Admin', role: 'Admin', is_winbox: true },
      ]);

    render(
      <Canvas
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        areas={[]}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Open device menu' }));

    await waitFor(() => {
      expect(apiMocks.fetchDeviceCredentialProfiles).toHaveBeenCalledTimes(1);
    });
    expect(await screen.findByRole('button', { name: 'Open in WinBox' })).toBeDisabled();

    fireEvent.click(screen.getByRole('button', { name: 'Open device menu' }));

    await waitFor(() => {
      expect(apiMocks.fetchDeviceCredentialProfiles).toHaveBeenCalledTimes(2);
    });
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Open in WinBox' })).not.toBeDisabled();
    });
  });

  it('does not keep WinBox disabled while profile availability is still unknown', async () => {
    apiMocks.fetchDeviceCredentialProfiles.mockImplementation(
      () => new Promise(() => {}),
    );

    render(
      <Canvas
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        areas={[]}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Open device menu' }));

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Open in WinBox' })).not.toBeDisabled();
    });
  });

  it('keeps WinBox enabled when the bridge health check reports unavailable', async () => {
    testState.bridgeRunningAfterCheck = false;
    apiMocks.fetchDeviceCredentialProfiles.mockResolvedValue([
      { profile_id: 'p1', name: 'Admin', role: 'Admin', is_winbox: true },
    ]);

    render(
      <Canvas
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        areas={[]}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Open device menu' }));

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Open in WinBox' })).not.toBeDisabled();
    });
    expect(screen.getByRole('button', { name: 'Open in WinBox' })).toHaveAttribute(
      'title',
      'WinBox bridge appears unavailable - click to try launch anyway',
    );
  });

  it('shows a toast when the bridge health check times out', async () => {
    testState.bridgeRunningAfterCheck = false;
    testState.bridgeErrorAfterCheck = BRIDGE_HEALTH_TIMEOUT_MESSAGE;
    apiMocks.fetchDeviceCredentialProfiles.mockResolvedValue([
      { profile_id: 'p1', name: 'Admin', role: 'Admin', is_winbox: true },
    ]);

    render(
      <Canvas
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        areas={[]}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Open device menu' }));

    expect(await screen.findByText(BRIDGE_HEALTH_TIMEOUT_MESSAGE)).toBeInTheDocument();
  });

  it('shows a toast when the launch request times out', async () => {
    apiMocks.fetchDeviceCredentialProfiles.mockResolvedValue([
      { profile_id: 'p1', name: 'Admin', role: 'Admin', is_winbox: true },
    ]);
    apiMocks.fetchBridgeToken.mockResolvedValue('bridge-token');
    (global.fetch as ReturnType<typeof vi.fn>).mockImplementation(() => new Promise(() => {}));

    render(
      <Canvas
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        areas={[]}
      />,
    );

    await act(async () => {
      await Promise.resolve();
    });

    fireEvent.click(screen.getByRole('button', { name: 'Open device menu' }));
    const winboxButton = await screen.findByRole('button', { name: 'Open in WinBox' });

    vi.useFakeTimers();
    fireEvent.click(winboxButton);

    await act(async () => { await vi.advanceTimersByTimeAsync(BRIDGE_REQUEST_TIMEOUT_MS); });

    expect(screen.getByText(BRIDGE_LAUNCH_TIMEOUT_MESSAGE)).toBeInTheDocument();
  });
});
