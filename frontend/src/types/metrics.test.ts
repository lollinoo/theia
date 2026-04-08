import { describe, it, expect } from 'vitest';
import {
  parseWSMessage,
  mergeSnapshotDelta,
  type SnapshotPayload,
} from './metrics';

// Helper to build a minimal SnapshotPayload
function makeSnapshot(overrides: Partial<SnapshotPayload> = {}): SnapshotPayload {
  return {
    device_metrics: {},
    link_metrics: {},
    alerts: [],
    device_statuses: {},
    device_hostnames: {},
    ...overrides,
  };
}

describe('parseWSMessage — snapshot_delta', () => {
  it('parses a snapshot_delta message with a valid sparse payload', () => {
    const message = parseWSMessage({
      type: 'snapshot_delta',
      payload: {
        device_metrics: {
          'dev-1': {
            device_id: 'dev-1',
            cpu_percent: 90,
            mem_percent: null,
            temp_celsius: null,
            uptime_secs: null,
            collected_at: '2026-01-01T00:00:00Z',
          },
        },
        link_metrics: {},
        alerts: [],
        device_statuses: {},
        device_hostnames: {},
      },
    });

    expect(message.type).toBe('snapshot_delta');
    // payload should be parsed as SnapshotPayload
    const payload = (message as { type: 'snapshot_delta'; payload: SnapshotPayload }).payload;
    expect(payload.device_metrics['dev-1'].cpu_percent).toBe(90);
  });

  it('parses a snapshot_delta message with empty sections without error', () => {
    const message = parseWSMessage({
      type: 'snapshot_delta',
      payload: {
        device_metrics: {},
        link_metrics: {},
        alerts: [],
        device_statuses: {},
        device_hostnames: {},
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
            temp_celsius: null,
            uptime_secs: null,
            collected_at: '2026-01-01T00:00:00Z',
          },
        },
        link_metrics: {},
        alerts: [],
        device_statuses: {},
        device_hostnames: {},
      },
    });

    expect(message.type).toBe('snapshot');
    const payload = (message as { type: 'snapshot'; payload: SnapshotPayload }).payload;
    expect(payload.device_metrics['dev-1'].cpu_percent).toBe(50);
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

  it('replaces alerts entirely when delta has non-empty alerts', () => {
    const existing = makeSnapshot({
      alerts: [
        { device_id: 'd1', severity: 'warning', alert_name: 'HighCPU', state: 'firing', summary: 'CPU high' },
      ],
    });
    const delta = makeSnapshot({
      alerts: [
        { device_id: 'd1', severity: 'critical', alert_name: 'Down', state: 'firing', summary: 'Device down' },
      ],
    });

    const result = mergeSnapshotDelta(existing, delta);

    expect(result.alerts).toHaveLength(1);
    expect(result.alerts[0].alert_name).toBe('Down');
  });

  it('preserves existing alerts when delta has empty alerts array', () => {
    const existing = makeSnapshot({
      alerts: [
        { device_id: 'd1', severity: 'warning', alert_name: 'HighCPU', state: 'firing', summary: 'CPU high' },
      ],
    });
    const delta = makeSnapshot({ alerts: [] });

    const result = mergeSnapshotDelta(existing, delta);

    expect(result.alerts).toHaveLength(1);
    expect(result.alerts[0].alert_name).toBe('HighCPU');
  });
});
