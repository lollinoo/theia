/**
 * Coordinates web socket WebSocket lifecycle and runtime update semantics.
 * Keeps reconnect, resync, and subscription behavior isolated from canvas rendering.
 */
import { useEffect, useRef, useState } from 'react';
import { fetchRuntimeOverview } from '../api/runtime';
import {
  getCanvasDiagnosticsSnapshot,
  incrementCanvasDiagnosticCount,
  recordCanvasDiagnosticEvent,
  updateCanvasDiagnosticsState,
} from '../components/canvas/canvasDiagnostics';
import { BACKEND_SESSION_CHECK_REQUIRED_EVENT } from '../events/backend';
import {
  type AlertDTO,
  type AlertWSMessage,
  mergeRuntimeDeltaPatch,
  mergeSnapshotDelta,
  type PollingHealthPayload,
  type PrometheusStatusPayload,
  parseWSMessage,
  type ReadyWSMessage,
  type ResyncRequiredPayload,
  type ResyncRequiredWSMessage,
  type RuntimeDeltaWSMessage,
  type RuntimeReplayWSMessage,
  type SnapshotDeltaWSMessage,
  type SnapshotPayload,
  type SnapshotWSMessage,
  type TopologyChangedWSMessage,
} from '../types/metrics';
import {
  getCanvasRuntimeBootstrap,
  subscribeCanvasRuntimeBootstrap,
} from './canvasRuntimeBootstrap';
import {
  buildRuntimeRecoveryDiagnosticMetadata,
  dispatchBackendResyncRequired,
  getRawWebSocketMessageType,
  recordIgnoredStaleRuntimeDelta,
  recordIgnoredStaleRuntimeSnapshot,
} from './websocket/diagnostics';
import { buildCanvasHelloPayload, type CanvasHelloPayload } from './websocket/hello';
import { createRuntimeAckScheduler, type RuntimeAckScheduler } from './websocket/runtimeAck';
import {
  advanceRuntimeRecoveryDeadline,
  applyRuntimeRecoveryReady,
  applyRuntimeRecoverySnapshot,
  beginRuntimeRecovery,
  classifyRuntimeReplay,
  createRuntimeRecoveryState,
  failRuntimeRecovery,
  RUNTIME_RECOVERY_DEADLINE_MS,
  type RuntimeCursor,
  type RuntimeRecoveryState,
} from './websocket/runtimeRecovery';
import { classifyRuntimeDelta, shouldIgnoreStaleRuntimeSnapshot } from './websocket/runtimeState';
import { appendHelloQueryParams, buildWebSocketURL } from './websocket/url';

/** Public runtime stream state consumed by App and canvas/dashboard views. */
interface UseWebSocketResult {
  snapshot: SnapshotPayload | null;
  alerts: AlertDTO[];
  connected: boolean;
  reconnecting: boolean;
  prometheusStatus: PrometheusStatusPayload | null;
  pollingHealth: PollingHealthPayload | null;
}

/** Options that coordinate HTTP runtime bootstrap and canvas interaction-paused rendering. */
interface UseWebSocketOptions {
  requireRuntimeBootstrap?: boolean;
  runtimeUpdatesPaused?: boolean;
}

/** Client control messages for per-device detail subscriptions on the shared socket. */
type DetailControlType = 'subscribe_detail' | 'unsubscribe_detail';

/**
 * Maintains the canvas WebSocket connection, runtime snapshot versioning, and alert/polling health state.
 * The hook rejects stale runtime deltas, requests resync when versions diverge, and buffers UI updates while
 * canvas interaction is paused so background updates do not fight drag/selection gestures.
 */
export function useWebSocket(
  url: string,
  detailDeviceId: string | null = null,
  options: UseWebSocketOptions = {},
): UseWebSocketResult {
  const requireRuntimeBootstrap = options.requireRuntimeBootstrap === true;
  const runtimeUpdatesPaused = options.runtimeUpdatesPaused === true;
  const initialRuntimeBootstrap = requireRuntimeBootstrap ? getCanvasRuntimeBootstrap() : null;
  const [snapshot, setSnapshot] = useState<SnapshotPayload | null>(
    initialRuntimeBootstrap?.snapshot ?? null,
  );
  const [alerts, setAlerts] = useState<AlertDTO[]>([]);
  const [connected, setConnected] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);
  const [prometheusStatus, setPrometheusStatus] = useState<PrometheusStatusPayload | null>(null);
  const [pollingHealth, setPollingHealth] = useState<PollingHealthPayload | null>(null);
  const [runtimeBootstrapReady, setRuntimeBootstrapReady] = useState(
    !requireRuntimeBootstrap || initialRuntimeBootstrap !== null,
  );

  const socketRef = useRef<WebSocket | null>(null);
  const detailDeviceIdRef = useRef<string | null>(detailDeviceId);
  const lastSubscribedDeviceIdRef = useRef<string | null>(null);
  const hasRuntimeSnapshotRef = useRef(initialRuntimeBootstrap !== null);
  const runtimeUpdatesPausedRef = useRef(runtimeUpdatesPaused);
  const snapshotStateRef = useRef<SnapshotPayload | null>(
    initialRuntimeBootstrap?.snapshot ?? null,
  );
  const pendingSnapshotFlushRef = useRef(false);
  const pollingHealthStateRef = useRef<PollingHealthPayload | null>(null);
  const pendingPollingHealthFlushRef = useRef(false);
  const snapshotVersionRef = useRef<number | null>(initialRuntimeBootstrap?.runtimeVersion ?? null);
  const runtimeIdentityRef = useRef<string | null>(
    initialRuntimeBootstrap?.runtimeIdentity ?? null,
  );
  const runtimeStreamIdRef = useRef<string | null>(
    initialRuntimeBootstrap?.runtimeStreamId ?? null,
  );
  const alertVersionRef = useRef<number | null>(null);
  const runtimeRecoveryStateRef = useRef<RuntimeRecoveryState>(createRuntimeRecoveryState());
  const runtimeAckSchedulerRef = useRef<RuntimeAckScheduler | null>(null);
  const awaitingLegacyRuntimeBootstrapRef = useRef(false);
  const runtimeRecoveryModeRef = useRef<'current' | 'replay' | 'snapshot' | 'http-fallback' | null>(
    null,
  );
  const runtimeRecoveryCountRef = useRef(0);
  const runtimeReplayRecoveryCountRef = useRef(0);
  const runtimeSnapshotRecoveryCountRef = useRef(0);
  const runtimeHttpFallbackCountRef = useRef(0);
  const runtimeRecoveryFailureCountRef = useRef(0);

  const reconnectAttemptRef = useRef(0);
  const reconnectCountRef = useRef(0);
  const resyncRequiredCountRef = useRef(0);
  const topologyChangedCountRef = useRef(0);
  const disposed = useRef(false);

  /** Builds the hello payload from diagnostics and the last applied runtime/alert versions. */
  function buildHelloPayload(): CanvasHelloPayload {
    const diagnostics = getCanvasDiagnosticsSnapshot();
    const cursor = getAppliedRuntimeCursor();
    return buildCanvasHelloPayload({
      topologyVersion: diagnostics.topology.topologyVersion,
      hasRuntimeSnapshot: hasRuntimeSnapshotRef.current,
      runtimeStreamId: cursor?.streamId,
      runtimeVersion: cursor?.version ?? null,
      runtimeIdentity: runtimeIdentityRef.current,
      alertVersion: alertVersionRef.current,
      detailDeviceId: detailDeviceIdRef.current,
    });
  }

  /** Sends hello only on open sockets; reconnects and bootstrap changes call this to avoid stale deltas. */
  function sendHello(socket: WebSocket | null = socketRef.current): void {
    if (socket?.readyState !== WebSocket.OPEN) {
      return;
    }

    socket.send(
      JSON.stringify({
        type: 'hello',
        payload: buildHelloPayload(),
      }),
    );
  }

  /** Publishes runtime state immediately or defers it until canvas interaction pause ends. */
  function publishRuntimeSnapshot(nextSnapshot: SnapshotPayload | null): void {
    snapshotStateRef.current = nextSnapshot;
    if (runtimeUpdatesPausedRef.current) {
      pendingSnapshotFlushRef.current = true;
      return;
    }

    pendingSnapshotFlushRef.current = false;
    setSnapshot(nextSnapshot);
  }

  /** Publishes polling health immediately or defers it alongside runtime snapshots during canvas interaction. */
  function publishPollingHealth(nextPollingHealth: PollingHealthPayload | null): void {
    pollingHealthStateRef.current = nextPollingHealth;
    if (runtimeUpdatesPausedRef.current) {
      pendingPollingHealthFlushRef.current = true;
      return;
    }

    pendingPollingHealthFlushRef.current = false;
    setPollingHealth(nextPollingHealth);
  }

  /** Clears alert version state when reconnecting so the server can send a fresh alert baseline. */
  function resetAlertState() {
    alertVersionRef.current = null;
    setAlerts([]);
  }

  /** Returns the last fully applied protocol-v2 cursor when both lineage fields are valid. */
  function getAppliedRuntimeCursor(): RuntimeCursor | null {
    const streamId = runtimeStreamIdRef.current;
    const version = snapshotVersionRef.current;
    if (
      !hasRuntimeSnapshotRef.current ||
      streamId === null ||
      streamId.trim().length === 0 ||
      version === null ||
      !Number.isSafeInteger(version) ||
      version < 0
    ) {
      return null;
    }
    return { streamId, version };
  }

  /** Sends detail subscription changes for the current device without opening a second socket. */
  function sendDetailControl(type: DetailControlType, deviceId: string | null): void {
    if (socketRef.current?.readyState !== WebSocket.OPEN) {
      return;
    }

    socketRef.current.send(
      JSON.stringify({
        type,
        payload: { device_id: deviceId },
      }),
    );
  }

  useEffect(() => {
    if (!requireRuntimeBootstrap) {
      return;
    }

    return subscribeCanvasRuntimeBootstrap((bootstrap) => {
      hasRuntimeSnapshotRef.current = true;
      const previousStreamId = runtimeStreamIdRef.current;
      runtimeStreamIdRef.current = bootstrap.runtimeStreamId ?? null;
      if (previousStreamId !== runtimeStreamIdRef.current) {
        runtimeAckSchedulerRef.current?.reset();
      }
      snapshotVersionRef.current = bootstrap.runtimeVersion ?? null;
      runtimeIdentityRef.current = bootstrap.runtimeIdentity ?? null;
      runtimeRecoveryStateRef.current = {
        phase: 'idle',
        generation: runtimeRecoveryStateRef.current.generation,
      };
      awaitingLegacyRuntimeBootstrapRef.current = false;
      updateCanvasDiagnosticsState({
        websocket: {
          runtimeRecoveryPhase: 'idle',
        },
      });
      publishRuntimeSnapshot(bootstrap.snapshot);
      setRuntimeBootstrapReady(true);
      updateCanvasDiagnosticsState({
        websocket: {
          lastAppliedSnapshotVersion:
            bootstrap.runtimeVersion === undefined ? undefined : String(bootstrap.runtimeVersion),
          ...(bootstrap.runtimeIdentity !== undefined
            ? { lastAppliedRuntimeIdentity: bootstrap.runtimeIdentity }
            : {}),
          runtimeStreamId: bootstrap.runtimeStreamId,
          lastRejectedDeltaReason: undefined,
        },
      });
      recordCanvasDiagnosticEvent({
        level: 'info',
        source: 'runtime',
        event: 'runtime.snapshot.applied',
        message: 'Runtime snapshot applied from HTTP canvas bootstrap',
        metadata: {
          version: bootstrap.runtimeVersion,
          deviceCount: Object.keys(bootstrap.snapshot.devices).length,
          linkCount: Object.keys(bootstrap.snapshot.links).length,
          runtimeIdentity: bootstrap.runtimeIdentity,
        },
      });
      sendHello();
    });
  }, [requireRuntimeBootstrap]);

  useEffect(() => {
    runtimeUpdatesPausedRef.current = runtimeUpdatesPaused;
    if (runtimeUpdatesPaused) {
      return;
    }

    if (pendingSnapshotFlushRef.current) {
      pendingSnapshotFlushRef.current = false;
      setSnapshot(snapshotStateRef.current);
      recordCanvasDiagnosticEvent({
        level: 'debug',
        source: 'runtime',
        event: 'runtime.snapshot.flushed',
        message: 'Deferred runtime snapshot state flushed after canvas interaction',
        metadata: {
          version: snapshotVersionRef.current,
        },
      });
    }

    if (pendingPollingHealthFlushRef.current) {
      pendingPollingHealthFlushRef.current = false;
      setPollingHealth(pollingHealthStateRef.current);
    }
  }, [runtimeUpdatesPaused]);

  useEffect(() => {
    if (!runtimeBootstrapReady) {
      return;
    }

    disposed.current = false;
    let reconnectTimer: number | null = null;
    let runtimeRecoveryDeadlineTimer: number | null = null;
    let activeRuntimeFallback: { generation: number; socket: WebSocket } | null = null;

    function clearReconnectTimer() {
      if (reconnectTimer !== null) {
        window.clearTimeout(reconnectTimer);
        reconnectTimer = null;
      }
    }

    function clearRuntimeRecoveryDeadline() {
      if (runtimeRecoveryDeadlineTimer !== null) {
        window.clearTimeout(runtimeRecoveryDeadlineTimer);
        runtimeRecoveryDeadlineTimer = null;
      }
    }

    /** Schedules exponential reconnect and clears alert baselines so the next connection can resync them. */
    function scheduleReconnect() {
      if (disposed.current || reconnectTimer !== null) return;

      resetAlertState();
      setReconnecting(true);
      const delay = Math.min(1_000 * 2 ** reconnectAttemptRef.current, 30_000);

      reconnectTimer = window.setTimeout(() => {
        reconnectTimer = null;
        reconnectAttemptRef.current += 1;
        connect();
      }, delay);
    }

    /** Opens one socket for overview runtime state and optional detail-device subscriptions. */
    function connect() {
      if (disposed.current) return;

      clearReconnectTimer();
      clearRuntimeRecoveryDeadline();

      // Close any existing socket before opening a new one
      if (socketRef.current) {
        socketRef.current.onopen = null;
        socketRef.current.onmessage = null;
        socketRef.current.onerror = null;
        socketRef.current.onclose = null;
        socketRef.current.close();
        socketRef.current = null;
      }

      resetAlertState();
      runtimeRecoveryStateRef.current = {
        phase: 'idle',
        generation: runtimeRecoveryStateRef.current.generation,
      };
      runtimeRecoveryModeRef.current = null;
      updateCanvasDiagnosticsState({
        websocket: {
          runtimeRecoveryPhase: 'idle',
        },
      });
      const initialHelloPayload = buildHelloPayload();
      const ws = new WebSocket(appendHelloQueryParams(buildWebSocketURL(url), initialHelloPayload));
      socketRef.current = ws;

      runtimeAckSchedulerRef.current?.cancel();
      const runtimeAckScheduler = createRuntimeAckScheduler({
        send(control) {
          ws.send(JSON.stringify(control));
          updateCanvasDiagnosticsState({
            websocket: {
              runtimeStreamId: control.payload.runtime_stream_id,
              lastRuntimeAckVersion: String(control.payload.runtime_version),
            },
          });
        },
      });
      runtimeAckSchedulerRef.current = runtimeAckScheduler;

      /** Applies the compact HTTP snapshot only while its socket and recovery generation are current. */
      async function runRuntimeFallback(generation: number): Promise<void> {
        if (
          activeRuntimeFallback?.socket === ws &&
          activeRuntimeFallback.generation === generation
        ) {
          return;
        }
        const fallbackToken = { generation, socket: ws };
        activeRuntimeFallback = fallbackToken;

        try {
          const response = await fetchRuntimeOverview();
          const recoveryState = runtimeRecoveryStateRef.current;
          if (
            disposed.current ||
            socketRef.current !== ws ||
            recoveryState.phase !== 'http-fallback' ||
            recoveryState.generation !== generation
          ) {
            return;
          }

          const currentCursor = getAppliedRuntimeCursor();
          if (
            currentCursor !== null &&
            currentCursor.streamId === response.runtime_stream_id &&
            response.runtime_version < currentCursor.version
          ) {
            throw new Error('runtime fallback cursor regressed');
          }

          if (runtimeStreamIdRef.current !== response.runtime_stream_id) {
            runtimeAckScheduler.reset();
          }
          hasRuntimeSnapshotRef.current = true;
          runtimeStreamIdRef.current = response.runtime_stream_id;
          snapshotVersionRef.current = response.runtime_version;
          runtimeIdentityRef.current = response.runtime_identity;
          const cursor = getAppliedRuntimeCursor();
          if (cursor === null) {
            throw new Error('invalid runtime fallback cursor');
          }

          const appliedSnapshot = applyRuntimeRecoverySnapshot(recoveryState, cursor);
          runtimeRecoveryStateRef.current = appliedSnapshot.state;
          runtimeRecoveryModeRef.current = 'http-fallback';
          updateCanvasDiagnosticsState({
            websocket: {
              lastAppliedSnapshotVersion: String(cursor.version),
              lastAppliedRuntimeIdentity: response.runtime_identity,
              runtimeStreamId: cursor.streamId,
              runtimeRecoveryPhase: appliedSnapshot.state.phase,
              lastRejectedDeltaReason: undefined,
            },
          });
          recordCanvasDiagnosticEvent({
            level: 'warn',
            source: 'runtime',
            event: 'runtime.recovery.http_snapshot_applied',
            message: 'Runtime-only HTTP recovery snapshot applied',
            metadata: {
              generation,
              version: cursor.version,
              deviceCount: Object.keys(response.runtime_snapshot.devices).length,
              linkCount: Object.keys(response.runtime_snapshot.links).length,
            },
          });
          publishRuntimeSnapshot(response.runtime_snapshot);
          runtimeAckScheduler.schedule(cursor);
          ws.send(
            JSON.stringify({
              type: 'resume_runtime',
              payload: {
                runtime_stream_id: cursor.streamId,
                runtime_version: cursor.version,
              },
            }),
          );
        } catch (error) {
          const recoveryState = runtimeRecoveryStateRef.current;
          if (
            disposed.current ||
            socketRef.current !== ws ||
            (recoveryState.phase !== 'stream' && recoveryState.phase !== 'http-fallback') ||
            recoveryState.generation !== generation
          ) {
            return;
          }

          const recoveryMetadata = buildRuntimeRecoveryDiagnosticMetadata(
            recoveryState,
            Date.now(),
          );
          runtimeRecoveryStateRef.current = failRuntimeRecovery(
            recoveryState,
            error instanceof Error ? error.message : 'runtime fallback failed',
          );
          runtimeRecoveryFailureCountRef.current = incrementCanvasDiagnosticCount(
            runtimeRecoveryFailureCountRef.current,
          );
          updateCanvasDiagnosticsState({
            websocket: {
              runtimeRecoveryPhase: 'failed',
              lastRuntimeRecoveryMode: 'http-fallback',
              lastRuntimeRecoveryDurationMs:
                recoveryMetadata.phase === 'stream' || recoveryMetadata.phase === 'http-fallback'
                  ? recoveryMetadata.durationMs
                  : undefined,
              runtimeRecoveryFailureCount: runtimeRecoveryFailureCountRef.current,
            },
          });
          recordCanvasDiagnosticEvent({
            level: 'error',
            source: 'runtime',
            event: 'runtime.recovery.failed',
            message: 'Runtime stream recovery failed; reconnecting the WebSocket',
            metadata: {
              generation,
              reason: error instanceof Error ? error.message : 'runtime fallback failed',
            },
          });
          runtimeAckScheduler.cancel();
          ws.close();
        } finally {
          if (activeRuntimeFallback === fallbackToken) {
            activeRuntimeFallback = null;
          }
        }
      }

      /** Arms the single fixed recovery deadline for the active stream generation. */
      function scheduleRuntimeRecoveryDeadline(state: RuntimeRecoveryState): void {
        if (state.phase !== 'stream' || runtimeRecoveryDeadlineTimer !== null) {
          return;
        }
        const remainingMs = Math.max(
          0,
          state.startedAt + RUNTIME_RECOVERY_DEADLINE_MS - Date.now(),
        );
        const generation = state.generation;
        runtimeRecoveryDeadlineTimer = window.setTimeout(() => {
          runtimeRecoveryDeadlineTimer = null;
          const currentState = runtimeRecoveryStateRef.current;
          if (
            disposed.current ||
            socketRef.current !== ws ||
            currentState.phase !== 'stream' ||
            currentState.generation !== generation
          ) {
            return;
          }

          const fallbackState = advanceRuntimeRecoveryDeadline(currentState, Date.now());
          if (fallbackState.phase !== 'http-fallback') {
            scheduleRuntimeRecoveryDeadline(currentState);
            return;
          }

          runtimeRecoveryStateRef.current = fallbackState;
          runtimeHttpFallbackCountRef.current = incrementCanvasDiagnosticCount(
            runtimeHttpFallbackCountRef.current,
          );
          updateCanvasDiagnosticsState({
            websocket: {
              runtimeRecoveryPhase: 'http-fallback',
              runtimeHttpFallbackCount: runtimeHttpFallbackCountRef.current,
            },
          });
          void runRuntimeFallback(generation);
        }, remainingMs);
      }

      /** Records a rejected runtime base and triggers the HTTP bootstrap/resync path. */
      function requestLegacyResync(
        payloadReason: ResyncRequiredPayload['reason'] = 'client_resync_scheduled',
        diagnosticReason = 'base_version_mismatch',
      ): void {
        if (awaitingLegacyRuntimeBootstrapRef.current) {
          return;
        }
        awaitingLegacyRuntimeBootstrapRef.current = true;
        resyncRequiredCountRef.current += 1;
        updateCanvasDiagnosticsState({
          websocket: {
            resyncRequiredCount: resyncRequiredCountRef.current,
            lastRejectedDeltaReason: diagnosticReason,
          },
        });
        recordCanvasDiagnosticEvent({
          level: 'warn',
          source: 'runtime',
          event: 'runtime.delta.rejected',
          message: 'Runtime delta rejected because the client runtime base is not usable',
          metadata: {
            reason: diagnosticReason,
          },
        });
        resetAlertState();
        dispatchBackendResyncRequired({
          scope: 'overview',
          reason: payloadReason,
        });
        if (!requireRuntimeBootstrap) {
          ws.close();
        }
      }

      /** Enters one protocol-v2 recovery generation and optionally asks the server to resume it. */
      function requestRuntimeRecovery(
        reason: string,
        targetVersion: number | null,
        sendResume: boolean,
      ): void {
        const previousState = runtimeRecoveryStateRef.current;
        const nextState = beginRuntimeRecovery(previousState, {
          now: Date.now(),
          reason,
          targetVersion,
        });
        runtimeRecoveryStateRef.current = nextState;
        scheduleRuntimeRecoveryDeadline(nextState);
        if (previousState.phase === 'idle' && nextState.phase === 'stream') {
          runtimeRecoveryModeRef.current = null;
        }
        if (previousState.phase !== 'idle' || nextState.phase !== 'stream') {
          if (nextState.phase === 'stream') {
            updateCanvasDiagnosticsState({
              websocket: {
                runtimeRecoveryTargetVersion:
                  nextState.targetVersion === null ? undefined : String(nextState.targetVersion),
              },
            });
          }
          return;
        }

        resyncRequiredCountRef.current += 1;
        runtimeRecoveryCountRef.current = incrementCanvasDiagnosticCount(
          runtimeRecoveryCountRef.current,
        );
        updateCanvasDiagnosticsState({
          websocket: {
            resyncRequiredCount: resyncRequiredCountRef.current,
            lastRejectedDeltaReason: reason,
            runtimeRecoveryPhase: 'stream',
            runtimeRecoveryTargetVersion:
              nextState.targetVersion === null ? undefined : String(nextState.targetVersion),
            runtimeRecoveryCount: runtimeRecoveryCountRef.current,
          },
        });
        if (!sendResume) {
          return;
        }

        const cursor = getAppliedRuntimeCursor();
        if (cursor === null) {
          return;
        }
        ws.send(
          JSON.stringify({
            type: 'resume_runtime',
            payload: {
              runtime_stream_id: cursor.streamId,
              runtime_version: cursor.version,
            },
          }),
        );
      }

      ws.onopen = () => {
        if (disposed.current) {
          ws.close();
          return;
        }
        const wasReconnect = reconnectAttemptRef.current > 0;
        if (wasReconnect) {
          reconnectCountRef.current += 1;
        }
        reconnectAttemptRef.current = 0;
        setConnected(true);
        setReconnecting(false);
        updateCanvasDiagnosticsState({
          websocket: {
            connected: true,
            reconnectCount: reconnectCountRef.current,
          },
        });
        recordCanvasDiagnosticEvent({
          level: 'info',
          source: 'websocket',
          event: wasReconnect ? 'websocket.reconnected' : 'websocket.connected',
          message: wasReconnect ? 'Canvas WebSocket reconnected' : 'Canvas WebSocket connected',
        });
        sendHello(ws);
        if (wasReconnect) {
          window.dispatchEvent(new Event('backend-reconnected'));
        }
        if (detailDeviceIdRef.current !== null) {
          sendDetailControl('subscribe_detail', detailDeviceIdRef.current);
          lastSubscribedDeviceIdRef.current = detailDeviceIdRef.current;
        }
      };

      ws.onmessage = (event: MessageEvent<string>) => {
        let raw: unknown;
        try {
          raw = JSON.parse(event.data) as unknown;
          const message = parseWSMessage(raw);
          const messageAt = new Date().toISOString();
          updateCanvasDiagnosticsState({
            websocket: {
              lastMessageAt: messageAt,
              lastMessageType: message.type,
            },
          });

          if (message.type === 'ready') {
            const payload = (message as ReadyWSMessage).payload;
            if (!hasRuntimeSnapshotRef.current) {
              if (payload.runtime_stream_id !== undefined) {
                requestRuntimeRecovery(
                  'missing_base_snapshot',
                  payload.runtime_version ?? null,
                  true,
                );
              } else {
                requestLegacyResync('client_missing_runtime_snapshot', 'missing_base_snapshot');
              }
              return;
            }
            const downgradeCandidate =
              runtimeRecoveryStateRef.current.phase === 'idle' &&
              runtimeStreamIdRef.current !== null &&
              payload.runtime_stream_id === undefined;
            const hasV2Cursor =
              !downgradeCandidate &&
              (runtimeStreamIdRef.current !== null || payload.runtime_stream_id !== undefined);
            if (hasV2Cursor) {
              const readyCursor =
                payload.runtime_stream_id !== undefined && payload.runtime_version !== undefined
                  ? {
                      streamId: payload.runtime_stream_id,
                      version: payload.runtime_version,
                    }
                  : null;
              const readyDecision = applyRuntimeRecoveryReady(
                runtimeRecoveryStateRef.current,
                getAppliedRuntimeCursor(),
                readyCursor,
              );
              if (readyDecision.kind === 'reject') {
                if (runtimeRecoveryStateRef.current.phase === 'idle') {
                  requestRuntimeRecovery(
                    readyDecision.reason,
                    payload.runtime_version ?? null,
                    true,
                  );
                }
                return;
              }
              const recoveryStateBeforeReady = runtimeRecoveryStateRef.current;
              runtimeRecoveryStateRef.current = readyDecision.state;
              awaitingLegacyRuntimeBootstrapRef.current = false;
              clearRuntimeRecoveryDeadline();
              if (payload.runtime_identity !== undefined) {
                runtimeIdentityRef.current = payload.runtime_identity;
              }
              if (payload.alert_version !== undefined) {
                alertVersionRef.current = payload.alert_version;
              }
              runtimeAckScheduler.schedule(readyDecision.cursor);
              runtimeAckScheduler.flush();
              const recoveryMetadata = buildRuntimeRecoveryDiagnosticMetadata(
                recoveryStateBeforeReady,
                Date.now(),
              );
              if (
                recoveryStateBeforeReady.phase === 'stream' ||
                recoveryStateBeforeReady.phase === 'http-fallback'
              ) {
                if (runtimeRecoveryModeRef.current === null) {
                  if (
                    payload.sync_mode === 'current' ||
                    payload.sync_mode === 'replay' ||
                    payload.sync_mode === 'snapshot'
                  ) {
                    runtimeRecoveryModeRef.current = payload.sync_mode;
                  }
                }
                const completedMode = runtimeRecoveryModeRef.current;
                if (completedMode === 'replay') {
                  runtimeReplayRecoveryCountRef.current = incrementCanvasDiagnosticCount(
                    runtimeReplayRecoveryCountRef.current,
                  );
                } else if (completedMode === 'snapshot') {
                  runtimeSnapshotRecoveryCountRef.current = incrementCanvasDiagnosticCount(
                    runtimeSnapshotRecoveryCountRef.current,
                  );
                }
                updateCanvasDiagnosticsState({
                  websocket: {
                    runtimeRecoveryPhase: readyDecision.state.phase,
                    ...(completedMode !== null ? { lastRuntimeRecoveryMode: completedMode } : {}),
                    lastRuntimeRecoveryDurationMs:
                      recoveryMetadata.phase === 'stream' ||
                      recoveryMetadata.phase === 'http-fallback'
                        ? recoveryMetadata.durationMs
                        : undefined,
                    runtimeReplayRecoveryCount: runtimeReplayRecoveryCountRef.current,
                    runtimeSnapshotRecoveryCount: runtimeSnapshotRecoveryCountRef.current,
                  },
                });
              }
            } else {
              const hasExactLegacyReady =
                payload.runtime_version !== undefined &&
                Number.isSafeInteger(payload.runtime_version) &&
                payload.runtime_version >= 0 &&
                snapshotVersionRef.current !== null &&
                payload.runtime_version === snapshotVersionRef.current;
              if (!hasExactLegacyReady) {
                requestLegacyResync('client_resync_scheduled', 'ready_version_mismatch');
                return;
              }
              if (downgradeCandidate) {
                runtimeAckScheduler.reset();
                runtimeStreamIdRef.current = null;
                updateCanvasDiagnosticsState({
                  websocket: {
                    runtimeStreamId: undefined,
                  },
                });
              }
              awaitingLegacyRuntimeBootstrapRef.current = false;
              if (payload.runtime_identity !== undefined) {
                runtimeIdentityRef.current = payload.runtime_identity;
              }
              if (payload.alert_version !== undefined) {
                alertVersionRef.current = payload.alert_version;
              }
            }
            updateCanvasDiagnosticsState({
              websocket: {
                ...(payload.runtime_identity !== undefined
                  ? { lastAppliedRuntimeIdentity: payload.runtime_identity }
                  : {}),
                lastRejectedDeltaReason: undefined,
              },
            });
            recordCanvasDiagnosticEvent({
              level: 'info',
              source: 'websocket',
              event: 'websocket.ready',
              message: 'Canvas WebSocket runtime state already current',
              metadata: {
                runtimeVersion: payload.runtime_version,
                runtimeIdentity: payload.runtime_identity,
                alertVersion: payload.alert_version,
              },
            });
          } else if (message.type === 'snapshot') {
            const payload = (message as SnapshotWSMessage).payload;
            const currentVersion = snapshotVersionRef.current;
            const hasRuntimeStreamField = payload.runtime_stream_id !== undefined;
            const incomingStreamId = payload.runtime_stream_id?.trim();
            const hasIncomingRuntimeStream =
              incomingStreamId !== undefined && incomingStreamId.length > 0;
            const hasSafeIncomingVersion =
              payload.version !== null &&
              Number.isSafeInteger(payload.version) &&
              payload.version >= 0;
            const isActiveRuntimeRecovery =
              runtimeRecoveryStateRef.current.phase === 'stream' ||
              runtimeRecoveryStateRef.current.phase === 'http-fallback';
            const hasValidIncomingCursor = hasIncomingRuntimeStream && hasSafeIncomingVersion;
            if (isActiveRuntimeRecovery && !hasValidIncomingCursor) {
              updateCanvasDiagnosticsState({
                websocket: {
                  lastRejectedDeltaReason: 'invalid_snapshot_cursor',
                },
              });
              recordCanvasDiagnosticEvent({
                level: 'warn',
                source: 'runtime',
                event: 'runtime.snapshot.rejected',
                message: 'Runtime recovery snapshot rejected because its cursor is invalid',
                metadata: {
                  version: payload.version,
                  runtimeStreamId: payload.runtime_stream_id,
                },
              });
              return;
            }
            if (hasRuntimeStreamField && !hasIncomingRuntimeStream) {
              requestRuntimeRecovery('invalid_snapshot_cursor', payload.version, true);
              return;
            }
            if (hasIncomingRuntimeStream && !hasSafeIncomingVersion) {
              requestRuntimeRecovery('invalid_snapshot_cursor', null, true);
              return;
            }
            const isLegacyDowngrade =
              !isActiveRuntimeRecovery &&
              runtimeStreamIdRef.current !== null &&
              !hasIncomingRuntimeStream;
            const hasDifferentRuntimeLineage =
              (hasIncomingRuntimeStream && incomingStreamId !== runtimeStreamIdRef.current) ||
              isLegacyDowngrade;
            if (
              !hasDifferentRuntimeLineage &&
              shouldIgnoreStaleRuntimeSnapshot(
                payload.version,
                currentVersion,
                hasRuntimeSnapshotRef.current,
              )
            ) {
              recordIgnoredStaleRuntimeSnapshot({
                version: payload.version!,
                currentVersion: currentVersion!,
                runtimeIdentity: payload.runtime_identity,
              });
              return;
            }
            hasRuntimeSnapshotRef.current = true;
            awaitingLegacyRuntimeBootstrapRef.current = false;
            if (hasIncomingRuntimeStream) {
              if (runtimeStreamIdRef.current !== incomingStreamId) {
                runtimeAckScheduler.reset();
              }
              runtimeStreamIdRef.current = incomingStreamId;
            } else if (isLegacyDowngrade) {
              runtimeAckScheduler.reset();
              runtimeStreamIdRef.current = null;
            }
            snapshotVersionRef.current = payload.version;
            runtimeIdentityRef.current = payload.runtime_identity ?? null;
            const snapshotCursor = getAppliedRuntimeCursor();
            if (
              snapshotCursor !== null &&
              (runtimeRecoveryStateRef.current.phase === 'stream' ||
                runtimeRecoveryStateRef.current.phase === 'http-fallback')
            ) {
              const appliedSnapshot = applyRuntimeRecoverySnapshot(
                runtimeRecoveryStateRef.current,
                snapshotCursor,
              );
              runtimeRecoveryStateRef.current = appliedSnapshot.state;
              if (
                (appliedSnapshot.state.phase === 'stream' ||
                  appliedSnapshot.state.phase === 'http-fallback') &&
                runtimeRecoveryModeRef.current === null
              ) {
                runtimeRecoveryModeRef.current = 'snapshot';
              }
            }
            updateCanvasDiagnosticsState({
              websocket: {
                lastAppliedSnapshotVersion:
                  payload.version === null ? undefined : String(payload.version),
                ...(payload.runtime_identity !== undefined
                  ? { lastAppliedRuntimeIdentity: payload.runtime_identity }
                  : {}),
                runtimeStreamId: runtimeStreamIdRef.current ?? undefined,
                runtimeRecoveryPhase: runtimeRecoveryStateRef.current.phase,
                lastRejectedDeltaReason: undefined,
              },
            });
            recordCanvasDiagnosticEvent({
              level: 'info',
              source: 'runtime',
              event: 'runtime.snapshot.applied',
              message: 'Runtime snapshot applied',
              metadata: {
                version: payload.version,
                deviceCount: Object.keys(payload.snapshot.devices).length,
                linkCount: Object.keys(payload.snapshot.links).length,
                runtimeIdentity: payload.runtime_identity,
              },
            });
            publishRuntimeSnapshot(payload.snapshot);
            runtimeAckScheduler.schedule(getAppliedRuntimeCursor());
          } else if (message.type === 'runtime_replay') {
            const payload = (message as RuntimeReplayWSMessage).payload;
            const cursor = getAppliedRuntimeCursor();
            const currentSnapshot = snapshotStateRef.current;
            if (cursor === null || currentSnapshot === null) {
              requestRuntimeRecovery('missing_base_snapshot', payload.version, true);
              return;
            }
            const replayDecision = classifyRuntimeReplay(cursor, payload);
            if (replayDecision.kind === 'reject') {
              requestRuntimeRecovery(replayDecision.reason, payload.version, true);
              return;
            }

            snapshotVersionRef.current = replayDecision.cursor.version;
            if (
              (runtimeRecoveryStateRef.current.phase === 'stream' ||
                runtimeRecoveryStateRef.current.phase === 'http-fallback') &&
              runtimeRecoveryModeRef.current === null
            ) {
              runtimeRecoveryModeRef.current = 'replay';
            }
            const nextSnapshot = mergeRuntimeDeltaPatch(currentSnapshot, payload.delta);
            updateCanvasDiagnosticsState({
              websocket: {
                lastAppliedDeltaVersion: String(replayDecision.cursor.version),
                runtimeStreamId: replayDecision.cursor.streamId,
                runtimeRecoveryPhase: runtimeRecoveryStateRef.current.phase,
                lastRejectedDeltaReason: undefined,
              },
            });
            recordCanvasDiagnosticEvent({
              level: 'debug',
              source: 'runtime',
              event: 'runtime.replay.applied',
              message: 'Runtime replay applied',
              metadata: {
                fromVersion: payload.from_version,
                version: payload.version,
                deviceCount: Object.keys(payload.delta.devices).length,
                linkCount: Object.keys(payload.delta.links).length,
              },
            });
            publishRuntimeSnapshot(nextSnapshot);
            runtimeAckScheduler.schedule(replayDecision.cursor);
          } else if (message.type === 'snapshot_delta' || message.type === 'runtime_delta') {
            const payload =
              message.type === 'runtime_delta'
                ? (message as RuntimeDeltaWSMessage).payload
                : (message as SnapshotDeltaWSMessage).payload;
            const incomingRuntimeStreamId =
              message.type === 'runtime_delta'
                ? (payload as RuntimeDeltaWSMessage['payload']).runtime_stream_id
                : undefined;
            if (awaitingLegacyRuntimeBootstrapRef.current) {
              return;
            }
            if (runtimeRecoveryStateRef.current.phase !== 'idle') {
              requestRuntimeRecovery(
                runtimeRecoveryStateRef.current.phase === 'stream'
                  ? runtimeRecoveryStateRef.current.reason
                  : 'runtime_recovery_in_progress',
                payload.version ?? null,
                true,
              );
              return;
            }

            const deltaDecision = classifyRuntimeDelta(message.type, payload, {
              currentVersion: snapshotVersionRef.current,
              currentStreamId: runtimeStreamIdRef.current,
              hasRuntimeSnapshot: hasRuntimeSnapshotRef.current,
            });
            if (deltaDecision.kind === 'request_resync') {
              if (runtimeStreamIdRef.current !== null || incomingRuntimeStreamId !== undefined) {
                requestRuntimeRecovery(
                  deltaDecision.diagnosticReason,
                  payload.version ?? null,
                  true,
                );
              } else {
                requestLegacyResync(deltaDecision.payloadReason, deltaDecision.diagnosticReason);
              }
              return;
            }
            if (deltaDecision.kind === 'ignore_stale') {
              recordIgnoredStaleRuntimeDelta({
                messageType: deltaDecision.messageType,
                baseVersion: deltaDecision.baseVersion,
                version: deltaDecision.version,
                currentVersion: deltaDecision.currentVersion,
              });
              return;
            }
            if (deltaDecision.kind === 'reject_missing_unversioned_base') {
              recordCanvasDiagnosticEvent({
                level: 'warn',
                source: 'runtime',
                event: 'runtime.delta.rejected',
                message: 'Runtime delta rejected because no base snapshot is available',
                metadata: {
                  reason: 'missing_base_snapshot',
                },
              });
              return;
            }
            if (deltaDecision.kind === 'apply_versioned') {
              snapshotVersionRef.current = deltaDecision.nextVersion;
              if (deltaDecision.runtimeIdentity !== undefined) {
                runtimeIdentityRef.current = deltaDecision.runtimeIdentity;
              }
              updateCanvasDiagnosticsState({
                websocket: {
                  lastAppliedDeltaVersion: String(deltaDecision.nextVersion),
                  ...(deltaDecision.runtimeIdentity !== undefined
                    ? { lastAppliedRuntimeIdentity: deltaDecision.runtimeIdentity }
                    : {}),
                  lastRejectedDeltaReason: undefined,
                },
              });
            }
            recordCanvasDiagnosticEvent({
              level: 'debug',
              source: 'runtime',
              event: 'runtime.delta.applied',
              message: 'Runtime delta applied',
              metadata: {
                type: message.type,
                baseVersion: payload.base_version,
                version: payload.version,
                deviceCount: Object.keys(payload.delta.devices).length,
                linkCount: Object.keys(payload.delta.links).length,
                runtimeIdentity: payload.runtime_identity,
              },
            });
            const currentSnapshot = snapshotStateRef.current;
            if (currentSnapshot === null) {
              return;
            }
            const nextSnapshot =
              message.type === 'runtime_delta'
                ? mergeRuntimeDeltaPatch(
                    currentSnapshot,
                    (payload as RuntimeDeltaWSMessage['payload']).delta,
                  )
                : mergeSnapshotDelta(
                    currentSnapshot,
                    (payload as SnapshotDeltaWSMessage['payload']).delta,
                  );
            publishRuntimeSnapshot(nextSnapshot);
            if (deltaDecision.kind === 'apply_versioned') {
              runtimeAckScheduler.schedule(getAppliedRuntimeCursor());
            }
          } else if (message.type === 'prometheus_status') {
            const payload = message.payload as PrometheusStatusPayload;
            setPrometheusStatus(payload);
            updateCanvasDiagnosticsState({
              runtime: {
                prometheusStatus:
                  payload.enabled === false
                    ? 'disabled'
                    : payload.available
                      ? 'available'
                      : 'unavailable',
                prometheusError: payload.error,
              },
            });
          } else if (message.type === 'polling_health_changed') {
            publishPollingHealth(message.payload as PollingHealthPayload);
          } else if (message.type === 'alert') {
            const payload = (message as AlertWSMessage).payload;
            if (
              payload.version !== undefined &&
              alertVersionRef.current !== null &&
              payload.version < alertVersionRef.current
            ) {
              return;
            }
            if (payload.version !== undefined) {
              alertVersionRef.current = payload.version;
            }
            setAlerts(payload.alerts);
          } else if (message.type === 'resync_required') {
            const payload = (message as ResyncRequiredWSMessage).payload;
            if (payload.strategy === 'stream') {
              requestRuntimeRecovery(payload.reason, payload.target_version ?? null, false);
              recordCanvasDiagnosticEvent({
                level: 'warn',
                source: 'websocket',
                event: 'websocket.resync_required',
                message: 'Backend scheduled runtime stream recovery',
                metadata: { ...payload },
              });
              return;
            }
            awaitingLegacyRuntimeBootstrapRef.current = true;
            resyncRequiredCountRef.current += 1;
            updateCanvasDiagnosticsState({
              websocket: {
                resyncRequiredCount: resyncRequiredCountRef.current,
              },
            });
            recordCanvasDiagnosticEvent({
              level: 'warn',
              source: 'websocket',
              event: 'websocket.resync_required',
              message: 'Backend requested a canvas runtime resync',
              metadata: { ...payload },
            });
            resetAlertState();
            dispatchBackendResyncRequired(payload);
          } else if (message.type === 'topology_changed' || message.type === 'topology_delta') {
            const topologyPayload =
              message.type === 'topology_changed'
                ? (message as TopologyChangedWSMessage).payload
                : null;
            topologyChangedCountRef.current += 1;
            updateCanvasDiagnosticsState({
              websocket: {
                topologyChangedCount: topologyChangedCountRef.current,
              },
            });
            recordCanvasDiagnosticEvent({
              level: 'info',
              source: 'topology',
              event: 'topology.changed.received',
              message: 'Topology change notification received',
              metadata: {
                type: message.type,
                ...(topologyPayload ?? {}),
              },
            });
            window.dispatchEvent(
              new CustomEvent('topology-changed', {
                detail: topologyPayload,
              }),
            );
          }
        } catch (error) {
          console.error('Failed to parse WebSocket message', error);
          const messageType = getRawWebSocketMessageType(raw);
          const rawPayload =
            raw !== null &&
            typeof raw === 'object' &&
            'payload' in raw &&
            raw.payload !== null &&
            typeof raw.payload === 'object'
              ? (raw.payload as Record<string, unknown>)
              : null;
          const rawRuntimeStreamId = rawPayload?.runtime_stream_id;
          const hasProtocolV2Lineage =
            getAppliedRuntimeCursor() !== null ||
            (typeof rawRuntimeStreamId === 'string' && rawRuntimeStreamId.trim().length > 0);
          if (
            hasProtocolV2Lineage &&
            (messageType === 'runtime_delta' || messageType === 'runtime_replay')
          ) {
            requestRuntimeRecovery(
              'invalid_runtime_delta_payload',
              typeof rawPayload?.version === 'number' ? rawPayload.version : null,
              true,
            );
          } else if (messageType === 'runtime_delta' || messageType === 'snapshot_delta') {
            requestLegacyResync('client_resync_scheduled', 'invalid_runtime_delta_payload');
          }
        }
      };

      ws.onerror = () => {
        ws.close();
      };

      ws.onclose = () => {
        clearRuntimeRecoveryDeadline();
        runtimeAckScheduler.cancel();
        if (socketRef.current === ws) {
          socketRef.current = null;
        }
        if (disposed.current) return;
        runtimeRecoveryStateRef.current = {
          phase: 'idle',
          generation: runtimeRecoveryStateRef.current.generation,
        };
        runtimeRecoveryModeRef.current = null;
        window.dispatchEvent(new Event(BACKEND_SESSION_CHECK_REQUIRED_EVENT));
        setConnected(false);
        updateCanvasDiagnosticsState({
          websocket: {
            connected: false,
          },
        });
        recordCanvasDiagnosticEvent({
          level: 'warn',
          source: 'websocket',
          event: 'websocket.disconnected',
          message: 'Canvas WebSocket disconnected',
          metadata: {
            reconnectCount: getCanvasDiagnosticsSnapshot().websocket.reconnectCount,
          },
        });
        scheduleReconnect();
      };
    }

    connect();

    return () => {
      disposed.current = true;
      clearReconnectTimer();
      clearRuntimeRecoveryDeadline();
      setConnected(false);
      setReconnecting(false);
      hasRuntimeSnapshotRef.current = false;
      snapshotStateRef.current = null;
      pendingSnapshotFlushRef.current = false;
      pollingHealthStateRef.current = null;
      pendingPollingHealthFlushRef.current = false;
      snapshotVersionRef.current = null;
      runtimeStreamIdRef.current = null;
      runtimeIdentityRef.current = null;
      updateCanvasDiagnosticsState({
        websocket: {
          connected: false,
        },
      });
      resetAlertState();
      runtimeAckSchedulerRef.current?.cancel();
      runtimeAckSchedulerRef.current = null;
      runtimeRecoveryStateRef.current = {
        phase: 'idle',
        generation: runtimeRecoveryStateRef.current.generation,
      };
      awaitingLegacyRuntimeBootstrapRef.current = false;

      if (socketRef.current) {
        // Detach handlers so no callbacks fire after cleanup
        socketRef.current.onopen = null;
        socketRef.current.onmessage = null;
        socketRef.current.onerror = null;
        socketRef.current.onclose = null;
        socketRef.current.close();
        socketRef.current = null;
      }
    };
  }, [runtimeBootstrapReady, url]);

  useEffect(() => {
    detailDeviceIdRef.current = detailDeviceId;

    const previousDeviceId = lastSubscribedDeviceIdRef.current;
    if (previousDeviceId === detailDeviceId) {
      return;
    }

    if (previousDeviceId !== null) {
      sendDetailControl('unsubscribe_detail', previousDeviceId);
    }

    if (detailDeviceId !== null) {
      sendDetailControl('subscribe_detail', detailDeviceId);
    }

    lastSubscribedDeviceIdRef.current = detailDeviceId;
  }, [detailDeviceId]);

  return {
    snapshot,
    alerts,
    connected,
    reconnecting,
    prometheusStatus,
    pollingHealth,
  };
}
