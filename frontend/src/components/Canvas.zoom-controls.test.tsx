import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import type React from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import Canvas from './Canvas';

const defaultCanvasProps = {
  mapId: null,
  mapName: 'Default',
  maps: [],
  onMapSelect: vi.fn(),
  onManageMaps: vi.fn(),
};

const xyflowMocks = vi.hoisted(() => ({
  fitView: vi.fn(),
  zoomIn: vi.fn(),
  zoomOut: vi.fn(),
  loadTopology: vi.fn(),
  nodes: [] as unknown[],
  devices: [] as unknown[],
  MiniMap: vi.fn(() => <div data-testid="topology-minimap" />),
}));

vi.mock('@xyflow/react', () => ({
  ConnectionMode: { Loose: 'loose' },
  SelectionMode: { Partial: 'partial' },
  Background: () => null,
  MiniMap: xyflowMocks.MiniMap,
  ReactFlow: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  applyEdgeChanges: (_changes: unknown, current: unknown) => current,
  useNodesState: () => [xyflowMocks.nodes, vi.fn(), vi.fn()],
  useReactFlow: () => ({
    fitView: xyflowMocks.fitView,
    zoomIn: xyflowMocks.zoomIn,
    zoomOut: xyflowMocks.zoomOut,
    getNodes: () => [],
    setCenter: vi.fn(),
  }),
  useNodesInitialized: () => true,
  useStore: <T,>(selector: (state: { width: number; height: number }) => T) =>
    selector({ width: 1200, height: 800 }),
}));

vi.mock('./DeviceCard', () => ({ default: () => null }));
vi.mock('./LinkEdge', () => ({ default: () => null }));
vi.mock('./SearchOverlay', () => ({ default: () => null }));
vi.mock('./ContextMenu', () => ({ ContextMenu: () => null }));
vi.mock('./SidePanel', () => ({ SidePanel: () => null }));
vi.mock('./ShortcutHelp', () => ({ ShortcutHelp: () => null }));
vi.mock('./Toolbar', () => ({ Toolbar: () => <div data-testid="topology-toolbar" /> }));
vi.mock('./MapSelector', () => ({ MapSelector: () => null }));
vi.mock('./canvas/CanvasPanels', () => ({ CanvasPanels: () => null }));
vi.mock('./canvas/CanvasOverlays', () => ({
  CanvasOverlays: () => <div data-testid="canvas-overlays" />,
}));
vi.mock('./canvas/detailSubscription', () => ({ getCanvasDetailDeviceId: () => null }));
vi.mock('./canvas/useAreaFilteredTopology', () => ({
  useAreaFilteredTopology: () => ({
    filteredDevices: xyflowMocks.devices,
    filteredLinks: [],
    ghostDevices: [],
  }),
}));
vi.mock('./canvas/useCanvasData', () => ({
  useCanvasData: () => ({
    devices: xyflowMocks.devices,
    setDevices: vi.fn(),
    topologyLinks: [],
    loading: false,
    error: null,
    renderedMapKey: 'default:',
    loadTopology: xyflowMocks.loadTopology,
    runtimeSummary: { alertCount: 0, prometheusDiagnosticsVisible: false },
    grafanaUrlRef: { current: '' },
    deviceGrafanaUrlsRef: { current: new Map<string, string>() },
    refreshSettings: vi.fn(),
    topologyRecoveryNotice: null,
    dismissTopologyRecoveryNotice: vi.fn(),
    retryTopologyRefresh: vi.fn(),
  }),
}));
vi.mock('../hooks/useKeyboardShortcuts', () => ({ useKeyboardShortcuts: () => undefined }));
vi.mock('../hooks/usePositions', () => ({ usePositions: () => ({ savePositions: vi.fn() }) }));
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

describe('Canvas zoom controls', () => {
  beforeEach(() => {
    window.localStorage.clear();
    xyflowMocks.nodes = [];
    xyflowMocks.devices = [];
    xyflowMocks.fitView.mockClear();
    xyflowMocks.MiniMap.mockClear();
    xyflowMocks.loadTopology.mockReset();
    xyflowMocks.loadTopology.mockResolvedValue(undefined);
  });

  it('fits the topology with a tight viewport padding from the bottom-left control', async () => {
    xyflowMocks.fitView.mockClear();

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

    fireEvent.click(screen.getByRole('button', { name: 'Fit view' }));

    await waitFor(() =>
      expect(xyflowMocks.fitView).toHaveBeenCalledWith({
        padding: { top: '96px', right: 0.08, bottom: 0.08, left: 0.08 },
        duration: 280,
      }),
    );
  });

  it('opens diagnostics with the physical D key even when Ctrl+Alt changes event.key', () => {
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

    expect(screen.queryByLabelText('Canvas Diagnostics')).not.toBeInTheDocument();

    fireEvent.keyDown(window, {
      key: '∂',
      code: 'KeyD',
      ctrlKey: true,
      altKey: true,
    });

    expect(screen.getByLabelText('Canvas Diagnostics')).toBeInTheDocument();
  });

  it('refreshes topology when the external topology revision changes', () => {
    const { rerender } = render(
      <Canvas
        {...defaultCanvasProps}
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        areas={[]}
        topologyRefreshRevision={0}
      />,
    );

    expect(xyflowMocks.loadTopology).not.toHaveBeenCalled();

    rerender(
      <Canvas
        {...defaultCanvasProps}
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        areas={[]}
        topologyRefreshRevision={1}
      />,
    );

    expect(xyflowMocks.loadTopology).toHaveBeenCalledWith(true);
  });

  it('hides canvas chrome and fits the viewport from the focus toggle', async () => {
    const onChromeHiddenChange = vi.fn();

    render(
      <Canvas
        {...defaultCanvasProps}
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        areas={[]}
        chromeHidden={false}
        onChromeHiddenChange={onChromeHiddenChange}
      />,
    );

    expect(screen.getByTestId('topology-toolbar')).toBeInTheDocument();
    expect(screen.getByTestId('canvas-overlays')).toBeInTheDocument();
    expect(screen.getByTestId('topology-minimap')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /hide canvas controls/i }));

    expect(onChromeHiddenChange).toHaveBeenCalledWith(true);
    await waitFor(() =>
      expect(xyflowMocks.fitView).toHaveBeenCalledWith({
        padding: 0.02,
        duration: 280,
      }),
    );
  });

  it('keeps the focus toggle visible when canvas chrome is hidden', () => {
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

    expect(screen.queryByTestId('topology-toolbar')).not.toBeInTheDocument();
    expect(screen.getByTestId('canvas-overlays')).toBeInTheDocument();
    expect(screen.queryByTestId('topology-minimap')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /show canvas controls/i })).toBeInTheDocument();
  });
});
