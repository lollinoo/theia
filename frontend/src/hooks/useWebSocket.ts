import { useEffect, useRef, useState } from 'react';
import {
  type AlertDTO,
  type AlertWSMessage,
  type PollingHealthPayload,
  type PrometheusStatusPayload,
  type ResyncRequiredPayload,
  type ResyncRequiredWSMessage,
  type SnapshotDeltaWSMessage,
  type SnapshotPayload,
  type SnapshotWSMessage,
  mergeSnapshotDelta,
  parseWSMessage,
} from '../types/metrics';

interface UseWebSocketResult {
  snapshot: SnapshotPayload | null;
  alerts: AlertDTO[];
  connected: boolean;
  reconnecting: boolean;
  prometheusStatus: PrometheusStatusPayload | null;
  pollingHealth: PollingHealthPayload | null;
}

type DetailControlType = 'subscribe_detail' | 'unsubscribe_detail';

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
): UseWebSocketResult {
  const [snapshot, setSnapshot] = useState<SnapshotPayload | null>(null);
  const [alerts, setAlerts] = useState<AlertDTO[]>([]);
  const [connected, setConnected] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);
  const [prometheusStatus, setPrometheusStatus] = useState<PrometheusStatusPayload | null>(null);
  const [pollingHealth, setPollingHealth] = useState<PollingHealthPayload | null>(null);

  const socketRef = useRef<WebSocket | null>(null);
  const detailDeviceIdRef = useRef<string | null>(detailDeviceId);
  const lastSubscribedDeviceIdRef = useRef<string | null>(null);
  const snapshotVersionRef = useRef<number | null>(null);
  const alertVersionRef = useRef<number | null>(null);
  const awaitingResyncRef = useRef(false);

  const reconnectAttemptRef = useRef(0);
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

      ws.onopen = () => {
        if (disposed.current) {
          ws.close();
          return;
        }
        const wasReconnect = reconnectAttemptRef.current > 0;
        reconnectAttemptRef.current = 0;
        setConnected(true);
        setReconnecting(false);
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

          if (message.type === 'snapshot') {
            const payload = (message as SnapshotWSMessage).payload;
            snapshotVersionRef.current = payload.version;
            awaitingResyncRef.current = false;
            setSnapshot(payload.snapshot);
          } else if (message.type === 'snapshot_delta' || message.type === 'runtime_delta') {
            const payload = (message as SnapshotDeltaWSMessage).payload;
            setSnapshot((prev) => {
              if (prev === null) {
                // No base snapshot yet — ignore delta until a full snapshot arrives.
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
                  awaitingResyncRef.current = true;
                  return prev;
                }
                snapshotVersionRef.current = payload.version;
              }

              return mergeSnapshotDelta(prev, payload.delta);
            });
          } else if (message.type === 'prometheus_status') {
            setPrometheusStatus(message.payload as PrometheusStatusPayload);
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
            resetAlertState();
            window.dispatchEvent(
              new CustomEvent<ResyncRequiredPayload>('backend-resync-required', {
                detail: (message as ResyncRequiredWSMessage).payload,
              }),
            );
          } else if (message.type === 'topology_changed' || message.type === 'topology_delta') {
            window.dispatchEvent(new Event('topology-changed'));
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
        scheduleReconnect();
      };
    }

    connect();

    return () => {
      disposed.current = true;
      clearReconnectTimer();
      setConnected(false);
      setReconnecting(false);
      snapshotVersionRef.current = null;
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
  }, [url]);

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
