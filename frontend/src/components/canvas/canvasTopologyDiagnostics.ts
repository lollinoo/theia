import type { CanvasMeasurementTrigger } from './canvasInstrumentation';
import { recordCanvasDiagnosticEvent, updateCanvasDiagnosticsState } from './canvasDiagnostics';

export interface CanvasTopologyLoadMetadata {
  reason: CanvasMeasurementTrigger;
  silent: boolean;
  mapId: string;
  mapName: string;
}

interface CanvasTopologyLoadSucceededInput {
  metadata: CanvasTopologyLoadMetadata;
  durationMs: number;
  notModified?: boolean;
  deviceCount?: number;
  linkCount?: number;
  positionCount?: number;
  placementDeviceCount?: number;
  structureChanged?: boolean;
}

interface CanvasTopologyLoadFailedInput {
  metadata: CanvasTopologyLoadMetadata;
  durationMs: number;
  error: string;
}

function roundTopologyDurationMs(durationMs: number): number {
  return Number(Math.max(0, durationMs).toFixed(3));
}

function topologyMetadataRecord(
  metadata: CanvasTopologyLoadMetadata,
  extra: Record<string, unknown> = {},
): Record<string, unknown> {
  return {
    reason: metadata.reason,
    silent: metadata.silent,
    mapId: metadata.mapId,
    mapName: metadata.mapName,
    ...extra,
  };
}

export function recordCanvasTopologyLoadStarted(metadata: CanvasTopologyLoadMetadata): void {
  updateCanvasDiagnosticsState({
    topology: {
      lastTopologyLoadReason: metadata.reason,
      lastTopologyLoadStatus: 'loading',
      lastTopologyLoadError: undefined,
    },
  });
  recordCanvasDiagnosticEvent({
    level: 'info',
    source: 'topology',
    event: 'topology.load.started',
    message: 'Canvas topology load started',
    metadata: topologyMetadataRecord(metadata),
  });
}

export function recordCanvasTopologyLoadSucceeded({
  metadata,
  durationMs,
  notModified,
  deviceCount,
  linkCount,
  positionCount,
  placementDeviceCount,
  structureChanged,
}: CanvasTopologyLoadSucceededInput): void {
  const basePatch = {
    topology: {
      lastTopologyLoadAt: new Date().toISOString(),
      lastTopologyLoadReason: metadata.reason,
      lastTopologyLoadDurationMs: roundTopologyDurationMs(durationMs),
      lastTopologyLoadStatus: 'success' as const,
      lastTopologyLoadError: undefined,
    },
  };

  if (notModified === true) {
    updateCanvasDiagnosticsState(basePatch);
    recordCanvasDiagnosticEvent({
      level: 'info',
      source: 'topology',
      event: 'topology.load.succeeded',
      message: 'Canvas topology read model not modified',
      metadata: topologyMetadataRecord(metadata, {
        notModified: true,
      }),
    });
    return;
  }

  updateCanvasDiagnosticsState({
    ...basePatch,
    graph: {
      canonicalNodeCount: deviceCount ?? 0,
      canonicalEdgeCount: linkCount ?? 0,
    },
    layout: {
      pendingLayout: false,
    },
  });
  recordCanvasDiagnosticEvent({
    level: 'info',
    source: 'topology',
    event: 'topology.load.succeeded',
    message: 'Canvas topology load succeeded',
    metadata: topologyMetadataRecord(metadata, {
      deviceCount: deviceCount ?? 0,
      linkCount: linkCount ?? 0,
      positionCount: positionCount ?? 0,
      placementDeviceCount: placementDeviceCount ?? 0,
      structureChanged: structureChanged ?? false,
    }),
  });
}

export function recordCanvasTopologyLoadFailed({
  metadata,
  durationMs,
  error,
}: CanvasTopologyLoadFailedInput): void {
  updateCanvasDiagnosticsState({
    topology: {
      lastTopologyLoadAt: new Date().toISOString(),
      lastTopologyLoadReason: metadata.reason,
      lastTopologyLoadDurationMs: roundTopologyDurationMs(durationMs),
      lastTopologyLoadStatus: 'error',
      lastTopologyLoadError: error,
    },
    layout: {
      pendingLayout: false,
    },
  });
  recordCanvasDiagnosticEvent({
    level: 'error',
    source: 'topology',
    event: 'topology.load.failed',
    message: 'Canvas topology load failed',
    metadata: topologyMetadataRecord(metadata, {
      error,
    }),
  });
}
