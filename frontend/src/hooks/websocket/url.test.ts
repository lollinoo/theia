/**
 * Exercises url hook lifecycle behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';
import { appendHelloQueryParams, buildWebSocketURL } from './url';

describe('buildWebSocketURL', () => {
  it('preserves explicit websocket URLs', () => {
    expect(buildWebSocketURL('ws://example.test/ws')).toBe('ws://example.test/ws');
    expect(buildWebSocketURL('wss://example.test/ws')).toBe('wss://example.test/ws');
  });

  it('converts HTTP URLs to websocket URLs', () => {
    expect(buildWebSocketURL('http://example.test/ws')).toBe('ws://example.test/ws');
    expect(buildWebSocketURL('https://example.test/ws')).toBe('wss://example.test/ws');
  });

  it('resolves relative URLs against the browser origin', () => {
    expect(buildWebSocketURL('api/ws')).toBe('ws://localhost:3000/api/ws');
    expect(buildWebSocketURL('/api/ws')).toBe('ws://localhost:3000/api/ws');
  });
});

describe('appendHelloQueryParams', () => {
  it('adds only defined hello handshake fields to the URL query', () => {
    const url = appendHelloQueryParams('ws://example.test/ws?existing=1', {
      canvas_schema_version: 1,
      topology_version: 'topology-7',
      runtime_version: 42,
      runtime_identity: 'rt-sha256:abc',
      alert_version: undefined,
      subscriptions: {
        runtime: true,
        topology: true,
        alerts: true,
        details_device_id: 'device-1',
      },
    });

    const parsed = new URL(url);
    expect(parsed.searchParams.get('existing')).toBe('1');
    expect(parsed.searchParams.get('canvas_schema_version')).toBe('1');
    expect(parsed.searchParams.get('topology_version')).toBe('topology-7');
    expect(parsed.searchParams.get('runtime_version')).toBe('42');
    expect(parsed.searchParams.get('runtime_identity')).toBe('rt-sha256:abc');
    expect(parsed.searchParams.has('alert_version')).toBe(false);
  });
});
