import { useEffect, useRef, useState } from 'react';
import {
  mergeSnapshotDelta,
  parseWSMessage,
  type PrometheusStatusPayload,
  type SnapshotDeltaWSMessage,
  type SnapshotPayload,
  type SnapshotWSMessage,
} from '../types/metrics';

interface UseWebSocketResult {
  snapshot: SnapshotPayload | null;
  connected: boolean;
  reconnecting: boolean;
  prometheusStatus: PrometheusStatusPayload | null;
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

export function useWebSocket(url: string): UseWebSocketResult {
  const [snapshot, setSnapshot] = useState<SnapshotPayload | null>(null);
  const [connected, setConnected] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);
  const [prometheusStatus, setPrometheusStatus] = useState<PrometheusStatusPayload | null>(null);

  const socketRef = useRef<WebSocket | null>(null);

  const reconnectAttemptRef = useRef(0);
  const disposed = useRef(false);

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
      };

      ws.onmessage = (event: MessageEvent<string>) => {
        try {
          const raw = JSON.parse(event.data) as unknown;
          const message = parseWSMessage(raw);

          if (message.type === 'snapshot') {
            setSnapshot((message as SnapshotWSMessage).payload);
          } else if (message.type === 'snapshot_delta') {
            setSnapshot((prev) => {
              if (prev === null) {
                // No base snapshot yet — ignore delta (first message is always full snapshot)
                return null;
              }
              return mergeSnapshotDelta(prev, (message as SnapshotDeltaWSMessage).payload);
            });
          } else if (message.type === 'prometheus_status') {
            setPrometheusStatus(message.payload as PrometheusStatusPayload);
          } else if (message.type === 'topology_changed') {
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

  return {
    snapshot,
    connected,
    reconnecting,
    prometheusStatus,
  };
}
