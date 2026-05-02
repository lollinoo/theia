import { useEffect, useRef, useState } from 'react';
import {
  getCanvasDiagnosticsSnapshot,
  recordCanvasDiagnosticEvent,
  updateCanvasDiagnosticsState,
} from '../components/canvas/canvasDiagnostics';
import {
  type AlertDTO,
  type AlertWSMessage,
  type PollingHealthPayload,
  type PrometheusStatusPayload,
  type ReadyWSMessage,
  type ResyncRequiredPayload,
  type ResyncRequiredWSMessage,
  type RuntimeDeltaWSMessage,
  type SnapshotDeltaWSMessage,
  type SnapshotPayload,
  type SnapshotWSMessage,
  type TopologyChangedWSMessage,
  mergeRuntimeDeltaPatch,
  mergeSnapshotDelta,
  parseWSMessage,
} from '../types/metrics';
import {
  getCanvasRuntimeBootstrap,
  subscribeCanvasRuntimeBootstrap,
} from './canvasRuntimeBootstrap';

interface UseWebSocketResult {
  snapshot: SnapshotPayload | null;
  alerts: AlertDTO[];
  connected: boolean;
  reconnecting: boolean;
  prometheusStatus: PrometheusStatusPayload | null;
  pollingHealth: PollingHealthPayload | null;
}

interface UseWebSocketOptions {
  requireRuntimeBootstrap?: boolean;
  runtimeUpdatesPaused?: boolean;
}

type DetailControlType = 'subscribe_detail' | 'unsubscribe_detail';
const canvasSchemaVersion = 1;

interface CanvasHelloPayload {
  canvas_schema_version: number;
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

function buildWebSocketURL(url: string): string {
  if (/^wss?:\/\//i.test(url)) {
    return url;
  }

  if (/^https?:\/\//i.test(url)) {
    return url.replace(/^http/i, 'ws');
  }

  if (typeof window === 'undefined') {
    return url;
  }

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const normalizedPath = url.startsWith('/') ? url : `/${url}`;
  return `${protocol}//${window.location.host}${normalizedPath}`;
}

function appendHelloQueryParams(url: string, payload: CanvasHelloPayload): string {
  const parsed = new URL(url);
  parsed.searchParams.set('canvas_schema_version', String(payload.canvas_schema_version));
  if (payload.topology_version !== undefined) {
    parsed.searchParams.set('topology_version', payload.topology_version);
  }
  if (payload.runtime_version !== undefined) {
    parsed.searchParams.set('runtime_version', String(payload.runtime_version));
  }
  if (payload.runtime_identity !== undefined) {
    parsed.searchParams.set('runtime_identity', payload.runtime_identity);
  }
  if (payload.alert_version !== undefined) {
    parsed.searchParams.set('alert_version', String(payload.alert_version));
  }
  return parsed.toString();
}

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
  const alertVersionRef = useRef<number | null>(null);
  const awaitingResyncRef = useRef(false);

  const reconnectAttemptRef = useRef(0);
  const reconnectCountRef = useRef(0);
  const resyncRequiredCountRef = useRef(0);
  const topologyChangedCountRef = useRef(0);
  const disposed = useRef(false);

  function buildHelloPayload(): CanvasHelloPayload {
    const diagnostics = getCanvasDiagnosticsSnapshot();
    const hasRuntimeBase = hasRuntimeSnapshotRef.current;
    return {
      canvas_schema_version: canvasSchemaVersion,
      topology_version: diagnostics.topology.topologyVersion,
      runtime_version: hasRuntimeBase ? (snapshotVersionRef.current ?? undefined) : undefined,
      runtime_identity: hasRuntimeBase ? (runtimeIdentityRef.current ?? undefined) : undefined,
      alert_version: alertVersionRef.current ?? undefined,
      subscriptions: {
        runtime: true,
        topology: true,
        alerts: true,
        details_device_id: detailDeviceIdRef.current,
      },
    };
  }

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

  function publishRuntimeSnapshot(nextSnapshot: SnapshotPayload | null): void {
    snapshotStateRef.current = nextSnapshot;
    if (runtimeUpdatesPausedRef.current) {
      pendingSnapshotFlushRef.current = true;
      return;
    }

    pendingSnapshotFlushRef.current = false;
    setSnapshot(nextSnapshot);
  }

  function publishPollingHealth(nextPollingHealth: PollingHealthPayload | null): void {
    pollingHealthStateRef.current = nextPollingHealth;
    if (runtimeUpdatesPausedRef.current) {
      pendingPollingHealthFlushRef.current = true;
      return;
    }

    pendingPollingHealthFlushRef.current = false;
    setPollingHealth(nextPollingHealth);
  }

  function resetAlertState() {
    alertVersionRef.current = null;
    setAlerts([]);
  }

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
      snapshotVersionRef.current = bootstrap.runtimeVersion ?? null;
      runtimeIdentityRef.current = bootstrap.runtimeIdentity ?? null;
      awaitingResyncRef.current = false;
      publishRuntimeSnapshot(bootstrap.snapshot);
      setRuntimeBootstrapReady(true);
      updateCanvasDiagnosticsState({
        websocket: {
          lastAppliedSnapshotVersion:
            bootstrap.runtimeVersion === undefined ? undefined : String(bootstrap.runtimeVersion),
          ...(bootstrap.runtimeIdentity !== undefined
            ? { lastAppliedRuntimeIdentity: bootstrap.runtimeIdentity }
            : {}),
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

    function clearReconnectTimer() {
      if (reconnectTimer !== null) {
        window.clearTimeout(reconnectTimer);
        reconnectTimer = null;
      }
    }

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

    function connect() {
      if (disposed.current) return;

      clearReconnectTimer();

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
      const initialHelloPayload = buildHelloPayload();
      const ws = new WebSocket(appendHelloQueryParams(buildWebSocketURL(url), initialHelloPayload));
      socketRef.current = ws;

      function dispatchResyncRequired(payload: ResyncRequiredPayload): void {
        window.dispatchEvent(
          new CustomEvent<ResyncRequiredPayload>('backend-resync-required', {
            detail: payload,
          }),
        );
      }

      function requestClientResync(
        payloadReason: ResyncRequiredPayload['reason'] = 'client_resync_scheduled',
        diagnosticReason = 'base_version_mismatch',
      ): void {
        if (awaitingResyncRef.current) {
          return;
        }
        awaitingResyncRef.current = true;
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
        dispatchResyncRequired({
          scope: 'overview',
          reason: payloadReason,
        });
        ws.close();
      }

      function ignoreStaleRuntimeDelta(
        messageType: 'snapshot_delta' | 'runtime_delta',
        baseVersion: number,
        version: number,
        currentVersion: number,
      ): void {
        recordCanvasDiagnosticEvent({
          level: 'debug',
          source: 'runtime',
          event: 'runtime.delta.ignored',
          message: 'Runtime delta ignored because it is older than the current client base',
          metadata: {
            type: messageType,
            reason: 'stale_delta',
            baseVersion,
            version,
            currentVersion,
          },
        });
      }

      function getRawMessageType(raw: unknown): string | null {
        if (raw === null || typeof raw !== 'object') {
          return null;
        }
        const type = (raw as { type?: unknown }).type;
        return typeof type === 'string' ? type : null;
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
              requestClientResync('client_missing_runtime_snapshot', 'missing_base_snapshot');
              return;
            }
            if (payload.runtime_version !== undefined) {
              snapshotVersionRef.current = payload.runtime_version;
            }
            if (payload.runtime_identity !== undefined) {
              runtimeIdentityRef.current = payload.runtime_identity;
            }
            if (payload.alert_version !== undefined) {
              alertVersionRef.current = payload.alert_version;
            }
            awaitingResyncRef.current = false;
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
            hasRuntimeSnapshotRef.current = true;
            snapshotVersionRef.current = payload.version;
            runtimeIdentityRef.current = payload.runtime_identity ?? null;
            awaitingResyncRef.current = false;
            updateCanvasDiagnosticsState({
              websocket: {
                lastAppliedSnapshotVersion:
                  payload.version === null ? undefined : String(payload.version),
                ...(payload.runtime_identity !== undefined
                  ? { lastAppliedRuntimeIdentity: payload.runtime_identity }
                  : {}),
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
          } else if (message.type === 'snapshot_delta' || message.type === 'runtime_delta') {
            const payload =
              message.type === 'runtime_delta'
                ? (message as RuntimeDeltaWSMessage).payload
                : (message as SnapshotDeltaWSMessage).payload;
            if (awaitingResyncRef.current) {
              return;
            }

            const hasVersionEnvelope =
              payload.version !== undefined || payload.base_version !== undefined;
            if (hasVersionEnvelope) {
              if (payload.version === undefined || payload.base_version === undefined) {
                requestClientResync('client_resync_scheduled', 'invalid_delta_version');
                return;
              }

              const currentVersion = snapshotVersionRef.current;
              if (currentVersion === null || !hasRuntimeSnapshotRef.current) {
                requestClientResync('client_missing_runtime_snapshot', 'missing_base_snapshot');
                return;
              }

              if (payload.base_version < currentVersion) {
                ignoreStaleRuntimeDelta(
                  message.type,
                  payload.base_version,
                  payload.version,
                  currentVersion,
                );
                return;
              }

              if (payload.base_version > currentVersion) {
                requestClientResync();
                return;
              }

              snapshotVersionRef.current = payload.version;
              if (payload.runtime_identity !== undefined) {
                runtimeIdentityRef.current = payload.runtime_identity;
              }
              updateCanvasDiagnosticsState({
                websocket: {
                  lastAppliedDeltaVersion: String(payload.version),
                  ...(payload.runtime_identity !== undefined
                    ? { lastAppliedRuntimeIdentity: payload.runtime_identity }
                    : {}),
                  lastRejectedDeltaReason: undefined,
                },
              });
            } else if (!hasRuntimeSnapshotRef.current) {
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
            awaitingResyncRef.current = true;
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
              metadata: { ...(message as ResyncRequiredWSMessage).payload },
            });
            resetAlertState();
            dispatchResyncRequired((message as ResyncRequiredWSMessage).payload);
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
          const messageType = getRawMessageType(raw);
          if (messageType === 'runtime_delta' || messageType === 'snapshot_delta') {
            requestClientResync('client_resync_scheduled', 'invalid_runtime_delta_payload');
          }
        }
      };

      ws.onerror = () => {
        ws.close();
      };

      ws.onclose = () => {
        if (socketRef.current === ws) {
          socketRef.current = null;
        }
        if (disposed.current) return;
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
      setConnected(false);
      setReconnecting(false);
      hasRuntimeSnapshotRef.current = false;
      snapshotStateRef.current = null;
      pendingSnapshotFlushRef.current = false;
      pollingHealthStateRef.current = null;
      pendingPollingHealthFlushRef.current = false;
      snapshotVersionRef.current = null;
      runtimeIdentityRef.current = null;
      updateCanvasDiagnosticsState({
        websocket: {
          connected: false,
        },
      });
      resetAlertState();
      awaitingResyncRef.current = false;

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
