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
  type SnapshotDeltaWSMessage,
  type SnapshotPayload,
  type SnapshotWSMessage,
  type TopologyChangedWSMessage,
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
}

type DetailControlType = 'subscribe_detail' | 'unsubscribe_detail';
const canvasSchemaVersion = 1;

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

export function useWebSocket(
  url: string,
  detailDeviceId: string | null = null,
  options: UseWebSocketOptions = {},
): UseWebSocketResult {
  const requireRuntimeBootstrap = options.requireRuntimeBootstrap === true;
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
      setSnapshot(bootstrap.snapshot);
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
    });
  }, [requireRuntimeBootstrap]);

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
      const ws = new WebSocket(buildWebSocketURL(url));
      socketRef.current = ws;

      function dispatchResyncRequired(payload: ResyncRequiredPayload): void {
        window.dispatchEvent(
          new CustomEvent<ResyncRequiredPayload>('backend-resync-required', {
            detail: payload,
          }),
        );
      }

      function sendHello(): void {
        const diagnostics = getCanvasDiagnosticsSnapshot();
        const hasRuntimeBase = hasRuntimeSnapshotRef.current;
        ws.send(
          JSON.stringify({
            type: 'hello',
            payload: {
              canvas_schema_version: canvasSchemaVersion,
              topology_version: diagnostics.topology.topologyVersion,
              runtime_version: hasRuntimeBase
                ? (snapshotVersionRef.current ?? undefined)
                : undefined,
              runtime_identity: hasRuntimeBase
                ? (runtimeIdentityRef.current ?? undefined)
                : undefined,
              alert_version: alertVersionRef.current ?? undefined,
              subscriptions: {
                runtime: true,
                topology: true,
                alerts: true,
                details_device_id: detailDeviceIdRef.current,
              },
            },
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
        sendHello();
        if (wasReconnect) {
          window.dispatchEvent(new Event('backend-reconnected'));
        }
        if (detailDeviceIdRef.current !== null) {
          sendDetailControl('subscribe_detail', detailDeviceIdRef.current);
          lastSubscribedDeviceIdRef.current = detailDeviceIdRef.current;
        }
      };

      ws.onmessage = (event: MessageEvent<string>) => {
        try {
          const raw = JSON.parse(event.data) as unknown;
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
            setSnapshot(payload.snapshot);
          } else if (message.type === 'snapshot_delta' || message.type === 'runtime_delta') {
            const payload = (message as SnapshotDeltaWSMessage).payload;
            setSnapshot((prev) => {
              if (prev === null) {
                // No base snapshot yet — ignore delta until a full snapshot arrives.
                updateCanvasDiagnosticsState({
                  websocket: {
                    lastRejectedDeltaReason: 'missing_base_snapshot',
                  },
                });
                recordCanvasDiagnosticEvent({
                  level: 'warn',
                  source: 'runtime',
                  event: 'runtime.delta.rejected',
                  message: 'Runtime delta rejected because no base snapshot is available',
                  metadata: {
                    reason: 'missing_base_snapshot',
                  },
                });
                return null;
              }
              if (awaitingResyncRef.current) {
                return prev;
              }

              if (payload.version !== undefined || payload.base_version !== undefined) {
                if (
                  snapshotVersionRef.current !== payload.base_version ||
                  payload.version === undefined
                ) {
                  requestClientResync();
                  return prev;
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
              return mergeSnapshotDelta(prev, payload.delta);
            });
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
            setPollingHealth(message.payload as PollingHealthPayload);
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
