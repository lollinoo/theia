/**
 * Exercises canvas edge details component behavior so refactors preserve the documented contract.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import type React from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { Device, Link, LinkRoute } from '../types/api';
import Canvas from './Canvas';

const defaultCanvasProps = {
  mapId: null,
  mapName: 'Default',
  maps: [],
  onMapSelect: vi.fn(),
  onManageMaps: vi.fn(),
};

const testState = vi.hoisted(() => ({
  link: {
    id: 'link-1',
    source_device_id: 'dev-1',
    source_if_name: 'ether1',
    target_device_id: 'dev-2',
    target_if_name: 'ether2',
    discovery_protocol: 'lldp',
    source_if_speed: 1_000_000_000,
    source_if_oper_status: 'up',
    target_if_speed: 1_000_000_000,
    target_if_oper_status: 'up',
  } satisfies Link,
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
    {
      id: 'dev-2',
      hostname: 'switch-01',
      ip: '10.0.0.2',
      device_type: 'switch',
      poll_class: 'core',
      poll_interval_override: null,
      status: 'up',
      sys_name: 'switch-01',
      sys_descr: 'RouterOS',
      hardware_model: 'CRS326',
      vendor: 'mikrotik',
      managed: true,
      interfaces: [],
      backup_supported: true,
      metrics_source: 'snmp',
      prometheus_label_name: 'instance',
      prometheus_label_value: '10.0.0.2:9100',
      area_ids: [],
    },
  ] satisfies Device[],
  canvasDataParams: null as {
    onLinkRouteCommit?: (edgeId: string, route: LinkRoute | null) => void;
  } | null,
}));

const apiMocks = vi.hoisted(() => ({
  createBridgeLaunchRequest: vi.fn(),
  deleteCanvasMapLinkRoute: vi.fn(),
  fetchSettings: vi.fn(),
  fetchUserSettings: vi.fn(),
  saveCanvasMapLinkRoute: vi.fn(),
}));

vi.mock('@xyflow/react', async () => {
  return {
    ConnectionMode: { Loose: 'loose' },
    SelectionMode: { Partial: 'partial' },
    Background: () => null,
    MiniMap: () => null,
    ReactFlow: ({
      children,
      onEdgeClick,
      onPaneClick,
    }: {
      children: React.ReactNode;
      onEdgeClick?: (event: unknown, edge: unknown) => void;
      onPaneClick?: () => void;
    }) => (
      <div>
        <button
          type="button"
          onClick={() => onEdgeClick?.({}, { id: 'edge-1', data: { link: testState.link } })}
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

vi.mock('./ContextMenu', () => ({
  ContextMenu: () => null,
}));

vi.mock('./SidePanel', () => ({
  SidePanel: ({ children, open }: { children: React.ReactNode; open: boolean }) =>
    open ? <div>{children}</div> : null,
}));

vi.mock('./ShortcutHelp', () => ({
  ShortcutHelp: () => null,
}));

vi.mock('./MapSelector', () => ({
  MapSelector: () => null,
}));

vi.mock('./Toolbar', () => ({
  Toolbar: ({
    editMode,
    onToggleEditMode,
  }: {
    editMode: boolean;
    onToggleEditMode: () => void;
  }) => (
    <button type="button" onClick={onToggleEditMode}>
      {editMode ? 'Disable edit mode' : 'Enable edit mode'}
    </button>
  ),
}));

vi.mock('./canvas/CanvasPanels', () => ({
  CanvasPanels: ({
    panelContent,
    editMode,
  }: {
    panelContent: { type: string; data?: { link?: { id?: string }; readOnly?: boolean } } | null;
    editMode?: boolean;
  }) => (
    <div data-testid="panel-state">
      {panelContent?.type}:{editMode ? 'edit' : 'view'}:{panelContent?.data?.link?.id}
    </div>
  ),
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
  useCanvasData: (params: {
    onLinkRouteCommit?: (edgeId: string, route: LinkRoute | null) => void;
  }) => {
    testState.canvasDataParams = params;
    return {
      devices: testState.devices,
      setDevices: vi.fn(),
      topologyLinks: [],
      topologyAreas: [],
      loading: false,
      error: null,
      renderedMapKey: 'default:',
      loadTopology: vi.fn().mockResolvedValue(undefined),
      requestNewNodePlacement: vi.fn().mockResolvedValue(undefined),
      runtimeSummary: { alertCount: 0, prometheusDiagnosticsVisible: false },
      grafanaUrlRef: { current: '' },
      grafanaDashboardConfigRef: { current: null },
      refreshSettings: vi.fn(),
      topologyRecoveryNotice: null,
      dismissTopologyRecoveryNotice: vi.fn(),
      retryTopologyRefresh: vi.fn(),
      updateNodePosition: vi.fn(),
      snapCurrentNodePositions: vi.fn(),
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

describe('Canvas link details edge clicks', () => {
  beforeEach(() => {
    testState.canvasDataParams = null;
    apiMocks.fetchSettings.mockReset();
    apiMocks.fetchSettings.mockResolvedValue({});
    apiMocks.fetchUserSettings.mockReset();
    apiMocks.fetchUserSettings.mockResolvedValue({ preferences: { bridge_port: 1337 } });
    apiMocks.createBridgeLaunchRequest.mockReset();
    apiMocks.deleteCanvasMapLinkRoute.mockReset();
    apiMocks.saveCanvasMapLinkRoute.mockReset();
  });

  it('opens link details when an edge is clicked in view mode', () => {
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

    fireEvent.click(screen.getByRole('button', { name: 'Trigger edge click' }));

    expect(screen.getByTestId('panel-state')).toHaveTextContent('link-details:view:link-1');
  });

  it('opens link details as editable when edit mode is enabled', async () => {
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

    fireEvent.click(screen.getByRole('button', { name: 'Enable edit mode' }));
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Disable edit mode' })).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole('button', { name: 'Trigger edge click' }));

    expect(screen.getByTestId('panel-state')).toHaveTextContent('link-details:edit:link-1');
  });

  it('updates open link details when edit mode is toggled', async () => {
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

    fireEvent.click(screen.getByRole('button', { name: 'Trigger edge click' }));
    expect(screen.getByTestId('panel-state')).toHaveTextContent('link-details:view:link-1');

    fireEvent.click(screen.getByRole('button', { name: 'Enable edit mode' }));
    await waitFor(() => {
      expect(screen.getByTestId('panel-state')).toHaveTextContent('link-details:edit:link-1');
    });
  });

  it('exposes one stable route commit only while a persisted map ID owns the canvas', () => {
    const props = {
      ...defaultCanvasProps,
      snapshot: null,
      reconnecting: false,
      prometheusStatus: null,
      selectedAreaId: null,
      areas: [],
    };
    const { rerender } = render(<Canvas {...props} mapId={null} />);

    expect(testState.canvasDataParams?.onLinkRouteCommit).toBeUndefined();

    rerender(<Canvas {...props} mapId="map-a" />);
    const savedMapCommit = testState.canvasDataParams?.onLinkRouteCommit;
    expect(savedMapCommit).toEqual(expect.any(Function));

    rerender(<Canvas {...props} mapId="map-b" />);
    expect(testState.canvasDataParams?.onLinkRouteCommit).toBe(savedMapCommit);
  });
});
