import type { CanvasHelloPayload } from './hello';

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

export function appendHelloQueryParams(url: string, payload: CanvasHelloPayload): string {
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
