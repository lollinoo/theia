/**
 * Defines canvas diagnostics panel behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import { type ReactNode, useEffect, useState } from 'react';

import {
  type CanvasDiagnosticEvent,
  type CanvasDiagnosticsSnapshot,
  clearCanvasDiagnosticEvents,
  exportCanvasDiagnostics,
  getCanvasDiagnosticEvents,
  getCanvasDiagnosticsSnapshot,
  subscribeCanvasDiagnostics,
} from './canvasDiagnostics';
import { clearCanvasMetrics } from './canvasInstrumentation';

interface CanvasDiagnosticsPanelProps {
  open: boolean;
  onClose: () => void;
  onForceRefresh?: () => void;
  onFitView?: () => void;
}

function formatValue(value: unknown): string {
  if (value === undefined || value === null || value === '') {
    return '-';
  }
  if (typeof value === 'boolean') {
    return value ? 'yes' : 'no';
  }
  if (typeof value === 'number') {
    return Number.isInteger(value) ? String(value) : value.toFixed(1);
  }
  return String(value);
}

function DiagnosticsRow({ label, value }: { label: string; value: unknown }) {
  return (
    <div className="grid grid-cols-[minmax(120px,1fr)_minmax(80px,auto)] gap-3 rounded-md bg-surface/50 px-2 py-1.5">
      <dt className="text-on-bg-secondary">{label}</dt>
      <dd
        className="max-w-[220px] truncate text-right font-medium text-on-bg"
        title={formatValue(value)}
      >
        {formatValue(value)}
      </dd>
    </div>
  );
}

function DiagnosticsSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="pt-1">
      <h3 className="mb-2 text-xs font-semibold uppercase text-on-bg-secondary">{title}</h3>
      <dl className="space-y-1 text-xs">{children}</dl>
    </section>
  );
}

function useDiagnosticsPanelState(open: boolean): {
  snapshot: CanvasDiagnosticsSnapshot;
  events: CanvasDiagnosticEvent[];
} {
  const [state, setState] = useState(() => ({
    snapshot: getCanvasDiagnosticsSnapshot(),
    events: getCanvasDiagnosticEvents(),
  }));

  useEffect(() => {
    if (!open) {
      return;
    }

    const refresh = () => {
      setState({
        snapshot: getCanvasDiagnosticsSnapshot(),
        events: getCanvasDiagnosticEvents(),
      });
    };
    refresh();
    const unsubscribe = subscribeCanvasDiagnostics(refresh);
    const intervalId = window.setInterval(refresh, 1000);
    return () => {
      unsubscribe();
      window.clearInterval(intervalId);
    };
  }, [open]);

  return state;
}

async function copyDiagnosticsJson(): Promise<void> {
  const payload = JSON.stringify(exportCanvasDiagnostics(), null, 2);
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(payload);
      return;
    } catch {
      // Fall back below for browsers that expose the API but block it in this context.
    }
  }

  if (!copyTextWithTextarea(payload)) {
    throw new Error('clipboard unavailable');
  }
}

function copyTextWithTextarea(value: string): boolean {
  if (typeof document.execCommand !== 'function') {
    return false;
  }
  const textarea = document.createElement('textarea');
  textarea.value = value;
  textarea.setAttribute('readonly', 'true');
  textarea.style.position = 'fixed';
  textarea.style.left = '-9999px';
  textarea.style.top = '0';
  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  try {
    return document.execCommand('copy');
  } finally {
    document.body.removeChild(textarea);
  }
}

/** Renders the CanvasDiagnosticsPanel component within the topology canvas. */
export function CanvasDiagnosticsPanel({
  open,
  onClose,
  onForceRefresh,
  onFitView,
}: CanvasDiagnosticsPanelProps) {
  const { snapshot, events } = useDiagnosticsPanelState(open);
  const [copyStatus, setCopyStatus] = useState<'idle' | 'copied' | 'failed'>('idle');

  if (!open) {
    return null;
  }

  const metrics = Object.entries(snapshot.performance.metrics).sort(([left], [right]) =>
    left.localeCompare(right),
  );
  const deviceCardRenderMetric = snapshot.performance.metrics['runtime:deviceCardRender'];
  const frameTimeMetric = snapshot.performance.metrics['runtime:frameTime'];
  const frameOverBudget16Metric = snapshot.performance.metrics['runtime:frameOverBudget16'];
  const frameOverBudget33Metric = snapshot.performance.metrics['runtime:frameOverBudget33'];
  const frameOverBudget50Metric = snapshot.performance.metrics['runtime:frameOverBudget50'];
  const longTaskMetric = snapshot.performance.metrics['runtime:longTask'];
  const recentEvents = events.slice(-8).reverse();

  return (
    <aside
      className="absolute right-4 top-4 z-[70] flex max-h-[calc(100%-2rem)] w-[380px] flex-col rounded-lg border border-outline bg-surface-container-high/95 text-on-bg shadow-floating backdrop-blur"
      aria-label="Canvas Diagnostics"
    >
      <header className="flex items-center justify-between bg-surface/45 px-4 py-3">
        <div>
          <h2 className="text-sm font-semibold">Canvas Diagnostics</h2>
          <p className="mt-0.5 text-xs text-on-bg-secondary">{snapshot.generatedAt}</p>
        </div>
        <button
          type="button"
          onClick={onClose}
          className="rounded-md border border-outline px-2 py-1 text-xs text-on-bg-secondary hover:text-on-bg"
        >
          Close
        </button>
      </header>

      <div className="flex-1 space-y-4 overflow-y-auto px-4 py-4">
        <DiagnosticsSection title="Topology">
          <DiagnosticsRow label="schema version" value={snapshot.topology.schemaVersion} />
          <DiagnosticsRow label="topology version" value={snapshot.topology.topologyVersion} />
          <DiagnosticsRow label="runtime version" value={snapshot.topology.runtimeVersion} />
          <DiagnosticsRow label="last load" value={snapshot.topology.lastTopologyLoadAt} />
          <DiagnosticsRow label="load reason" value={snapshot.topology.lastTopologyLoadReason} />
          <DiagnosticsRow
            label="load duration"
            value={snapshot.topology.lastTopologyLoadDurationMs}
          />
          <DiagnosticsRow label="status" value={snapshot.topology.lastTopologyLoadStatus} />
          <DiagnosticsRow label="error" value={snapshot.topology.lastTopologyLoadError} />
        </DiagnosticsSection>

        <DiagnosticsSection title="WebSocket">
          <DiagnosticsRow label="connected" value={snapshot.websocket.connected} />
          <DiagnosticsRow label="last message" value={snapshot.websocket.lastMessageType} />
          <DiagnosticsRow label="last message at" value={snapshot.websocket.lastMessageAt} />
          <DiagnosticsRow label="reconnects" value={snapshot.websocket.reconnectCount} />
          <DiagnosticsRow label="resync required" value={snapshot.websocket.resyncRequiredCount} />
          <DiagnosticsRow
            label="topology changed"
            value={snapshot.websocket.topologyChangedCount}
          />
          <DiagnosticsRow
            label="last applied snapshot version"
            value={snapshot.websocket.lastAppliedSnapshotVersion}
          />
          <DiagnosticsRow
            label="last applied delta version"
            value={snapshot.websocket.lastAppliedDeltaVersion}
          />
          <DiagnosticsRow
            label="runtime identity"
            value={snapshot.websocket.lastAppliedRuntimeIdentity}
          />
          <DiagnosticsRow
            label="rejected delta"
            value={snapshot.websocket.lastRejectedDeltaReason}
          />
        </DiagnosticsSection>

        <DiagnosticsSection title="Graph">
          <DiagnosticsRow label="canonical nodes" value={snapshot.graph.canonicalNodeCount} />
          <DiagnosticsRow label="canonical edges" value={snapshot.graph.canonicalEdgeCount} />
          <DiagnosticsRow label="displayed nodes" value={snapshot.graph.displayedNodeCount} />
          <DiagnosticsRow label="displayed edges" value={snapshot.graph.displayedEdgeCount} />
          <DiagnosticsRow label="ghost nodes" value={snapshot.graph.ghostNodeCount} />
          <DiagnosticsRow label="selected area" value={snapshot.graph.selectedAreaId} />
          <DiagnosticsRow label="selected nodes" value={snapshot.graph.selectedNodeCount} />
          <DiagnosticsRow label="selected edges" value={snapshot.graph.selectedEdgeCount} />
        </DiagnosticsSection>

        <DiagnosticsSection title="Layout">
          <DiagnosticsRow label="last layout" value={snapshot.layout.lastLayoutAt} />
          <DiagnosticsRow label="duration" value={snapshot.layout.lastLayoutDurationMs} />
          <DiagnosticsRow label="nodes" value={snapshot.layout.lastLayoutNodeCount} />
          <DiagnosticsRow label="reason" value={snapshot.layout.lastLayoutReason} />
          <DiagnosticsRow label="pending" value={snapshot.layout.pendingLayout} />
        </DiagnosticsSection>

        <DiagnosticsSection title="Positions">
          <DiagnosticsRow label="pending saves" value={snapshot.positions.pendingSaveCount} />
          <DiagnosticsRow label="last save" value={snapshot.positions.lastSaveAt} />
          <DiagnosticsRow label="save duration" value={snapshot.positions.lastSaveDurationMs} />
          <DiagnosticsRow label="status" value={snapshot.positions.lastSaveStatus} />
          <DiagnosticsRow label="error" value={snapshot.positions.lastSaveError} />
          <DiagnosticsRow label="revision" value={snapshot.positions.positionRevision} />
        </DiagnosticsSection>

        <DiagnosticsSection title="Manual Edge Migration">
          <DiagnosticsRow label="status" value={snapshot.manualEdgeMigration.status} />
          <DiagnosticsRow label="attempts" value={snapshot.manualEdgeMigration.attemptCount} />
          <DiagnosticsRow label="pending edges" value={snapshot.manualEdgeMigration.pendingCount} />
          <DiagnosticsRow label="applied edges" value={snapshot.manualEdgeMigration.appliedCount} />
          <DiagnosticsRow label="failed edges" value={snapshot.manualEdgeMigration.failedCount} />
          <DiagnosticsRow label="skipped edges" value={snapshot.manualEdgeMigration.skippedCount} />
          <DiagnosticsRow label="last attempt" value={snapshot.manualEdgeMigration.lastAttemptAt} />
          <DiagnosticsRow
            label="last completed"
            value={snapshot.manualEdgeMigration.lastCompletedAt}
          />
          <DiagnosticsRow label="error" value={snapshot.manualEdgeMigration.lastError} />
        </DiagnosticsSection>

        <DiagnosticsSection title="Runtime">
          <DiagnosticsRow label="Prometheus" value={snapshot.runtime.prometheusStatus} />
          <DiagnosticsRow label="Prometheus error" value={snapshot.runtime.prometheusError} />
        </DiagnosticsSection>

        <DiagnosticsSection title="Component Renders">
          <DiagnosticsRow label="DeviceCard renders" value={deviceCardRenderMetric?.count} />
          <DiagnosticsRow label="DeviceCard avg ms" value={deviceCardRenderMetric?.avgMs} />
          <DiagnosticsRow label="DeviceCard p95 ms" value={deviceCardRenderMetric?.p95Ms} />
          <DiagnosticsRow label="DeviceCard max ms" value={deviceCardRenderMetric?.maxMs} />
        </DiagnosticsSection>

        <DiagnosticsSection title="Browser Responsiveness">
          <DiagnosticsRow label="Frame samples" value={frameTimeMetric?.count} />
          <DiagnosticsRow label="Frame avg ms" value={frameTimeMetric?.avgMs} />
          <DiagnosticsRow label="Frame p95 ms" value={frameTimeMetric?.p95Ms} />
          <DiagnosticsRow label="Frame max ms" value={frameTimeMetric?.maxMs} />
          <DiagnosticsRow label="Frames >16.7ms" value={frameOverBudget16Metric?.count} />
          <DiagnosticsRow label="Frames >33.3ms" value={frameOverBudget33Metric?.count} />
          <DiagnosticsRow label="Frames >50ms" value={frameOverBudget50Metric?.count} />
          <DiagnosticsRow label="Long tasks" value={longTaskMetric?.count} />
          <DiagnosticsRow label="Long task max ms" value={longTaskMetric?.maxMs} />
        </DiagnosticsSection>

        <section className="pt-1">
          <h3 className="mb-2 text-xs font-semibold uppercase text-on-bg-secondary">Performance</h3>
          <div className="space-y-1 text-xs">
            {metrics.length === 0 ? (
              <p className="text-on-bg-secondary">No metrics recorded</p>
            ) : (
              metrics.slice(0, 8).map(([name, aggregate]) => (
                <div key={name} className="flex items-center justify-between gap-3">
                  <span className="truncate text-on-bg-secondary">{name}</span>
                  <span className="font-medium text-on-bg">
                    {aggregate.avgMs.toFixed(1)} ms p95 {aggregate.p95Ms.toFixed(1)}
                  </span>
                </div>
              ))
            )}
          </div>
        </section>

        <section className="pt-1">
          <h3 className="mb-2 text-xs font-semibold uppercase text-on-bg-secondary">
            Recent Events
          </h3>
          <div className="space-y-2 text-xs">
            {recentEvents.length === 0 ? (
              <p className="text-on-bg-secondary">No events recorded</p>
            ) : (
              recentEvents.map((event) => (
                <div key={event.id} className="rounded-md bg-surface/70 px-2 py-1.5">
                  <div className="flex items-center justify-between gap-2">
                    <span className="font-medium text-on-bg">{event.event}</span>
                    <span className="text-on-bg-secondary">{event.level}</span>
                  </div>
                  <p className="mt-0.5 truncate text-on-bg-secondary">{event.message}</p>
                </div>
              ))
            )}
          </div>
        </section>
      </div>

      <footer className="grid grid-cols-2 gap-2 bg-surface/45 px-4 py-3">
        <button
          type="button"
          onClick={() => {
            void copyDiagnosticsJson()
              .then(() => setCopyStatus('copied'))
              .catch(() => setCopyStatus('failed'));
          }}
          className="rounded-md border border-outline px-3 py-2 text-xs hover:bg-surface"
        >
          Copy diagnostics JSON
        </button>
        <button
          type="button"
          onClick={() => {
            clearCanvasMetrics();
          }}
          className="rounded-md border border-outline px-3 py-2 text-xs hover:bg-surface"
        >
          Clear metrics
        </button>
        <button
          type="button"
          onClick={clearCanvasDiagnosticEvents}
          className="rounded-md border border-outline px-3 py-2 text-xs hover:bg-surface"
        >
          Clear events
        </button>
        <button
          type="button"
          onClick={onForceRefresh}
          className="rounded-md border border-outline px-3 py-2 text-xs hover:bg-surface"
        >
          Force refresh
        </button>
        <button
          type="button"
          onClick={onFitView}
          className="rounded-md border border-outline px-3 py-2 text-xs hover:bg-surface"
        >
          Fit view
        </button>
        <span className="flex items-center justify-end text-xs text-on-bg-secondary">
          {copyStatus === 'copied' ? 'Copied' : copyStatus === 'failed' ? 'Copy failed' : ''}
        </span>
      </footer>
    </aside>
  );
}
