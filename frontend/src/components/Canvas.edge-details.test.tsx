import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import type React from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { Device, Link } from '../types/api';
import Canvas from './Canvas';

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
}));

const apiMocks = vi.hoisted(() => ({
  fetchSettings: vi.fn(),
}));

vi.mock('@xyflow/react', async () => {
  const ReactModule = await import('react');

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
  useCanvasData: () => ({
    devices: testState.devices,
    setDevices: vi.fn(),
    topologyLinks: [],
    loading: false,
    error: null,
    loadTopology: vi.fn().mockResolvedValue(undefined),
    runtimeSummary: { alertCount: 0, prometheusDiagnosticsVisible: false },
    grafanaUrlRef: { current: '' },
    deviceGrafanaUrlsRef: { current: new Map<string, string>() },
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

describe('Canvas link details edge clicks', () => {
  beforeEach(() => {
    apiMocks.fetchSettings.mockReset();
    apiMocks.fetchSettings.mockResolvedValue({});
  });

  it('opens link details when an edge is clicked in view mode', () => {
    render(
      <Canvas
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
});
