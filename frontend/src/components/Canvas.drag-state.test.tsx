/**
 * Exercises canvas drag state component behavior so refactors preserve the documented contract.
 */
import { act, fireEvent, render, screen } from '@testing-library/react';
import type React from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { CanvasMap, Device, Link, LinkRoute } from '../types/api';
import Canvas from './Canvas';
import type { DeviceNode } from './DeviceCard';
import { FloatingConnectionLine } from './FloatingConnectionLine';

const defaultCanvasProps = {
  mapId: null,
  mapName: 'Default',
  maps: [],
  onMapSelect: vi.fn(),
  onManageMaps: vi.fn(),
};

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

function mockMap(overrides: Partial<CanvasMap> = {}): CanvasMap {
  return {
    id: 'map-backbone',
    name: 'Backbone',
    description: '',
    source_area_id: 'area-1',
    filter: { area_id: 'area-1' },
    is_default: false,
    device_count: 1,
    link_count: 1,
    position_count: 1,
    created_at: '2026-05-07T00:00:00Z',
    updated_at: '2026-05-07T00:00:00Z',
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

type MockNodeChange = {
  id: string;
  type: string;
  dimensions?: { width: number; height: number };
};

const testState = vi.hoisted(() => ({
  devices: [] as Device[],
  links: [] as Link[],
  canonicalNodes: [] as DeviceNode[],
  displayedNodes: [] as DeviceNode[],
  setEdges: vi.fn(),
  deleteCanvasMapLinkRoute: vi.fn(),
  saveCanvasMapLinkRoute: vi.fn(),
  applyNodeChanges: vi.fn((changes: MockNodeChange[], currentNodes: DeviceNode[]) => {
    let nextNodes = currentNodes;
    for (const change of changes) {
      if (change.type === 'remove') {
        nextNodes = nextNodes.filter((node) => node.id !== change.id);
        continue;
      }

      if (change.type === 'dimensions' && change.dimensions) {
        nextNodes = nextNodes.map((node) =>
          node.id === change.id ? { ...node, measured: { ...change.dimensions } } : node,
        );
      }
    }
    return nextNodes;
  }),
  savePositions: vi.fn(),
  loadTopology: vi.fn(),
  removeDeviceFromCanvasMap: vi.fn(),
  updateNodePosition: vi.fn(),
  fitView: vi.fn(),
  nodesInitialized: true,
  renderedMapKey: 'default:' as string | null,
  reactFlowStore: { width: 1200, height: 800 },
  canvasDataParams: null as null | {
    mapId: string | null;
    mapName?: string;
    getCanvasClientRect: () => unknown;
    snapGrid: [number, number] | null;
    onLinkRouteCommit?: (edgeId: string, route: LinkRoute | null) => void;
  },
  canvasPanelsProps: {} as Record<string, unknown>,
  backgroundProps: {} as Record<string, unknown>,
  reactFlowProps: {} as Record<string, unknown>,
  reactFlowRenderCount: 0,
  snapRenderHistory: [] as {
    snapToGrid: boolean;
    positions: { x: number; y: number }[];
  }[],
}));

vi.mock('@xyflow/react', () => ({
  ConnectionMode: { Loose: 'loose' },
  SelectionMode: { Partial: 'partial' },
  Background: (props: Record<string, unknown>) => {
    testState.backgroundProps = props;
    return null;
  },
  MiniMap: () => <div data-testid="topology-minimap" />,
  ReactFlow: ({
    children,
    nodes,
    onlyRenderVisibleElements,
    proOptions,
    onMoveStart,
    onMoveEnd,
    onMove,
    onConnectStart,
    onConnectEnd,
    onNodeDragStop,
    onNodesChange,
    snapToGrid,
    snapGrid,
    connectionLineComponent,
    connectionLineStyle,
    connectionMode,
  }: {
    children: React.ReactNode;
    nodes: DeviceNode[];
    onlyRenderVisibleElements?: boolean;
    proOptions?: { hideAttribution?: boolean };
    onMoveStart?: () => void;
    onMoveEnd?: () => void;
    onMove?: (event: unknown, viewport: { zoom: number }) => void;
    onConnectStart?: () => void;
    onConnectEnd?: () => void;
    onNodeDragStop?: (event: unknown, node: DeviceNode) => void;
    onNodesChange?: (changes: unknown[]) => void;
    snapToGrid?: boolean;
    snapGrid?: [number, number];
    connectionLineComponent?: unknown;
    connectionLineStyle?: React.CSSProperties;
    connectionMode?: unknown;
  }) => {
    testState.reactFlowRenderCount += 1;
    testState.displayedNodes = nodes;
    testState.snapRenderHistory.push({
      snapToGrid: snapToGrid === true,
      positions: nodes.map((node) => ({ ...node.position })),
    });
    testState.reactFlowProps = {
      onlyRenderVisibleElements,
      proOptions,
      snapToGrid,
      snapGrid,
      connectionLineComponent,
      connectionLineStyle,
      connectionMode,
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
        <button type="button" onClick={() => onMove?.({}, { zoom: 0.3 })}>
          Move fit overview zoom
        </button>
        <button type="button" onClick={() => onMove?.({}, { zoom: 0.4 })}>
          Move compact zoom
        </button>
        <button type="button" onClick={() => onMove?.({}, { zoom: 0.2 })}>
          Move overview zoom
        </button>
        <button type="button" onClick={() => onMove?.({}, { zoom: 0.45 })}>
          Move summary zoom
        </button>
        <button type="button" onClick={() => onMove?.({}, { zoom: 1 })}>
          Move detail zoom
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
            const exactPosition = { x: 444, y: 555 };
            const position =
              snapToGrid && snapGrid
                ? {
                    x: Math.round(exactPosition.x / snapGrid[0]) * snapGrid[0],
                    y: Math.round(exactPosition.y / snapGrid[1]) * snapGrid[1],
                  }
                : exactPosition;
            onNodeDragStop?.(
              {},
              {
                ...draggedNode,
                position,
              },
            );
          }}
        >
          Drag area node
        </button>
        <button
          type="button"
          onClick={() =>
            onNodesChange?.([
              {
                id: 'dev-c',
                type: 'dimensions',
                dimensions: { width: 132, height: 58 },
              },
            ])
          }
        >
          Measure ghost node
        </button>
        <button
          type="button"
          onClick={() =>
            onNodesChange?.([
              {
                id: 'dev-a',
                type: 'dimensions',
                dimensions: { width: 88, height: 44 },
              },
            ])
          }
        >
          Measure real node
        </button>
        {children}
      </div>
    );
  },
  applyNodeChanges: (changes: MockNodeChange[], currentNodes: DeviceNode[]) =>
    testState.applyNodeChanges(changes, currentNodes),
  applyEdgeChanges: (_changes: unknown, current: unknown) => current,
  useReactFlow: () => ({
    fitView: testState.fitView,
    zoomIn: vi.fn(),
    zoomOut: vi.fn(),
    getNodes: () => testState.displayedNodes,
    setCenter: vi.fn(),
    screenToFlowPosition: ({ x, y }: { x: number; y: number }) => ({ x, y }),
  }),
  useNodesInitialized: () => testState.nodesInitialized,
  useStore: <T,>(selector: (state: { width: number; height: number }) => T) =>
    selector(testState.reactFlowStore),
}));

vi.mock('./DeviceCard', () => ({
  default: () => null,
  resolveDeviceNodeReadabilityScale: (zoom: number) => (zoom <= 0.6 ? 1.12 : 1),
}));
vi.mock('./LinkEdge', () => ({ default: () => null }));
vi.mock('./SearchOverlay', () => ({ default: () => null }));
vi.mock('./ZoomControls', () => ({ default: () => null }));
vi.mock('./ContextMenu', () => ({ ContextMenu: () => null }));
vi.mock('./SidePanel', () => ({
  SidePanel: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));
vi.mock('./ShortcutHelp', () => ({ ShortcutHelp: () => null }));
vi.mock('./Toolbar', () => ({
  Toolbar: ({
    onToggleSnapToGrid,
    snapToGrid,
  }: {
    onToggleSnapToGrid: () => void;
    snapToGrid: boolean;
  }) => (
    <button
      type="button"
      aria-label={`Snap to grid: ${snapToGrid ? 'On' : 'Off'}`}
      aria-pressed={snapToGrid}
      onClick={onToggleSnapToGrid}
    >
      Toggle snap
    </button>
  ),
}));
vi.mock('./MapSelector', () => ({ MapSelector: () => null }));
vi.mock('./canvas/CanvasPanels', () => ({
  CanvasPanels: (props: Record<string, unknown>) => {
    testState.canvasPanelsProps = props;
    return null;
  },
}));
vi.mock('./canvas/CanvasOverlays', () => ({ CanvasOverlays: () => null }));
vi.mock('./canvas/detailSubscription', () => ({
  getCanvasDetailDeviceId: () => null,
}));
vi.mock('../hooks/useKeyboardShortcuts', () => ({
  useKeyboardShortcuts: () => undefined,
}));
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
vi.mock('../api/client', () => ({
  deleteCanvasMapLinkRoute: (...args: unknown[]) => testState.deleteCanvasMapLinkRoute(...args),
  removeDeviceFromCanvasMap: (...args: unknown[]) => testState.removeDeviceFromCanvasMap(...args),
  saveCanvasMapLinkRoute: (...args: unknown[]) => testState.saveCanvasMapLinkRoute(...args),
}));
vi.mock('./canvas/useCanvasData', async () => {
  const ReactRuntime = await import('react');
  return {
    useCanvasData: (params: {
      mapId: string | null;
      mapName?: string;
      getCanvasClientRect: () => unknown;
      setNodes: React.Dispatch<React.SetStateAction<DeviceNode[]>>;
      snapGrid: [number, number] | null;
      onLinkRouteCommit?: (edgeId: string, route: LinkRoute | null) => void;
    }) => {
      const lastSeededNodesRef = ReactRuntime.useRef<DeviceNode[] | null>(null);
      ReactRuntime.useLayoutEffect(() => {
        if (lastSeededNodesRef.current === testState.canonicalNodes) {
          return;
        }
        lastSeededNodesRef.current = testState.canonicalNodes;
        params.setNodes(testState.canonicalNodes);
      });
      const snapCurrentNodePositions = ReactRuntime.useCallback(
        (grid: [number, number]) => {
          params.setNodes((currentNodes) =>
            currentNodes.map((node) => ({
              ...node,
              position: {
                x: Math.round(node.position.x / grid[0]) * grid[0],
                y: Math.round(node.position.y / grid[1]) * grid[1],
              },
            })),
          );
        },
        [params.setNodes],
      );

      testState.canvasDataParams = params;
      return {
        devices: testState.devices,
        setDevices: vi.fn(),
        topologyLinks: testState.links,
        loading: false,
        error: null,
        loadTopology: testState.loadTopology,
        requestNewNodePlacement: vi.fn().mockResolvedValue(undefined),
        runtimeSummary: { alertCount: 0, prometheusDiagnosticsVisible: false },
        grafanaUrlRef: { current: '' },
        grafanaDashboardConfigRef: { current: null },
        refreshSettings: vi.fn(),
        topologyRecoveryNotice: null,
        dismissTopologyRecoveryNotice: vi.fn(),
        retryTopologyRefresh: vi.fn(),
        updateNodePosition: testState.updateNodePosition,
        snapCurrentNodePositions,
        renderedMapKey: testState.renderedMapKey,
      };
    },
  };
});

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
    testState.setEdges.mockReset();
    testState.deleteCanvasMapLinkRoute.mockReset();
    testState.saveCanvasMapLinkRoute.mockReset();
    testState.applyNodeChanges.mockClear();
    testState.savePositions.mockReset();
    testState.loadTopology.mockReset();
    testState.loadTopology.mockResolvedValue(undefined);
    testState.removeDeviceFromCanvasMap.mockReset();
    testState.removeDeviceFromCanvasMap.mockResolvedValue(undefined);
    testState.updateNodePosition.mockReset();
    testState.fitView.mockReset();
    testState.nodesInitialized = true;
    testState.renderedMapKey = 'default:';
    testState.reactFlowStore = { width: 1200, height: 800 };
    testState.canvasDataParams = null;
    testState.canvasPanelsProps = {};
    testState.backgroundProps = {};
    testState.reactFlowProps = {};
    testState.reactFlowRenderCount = 0;
    testState.snapRenderHistory = [];
    window.localStorage.clear();
  });

  it('keeps React Flow internals mounted while the canvas is hidden', () => {
    render(
      <Canvas
        {...defaultCanvasProps}
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        areas={[]}
        visible={false}
      />,
    );

    expect(screen.getByRole('button', { name: 'Start pan' })).toBeInTheDocument();
    expect(screen.getByTestId('topology-minimap')).toBeInTheDocument();
    expect(testState.displayedNodes.map((node) => node.id)).toEqual(['dev-a', 'dev-b', 'dev-c']);
  });

  it('hides the React Flow attribution watermark', () => {
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

    expect(testState.reactFlowProps.proOptions).toEqual({ hideAttribution: true });
  });

  it('uses the floating connection preview while preserving loose connection behavior', () => {
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

    expect(testState.reactFlowProps).toMatchObject({
      connectionLineComponent: FloatingConnectionLine,
      connectionLineStyle: {
        stroke: 'var(--color-edge-default)',
        strokeWidth: 10,
      },
      connectionMode: 'loose',
    });
  });

  it('wires native snapping and persists snapped or exact drag coordinates from the preference', () => {
    render(
      <Canvas
        {...defaultCanvasProps}
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
    expect(testState.reactFlowProps).toMatchObject({
      snapToGrid: true,
      snapGrid: [30, 30],
    });
    expect(testState.backgroundProps.gap).toBe(30);
    expect(testState.canvasDataParams?.snapGrid).toEqual([30, 30]);

    testState.applyNodeChanges.mockClear();
    fireEvent.click(screen.getByRole('button', { name: 'Drag area node' }));

    expect(testState.applyNodeChanges).not.toHaveBeenCalled();
    expect(testState.savePositions).not.toHaveBeenCalled();
    expect(testState.updateNodePosition).toHaveBeenCalledWith('dev-a', {
      x: 450,
      y: 570,
    });

    testState.updateNodePosition.mockClear();
    fireEvent.click(screen.getByRole('button', { name: 'Snap to grid: On' }));

    expect(screen.getByRole('button', { name: 'Snap to grid: Off' })).toHaveAttribute(
      'aria-pressed',
      'false',
    );
    expect(testState.reactFlowProps).toMatchObject({
      snapToGrid: false,
      snapGrid: [30, 30],
    });
    expect(testState.canvasDataParams?.snapGrid).toBeNull();

    fireEvent.click(screen.getByRole('button', { name: 'Drag area node' }));

    expect(testState.updateNodePosition).toHaveBeenCalledWith('dev-a', {
      x: 444,
      y: 555,
    });
  });

  it('never renders enabled snapping with off-grid controlled nodes', () => {
    window.localStorage.setItem('theia.canvas.snapToGrid', 'false');
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

    expect(testState.displayedNodes[0]?.position).toEqual({ x: 100, y: 100 });
    testState.snapRenderHistory = [];

    act(() => {
      fireEvent.click(screen.getByRole('button', { name: 'Snap to grid: Off' }));
    });

    const enabledRenders = testState.snapRenderHistory.filter((render) => render.snapToGrid);
    expect(enabledRenders.length).toBeGreaterThan(0);
    expect(
      enabledRenders.every((render) =>
        render.positions.every((position) => position.x % 30 === 0 && position.y % 30 === 0),
      ),
    ).toBe(true);
  });

  it('filters ghost measurements while applying real node changes through graph state', () => {
    const props = {
      snapshot: null,
      reconnecting: false,
      prometheusStatus: null,
      selectedAreaId: 'area-1',
      areas: [
        { id: 'area-1', name: 'Area 1', color: '#00aaff' },
        { id: 'area-2', name: 'Area 2', color: '#ffaa00' },
      ],
    } as const;
    const { rerender } = render(
      <Canvas
        {...defaultCanvasProps}
        snapshot={props.snapshot}
        reconnecting={props.reconnecting}
        prometheusStatus={props.prometheusStatus}
        selectedAreaId={props.selectedAreaId}
        areas={props.areas}
      />,
    );

    expect(testState.displayedNodes.find((node) => node.id === 'dev-c')?.data.isGhost).toBe(true);

    testState.applyNodeChanges.mockClear();
    fireEvent.click(screen.getByRole('button', { name: 'Measure ghost node' }));

    expect(testState.applyNodeChanges).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole('button', { name: 'Measure real node' }));

    expect(testState.applyNodeChanges).toHaveBeenCalledWith(
      [
        {
          id: 'dev-a',
          type: 'dimensions',
          dimensions: { width: 88, height: 44 },
        },
      ],
      expect.arrayContaining([expect.objectContaining({ id: 'dev-a' })]),
    );
    expect(testState.displayedNodes.find((node) => node.id === 'dev-a')?.measured).toEqual({
      width: 88,
      height: 44,
    });

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
        {...defaultCanvasProps}
        snapshot={props.snapshot}
        reconnecting={props.reconnecting}
        prometheusStatus={props.prometheusStatus}
        selectedAreaId={props.selectedAreaId}
        areas={props.areas}
      />,
    );

    expect(testState.displayedNodes.find((node) => node.id === 'dev-c')?.measured).toEqual({
      width: 132,
      height: 58,
    });
  });

  it('keeps visible-element rendering enabled and the minimap visible during canvas gestures', () => {
    vi.useFakeTimers();
    try {
      const onInteractionActiveChange = vi.fn();

      render(
        <Canvas
          {...defaultCanvasProps}
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
        {...defaultCanvasProps}
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

    fireEvent.click(screen.getByRole('button', { name: 'Move fit overview zoom' }));

    expect(root.style.getPropertyValue('--theia-device-node-readability-scale')).toBe('1.12');
    expect(root.style.getPropertyValue('--theia-link-badge-readability-scale')).toBe('1.2');
  });

  it('stores the current semantic zoom band on the canvas root from viewport changes', () => {
    render(
      <Canvas
        {...defaultCanvasProps}
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

    expect(root).toHaveAttribute('data-topology-zoom-band', 'detail');

    fireEvent.click(screen.getByRole('button', { name: 'Move overview zoom' }));
    expect(root).toHaveAttribute('data-topology-zoom-band', 'overview');

    fireEvent.click(screen.getByRole('button', { name: 'Move fit overview zoom' }));
    expect(root).toHaveAttribute('data-topology-zoom-band', 'overview');

    fireEvent.click(screen.getByRole('button', { name: 'Move compact zoom' }));
    expect(root).toHaveAttribute('data-topology-zoom-band', 'compact');

    fireEvent.click(screen.getByRole('button', { name: 'Move summary zoom' }));
    expect(root).toHaveAttribute('data-topology-zoom-band', 'summary');

    fireEvent.click(screen.getByRole('button', { name: 'Move detail zoom' }));
    expect(root).toHaveAttribute('data-topology-zoom-band', 'detail');
  });

  it('updates semantic zoom DOM state without forcing React Flow to render again', () => {
    render(
      <Canvas
        {...defaultCanvasProps}
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
    const renderCount = testState.reactFlowRenderCount;

    fireEvent.click(screen.getByRole('button', { name: 'Move overview zoom' }));
    fireEvent.click(screen.getByRole('button', { name: 'Move compact zoom' }));

    expect(root).toHaveAttribute('data-topology-zoom-band', 'compact');
    expect(root.style.getPropertyValue('--theia-device-node-readability-scale')).toBe('1.12');
    expect(testState.reactFlowRenderCount).toBe(renderCount);
  });

  it('preserves unchanged area-colored display node references when one canonical node changes', () => {
    const { rerender } = render(
      <Canvas
        {...defaultCanvasProps}
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
        ...testState.displayedNodes[0],
        position: { x: 125, y: 125 },
      },
      testState.displayedNodes[1],
      testState.displayedNodes[2],
    ];

    rerender(
      <Canvas
        {...defaultCanvasProps}
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

  it('passes saved map metadata to canvas data and applies map-local area projection', () => {
    render(
      <Canvas
        {...defaultCanvasProps}
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId="area-1"
        mapId="map-backbone"
        mapName="Backbone"
        maps={[mockMap()]}
        areas={[
          { id: 'area-1', name: 'Area 1', color: '#00aaff' },
          { id: 'area-2', name: 'Area 2', color: '#ffaa00' },
          { id: 'area-3', name: 'Area 3', color: '#22cc88' },
        ]}
      />,
    );

    expect(testState.canvasDataParams).toMatchObject({
      mapId: 'map-backbone',
      mapName: 'Backbone',
      onLinkRouteCommit: expect.any(Function),
    });
    expect(
      testState.displayedNodes.map((node) => `${node.id}:${node.data.isGhost === true}`),
    ).toEqual(['dev-a:false', 'dev-c:true']);
  });

  it('fits the visible graph when the external fitView revision changes', () => {
    const CanvasWithFitRevision = Canvas as React.ComponentType<
      React.ComponentProps<typeof Canvas> & { fitViewRevision: number }
    >;
    const originalRequestAnimationFrame = window.requestAnimationFrame;
    const frameCallbacks: FrameRequestCallback[] = [];
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      frameCallbacks.push(callback);
      return frameCallbacks.length;
    });

    try {
      const { rerender } = render(
        <CanvasWithFitRevision
          {...defaultCanvasProps}
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          areas={[]}
          fitViewRevision={0}
        />,
      );

      frameCallbacks.length = 0;
      testState.fitView.mockClear();

      rerender(
        <CanvasWithFitRevision
          {...defaultCanvasProps}
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          areas={[]}
          fitViewRevision={1}
        />,
      );

      const callbackCount = frameCallbacks.length;
      for (let index = 0; index < callbackCount; index += 1) {
        frameCallbacks[index]?.(0);
      }

      expect(testState.fitView).toHaveBeenCalledWith({
        padding: { top: '96px', right: 0.08, bottom: 0.08, left: 0.08 },
        duration: 280,
      });
    } finally {
      vi.unstubAllGlobals();
      if (originalRequestAnimationFrame) {
        window.requestAnimationFrame = originalRequestAnimationFrame;
      }
    }
  });

  it('uses compact padding for external fit view requests while canvas chrome is hidden', () => {
    const CanvasWithFitRevision = Canvas as React.ComponentType<
      React.ComponentProps<typeof Canvas> & { fitViewRevision: number }
    >;
    const originalRequestAnimationFrame = window.requestAnimationFrame;
    const frameCallbacks: FrameRequestCallback[] = [];
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      frameCallbacks.push(callback);
      return frameCallbacks.length;
    });

    try {
      const { rerender } = render(
        <CanvasWithFitRevision
          {...defaultCanvasProps}
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          areas={[]}
          chromeHidden
          fitViewRevision={0}
        />,
      );

      frameCallbacks.length = 0;
      testState.fitView.mockClear();

      rerender(
        <CanvasWithFitRevision
          {...defaultCanvasProps}
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          areas={[]}
          chromeHidden
          fitViewRevision={1}
        />,
      );

      const callbackCount = frameCallbacks.length;
      for (let index = 0; index < callbackCount; index += 1) {
        frameCallbacks[index]?.(0);
      }

      expect(testState.fitView).toHaveBeenCalledWith({
        padding: 0.02,
        duration: 280,
      });
    } finally {
      vi.unstubAllGlobals();
      if (originalRequestAnimationFrame) {
        window.requestAnimationFrame = originalRequestAnimationFrame;
      }
    }
  });

  it('re-applies compact fit view when the canvas starts with chrome hidden', () => {
    const originalRequestAnimationFrame = window.requestAnimationFrame;
    const frameCallbacks: FrameRequestCallback[] = [];
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      frameCallbacks.push(callback);
      return frameCallbacks.length;
    });

    try {
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

      const callbackCount = frameCallbacks.length;
      for (let index = 0; index < callbackCount; index += 1) {
        frameCallbacks[index]?.(0);
      }

      expect(testState.fitView).toHaveBeenCalledWith({
        padding: 0.02,
        duration: 280,
      });
    } finally {
      vi.unstubAllGlobals();
      if (originalRequestAnimationFrame) {
        window.requestAnimationFrame = originalRequestAnimationFrame;
      }
    }
  });

  it('keeps a pending external fit view revision until graph nodes are available', () => {
    const CanvasWithFitRevision = Canvas as React.ComponentType<
      React.ComponentProps<typeof Canvas> & { fitViewRevision: number }
    >;
    const originalRequestAnimationFrame = window.requestAnimationFrame;
    const frameCallbacks: FrameRequestCallback[] = [];
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      frameCallbacks.push(callback);
      return frameCallbacks.length;
    });

    try {
      const populatedNodes = [...testState.canonicalNodes];
      testState.canonicalNodes = [];
      const { rerender } = render(
        <CanvasWithFitRevision
          {...defaultCanvasProps}
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          areas={[]}
          fitViewRevision={0}
        />,
      );

      frameCallbacks.length = 0;
      testState.fitView.mockClear();

      rerender(
        <CanvasWithFitRevision
          {...defaultCanvasProps}
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          areas={[]}
          fitViewRevision={1}
        />,
      );

      expect(frameCallbacks).toHaveLength(0);
      expect(testState.fitView).not.toHaveBeenCalled();

      testState.canonicalNodes = populatedNodes;
      rerender(
        <CanvasWithFitRevision
          {...defaultCanvasProps}
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          areas={[]}
          fitViewRevision={1}
        />,
      );

      const callbackCount = frameCallbacks.length;
      for (let index = 0; index < callbackCount; index += 1) {
        frameCallbacks[index]?.(0);
      }

      expect(testState.fitView).toHaveBeenCalledWith({
        padding: { top: '96px', right: 0.08, bottom: 0.08, left: 0.08 },
        duration: 280,
      });
    } finally {
      vi.unstubAllGlobals();
      if (originalRequestAnimationFrame) {
        window.requestAnimationFrame = originalRequestAnimationFrame;
      }
    }
  });

  it('keeps a pending external fit view revision until the canvas is visible', () => {
    const CanvasWithVisibility = Canvas as React.ComponentType<
      React.ComponentProps<typeof Canvas> & { visible: boolean }
    >;
    const originalRequestAnimationFrame = window.requestAnimationFrame;
    const frameCallbacks: FrameRequestCallback[] = [];
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      frameCallbacks.push(callback);
      return frameCallbacks.length;
    });

    try {
      const { rerender } = render(
        <CanvasWithVisibility
          {...defaultCanvasProps}
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          areas={[]}
          fitViewRevision={0}
          visible={false}
        />,
      );

      frameCallbacks.length = 0;
      testState.fitView.mockClear();

      rerender(
        <CanvasWithVisibility
          {...defaultCanvasProps}
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          areas={[]}
          fitViewRevision={1}
          visible={false}
        />,
      );

      expect(frameCallbacks).toHaveLength(0);
      expect(testState.fitView).not.toHaveBeenCalled();

      rerender(
        <CanvasWithVisibility
          {...defaultCanvasProps}
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          areas={[]}
          fitViewRevision={1}
          visible
        />,
      );

      const callbackCount = frameCallbacks.length;
      for (let index = 0; index < callbackCount; index += 1) {
        frameCallbacks[index]?.(0);
      }

      expect(testState.fitView).toHaveBeenCalledWith({
        padding: { top: '96px', right: 0.08, bottom: 0.08, left: 0.08 },
        duration: 280,
      });
    } finally {
      vi.unstubAllGlobals();
      if (originalRequestAnimationFrame) {
        window.requestAnimationFrame = originalRequestAnimationFrame;
      }
    }
  });

  it('keeps a pending external fit view revision until the displayed graph belongs to the selected map', () => {
    const CanvasWithFitRevision = Canvas as React.ComponentType<
      React.ComponentProps<typeof Canvas> & { fitViewRevision: number }
    >;
    const originalRequestAnimationFrame = window.requestAnimationFrame;
    const frameCallbacks: FrameRequestCallback[] = [];
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      frameCallbacks.push(callback);
      return frameCallbacks.length;
    });

    try {
      const { rerender } = render(
        <CanvasWithFitRevision
          {...defaultCanvasProps}
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          areas={[]}
          fitViewRevision={0}
        />,
      );

      frameCallbacks.length = 0;
      testState.fitView.mockClear();

      rerender(
        <CanvasWithFitRevision
          {...defaultCanvasProps}
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          mapId="map-new"
          mapName="New map"
          areas={[]}
          fitViewRevision={1}
        />,
      );

      expect(frameCallbacks).toHaveLength(0);
      expect(testState.fitView).not.toHaveBeenCalled();

      testState.renderedMapKey = 'map:map-new';
      rerender(
        <CanvasWithFitRevision
          {...defaultCanvasProps}
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          mapId="map-new"
          mapName="New map"
          areas={[]}
          fitViewRevision={1}
        />,
      );

      const callbackCount = frameCallbacks.length;
      for (let index = 0; index < callbackCount; index += 1) {
        frameCallbacks[index]?.(0);
      }

      expect(testState.fitView).toHaveBeenCalledWith({
        padding: { top: '96px', right: 0.08, bottom: 0.08, left: 0.08 },
        duration: 280,
      });
    } finally {
      vi.unstubAllGlobals();
      if (originalRequestAnimationFrame) {
        window.requestAnimationFrame = originalRequestAnimationFrame;
      }
    }
  });

  it('keeps a pending external fit view revision until nodes are initialized', () => {
    const CanvasWithFitRevision = Canvas as React.ComponentType<
      React.ComponentProps<typeof Canvas> & { fitViewRevision: number }
    >;
    const originalRequestAnimationFrame = window.requestAnimationFrame;
    const frameCallbacks: FrameRequestCallback[] = [];
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      frameCallbacks.push(callback);
      return frameCallbacks.length;
    });

    try {
      const { rerender } = render(
        <CanvasWithFitRevision
          {...defaultCanvasProps}
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          areas={[]}
          fitViewRevision={0}
        />,
      );

      frameCallbacks.length = 0;
      testState.fitView.mockClear();
      testState.nodesInitialized = false;

      rerender(
        <CanvasWithFitRevision
          {...defaultCanvasProps}
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          areas={[]}
          fitViewRevision={1}
        />,
      );

      expect(frameCallbacks).toHaveLength(0);
      expect(testState.fitView).not.toHaveBeenCalled();

      testState.nodesInitialized = true;
      rerender(
        <CanvasWithFitRevision
          {...defaultCanvasProps}
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          areas={[]}
          fitViewRevision={1}
        />,
      );

      const callbackCount = frameCallbacks.length;
      for (let index = 0; index < callbackCount; index += 1) {
        frameCallbacks[index]?.(0);
      }

      expect(testState.fitView).toHaveBeenCalledWith({
        padding: { top: '96px', right: 0.08, bottom: 0.08, left: 0.08 },
        duration: 280,
      });
    } finally {
      vi.unstubAllGlobals();
      if (originalRequestAnimationFrame) {
        window.requestAnimationFrame = originalRequestAnimationFrame;
      }
    }
  });

  it('keeps a pending external fit view revision until the React Flow viewport has dimensions', () => {
    const CanvasWithFitRevision = Canvas as React.ComponentType<
      React.ComponentProps<typeof Canvas> & { fitViewRevision: number }
    >;
    const originalRequestAnimationFrame = window.requestAnimationFrame;
    const frameCallbacks: FrameRequestCallback[] = [];
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      frameCallbacks.push(callback);
      return frameCallbacks.length;
    });

    try {
      testState.reactFlowStore = { width: 0, height: 0 };
      const { rerender } = render(
        <CanvasWithFitRevision
          {...defaultCanvasProps}
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          areas={[]}
          fitViewRevision={0}
        />,
      );

      frameCallbacks.length = 0;
      testState.fitView.mockClear();

      rerender(
        <CanvasWithFitRevision
          {...defaultCanvasProps}
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          areas={[]}
          fitViewRevision={1}
        />,
      );

      expect(frameCallbacks).toHaveLength(0);
      expect(testState.fitView).not.toHaveBeenCalled();

      testState.reactFlowStore = { width: 1200, height: 800 };
      rerender(
        <CanvasWithFitRevision
          {...defaultCanvasProps}
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          areas={[]}
          fitViewRevision={1}
        />,
      );

      const callbackCount = frameCallbacks.length;
      for (let index = 0; index < callbackCount; index += 1) {
        frameCallbacks[index]?.(0);
      }

      expect(testState.fitView).toHaveBeenCalledWith({
        padding: { top: '96px', right: 0.08, bottom: 0.08, left: 0.08 },
        duration: 280,
      });
    } finally {
      vi.unstubAllGlobals();
      if (originalRequestAnimationFrame) {
        window.requestAnimationFrame = originalRequestAnimationFrame;
      }
    }
  });

  it('cancels a pending external fit view when the selected map changes before the frame runs', () => {
    const CanvasWithFitRevision = Canvas as React.ComponentType<
      React.ComponentProps<typeof Canvas> & { fitViewRevision: number }
    >;
    const originalRequestAnimationFrame = window.requestAnimationFrame;
    const frameCallbacks: FrameRequestCallback[] = [];
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      frameCallbacks.push(callback);
      return frameCallbacks.length;
    });

    try {
      const { rerender } = render(
        <CanvasWithFitRevision
          {...defaultCanvasProps}
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          areas={[]}
          fitViewRevision={0}
        />,
      );

      frameCallbacks.length = 0;
      testState.fitView.mockClear();

      rerender(
        <CanvasWithFitRevision
          {...defaultCanvasProps}
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          areas={[]}
          fitViewRevision={1}
        />,
      );

      expect(frameCallbacks).toHaveLength(1);

      rerender(
        <CanvasWithFitRevision
          {...defaultCanvasProps}
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          mapId="map-new"
          mapName="New map"
          areas={[]}
          fitViewRevision={1}
        />,
      );

      const staleCallbackCount = frameCallbacks.length;
      for (let index = 0; index < staleCallbackCount; index += 1) {
        frameCallbacks[index]?.(0);
      }

      expect(testState.fitView).not.toHaveBeenCalled();

      testState.renderedMapKey = 'map:map-new';
      rerender(
        <CanvasWithFitRevision
          {...defaultCanvasProps}
          snapshot={null}
          reconnecting={false}
          prometheusStatus={null}
          selectedAreaId={null}
          mapId="map-new"
          mapName="New map"
          areas={[]}
          fitViewRevision={1}
        />,
      );

      const readyCallbackCount = frameCallbacks.length;
      for (let index = staleCallbackCount; index < readyCallbackCount; index += 1) {
        frameCallbacks[index]?.(0);
      }

      expect(testState.fitView).toHaveBeenCalledWith({
        padding: { top: '96px', right: 0.08, bottom: 0.08, left: 0.08 },
        duration: 280,
      });
    } finally {
      vi.unstubAllGlobals();
      if (originalRequestAnimationFrame) {
        window.requestAnimationFrame = originalRequestAnimationFrame;
      }
    }
  });

  it('passes saved map removal context to canvas panels', async () => {
    render(
      <Canvas
        {...defaultCanvasProps}
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        mapId="map-backbone"
        mapName="Backbone"
        maps={[mockMap()]}
        areas={[]}
      />,
    );

    expect(testState.canvasPanelsProps.mapId).toBe('map-backbone');
    expect(testState.canvasPanelsProps.mapName).toBe('Backbone');

    const remove = testState.canvasPanelsProps.onRemoveDeviceFromMap as (
      deviceId: string,
    ) => Promise<void>;
    await act(async () => {
      await remove('dev-a');
    });

    expect(testState.removeDeviceFromCanvasMap).toHaveBeenCalledWith('map-backbone', 'dev-a');
    expect(testState.loadTopology).toHaveBeenCalledWith(true);
  });

  it('clears selected canonical nodes when the active map changes', () => {
    const selectedNode = {
      ...testState.canonicalNodes[0],
      selected: true,
    };
    testState.canonicalNodes = [
      selectedNode,
      testState.canonicalNodes[1],
      testState.canonicalNodes[2],
    ];

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

    rerender(
      <Canvas
        {...defaultCanvasProps}
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        mapId="map-backbone"
        mapName="Backbone"
        maps={[mockMap()]}
        areas={[]}
      />,
    );

    expect(testState.displayedNodes.find((node) => node.id === 'dev-a')?.selected).toBe(false);
  });
});
