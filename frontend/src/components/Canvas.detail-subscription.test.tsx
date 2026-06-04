import { fireEvent, render, screen } from '@testing-library/react';
import type React from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { Device } from '../types/api';
import Canvas from './Canvas';

const defaultCanvasProps = {
  mapId: null,
  mapName: 'Default',
  maps: [],
  onMapSelect: vi.fn(),
  onManageMaps: vi.fn(),
};

const testState = vi.hoisted(() => ({
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
  ] satisfies Device[],
}));

const apiMocks = vi.hoisted(() => ({
  createBridgeLaunchRequest: vi.fn(),
  fetchSettings: vi.fn(),
  fetchUserSettings: vi.fn(),
}));

const xyflowMocks = vi.hoisted(() => ({
  MiniMap: vi.fn(() => null),
}));

vi.mock('@xyflow/react', async () => {
  const ReactModule = await import('react');

  return {
    ConnectionMode: { Loose: 'loose' },
    SelectionMode: { Partial: 'partial' },
    Background: () => null,
    MiniMap: xyflowMocks.MiniMap,
    ReactFlow: ({
      children,
      onNodeClick,
      onEdgeClick,
      onPaneClick,
    }: {
      children: React.ReactNode;
      onNodeClick?: (event: unknown, node: { id: string; data: Record<string, never> }) => void;
      onEdgeClick?: (event: unknown, edge: { id: string; data: Record<string, unknown> }) => void;
      onPaneClick?: () => void;
    }) => (
      <div>
        <button type="button" onClick={() => onNodeClick?.({}, { id: 'dev-1', data: {} })}>
          Trigger node click
        </button>
        <button
          type="button"
          onClick={() =>
            onEdgeClick?.(
              {},
              {
                id: 'link-1',
                data: {
                  link: {
                    id: 'link-1',
                    source_device_id: 'dev-1',
                    target_device_id: 'dev-2',
                    source_if_name: 'ether1',
                    target_if_name: 'ether1',
                    discovery_protocol: 'lldp',
                  },
                },
              },
            )
          }
        >
          Trigger edge click
        </button>
        <button type="button" onClick={() => onPaneClick?.()}>
          Trigger pane click
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
      screenToFlowPosition: ({ x, y }: { x: number; y: number }) => ({ x, y }),
    }),
    useNodesInitialized: () => true,
    useStore: <T,>(selector: (state: { width: number; height: number }) => T) =>
      selector({ width: 1200, height: 800 }),
  };
});

vi.mock('./DeviceCard', () => ({ default: () => null }));
vi.mock('./LinkEdge', () => ({ default: () => null }));
vi.mock('./SearchOverlay', () => ({ default: () => null }));
vi.mock('./ZoomControls', () => ({ default: () => null }));
vi.mock('./ContextMenu', () => ({ ContextMenu: () => null }));
vi.mock('./SidePanel', () => ({
  SidePanel: ({ children, open }: { children: React.ReactNode; open: boolean }) =>
    open ? <div>{children}</div> : null,
}));
vi.mock('./ShortcutHelp', () => ({ ShortcutHelp: () => null }));
vi.mock('./MapSelector', () => ({ MapSelector: () => null }));
vi.mock('./Toolbar', () => ({
  Toolbar: ({ onAlerts }: { onAlerts: () => void }) => (
    <button type="button" onClick={onAlerts}>
      Open alerts
    </button>
  ),
}));
vi.mock('./canvas/CanvasPanels', () => ({
  CanvasPanels: ({ panelContent }: { panelContent: { type: string } | null }) => (
    <div data-testid="panel-state">{panelContent?.type ?? 'none'}</div>
  ),
}));
vi.mock('./canvas/CanvasOverlays', () => ({ CanvasOverlays: () => null }));
vi.mock('../hooks/useKeyboardShortcuts', () => ({ useKeyboardShortcuts: () => undefined }));
vi.mock('../hooks/usePositions', () => ({ usePositions: () => ({ savePositions: vi.fn() }) }));
vi.mock('../contexts/ThemeContext', () => ({
  useTheme: () => ({ resolvedTheme: 'dark' as const }),
  adaptAreaColor: (color: string) => color,
}));
vi.mock('../hooks/useBridgeHealth', () => ({
  useBridgeHealth: () => ({
    bridgeRunning: false,
    bridgeChecked: false,
    bridgeError: null,
    checkBridgeHealth: vi.fn(),
  }),
}));
vi.mock('../hooks/useDeviceWinboxAvailability', () => ({
  useDeviceWinboxAvailability: () => ({
    deviceWinboxState: {},
    refreshDeviceWinboxAvailability: vi.fn(),
    setDeviceWinboxAvailability: vi.fn(),
  }),
}));
vi.mock('./canvas/useCanvasData', () => ({
  useCanvasData: () => ({
    devices: testState.devices,
    setDevices: vi.fn(),
    topologyLinks: [],
    loading: false,
    error: null,
    loadTopology: vi.fn().mockResolvedValue(undefined),
    runtimeSummary: { alertCount: 0, prometheusDiagnosticsVisible: false },
    grafanaUrlRef: { current: '' },
    grafanaDashboardConfigRef: { current: null },
    refreshSettings: vi.fn(),
    topologyRecoveryNotice: null,
    dismissTopologyRecoveryNotice: vi.fn(),
    retryTopologyRefresh: vi.fn(),
  }),
}));
vi.mock('./canvas/useAreaFilteredTopology', () => ({
  useAreaFilteredTopology: (devices: Device[]) => ({
    filteredDevices: devices,
    filteredLinks: [],
    ghostDevices: [],
  }),
}));
vi.mock('../api/client', () => apiMocks);

describe('Canvas detail subscription', () => {
  beforeEach(() => {
    apiMocks.fetchSettings.mockReset();
    apiMocks.fetchSettings.mockImplementation(() => new Promise<never>(() => {}));
    apiMocks.fetchUserSettings.mockReset();
    apiMocks.fetchUserSettings.mockImplementation(() => new Promise<never>(() => {}));
    apiMocks.createBridgeLaunchRequest.mockReset();
    xyflowMocks.MiniMap.mockClear();
  });

  it('does not emit transient nulls when panel detail target changes', () => {
    const onDetailDeviceChange = vi.fn();

    render(
      <Canvas
        {...defaultCanvasProps}
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        areas={[]}
        onDetailDeviceChange={onDetailDeviceChange}
      />,
    );

    onDetailDeviceChange.mockClear();

    fireEvent.click(screen.getByRole('button', { name: 'Trigger node click' }));
    expect(onDetailDeviceChange.mock.calls).toEqual([['dev-1']]);
    expect(screen.getByTestId('panel-state')).toHaveTextContent('deviceDetails');

    onDetailDeviceChange.mockClear();

    fireEvent.click(screen.getByRole('button', { name: 'Open alerts' }));
    expect(onDetailDeviceChange.mock.calls).toEqual([[null]]);
  });

  it('opens node and link detail panels while canvas chrome is hidden', () => {
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

    fireEvent.click(screen.getByRole('button', { name: 'Trigger node click' }));
    expect(screen.getByTestId('panel-state')).toHaveTextContent('deviceDetails');

    fireEvent.click(screen.getByRole('button', { name: 'Trigger edge click' }));
    expect(screen.getByTestId('panel-state')).toHaveTextContent('link-details');
  });

  it('does not re-render the minimap for runtime-only snapshot prop changes', () => {
    const { rerender } = render(
      <Canvas
        {...defaultCanvasProps}
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        areas={[]}
      />,
    );

    expect(xyflowMocks.MiniMap).toHaveBeenCalledTimes(1);

    rerender(
      <Canvas
        {...defaultCanvasProps}
        snapshot={{ devices: {}, links: {} }}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        areas={[]}
      />,
    );

    expect(xyflowMocks.MiniMap).toHaveBeenCalledTimes(1);
  });

  it('passes responsive placement classes to the minimap without changing its behavior props', () => {
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

    const minimapProps = xyflowMocks.MiniMap.mock.calls[0]?.[0];
    expect(minimapProps.className).toContain('!m-0');
    expect(minimapProps.className).toContain('!bottom-[calc(6rem+env(safe-area-inset-bottom))]');
    expect(minimapProps.className).toContain('sm:!bottom-4');
    expect(minimapProps.className).toContain('!right-4');
    expect(minimapProps.className).not.toContain('!right-0');
    expect(minimapProps.className).not.toContain('!bottom-4 !right-4');
    expect(minimapProps.className).not.toContain('!right-5');
    expect(minimapProps.pannable).toBe(true);
    expect(minimapProps.zoomable).toBe(true);
    expect(minimapProps.nodeColor).toBeInstanceOf(Function);
    expect(minimapProps.maskColor).toBe('var(--nt-minimap-mask, rgba(45, 45, 61, 0.55))');
    expect(minimapProps.style).toMatchObject({
      backgroundColor: 'var(--nt-surface-container)',
      border: '1px solid var(--nt-outline)',
      borderRadius: 16,
    });
  });
});
