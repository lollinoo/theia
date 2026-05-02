import { type CanvasMetricAggregate, exportCanvasMetrics } from './canvasInstrumentation';

export type CanvasTopologyLoadStatus = 'idle' | 'loading' | 'success' | 'error';
export type CanvasPositionSaveStatus = 'idle' | 'pending' | 'success' | 'error';
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

function flushDiagnosticsListeners(): void {
  diagnosticsNotificationTimer = undefined;
  for (const listener of Array.from(listeners)) {
    listener();
  }
}

function notifyDiagnosticsListeners(): void {
  if (listeners.size === 0 || diagnosticsNotificationTimer !== undefined) {
    return;
  }

  diagnosticsNotificationTimer = setTimeout(flushDiagnosticsListeners, 0);
}

function installCanvasDiagnosticsWindowHelpers(): void {
  if (typeof window === 'undefined') {
    return;
  }

  window.__THEIA_CANVAS_DIAGNOSTICS__ = getCanvasDiagnosticsSnapshot;
  window.__THEIA_CANVAS_DIAGNOSTICS_EXPORT__ = exportCanvasDiagnostics;
  window.__THEIA_CANVAS_DIAGNOSTICS_CLEAR_EVENTS__ = clearCanvasDiagnosticEvents;
  window.__THEIA_CANVAS_DIAGNOSTIC_EVENTS__ = diagnosticEvents;
}

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

function snapshotFromState(): CanvasDiagnosticsSnapshot {
  const metrics = exportCanvasMetrics().aggregates;

  return {
    generatedAt: new Date().toISOString(),
    topology: { ...diagnosticsState.topology },
    websocket: { ...diagnosticsState.websocket },
    graph: { ...diagnosticsState.graph },
    layout: { ...diagnosticsState.layout },
    positions: { ...diagnosticsState.positions },
    runtime: { ...diagnosticsState.runtime },
    performance: {
      metrics,
    },
  };
}

export function getCanvasDiagnosticsSnapshot(): CanvasDiagnosticsSnapshot {
  installCanvasDiagnosticsWindowHelpers();
  return snapshotFromState();
}

export function getCanvasDiagnosticEvents(): CanvasDiagnosticEvent[] {
  return diagnosticEvents.map((event) => ({
    ...event,
    metadata: event.metadata ? { ...event.metadata } : undefined,
  }));
}

export function subscribeCanvasDiagnostics(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function updateCanvasDiagnosticsState(patch: CanvasDiagnosticsPatch): void {
  diagnosticsState = {
    topology: { ...diagnosticsState.topology, ...patch.topology },
    websocket: { ...diagnosticsState.websocket, ...patch.websocket },
    graph: { ...diagnosticsState.graph, ...patch.graph },
    layout: { ...diagnosticsState.layout, ...patch.layout },
    positions: { ...diagnosticsState.positions, ...patch.positions },
    runtime: { ...diagnosticsState.runtime, ...patch.runtime },
  };
  installCanvasDiagnosticsWindowHelpers();
  notifyDiagnosticsListeners();
}

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

export function clearCanvasDiagnosticEvents(): void {
  diagnosticEvents = [];
  installCanvasDiagnosticsWindowHelpers();
  notifyDiagnosticsListeners();
}

export function resetCanvasDiagnostics(): void {
  diagnosticsState = createInitialState();
  diagnosticEvents = [];
  diagnosticEventSequence = 0;
  installCanvasDiagnosticsWindowHelpers();
  notifyDiagnosticsListeners();
}

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
