import { useCallback, useEffect, useRef, useState } from 'react';
import {
  parseWSMessage,
  type SnapshotPayload,
  type SnapshotWSMessage,
} from '../types/metrics';

interface UseWebSocketResult {
  snapshot: SnapshotPayload | null;
  connected: boolean;
  reconnecting: boolean;
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

  const socketRef = useRef<WebSocket | null>(null);
  const reconnectTimerRef = useRef<number | null>(null);
  const reconnectAttemptRef = useRef(0);
  const connectRef = useRef<() => void>(() => undefined);
  const mountedRef = useRef(false);
  const closedRef = useRef(false);

  const clearReconnectTimer = useCallback(() => {
    if (reconnectTimerRef.current !== null) {
      window.clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
  }, []);

  const handleMessage = useCallback((event: MessageEvent<string>) => {
    try {
      const raw = JSON.parse(event.data) as unknown;
      const message = parseWSMessage(raw);

      if (message.type === 'snapshot') {
        setSnapshot((message as SnapshotWSMessage).payload);
      }
    } catch (error) {
      console.error('Failed to parse WebSocket message', error);
    }
  }, []);

  const scheduleReconnect = useCallback(() => {
    if (!mountedRef.current || closedRef.current || reconnectTimerRef.current !== null) {
      return;
    }

    setReconnecting(true);
    const delay = Math.min(1_000 * 2 ** reconnectAttemptRef.current, 30_000);

    reconnectTimerRef.current = window.setTimeout(() => {
      reconnectTimerRef.current = null;
      reconnectAttemptRef.current += 1;
      connectRef.current();
    }, delay);
  }, []);

  const connect = useCallback(() => {
    if (!mountedRef.current || closedRef.current) {
      return;
    }

    clearReconnectTimer();

    const nextSocket = new WebSocket(buildWebSocketURL(url));
    socketRef.current = nextSocket;

    nextSocket.onopen = () => {
      if (!mountedRef.current) {
        return;
      }

      reconnectAttemptRef.current = 0;
      setConnected(true);
      setReconnecting(false);
    };

    nextSocket.onmessage = handleMessage;

    nextSocket.onerror = () => {
      nextSocket.close();
    };

    nextSocket.onclose = () => {
      socketRef.current = null;

      if (!mountedRef.current) {
        return;
      }

      setConnected(false);
      scheduleReconnect();
    };
  }, [clearReconnectTimer, handleMessage, scheduleReconnect, url]);

  connectRef.current = connect;

  useEffect(() => {
    mountedRef.current = true;
    closedRef.current = false;
    connect();

    return () => {
      mountedRef.current = false;
      closedRef.current = true;
      clearReconnectTimer();
      setConnected(false);
      setReconnecting(false);

      if (socketRef.current) {
        socketRef.current.close();
        socketRef.current = null;
      }
    };
  }, [clearReconnectTimer, connect]);

  return {
    snapshot,
    connected,
    reconnecting,
  };
}
