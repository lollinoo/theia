import { fireEvent, render, screen, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { CanvasDiagnosticsPanel } from './CanvasDiagnosticsPanel';
import {
  recordCanvasDiagnosticEvent,
  resetCanvasDiagnostics,
  updateCanvasDiagnosticsState,
} from './canvasDiagnostics';
import {
  clearCanvasMetrics,
  finishCanvasRenderMetric,
  recordCanvasMetric,
  setCanvasRenderMetricsEnabled,
  startCanvasRenderMetric,
} from './canvasInstrumentation';

describe('CanvasDiagnosticsPanel', () => {
  beforeEach(() => {
    clearCanvasMetrics();
    resetCanvasDiagnostics();
  });

  it('renders graph, websocket, positions and performance diagnostics', () => {
    updateCanvasDiagnosticsState({
      topology: {
        topologyVersion: 'topo-1',
        runtimeVersion: 'rt-7',
        schemaVersion: 1,
        lastTopologyLoadStatus: 'success',
        lastTopologyLoadReason: 'topology_changed',
        lastTopologyLoadDurationMs: 42,
      },
      websocket: {
        connected: true,
        lastMessageType: 'runtime_delta',
        reconnectCount: 1,
        resyncRequiredCount: 2,
        topologyChangedCount: 3,
        lastAppliedSnapshotVersion: '42',
        lastAppliedDeltaVersion: '43',
        lastAppliedRuntimeIdentity: 'rt-sha256:abc',
      },
      graph: {
        canonicalNodeCount: 10,
        canonicalEdgeCount: 12,
        displayedNodeCount: 5,
        displayedEdgeCount: 7,
        ghostNodeCount: 2,
        selectedAreaId: 'area-1',
      },
      positions: {
        pendingSaveCount: 0,
        lastSaveStatus: 'success',
        lastSaveDurationMs: 8,
      },
    });
    recordCanvasMetric({
      name: 'areaProjection',
      scenario: 'runtime',
      durationMs: 3,
      timestamp: 1,
    });
    setCanvasRenderMetricsEnabled(true);
    finishCanvasRenderMetric(startCanvasRenderMetric('DeviceCard'), { deviceId: 'dev-1' });
    recordCanvasDiagnosticEvent({
      level: 'info',
      source: 'projection',
      event: 'projection.area.changed',
      message: 'Area projection changed',
    });

    render(
      <CanvasDiagnosticsPanel
        open
        onClose={vi.fn()}
        onForceRefresh={vi.fn()}
        onFitView={vi.fn()}
      />,
    );

    expect(screen.getByText('Canvas Diagnostics')).toBeInTheDocument();
    expect(screen.getByText('topo-1')).toBeInTheDocument();
    expect(screen.getByText('rt-7')).toBeInTheDocument();
    expect(screen.getByText('runtime_delta')).toBeInTheDocument();
    const snapshotVersionRow = screen.getByText('last applied snapshot version').closest('div');
    const deltaVersionRow = screen.getByText('last applied delta version').closest('div');
    expect(snapshotVersionRow).not.toBeNull();
    expect(deltaVersionRow).not.toBeNull();
    expect(within(snapshotVersionRow as HTMLElement).getByText('42')).toBeInTheDocument();
    expect(within(deltaVersionRow as HTMLElement).getByText('43')).toBeInTheDocument();
    expect(screen.getByText('rt-sha256:abc')).toBeInTheDocument();
    expect(screen.getByText('10')).toBeInTheDocument();
    expect(screen.getByText('area-1')).toBeInTheDocument();
    expect(screen.getAllByText('success').length).toBeGreaterThan(0);
    expect(screen.getByText('runtime:areaProjection')).toBeInTheDocument();
    expect(screen.getByText('DeviceCard renders')).toBeInTheDocument();
    expect(screen.getByText('projection.area.changed')).toBeInTheDocument();
  });

  it('runs safe actions from the panel', () => {
    const onForceRefresh = vi.fn();
    const onFitView = vi.fn();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });

    render(
      <CanvasDiagnosticsPanel
        open
        onClose={vi.fn()}
        onForceRefresh={onForceRefresh}
        onFitView={onFitView}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: /force refresh/i }));
    fireEvent.click(screen.getByRole('button', { name: /fit view/i }));
    fireEvent.click(screen.getByRole('button', { name: /copy diagnostics json/i }));

    expect(onForceRefresh).toHaveBeenCalledTimes(1);
    expect(onFitView).toHaveBeenCalledTimes(1);
    expect(writeText).toHaveBeenCalledTimes(1);
  });
});
