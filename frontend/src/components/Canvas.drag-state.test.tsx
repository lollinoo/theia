import { act, fireEvent, render, screen } from '@testing-library/react';
import type React from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { Device, Link } from '../types/api';
import Canvas from './Canvas';
import type { DeviceNode } from './DeviceCard';

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-a',
    hostname: 'router-a',
    ip: '10.0.0.1',
    device_type: 'router',
    poll_class: 'core',
    poll_interval_override: null,
    polling_enabled: true,
    status: 'up',
    sys_name: 'router-a',
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
    ...overrides,
  };
}

function mockNode(device: Device, x: number, y: number): DeviceNode {
  return {
    id: device.id,
    type: 'device',
    position: { x, y },
    data: {
      kind: 'device',
      device,
      pinned: false,
    },
  } as DeviceNode;
}

const testState = vi.hoisted(() => ({
  devices: [] as Device[],
  links: [] as Link[],
  canonicalNodes: [] as DeviceNode[],
  displayedNodes: [] as DeviceNode[],
  setNodes: vi.fn(),
  setEdges: vi.fn(),
  onNodesChange: vi.fn(),
  savePositions: vi.fn(),
  updateNodePosition: vi.fn(),
  reactFlowProps: {} as Record<string, unknown>,
}));

vi.mock('@xyflow/react', () => ({
  ConnectionMode: { Loose: 'loose' },
  SelectionMode: { Partial: 'partial' },
  Background: () => null,
  MiniMap: () => <div data-testid="topology-minimap" />,
  ReactFlow: ({
    children,
    nodes,
    onlyRenderVisibleElements,
    onMoveStart,
    onMoveEnd,
    onMove,
    onConnectStart,
    onConnectEnd,
    onNodeDragStop,
  }: {
    children: React.ReactNode;
    nodes: DeviceNode[];
    onlyRenderVisibleElements?: boolean;
    onMoveStart?: () => void;
    onMoveEnd?: () => void;
    onMove?: (event: unknown, viewport: { zoom: number }) => void;
    onConnectStart?: () => void;
    onConnectEnd?: () => void;
    onNodeDragStop?: (event: unknown, node: DeviceNode) => void;
  }) => {
    testState.displayedNodes = nodes;
    testState.reactFlowProps = {
      onlyRenderVisibleElements,
    };
    const draggedNode = nodes.find((node) => node.id === 'dev-a');
    return (
      <div>
        <button type="button" onClick={() => onMoveStart?.()}>
          Start pan
        </button>
        <button type="button" onClick={() => onMoveEnd?.()}>
          End pan
        </button>
        <button type="button" onClick={() => onMove?.({}, { zoom: 0.6 })}>
          Move low zoom
        </button>
        <button type="button" onClick={() => onConnectStart?.()}>
          Start connect
        </button>
        <button type="button" onClick={() => onConnectEnd?.()}>
          End connect
        </button>
        <button
          type="button"
          onClick={() => {
            if (!draggedNode) return;
            onNodeDragStop?.(
              {},
              {
                ...draggedNode,
                position: { x: 444, y: 555 },
              },
            );
          }}
        >
          Drag area node
        </button>
        {children}
      </div>
    );
  },
  applyEdgeChanges: (_changes: unknown, current: unknown) => current,
  useNodesState: () => [testState.canonicalNodes, testState.setNodes, testState.onNodesChange],
  useReactFlow: () => ({
    fitView: vi.fn(),
    zoomIn: vi.fn(),
    zoomOut: vi.fn(),
    getNodes: () => testState.displayedNodes,
    setCenter: vi.fn(),
    screenToFlowPosition: ({ x, y }: { x: number; y: number }) => ({ x, y }),
  }),
}));

vi.mock('./DeviceCard', () => ({
  default: () => null,
  resolveDeviceNodeReadabilityScale: (zoom: number) => (zoom <= 0.6 ? 1.12 : 1),
}));
vi.mock('./LinkEdge', () => ({ default: () => null }));
vi.mock('./SearchOverlay', () => ({ default: () => null }));
vi.mock('./ZoomControls', () => ({ default: () => null }));
vi.mock('./ContextMenu', () => ({ ContextMenu: () => null }));
vi.mock('./SidePanel', () => ({ SidePanel: () => null }));
vi.mock('./ShortcutHelp', () => ({ ShortcutHelp: () => null }));
vi.mock('./Toolbar', () => ({ Toolbar: () => null }));
vi.mock('./canvas/CanvasPanels', () => ({ CanvasPanels: () => null }));
vi.mock('./canvas/CanvasOverlays', () => ({ CanvasOverlays: () => null }));
vi.mock('./canvas/detailSubscription', () => ({ getCanvasDetailDeviceId: () => null }));
vi.mock('../hooks/useKeyboardShortcuts', () => ({ useKeyboardShortcuts: () => undefined }));
vi.mock('../hooks/usePositions', () => ({
  usePositions: () => ({ savePositions: testState.savePositions }),
}));
vi.mock('../hooks/useWinboxFlow', () => ({
  useWinboxFlow: () => ({
    bridgeChecked: false,
    bridgeRunning: true,
    deviceWinboxState: {},
    winboxError: null,
    openDeviceMenu: vi.fn(),
    launchWinbox: vi.fn(),
    clearWinboxError: vi.fn(),
    setDeviceWinboxAvailability: vi.fn(),
  }),
}));
vi.mock('../contexts/ThemeContext', () => ({
  useTheme: () => ({ resolvedTheme: 'dark' as const }),
  adaptAreaColor: (color: string) => color,
}));
vi.mock('./canvas/useCanvasData', () => ({
  useCanvasData: () => ({
    devices: testState.devices,
    setDevices: vi.fn(),
    topologyLinks: testState.links,
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
    updateNodePosition: testState.updateNodePosition,
  }),
}));

describe('Canvas drag state ownership', () => {
  beforeEach(() => {
    const deviceA = mockDevice({ id: 'dev-a', area_ids: ['area-1'] });
    const deviceB = mockDevice({
      id: 'dev-b',
      hostname: 'router-b',
      ip: '10.0.0.2',
      sys_name: 'router-b',
      area_ids: ['area-3'],
    });
    const deviceC = mockDevice({
      id: 'dev-c',
      hostname: 'router-c',
      ip: '10.0.0.3',
      sys_name: 'router-c',
      area_ids: ['area-2'],
    });

    testState.devices = [deviceA, deviceB, deviceC];
    testState.links = [
      {
        id: 'link-ac',
        source_device_id: 'dev-a',
        source_if_name: 'ether1',
        target_device_id: 'dev-c',
        target_if_name: 'ether1',
        discovery_protocol: 'lldp',
        source_if_speed: 0,
        source_if_oper_status: 'up',
        target_if_speed: 0,
        target_if_oper_status: 'up',
      },
    ];
    testState.canonicalNodes = [
      mockNode(deviceA, 100, 100),
      mockNode(deviceB, 300, 100),
      mockNode(deviceC, 500, 100),
    ];
    testState.displayedNodes = [];
    testState.setNodes.mockReset();
    testState.setEdges.mockReset();
    testState.onNodesChange.mockReset();
    testState.savePositions.mockReset();
    testState.updateNodePosition.mockReset();
    testState.reactFlowProps = {};
  });

  it('patches the dragged real node without replacing canonical nodes with the area projection', () => {
    render(
      <Canvas
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId="area-1"
        areas={[
          { id: 'area-1', name: 'Area 1', color: '#00aaff' },
          { id: 'area-2', name: 'Area 2', color: '#ffaa00' },
          { id: 'area-3', name: 'Area 3', color: '#22cc88' },
        ]}
      />,
    );

    expect(
      testState.displayedNodes.map((node) => `${node.id}:${node.data.isGhost === true}`),
    ).toEqual(['dev-a:false', 'dev-c:true']);

    testState.setNodes.mockClear();
    fireEvent.click(screen.getByRole('button', { name: 'Drag area node' }));

    expect(testState.setNodes).not.toHaveBeenCalled();
    expect(testState.savePositions).not.toHaveBeenCalled();
    expect(testState.updateNodePosition).toHaveBeenCalledWith('dev-a', { x: 444, y: 555 });
  });

  it('keeps visible-element rendering enabled and the minimap visible during canvas gestures', () => {
    vi.useFakeTimers();
    try {
      const onInteractionActiveChange = vi.fn();

      render(
        <Canvas
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          areas={[
            { id: 'area-1', name: 'Area 1', color: '#00aaff' },
            { id: 'area-2', name: 'Area 2', color: '#ffaa00' },
          ]}
          onInteractionActiveChange={onInteractionActiveChange}
        />,
      );

      expect(testState.reactFlowProps.onlyRenderVisibleElements).toBe(true);
      expect(screen.getByTestId('topology-minimap')).toBeInTheDocument();
      onInteractionActiveChange.mockClear();

      fireEvent.click(screen.getByRole('button', { name: 'Start pan' }));
      expect(screen.getByTestId('topology-minimap')).toBeInTheDocument();
      expect(onInteractionActiveChange).toHaveBeenLastCalledWith(true);

      fireEvent.click(screen.getByRole('button', { name: 'End pan' }));
      expect(screen.getByTestId('topology-minimap')).toBeInTheDocument();

      act(() => {
        vi.advanceTimersByTime(180);
      });

      expect(screen.getByTestId('topology-minimap')).toBeInTheDocument();
      expect(onInteractionActiveChange).toHaveBeenLastCalledWith(false);
    } finally {
      vi.useRealTimers();
    }
  });

  it('updates canvas readability scales from viewport changes without React state churn', () => {
    render(
      <Canvas
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        areas={[
          { id: 'area-1', name: 'Area 1', color: '#00aaff' },
          { id: 'area-2', name: 'Area 2', color: '#ffaa00' },
        ]}
      />,
    );

    const root = screen.getByTestId('topology-canvas-root');

    expect(root.style.getPropertyValue('--theia-device-node-readability-scale')).toBe('1');
    expect(root.style.getPropertyValue('--theia-link-badge-readability-scale')).toBe('1');

    fireEvent.click(screen.getByRole('button', { name: 'Move low zoom' }));

    expect(root.style.getPropertyValue('--theia-device-node-readability-scale')).toBe('1.12');
    expect(root.style.getPropertyValue('--theia-link-badge-readability-scale')).toBe('1.2');
  });

  it('preserves unchanged area-colored display node references when one canonical node changes', () => {
    const { rerender } = render(
      <Canvas
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        areas={[
          { id: 'area-1', name: 'Area 1', color: '#00aaff' },
          { id: 'area-2', name: 'Area 2', color: '#ffaa00' },
          { id: 'area-3', name: 'Area 3', color: '#22cc88' },
        ]}
      />,
    );

    const [, firstStableNode, secondStableNode] = testState.displayedNodes;

    testState.canonicalNodes = [
      {
        ...testState.canonicalNodes[0],
        position: { x: 125, y: 125 },
      },
      testState.canonicalNodes[1],
      testState.canonicalNodes[2],
    ];

    rerender(
      <Canvas
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        areas={[
          { id: 'area-1', name: 'Area 1', color: '#00aaff' },
          { id: 'area-2', name: 'Area 2', color: '#ffaa00' },
          { id: 'area-3', name: 'Area 3', color: '#22cc88' },
        ]}
      />,
    );

    expect(testState.displayedNodes[1]).toBe(firstStableNode);
    expect(testState.displayedNodes[2]).toBe(secondStableNode);
  });
});
