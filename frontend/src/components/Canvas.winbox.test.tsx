/**
 * Exercises canvas Winbox component behavior so refactors preserve the documented contract.
 */
import { fireEvent, render, screen } from '@testing-library/react';
import type React from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Device } from '../types/api';
import { BRIDGE_HEALTH_TIMEOUT_MESSAGE } from '../utils/bridgeRequests';
import Canvas from './Canvas';

const defaultCanvasProps = {
  mapId: null,
  mapName: 'Default',
  maps: [],
  onMapSelect: vi.fn(),
  onManageMaps: vi.fn(),
};

const testState = vi.hoisted(() => ({
  openDeviceMenu: null as null | ((event: unknown, deviceId: string) => void),
  bridgeChecked: false,
  bridgeRunning: true,
  deviceWinboxState: {} as Record<string, boolean>,
  winboxError: null as string | null,
  openDeviceMenuFlow: vi.fn(),
  launchWinbox: vi.fn(),
  clearWinboxError: vi.fn(),
  setDeviceWinboxAvailability: vi.fn(),
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

vi.mock('@xyflow/react', async () => {
  return {
    ConnectionMode: { Loose: 'loose' },
    SelectionMode: { Partial: 'partial' },
    Background: () => null,
    MiniMap: () => null,
    ReactFlow: ({ children }: { children: React.ReactNode }) => (
      <div>
        <button type="button" onClick={(event) => testState.openDeviceMenu?.(event, 'dev-1')}>
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
    useNodesInitialized: () => true,
    useStore: <T,>(selector: (state: { width: number; height: number }) => T) =>
      selector({ width: 1200, height: 800 }),
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

vi.mock('./MapSelector', () => ({
  MapSelector: () => null,
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

vi.mock('../hooks/useWinboxFlow', () => ({
  useWinboxFlow: () => ({
    bridgeChecked: testState.bridgeChecked,
    bridgeRunning: testState.bridgeRunning,
    deviceWinboxState: testState.deviceWinboxState,
    winboxError: testState.winboxError,
    openDeviceMenu: testState.openDeviceMenuFlow,
    launchWinbox: testState.launchWinbox,
    clearWinboxError: testState.clearWinboxError,
    setDeviceWinboxAvailability: testState.setDeviceWinboxAvailability,
  }),
}));

vi.mock('./canvas/useCanvasMenus', async () => {
  const ReactModule = await import('react');

  return {
    useCanvasMenus: () => {
      const [deviceMenu, setDeviceMenu] = ReactModule.useState<null | {
        deviceId: string;
        x: number;
        y: number;
      }>(null);
      const [edgeMenu, setEdgeMenu] = ReactModule.useState<null | {
        edgeID: string;
        x: number;
        y: number;
      }>(null);
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
  useCanvasData: ({
    openDeviceMenu,
  }: {
    openDeviceMenu: (event: unknown, deviceId: string) => void;
  }) => {
    testState.openDeviceMenu = openDeviceMenu;
    return {
      devices: testState.devices,
      setDevices: vi.fn(),
      topologyLinks: [],
      loading: false,
      error: null,
      loadTopology: vi.fn().mockResolvedValue(undefined),
      requestNewNodePlacement: vi.fn().mockResolvedValue(undefined),
      runtimeSummary: { alertCount: 0, prometheusDiagnosticsVisible: false },
      grafanaUrlRef: { current: '' },
      grafanaDashboardConfigRef: { current: null },
      refreshSettings: vi.fn(),
      topologyRecoveryNotice: null,
      dismissTopologyRecoveryNotice: vi.fn(),
      retryTopologyRefresh: vi.fn(),
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

describe('Canvas WinBox gating', () => {
  beforeEach(() => {
    testState.openDeviceMenu = null;
    testState.bridgeChecked = false;
    testState.bridgeRunning = true;
    testState.deviceWinboxState = {};
    testState.winboxError = null;
    testState.openDeviceMenuFlow.mockReset();
    testState.launchWinbox.mockReset();
    testState.clearWinboxError.mockReset();
    testState.setDeviceWinboxAvailability.mockReset();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('refreshes WinBox flow state whenever the device menu opens', () => {
    render(
      <Canvas
        {...defaultCanvasProps}
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        areas={[]}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Open device menu' }));
    fireEvent.click(screen.getByRole('button', { name: 'Open device menu' }));

    expect(testState.openDeviceMenuFlow).toHaveBeenCalledTimes(2);
    expect(testState.openDeviceMenuFlow).toHaveBeenNthCalledWith(1, 'dev-1');
    expect(testState.openDeviceMenuFlow).toHaveBeenNthCalledWith(2, 'dev-1');
  });

  it('does not keep WinBox disabled while profile availability is still unknown', async () => {
    render(
      <Canvas
        {...defaultCanvasProps}
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        areas={[]}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Open device menu' }));

    expect(await screen.findByRole('button', { name: 'Open in WinBox' })).not.toBeDisabled();
  });

  it('keeps the node context menu visible while canvas chrome is hidden', async () => {
    render(
      <Canvas
        {...defaultCanvasProps}
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        areas={[]}
        chromeHidden
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Open device menu' }));

    expect(await screen.findByRole('button', { name: 'Open in WinBox' })).toBeInTheDocument();
  });

  it('keeps WinBox enabled when the bridge health check reports unavailable', async () => {
    testState.bridgeChecked = true;
    testState.bridgeRunning = false;
    testState.deviceWinboxState = { 'dev-1': true };

    render(
      <Canvas
        {...defaultCanvasProps}
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        areas={[]}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Open device menu' }));

    expect(await screen.findByRole('button', { name: 'Open in WinBox' })).not.toBeDisabled();
    expect(screen.getByRole('button', { name: 'Open in WinBox' })).toHaveAttribute(
      'title',
      'WinBox bridge appears unavailable - click to try launch anyway',
    );
  });

  it('shows a toast when the flow reports a bridge health timeout', async () => {
    testState.winboxError = BRIDGE_HEALTH_TIMEOUT_MESSAGE;

    render(
      <Canvas
        {...defaultCanvasProps}
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        areas={[]}
      />,
    );

    expect(await screen.findByText(BRIDGE_HEALTH_TIMEOUT_MESSAGE)).toBeInTheDocument();
    const toast = screen.getByTestId('winbox-error-toast');
    expect(toast.className).toContain('top-20');
    expect(toast.className).toContain('bottom-auto');
    expect(toast.className).toContain('sm:bottom-16');
    expect(toast.className).toContain('sm:top-auto');
  });

  it('keeps the WinBox error toast visible while canvas chrome is hidden', async () => {
    testState.winboxError = BRIDGE_HEALTH_TIMEOUT_MESSAGE;

    render(
      <Canvas
        {...defaultCanvasProps}
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        areas={[]}
        chromeHidden
      />,
    );

    expect(await screen.findByTestId('winbox-error-toast')).toHaveTextContent(
      BRIDGE_HEALTH_TIMEOUT_MESSAGE,
    );
  });

  it('launches WinBox through the flow hook from the menu action', async () => {
    testState.deviceWinboxState = { 'dev-1': true };

    render(
      <Canvas
        {...defaultCanvasProps}
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        areas={[]}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Open device menu' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Open in WinBox' }));

    expect(testState.launchWinbox).toHaveBeenCalledWith('dev-1');
  });
});
