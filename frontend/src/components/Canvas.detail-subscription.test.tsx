import type React from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import Canvas from './Canvas';
import type { Device } from '../types/api';

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
      onNodeClick,
      onPaneClick,
    }: {
      children: React.ReactNode;
      onNodeClick?: (event: unknown, node: { id: string; data: Record<string, never> }) => void;
      onPaneClick?: () => void;
    }) => (
      <div>
        <button
          type="button"
          onClick={() => onNodeClick?.({}, { id: 'dev-1', data: {} })}
        >
          Trigger node click
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
vi.mock('./Toolbar', () => ({
  Toolbar: ({ onAlerts }: { onAlerts: () => void }) => (
    <button type="button" onClick={onAlerts}>Open alerts</button>
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
    runtimeSummary: { alertCount: 0, prometheusDown: false },
    grafanaUrlRef: { current: '' },
    deviceGrafanaUrlsRef: { current: new Map<string, string>() },
    refreshSettings: vi.fn(),
    prometheusAlertDismissed: false,
    setPrometheusAlertDismissed: vi.fn(),
    showRecoveryToast: false,
    setShowRecoveryToast: vi.fn(),
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
    apiMocks.fetchSettings.mockResolvedValue({});
  });

  it('does not emit transient nulls when panel detail target changes', () => {
    const onDetailDeviceChange = vi.fn();

    render(
      <Canvas
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

    onDetailDeviceChange.mockClear();

    fireEvent.click(screen.getByRole('button', { name: 'Open alerts' }));
    expect(onDetailDeviceChange.mock.calls).toEqual([[null]]);
  });
});
