/**
 * Coordinates url WebSocket lifecycle and runtime update semantics.
 * Keeps reconnect, resync, and subscription behavior isolated from canvas rendering.
 */
import type { CanvasHelloPayload } from './hello';

/** Builds web socket url for the React hook lifecycle. */
export function buildWebSocketURL(url: string): string {
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

/** Append hello query params for the React hook lifecycle. */
export function appendHelloQueryParams(url: string, payload: CanvasHelloPayload): string {
  const parsed = new URL(url);
  parsed.searchParams.set('canvas_schema_version', String(payload.canvas_schema_version));
  parsed.searchParams.set('runtime_protocol', String(payload.runtime_protocol));
  if (payload.topology_version !== undefined) {
    parsed.searchParams.set('topology_version', payload.topology_version);
  }
  if (payload.runtime_stream_id !== undefined) {
    parsed.searchParams.set('runtime_stream_id', payload.runtime_stream_id);
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
