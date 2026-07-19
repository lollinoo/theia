/**
 * Coordinates hello WebSocket lifecycle and runtime update semantics.
 * Keeps reconnect, resync, and subscription behavior isolated from canvas rendering.
 */
const canvasSchemaVersion = 1;
const runtimeProtocolVersion = 2;

/** Describes the canvas hello payload contract used by the React hook lifecycle. */
export interface CanvasHelloPayload {
  canvas_schema_version: number;
  runtime_protocol: 2;
  runtime_stream_id?: string;
  topology_version?: string;
  runtime_version?: number;
  runtime_identity?: string;
  alert_version?: number;
  subscriptions: {
    runtime: boolean;
    topology: boolean;
    alerts: boolean;
    details_device_id: string | null;
  };
}

interface BuildCanvasHelloPayloadOptions {
  topologyVersion?: string;
  hasRuntimeSnapshot: boolean;
  runtimeStreamId?: string | null;
  runtimeVersion: number | null;
  runtimeIdentity: string | null;
  alertVersion: number | null;
  detailDeviceId: string | null;
}

/** Builds canvas hello payload for the React hook lifecycle. */
export function buildCanvasHelloPayload({
  topologyVersion,
  hasRuntimeSnapshot,
  runtimeStreamId,
  runtimeVersion,
  runtimeIdentity,
  alertVersion,
  detailDeviceId,
}: BuildCanvasHelloPayloadOptions): CanvasHelloPayload {
  return {
    canvas_schema_version: canvasSchemaVersion,
    runtime_protocol: runtimeProtocolVersion,
    topology_version: topologyVersion,
    runtime_stream_id:
      hasRuntimeSnapshot && runtimeVersion !== null ? (runtimeStreamId ?? undefined) : undefined,
    runtime_version: hasRuntimeSnapshot ? (runtimeVersion ?? undefined) : undefined,
    runtime_identity: hasRuntimeSnapshot ? (runtimeIdentity ?? undefined) : undefined,
    alert_version: alertVersion ?? undefined,
    subscriptions: {
      runtime: true,
      topology: true,
      alerts: true,
      details_device_id: detailDeviceId,
    },
  };
}
