import { type CanvasMetricAggregate, exportCanvasMetrics } from './canvasInstrumentation';

export type CanvasTopologyLoadStatus = 'idle' | 'loading' | 'success' | 'error';
export type CanvasPositionSaveStatus = 'idle' | 'pending' | 'success' | 'error';
export type CanvasManualEdgeMigrationStatus = 'idle' | 'pending' | 'retried' | 'applied' | 'failed';
export type CanvasDiagnosticLevel = 'debug' | 'info' | 'warn' | 'error';
export type CanvasDiagnosticSource =
  | 'topology'
  | 'runtime'
  | 'websocket'
  | 'layout'
  | 'positions'
  | 'performance'
  | 'projection'
  | 'reactflow';

export interface CanvasDiagnosticsSnapshot {
  generatedAt: string;
  topology: {
    topologyVersion?: string;
    runtimeVersion?: string;
    schemaVersion?: number;
    lastTopologyLoadAt?: string;
    lastTopologyLoadReason?: string;
    lastTopologyLoadDurationMs?: number;
    lastTopologyLoadStatus: CanvasTopologyLoadStatus;
    lastTopologyLoadError?: string;
  };
  websocket: {
    connected: boolean;
    lastMessageAt?: string;
    lastMessageType?: string;
    reconnectCount: number;
    resyncRequiredCount: number;
    topologyChangedCount: number;
    lastAppliedSnapshotVersion?: string;
    lastAppliedDeltaVersion?: string;
    lastAppliedRuntimeIdentity?: string;
    lastRejectedDeltaReason?: string;
  };
  graph: {
    canonicalNodeCount: number;
    canonicalEdgeCount: number;
    displayedNodeCount: number;
    displayedEdgeCount: number;
    ghostNodeCount: number;
    selectedAreaId?: string | null;
    selectedNodeCount: number;
    selectedEdgeCount: number;
  };
  layout: {
    lastLayoutAt?: string;
    lastLayoutDurationMs?: number;
    lastLayoutNodeCount?: number;
    lastLayoutReason?: string;
    pendingLayout: boolean;
  };
  positions: {
    pendingSaveCount: number;
    lastSaveAt?: string;
    lastSaveDurationMs?: number;
    lastSaveStatus: CanvasPositionSaveStatus;
    lastSaveError?: string;
    positionRevision?: string;
  };
  manualEdgeMigration: {
    status: CanvasManualEdgeMigrationStatus;
    pendingCount: number;
    appliedCount: number;
    failedCount: number;
    skippedCount: number;
    attemptCount: number;
    lastAttemptAt?: string;
    lastCompletedAt?: string;
    lastError?: string;
  };
  runtime: {
    prometheusStatus?: 'unknown' | 'disabled' | 'available' | 'unavailable';
    prometheusError?: string;
  };
  performance: {
    metrics: Record<string, CanvasMetricAggregate>;
  };
}

export interface CanvasDiagnosticEvent {
  id: string;
  timestamp: string;
  level: CanvasDiagnosticLevel;
  source: CanvasDiagnosticSource;
  event: string;
  message: string;
  metadata?: Record<string, unknown>;
}

export interface CanvasDiagnosticsExport {
  version: 1;
  generatedAt: string;
  diagnostics: CanvasDiagnosticsSnapshot;
  events: CanvasDiagnosticEvent[];
  metrics: Record<string, CanvasMetricAggregate>;
}

type CanvasDiagnosticsState = Omit<CanvasDiagnosticsSnapshot, 'generatedAt' | 'performance'>;
type CanvasDiagnosticsPatch = {
  [Section in keyof CanvasDiagnosticsState]?: Partial<CanvasDiagnosticsState[Section]>;
};
type CanvasDiagnosticEventInput = Omit<CanvasDiagnosticEvent, 'id' | 'timestamp'> & {
  timestamp?: string;
};

const maxCanvasDiagnosticEvents = 200;

// createInitialState builds the diagnostics baseline used after reset and module load.
function createInitialState(): CanvasDiagnosticsState {
  return {
    topology: {
      lastTopologyLoadStatus: 'idle',
    },
    websocket: {
      connected: false,
      reconnectCount: 0,
      resyncRequiredCount: 0,
      topologyChangedCount: 0,
    },
    graph: {
      canonicalNodeCount: 0,
      canonicalEdgeCount: 0,
      displayedNodeCount: 0,
      displayedEdgeCount: 0,
      ghostNodeCount: 0,
      selectedAreaId: null,
      selectedNodeCount: 0,
      selectedEdgeCount: 0,
    },
    layout: {
      pendingLayout: false,
    },
    positions: {
      pendingSaveCount: 0,
      lastSaveStatus: 'idle',
    },
    manualEdgeMigration: {
      status: 'idle',
      pendingCount: 0,
      appliedCount: 0,
      failedCount: 0,
      skippedCount: 0,
      attemptCount: 0,
    },
    runtime: {
      prometheusStatus: 'unknown',
    },
  };
}

let diagnosticsState: CanvasDiagnosticsState = createInitialState();
let diagnosticEvents: CanvasDiagnosticEvent[] = [];
const listeners = new Set<() => void>();
let diagnosticEventSequence = 0;
let diagnosticsNotificationTimer: ReturnType<typeof setTimeout> | undefined;

// flushDiagnosticsListeners delivers a coalesced diagnostics notification batch.
function flushDiagnosticsListeners(): void {
  diagnosticsNotificationTimer = undefined;
  for (const listener of Array.from(listeners)) {
    listener();
  }
}

// notifyDiagnosticsListeners schedules one async notification for all pending diagnostics changes.
function notifyDiagnosticsListeners(): void {
  if (listeners.size === 0 || diagnosticsNotificationTimer !== undefined) {
    return;
  }

  diagnosticsNotificationTimer = setTimeout(flushDiagnosticsListeners, 0);
}

// installCanvasDiagnosticsWindowHelpers exposes diagnostics helpers for browser inspection.
function installCanvasDiagnosticsWindowHelpers(): void {
  if (typeof window === 'undefined') {
    return;
  }

  window.__THEIA_CANVAS_DIAGNOSTICS__ = getCanvasDiagnosticsSnapshot;
  window.__THEIA_CANVAS_DIAGNOSTICS_EXPORT__ = exportCanvasDiagnostics;
  window.__THEIA_CANVAS_DIAGNOSTICS_CLEAR_EVENTS__ = clearCanvasDiagnosticEvents;
  window.__THEIA_CANVAS_DIAGNOSTIC_EVENTS__ = diagnosticEvents;
}

// sanitizeMetadata clones event metadata and records serialization failures instead of throwing.
function sanitizeMetadata(
  metadata: Record<string, unknown> | undefined,
): Record<string, unknown> | undefined {
  if (!metadata) {
    return undefined;
  }

  try {
    return JSON.parse(JSON.stringify(metadata)) as Record<string, unknown>;
  } catch {
    return { serialization_error: true };
  }
}

// snapshotFromState builds an immutable diagnostics snapshot with current performance aggregates.
function snapshotFromState(): CanvasDiagnosticsSnapshot {
  const metrics = exportCanvasMetrics().aggregates;

  return {
    generatedAt: new Date().toISOString(),
    topology: { ...diagnosticsState.topology },
    websocket: { ...diagnosticsState.websocket },
    graph: { ...diagnosticsState.graph },
    layout: { ...diagnosticsState.layout },
    positions: { ...diagnosticsState.positions },
    manualEdgeMigration: { ...diagnosticsState.manualEdgeMigration },
    runtime: { ...diagnosticsState.runtime },
    performance: {
      metrics,
    },
  };
}

// getCanvasDiagnosticsSnapshot returns the latest canvas diagnostics snapshot.
export function getCanvasDiagnosticsSnapshot(): CanvasDiagnosticsSnapshot {
  installCanvasDiagnosticsWindowHelpers();
  return snapshotFromState();
}

// getCanvasDiagnosticEvents returns cloned diagnostic events for subscribers and exports.
export function getCanvasDiagnosticEvents(): CanvasDiagnosticEvent[] {
  return diagnosticEvents.map((event) => ({
    ...event,
    metadata: event.metadata ? { ...event.metadata } : undefined,
  }));
}

// subscribeCanvasDiagnostics registers a listener for coalesced diagnostics updates.
export function subscribeCanvasDiagnostics(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

// updateCanvasDiagnosticsState merges partial diagnostics patches by section.
export function updateCanvasDiagnosticsState(patch: CanvasDiagnosticsPatch): void {
  diagnosticsState = {
    topology: { ...diagnosticsState.topology, ...patch.topology },
    websocket: { ...diagnosticsState.websocket, ...patch.websocket },
    graph: { ...diagnosticsState.graph, ...patch.graph },
    layout: { ...diagnosticsState.layout, ...patch.layout },
    positions: { ...diagnosticsState.positions, ...patch.positions },
    manualEdgeMigration: {
      ...diagnosticsState.manualEdgeMigration,
      ...patch.manualEdgeMigration,
    },
    runtime: { ...diagnosticsState.runtime, ...patch.runtime },
  };
  installCanvasDiagnosticsWindowHelpers();
  notifyDiagnosticsListeners();
}

// recordCanvasDiagnosticEvent appends one bounded diagnostic event with sanitized metadata.
export function recordCanvasDiagnosticEvent(event: CanvasDiagnosticEventInput): void {
  diagnosticEvents = [
    ...diagnosticEvents,
    {
      id: `event-${diagnosticEventSequence}`,
      timestamp: event.timestamp ?? new Date().toISOString(),
      level: event.level,
      source: event.source,
      event: event.event,
      message: event.message,
      metadata: sanitizeMetadata(event.metadata),
    },
  ];
  diagnosticEventSequence += 1;

  if (diagnosticEvents.length > maxCanvasDiagnosticEvents) {
    diagnosticEvents.splice(0, diagnosticEvents.length - maxCanvasDiagnosticEvents);
  }

  installCanvasDiagnosticsWindowHelpers();
  notifyDiagnosticsListeners();
}

// clearCanvasDiagnosticEvents clears event history without resetting aggregate diagnostics state.
export function clearCanvasDiagnosticEvents(): void {
  diagnosticEvents = [];
  installCanvasDiagnosticsWindowHelpers();
  notifyDiagnosticsListeners();
}

// resetCanvasDiagnostics resets state, events, and sequence counters for tests and diagnostics clear.
export function resetCanvasDiagnostics(): void {
  diagnosticsState = createInitialState();
  diagnosticEvents = [];
  diagnosticEventSequence = 0;
  installCanvasDiagnosticsWindowHelpers();
  notifyDiagnosticsListeners();
}

// exportCanvasDiagnostics packages state, events, and metrics for support downloads.
export function exportCanvasDiagnostics(): CanvasDiagnosticsExport {
  const diagnostics = getCanvasDiagnosticsSnapshot();

  return {
    version: 1,
    generatedAt: new Date().toISOString(),
    diagnostics,
    events: getCanvasDiagnosticEvents(),
    metrics: diagnostics.performance.metrics,
  };
}

installCanvasDiagnosticsWindowHelpers();
