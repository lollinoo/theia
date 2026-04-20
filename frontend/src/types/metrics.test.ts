import { describe, it, expect } from 'vitest';
import {
  parseWSMessage,
  mergeSnapshotDelta,
  parseDeviceMetrics,
  type SnapshotPayload,
  type SnapshotEnvelopePayload,
  type SnapshotDeltaEnvelopePayload,
} from './metrics';

// Helper to build a minimal SnapshotPayload
function makeSnapshot(overrides: Partial<SnapshotPayload> = {}): SnapshotPayload {
  return {
    device_metrics: {},
    link_metrics: {},
    device_statuses: {},
    ...overrides,
  };
}

describe('parseWSMessage — snapshot_delta', () => {
  it('parses a versioned snapshot_delta envelope', () => {
    const message = parseWSMessage({
      type: 'snapshot_delta',
      payload: {
        base_version: 10,
        version: 11,
        delta: {
          device_metrics: {
            'dev-1': {
              device_id: 'dev-1',
              cpu_percent: 90,
              mem_percent: null,
              collected_at: '2026-01-01T00:00:00Z',
            },
          },
          link_metrics: {},
          device_statuses: {},
        },
      },
    });

    const payload = (message as { type: 'snapshot_delta'; payload: SnapshotDeltaEnvelopePayload }).payload;
    expect(payload.base_version).toBe(10);
    expect(payload.version).toBe(11);
    expect(payload.delta.device_metrics['dev-1'].cpu_percent).toBe(90);
  });

  it('parses a snapshot_delta message with a valid sparse payload', () => {
    const message = parseWSMessage({
      type: 'snapshot_delta',
      payload: {
        device_metrics: {
          'dev-1': {
            device_id: 'dev-1',
            cpu_percent: 90,
            mem_percent: null,
            collected_at: '2026-01-01T00:00:00Z',
          },
        },
        link_metrics: {},
        device_statuses: {},
      },
    });

    expect(message.type).toBe('snapshot_delta');
    // payload should be parsed as SnapshotPayload
    const payload = (message as { type: 'snapshot_delta'; payload: SnapshotDeltaEnvelopePayload }).payload;
    expect(payload.delta.device_metrics['dev-1'].cpu_percent).toBe(90);
  });

  it('parses a snapshot_delta message with empty sections without error', () => {
    const message = parseWSMessage({
      type: 'snapshot_delta',
      payload: {
        device_metrics: {},
        link_metrics: {},
        device_statuses: {},
      },
    });

    expect(message.type).toBe('snapshot_delta');
  });

  it('still parses snapshot messages correctly (no regression)', () => {
    const message = parseWSMessage({
      type: 'snapshot',
      payload: {
        device_metrics: {
          'dev-1': {
            device_id: 'dev-1',
            cpu_percent: 50,
            mem_percent: null,
            collected_at: '2026-01-01T00:00:00Z',
          },
        },
        link_metrics: {},
        device_statuses: {},
      },
    });

    expect(message.type).toBe('snapshot');
    const payload = (message as { type: 'snapshot'; payload: SnapshotEnvelopePayload }).payload;
    expect(payload.snapshot.device_metrics['dev-1'].cpu_percent).toBe(50);
  });

  it('parses a versioned snapshot message envelope', () => {
    const message = parseWSMessage({
      type: 'snapshot',
      payload: {
        version: 7,
        snapshot: {
          device_metrics: {},
          link_metrics: {},
          device_statuses: {},
        },
      },
    });

    const payload = (message as { type: 'snapshot'; payload: SnapshotEnvelopePayload }).payload;
    expect(payload.version).toBe(7);
    expect(payload.snapshot.device_metrics).toEqual({});
  });
});

describe('mergeSnapshotDelta', () => {
  it('overwrites only entries present in the delta', () => {
    const existing = makeSnapshot({
      device_metrics: {
        'dev-1': {
          device_id: 'dev-1',
          cpu_percent: 50,
          mem_percent: null,
          temp_celsius: null,
          uptime_secs: null,
          collected_at: '2026-01-01T00:00:00Z',
        },
        'dev-2': {
          device_id: 'dev-2',
          cpu_percent: 75,
          mem_percent: null,
          temp_celsius: null,
          uptime_secs: null,
          collected_at: '2026-01-01T00:00:00Z',
        },
      },
    });
    const delta = makeSnapshot({
      device_metrics: {
        'dev-1': {
          device_id: 'dev-1',
          cpu_percent: 90,
          mem_percent: null,
          temp_celsius: null,
          uptime_secs: null,
          collected_at: '2026-01-01T00:01:00Z',
        },
      },
    });

    const result = mergeSnapshotDelta(existing, delta);

    expect(result.device_metrics['dev-1'].cpu_percent).toBe(90);
    expect(result.device_metrics['dev-2'].cpu_percent).toBe(75);
  });

  it('preserves prior non-freshness detail-only device metric fields when a later slim delta updates the same device', () => {
    const existing = makeSnapshot({
      device_metrics: {
        'dev-1': {
          device_id: 'dev-1',
          cpu_percent: 50,
          mem_percent: 25,
          temp_celsius: 55,
          uptime_secs: 86400,
          last_polled_at: '2026-01-01T00:00:30Z',
          expected_poll_interval_seconds: 30,
          collected_at: '2026-01-01T00:00:30Z',
        },
      },
    });
    const delta = makeSnapshot({
      device_metrics: {
        'dev-1': {
          device_id: 'dev-1',
          cpu_percent: 90,
          mem_percent: 35,
          collected_at: '2026-01-01T00:01:00Z',
        },
      },
    });

    const result = mergeSnapshotDelta(existing, delta);

    expect(result.device_metrics['dev-1']).toMatchObject({
      cpu_percent: 90,
      mem_percent: 35,
      temp_celsius: 55,
      uptime_secs: 86400,
      expected_poll_interval_seconds: 30,
      collected_at: '2026-01-01T00:01:00Z',
    });
    expect(result.device_metrics['dev-1'].last_polled_at).toBeUndefined();
  });

  it('does not preserve a stale last_polled_at behind a newer collected_at', () => {
    const existing = makeSnapshot({
      device_metrics: {
        'dev-1': {
          device_id: 'dev-1',
          cpu_percent: 50,
          mem_percent: 25,
          temp_celsius: 55,
          uptime_secs: 86400,
          last_polled_at: '2026-01-01T00:00:30Z',
          expected_poll_interval_seconds: 30,
          collected_at: '2026-01-01T00:00:30Z',
        },
      },
    });
    const delta = makeSnapshot({
      device_metrics: {
        'dev-1': {
          device_id: 'dev-1',
          cpu_percent: 90,
          mem_percent: 35,
          collected_at: '2026-01-01T00:01:00Z',
        },
      },
    });

    const result = mergeSnapshotDelta(existing, delta);

    expect(result.device_metrics['dev-1']).toMatchObject({
      temp_celsius: 55,
      uptime_secs: 86400,
      expected_poll_interval_seconds: 30,
      collected_at: '2026-01-01T00:01:00Z',
    });
    expect(result.device_metrics['dev-1'].last_polled_at).toBeUndefined();
  });

  it('merges targeted link_metrics for one device without clearing other devices', () => {
    const existing = makeSnapshot({
      link_metrics: {
        'dev-1': [
          {
            device_id: 'dev-1',
            if_name: 'ether1',
            tx_bps: 100,
            rx_bps: 200,
            utilization: 0.1,
            collected_at: '2026-01-01T00:00:00Z',
          },
        ],
        'dev-2': [
          {
            device_id: 'dev-2',
            if_name: 'ether2',
            tx_bps: 300,
            rx_bps: 400,
            utilization: 0.2,
            collected_at: '2026-01-01T00:00:00Z',
          },
        ],
      },
    });
    const delta = makeSnapshot({
      link_metrics: {
        'dev-1': [
          {
            device_id: 'dev-1',
            if_name: 'ether1',
            tx_bps: 150,
            rx_bps: 250,
            utilization: 0.15,
            collected_at: '2026-01-01T00:01:00Z',
          },
        ],
      },
    });

    const result = mergeSnapshotDelta(existing, delta);

    expect(result.link_metrics['dev-1']).toEqual([
      {
        device_id: 'dev-1',
        if_name: 'ether1',
        tx_bps: 150,
        rx_bps: 250,
        utilization: 0.15,
        collected_at: '2026-01-01T00:01:00Z',
      },
    ]);
    expect(result.link_metrics['dev-2']).toEqual(existing.link_metrics['dev-2']);
  });

  it('replaces alerts entirely when delta has non-empty alerts', () => {
    const existing = makeSnapshot({
      device_statuses: { 'dev-1': 'up' },
    });
    const delta = makeSnapshot({
      device_statuses: { 'dev-1': 'down' },
    });

    const result = mergeSnapshotDelta(existing, delta);

    expect(result.device_statuses['dev-1']).toBe('down');
  });
});

describe('parseWSMessage — topology_changed', () => {
  it('parses a topology_changed message with null payload', () => {
    const message = parseWSMessage({
      type: 'topology_changed',
      payload: null,
    });
    expect(message.type).toBe('topology_changed');
    expect(message.payload).toBeNull();
  });
});

describe('parseWSMessage — alert', () => {
  it('parses a versioned alert envelope', () => {
    const message = parseWSMessage({
      type: 'alert',
      payload: {
        version: 12,
        alerts: [
          {
            device_id: 'dev-1',
            severity: 'critical',
            alert_name: 'DeviceDown',
            state: 'firing',
            summary: 'device down',
          },
        ],
      },
    });

    expect(message.type).toBe('alert');
    expect(message.payload).toEqual({
      version: 12,
      alerts: [
        {
          device_id: 'dev-1',
          severity: 'critical',
          alert_name: 'DeviceDown',
          state: 'firing',
          summary: 'device down',
        },
      ],
    });
  });
});

describe('parseDeviceMetrics', () => {
  it('preserves optional detail fields when present', () => {
    const metrics = parseDeviceMetrics({
      device_id: 'dev-1',
      cpu_percent: 50,
      mem_percent: null,
      temp_celsius: 55,
      uptime_secs: 86400,
      collected_at: '2026-01-01T00:00:00Z',
      health: 'warning',
      reachability: 'up',
      stale: false,
      last_polled_at: '2026-01-01T00:00:00Z',
      expected_poll_interval_seconds: 30,
    });

    expect(metrics.health).toBe('warning');
    expect(metrics.reachability).toBe('up');
    expect(metrics.stale).toBe(false);
    expect(metrics.temp_celsius).toBe(55);
    expect(metrics.uptime_secs).toBe(86400);
    expect(metrics.last_polled_at).toBe('2026-01-01T00:00:00Z');
    expect(metrics.expected_poll_interval_seconds).toBe(30);
  });
});

describe('parseSnapshotPayload', () => {
  it('does not synthesize removed overview sections', () => {
    const message = parseWSMessage({
      type: 'snapshot',
      payload: {
        version: 7,
        snapshot: {
          device_metrics: {},
          link_metrics: {},
          device_statuses: {},
        },
      },
    });

    const payload = (message as { type: 'snapshot'; payload: SnapshotEnvelopePayload }).payload;
    expect((payload.snapshot as Record<string, unknown>).alerts).toBeUndefined();
    expect((payload.snapshot as Record<string, unknown>).device_hostnames).toBeUndefined();
    expect((payload.snapshot as Record<string, unknown>).device_models).toBeUndefined();
  });
});
