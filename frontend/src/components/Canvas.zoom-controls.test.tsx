import { fireEvent, render, screen } from '@testing-library/react';
import type React from 'react';
import { describe, expect, it, vi } from 'vitest';

import Canvas from './Canvas';

const xyflowMocks = vi.hoisted(() => ({
  fitView: vi.fn(),
  zoomIn: vi.fn(),
  zoomOut: vi.fn(),
}));

vi.mock('@xyflow/react', () => ({
  ConnectionMode: { Loose: 'loose' },
  SelectionMode: { Partial: 'partial' },
  Background: () => null,
  MiniMap: () => null,
  ReactFlow: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  applyEdgeChanges: (_changes: unknown, current: unknown) => current,
  useNodesState: () => [[], vi.fn(), vi.fn()],
  useReactFlow: () => ({
    fitView: xyflowMocks.fitView,
    zoomIn: xyflowMocks.zoomIn,
    zoomOut: xyflowMocks.zoomOut,
    getNodes: () => [],
    setCenter: vi.fn(),
  }),
}));

vi.mock('./DeviceCard', () => ({ default: () => null }));
vi.mock('./LinkEdge', () => ({ default: () => null }));
vi.mock('./SearchOverlay', () => ({ default: () => null }));
vi.mock('./ContextMenu', () => ({ ContextMenu: () => null }));
vi.mock('./SidePanel', () => ({ SidePanel: () => null }));
vi.mock('./ShortcutHelp', () => ({ ShortcutHelp: () => null }));
vi.mock('./Toolbar', () => ({ Toolbar: () => null }));
vi.mock('./canvas/CanvasPanels', () => ({ CanvasPanels: () => null }));
vi.mock('./canvas/CanvasOverlays', () => ({ CanvasOverlays: () => null }));
vi.mock('./canvas/detailSubscription', () => ({ getCanvasDetailDeviceId: () => null }));
vi.mock('./canvas/useAreaFilteredTopology', () => ({
  useAreaFilteredTopology: () => ({
    filteredDevices: [],
    filteredLinks: [],
    ghostDevices: [],
  }),
}));
vi.mock('./canvas/useCanvasData', () => ({
  useCanvasData: () => ({
    devices: [],
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
  it('fits the topology with a tight viewport padding from the bottom-left control', () => {
    xyflowMocks.fitView.mockClear();

    render(
      <Canvas
        snapshot={null}
        reconnecting={false}
        prometheusStatus={null}
        selectedAreaId={null}
        areas={[]}
      />,
    );

    const fitButton = screen.getAllByRole('button')[2];
    fireEvent.click(fitButton);

    expect(xyflowMocks.fitView).toHaveBeenCalledWith({
      padding: { top: '96px', right: 0.08, bottom: 0.08, left: 0.08 },
      duration: 280,
    });
  });
});
