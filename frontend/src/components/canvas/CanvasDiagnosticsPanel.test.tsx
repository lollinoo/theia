/**
 * Exercises canvas diagnostics panel topology canvas behavior so refactors preserve the documented contract.
 */
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
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
  recordCanvasFrameTime,
  recordCanvasLongTask,
  recordCanvasMetric,
  setCanvasRenderMetricsEnabled,
  startCanvasRenderMetric,
} from './canvasInstrumentation';

describe('CanvasDiagnosticsPanel', () => {
  beforeEach(() => {
    clearCanvasMetrics();
    resetCanvasDiagnostics();
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: undefined,
    });
    Object.defineProperty(document, 'execCommand', {
      configurable: true,
      value: undefined,
    });
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
        runtimeStreamId: 'runtime-stream-43',
        runtimeRecoveryPhase: 'idle',
        runtimeRecoveryTargetVersion: '43',
        lastRuntimeRecoveryMode: 'replay',
        lastRuntimeRecoveryDurationMs: 125,
        lastRuntimeAckVersion: '43',
        runtimeRecoveryCount: 4,
        runtimeReplayRecoveryCount: 2,
        runtimeSnapshotRecoveryCount: 1,
        runtimeHttpFallbackCount: 1,
        runtimeRecoveryFailureCount: 0,
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
      manualEdgeMigration: {
        status: 'retried',
        pendingCount: 2,
        appliedCount: 3,
        failedCount: 1,
        skippedCount: 4,
        attemptCount: 5,
        lastAttemptAt: '2026-05-05T00:00:00.000Z',
        lastCompletedAt: '2026-05-05T00:00:01.000Z',
        lastError: 'backend unavailable',
      },
    });
    recordCanvasMetric({
      name: 'areaProjection',
      scenario: 'runtime',
      durationMs: 3,
      timestamp: 1,
    });
    recordCanvasFrameTime(18);
    recordCanvasFrameTime(42);
    recordCanvasLongTask(88);
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
    expect(screen.getByText('runtime-stream-43')).toBeInTheDocument();
    expect(screen.getByText('recovery phase')).toBeInTheDocument();
    expect(screen.getByText('last recovery mode')).toBeInTheDocument();
    expect(screen.getByText('replay')).toBeInTheDocument();
    expect(screen.getByText('last ACK version')).toBeInTheDocument();
    expect(screen.getByText('10')).toBeInTheDocument();
    expect(screen.getByText('area-1')).toBeInTheDocument();
    expect(screen.getAllByText('success').length).toBeGreaterThan(0);
    expect(screen.getByText('runtime:areaProjection')).toBeInTheDocument();
    expect(screen.getByText('DeviceCard renders')).toBeInTheDocument();
    expect(screen.getByText('Browser Responsiveness')).toBeInTheDocument();
    expect(screen.getByText('Frame p95 ms')).toBeInTheDocument();
    expect(screen.getByText('Long tasks')).toBeInTheDocument();
    expect(screen.getByText('projection.area.changed')).toBeInTheDocument();
    expect(screen.getByText('Manual Edge Migration')).toBeInTheDocument();
    expect(screen.getByText('retried')).toBeInTheDocument();
    expect(screen.getByText('failed edges')).toBeInTheDocument();
    expect(screen.getByText('backend unavailable')).toBeInTheDocument();
  });

  it('runs safe actions from the panel', async () => {
    const onForceRefresh = vi.fn();
    const onFitView = vi.fn();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    });

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
    await waitFor(() => expect(screen.getByText('Copied')).toBeInTheDocument());
  });

  it('falls back to textarea copy when clipboard API is unavailable', async () => {
    const execCommand = vi.fn().mockReturnValue(true);
    Object.defineProperty(document, 'execCommand', {
      configurable: true,
      value: execCommand,
    });

    render(
      <CanvasDiagnosticsPanel
        open
        onClose={vi.fn()}
        onForceRefresh={vi.fn()}
        onFitView={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: /copy diagnostics json/i }));

    await waitFor(() => {
      expect(execCommand).toHaveBeenCalledWith('copy');
      expect(screen.getByText('Copied')).toBeInTheDocument();
    });
  });

  it('falls back to textarea copy when clipboard API rejects', async () => {
    const writeText = vi.fn().mockRejectedValue(new Error('clipboard blocked'));
    const execCommand = vi.fn().mockReturnValue(true);
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    });
    Object.defineProperty(document, 'execCommand', {
      configurable: true,
      value: execCommand,
    });

    render(
      <CanvasDiagnosticsPanel
        open
        onClose={vi.fn()}
        onForceRefresh={vi.fn()}
        onFitView={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: /copy diagnostics json/i }));

    await waitFor(() => {
      expect(writeText).toHaveBeenCalledTimes(1);
      expect(execCommand).toHaveBeenCalledWith('copy');
      expect(screen.getByText('Copied')).toBeInTheDocument();
    });
  });
});
