import { describe, expect, it } from 'vitest';
import {
  type DeviceRuntimeDTO,
  type LinkRuntimeDTO,
  type SnapshotDeltaEnvelopePayload,
  type SnapshotEnvelopePayload,
  type SnapshotPayload,
  mergeSnapshotDelta,
  parseDeviceRuntime,
  parseLinkRuntime,
  parseWSMessage,
} from './metrics';

function makeDeviceRuntime(overrides: Partial<DeviceRuntimeDTO> = {}): DeviceRuntimeDTO {
  return {
    device_id: 'dev-1',
    operational_status: 'up',
    reachability: 'up',
    health: 'healthy',
    freshness: 'fresh',
    primary_reason: 'ok',
    metrics_status: 'available',
    metrics_reason: 'ok',
    alert_status: 'normal',
    firing_alert_count: 0,
    last_collected_at: '2026-01-01T00:00:00Z',
    last_polled_at: '2026-01-01T00:00:00Z',
    expected_poll_interval_seconds: 30,
    cpu_percent: 50,
    mem_percent: 25,
    temp_celsius: 55,
    uptime_secs: 86400,
    ...overrides,
  };
}

function makeLinkRuntime(overrides: Partial<LinkRuntimeDTO> = {}): LinkRuntimeDTO {
  return {
    link_id: 'link-1',
    source_device_id: 'dev-1',
    target_device_id: 'dev-2',
    source_if_name: 'ether1',
    target_if_name: 'ether2',
    metrics_status: 'available',
    metrics_reason: 'ok',
    last_collected_at: '2026-01-01T00:00:00Z',
    tx_bps: 100,
    rx_bps: 200,
    utilization: 0.1,
    ...overrides,
  };
}

function makeSnapshot(overrides: Partial<SnapshotPayload> = {}): SnapshotPayload {
  return {
    devices: {},
    links: {},
    ...overrides,
  };
}

describe('parseWSMessage', () => {
  it('parses a versioned snapshot_delta envelope with normalized runtime records', () => {
    const message = parseWSMessage({
      type: 'snapshot_delta',
      payload: {
        base_version: 10,
        version: 11,
        delta: {
          devices: {
            'dev-1': makeDeviceRuntime({ cpu_percent: 90 }),
          },
          links: {},
        },
      },
    });

    const payload = (message as { type: 'snapshot_delta'; payload: SnapshotDeltaEnvelopePayload })
      .payload;
    expect(payload.base_version).toBe(10);
    expect(payload.version).toBe(11);
    expect(payload.delta.devices['dev-1'].cpu_percent).toBe(90);
  });

  it('parses a sparse snapshot_delta payload without a versioned envelope', () => {
    const message = parseWSMessage({
      type: 'snapshot_delta',
      payload: {
        devices: {
          'dev-1': makeDeviceRuntime({ metrics_reason: 'awaiting_poll' }),
        },
        links: {},
      },
    });

    const payload = (message as { type: 'snapshot_delta'; payload: SnapshotDeltaEnvelopePayload })
      .payload;
    expect(payload.delta.devices['dev-1'].metrics_reason).toBe('awaiting_poll');
  });

  it('still parses versioned snapshot envelopes', () => {
    const message = parseWSMessage({
      type: 'snapshot',
      payload: {
        version: 7,
        snapshot: {
          devices: {
            'dev-1': makeDeviceRuntime(),
          },
          links: {},
        },
      },
    });

    const payload = (message as { type: 'snapshot'; payload: SnapshotEnvelopePayload }).payload;
    expect(payload.version).toBe(7);
    expect(payload.snapshot.devices['dev-1'].device_id).toBe('dev-1');
  });

  it('still parses non-versioned snapshot envelopes', () => {
    const message = parseWSMessage({
      type: 'snapshot',
      payload: {
        devices: {
          'dev-1': makeDeviceRuntime({ operational_status: 'unknown' }),
        },
        links: {},
      },
    });

    const payload = (message as { type: 'snapshot'; payload: SnapshotEnvelopePayload }).payload;
    expect(payload.version).toBeNull();
    expect(payload.snapshot.devices['dev-1'].operational_status).toBe('unknown');
  });

  it('rejects alert envelope objects without an alerts array', () => {
    expect(() =>
      parseWSMessage({
        type: 'alert',
        payload: {
          device_id: 'dev-1',
          severity: 'critical',
          alert_name: 'DeviceDown',
          state: 'firing',
          summary: 'device down',
        },
      }),
    ).toThrow('invalid alert payload');
  });

  it('rejects malformed alert records inside an alert envelope', () => {
    expect(() =>
      parseWSMessage({
        type: 'alert',
        payload: {
          version: 4,
          alerts: [
            {
              device_id: 'dev-1',
              severity: 'critical',
              alert_name: 'DeviceDown',
              state: 'firing',
            },
          ],
        },
      }),
    ).toThrow('invalid alert payload');
  });

  it('accepts alert records with an empty summary string', () => {
    const message = parseWSMessage({
      type: 'alert',
      payload: {
        version: 4,
        alerts: [
          {
            device_id: 'dev-1',
            severity: 'critical',
            alert_name: 'DeviceDown',
            state: 'firing',
            summary: '',
          },
        ],
      },
    });

    expect(message.type).toBe('alert');
    expect((message as { type: 'alert'; payload: { alerts: Array<{ summary: string }> } }).payload.alerts[0].summary).toBe('');
  });
});

describe('mergeSnapshotDelta', () => {
  it('replaces atomic device records only for device keys present in the delta', () => {
    const existing = makeSnapshot({
      devices: {
        'dev-1': makeDeviceRuntime(),
        'dev-2': makeDeviceRuntime({
          device_id: 'dev-2',
          cpu_percent: 75,
          mem_percent: 35,
        }),
      },
    });
    const delta = makeSnapshot({
      devices: {
        'dev-1': makeDeviceRuntime({
          cpu_percent: 90,
          mem_percent: null,
          temp_celsius: null,
          uptime_secs: null,
          last_collected_at: '2026-01-01T00:01:00Z',
          last_polled_at: null,
          expected_poll_interval_seconds: null,
          metrics_status: 'partial',
          metrics_reason: 'no_data',
        }),
      },
    });

    const result = mergeSnapshotDelta(existing, delta);

    expect(result.devices['dev-1']).toEqual(delta.devices['dev-1']);
    expect(result.devices['dev-2']).toEqual(existing.devices['dev-2']);
  });

  it('replaces atomic link records only for link keys present in the delta', () => {
    const existing = makeSnapshot({
      links: {
        'link-1': makeLinkRuntime(),
        'link-2': makeLinkRuntime({
          link_id: 'link-2',
          source_device_id: 'dev-3',
          target_device_id: 'dev-4',
          source_if_name: 'ether3',
          target_if_name: 'ether4',
          tx_bps: 300,
          rx_bps: 400,
          utilization: 0.2,
        }),
      },
    });
    const delta = makeSnapshot({
      links: {
        'link-1': makeLinkRuntime({
          tx_bps: null,
          rx_bps: 250,
          utilization: null,
          metrics_status: 'unavailable',
          metrics_reason: 'upstream_unavailable',
          last_collected_at: null,
        }),
      },
    });

    const result = mergeSnapshotDelta(existing, delta);

    expect(result.links['link-1']).toEqual(delta.links['link-1']);
    expect(result.links['link-2']).toEqual(existing.links['link-2']);
  });

  it('preserves explicit null values when replacing runtime records', () => {
    const existing = makeSnapshot({
      devices: {
        'dev-1': makeDeviceRuntime({
          cpu_percent: 50,
          temp_celsius: 55,
          last_polled_at: '2026-01-01T00:00:00Z',
        }),
      },
    });
    const delta = makeSnapshot({
      devices: {
        'dev-1': makeDeviceRuntime({
          cpu_percent: null,
          mem_percent: null,
          temp_celsius: null,
          uptime_secs: null,
          last_collected_at: null,
          last_polled_at: null,
          expected_poll_interval_seconds: null,
          metrics_status: 'unavailable',
          metrics_reason: 'stale',
        }),
      },
    });

    const result = mergeSnapshotDelta(existing, delta);

    expect(result.devices['dev-1']).toEqual(delta.devices['dev-1']);
    expect(result.devices['dev-1'].last_polled_at).toBeNull();
    expect(result.devices['dev-1'].cpu_percent).toBeNull();
  });
});

describe('parseDeviceRuntime', () => {
  it('preserves explicit null runtime values and approved reason enums', () => {
    const runtime = parseDeviceRuntime({
      device_id: 'dev-1',
      operational_status: 'probing',
      reachability: 'soft_down',
      health: 'warning',
      freshness: 'awaiting_poll',
      primary_reason: 'awaiting_poll',
      metrics_status: 'unavailable',
      metrics_reason: 'upstream_unavailable',
      alert_status: 'degraded',
      firing_alert_count: 2,
      last_collected_at: null,
      last_polled_at: null,
      expected_poll_interval_seconds: null,
      cpu_percent: null,
      mem_percent: null,
      temp_celsius: null,
      uptime_secs: null,
    });

    expect(runtime.freshness).toBe('awaiting_poll');
    expect(runtime.primary_reason).toBe('awaiting_poll');
    expect(runtime.metrics_reason).toBe('upstream_unavailable');
    expect(runtime.last_collected_at).toBeNull();
    expect(runtime.cpu_percent).toBeNull();
  });

  it('rejects device runtime records with invalid required semantic fields', () => {
    expect(() =>
      parseDeviceRuntime({
        device_id: 'dev-1',
        operational_status: 'invalid-status',
        reachability: 'up',
        health: 'healthy',
        freshness: 'fresh',
        primary_reason: 'ok',
        metrics_status: 'available',
        metrics_reason: 'ok',
        alert_status: 'normal',
        firing_alert_count: 0,
        last_collected_at: null,
        last_polled_at: null,
        expected_poll_interval_seconds: null,
        cpu_percent: null,
        mem_percent: null,
        temp_celsius: null,
        uptime_secs: null,
      }),
    ).toThrow('invalid device runtime payload');
  });

  it('rejects device runtime records when required nullable runtime fields are missing or invalid', () => {
    expect(() =>
      parseDeviceRuntime({
        device_id: 'dev-1',
        operational_status: 'up',
        reachability: 'up',
        health: 'healthy',
        freshness: 'fresh',
        primary_reason: 'ok',
        metrics_status: 'available',
        metrics_reason: 'ok',
        alert_status: 'normal',
        firing_alert_count: 0,
        last_polled_at: null,
        expected_poll_interval_seconds: null,
        cpu_percent: null,
        mem_percent: null,
        temp_celsius: null,
        uptime_secs: null,
      }),
    ).toThrow('invalid device runtime payload');

    expect(() =>
      parseDeviceRuntime({
        device_id: 'dev-1',
        operational_status: 'up',
        reachability: 'up',
        health: 'healthy',
        freshness: 'fresh',
        primary_reason: 'ok',
        metrics_status: 'available',
        metrics_reason: 'ok',
        alert_status: 'normal',
        firing_alert_count: 0,
        last_collected_at: null,
        last_polled_at: null,
        expected_poll_interval_seconds: '60',
        cpu_percent: null,
        mem_percent: null,
        temp_celsius: null,
        uptime_secs: null,
      }),
    ).toThrow('invalid device runtime payload');
  });
});

describe('parseLinkRuntime', () => {
  it('rejects link runtime records with invalid required semantic fields', () => {
    expect(() =>
      parseLinkRuntime({
        link_id: 'link-1',
        source_device_id: 'dev-1',
        target_device_id: 'dev-2',
        source_if_name: 'ether1',
        target_if_name: 'ether2',
        metrics_status: 'unmonitored',
        metrics_reason: 'unmonitored',
        last_collected_at: null,
        tx_bps: null,
        rx_bps: null,
        utilization: null,
      }),
    ).toThrow('invalid link runtime payload');
  });

  it('rejects link runtime records when required nullable telemetry fields are missing or invalid', () => {
    expect(() =>
      parseLinkRuntime({
        link_id: 'link-1',
        source_device_id: 'dev-1',
        target_device_id: 'dev-2',
        source_if_name: 'ether1',
        target_if_name: 'ether2',
        metrics_status: 'available',
        metrics_reason: 'ok',
        tx_bps: null,
        rx_bps: null,
        utilization: null,
      }),
    ).toThrow('invalid link runtime payload');

    expect(() =>
      parseLinkRuntime({
        link_id: 'link-1',
        source_device_id: 'dev-1',
        target_device_id: 'dev-2',
        source_if_name: 'ether1',
        target_if_name: 'ether2',
        metrics_status: 'available',
        metrics_reason: 'ok',
        last_collected_at: null,
        tx_bps: '1000',
        rx_bps: null,
        utilization: null,
      }),
    ).toThrow('invalid link runtime payload');
  });
});

describe('parseSnapshotPayload', () => {
  it('does not synthesize removed split-map sections', () => {
    const message = parseWSMessage({
      type: 'snapshot',
      payload: {
        version: 7,
        snapshot: {
          devices: {},
          links: {},
        },
      },
    });

    const payload = (message as { type: 'snapshot'; payload: SnapshotEnvelopePayload }).payload;
    expect((payload.snapshot as Record<string, unknown>).device_metrics).toBeUndefined();
    expect((payload.snapshot as Record<string, unknown>).link_metrics).toBeUndefined();
    expect((payload.snapshot as Record<string, unknown>).device_statuses).toBeUndefined();
  });

  it('fails snapshot parsing when normalized sections are missing', () => {
    expect(() =>
      parseWSMessage({
        type: 'snapshot',
        payload: {
          devices: {},
        },
      }),
    ).toThrow('invalid snapshot payload');

    expect(() =>
      parseWSMessage({
        type: 'snapshot_delta',
        payload: {
          delta: {
            links: {},
          },
        },
      }),
    ).toThrow('invalid snapshot payload');
  });

  it('fails snapshot parsing when a normalized device runtime record is malformed', () => {
    expect(() =>
      parseWSMessage({
        type: 'snapshot',
        payload: {
          devices: {
            'dev-1': {
              ...makeDeviceRuntime(),
              operational_status: 'broken',
            },
            'dev-2': makeDeviceRuntime({ device_id: 'dev-2' }),
          },
          links: {},
        },
      }),
    ).toThrow('invalid device runtime payload');
  });

  it('fails snapshot parsing when device map keys do not match device_id', () => {
    expect(() =>
      parseWSMessage({
        type: 'snapshot',
        payload: {
          devices: {
            'dev-1': makeDeviceRuntime({ device_id: 'dev-2' }),
          },
          links: {},
        },
      }),
    ).toThrow('invalid snapshot payload');
  });

  it('fails snapshot parsing when a normalized link runtime record is malformed', () => {
    expect(() =>
      parseWSMessage({
        type: 'snapshot',
        payload: {
          devices: {},
          links: {
            'link-1': {
              ...makeLinkRuntime(),
              metrics_status: 'broken',
            },
            'link-2': makeLinkRuntime({
              link_id: 'link-2',
              source_device_id: 'dev-2',
              target_device_id: 'dev-3',
            }),
          },
        },
      }),
    ).toThrow('invalid link runtime payload');
  });

  it('accepts normalized link runtime records when interface names are present but empty', () => {
    const message = parseWSMessage({
      type: 'snapshot_delta',
      payload: {
        delta: {
          devices: {},
          links: {
            'link-1': makeLinkRuntime({
              source_if_name: '',
              target_if_name: '',
              metrics_status: 'unavailable',
              metrics_reason: 'no_data',
              last_collected_at: null,
              tx_bps: null,
              rx_bps: null,
              utilization: null,
            }),
          },
        },
      },
    });

    expect(message.type).toBe('snapshot_delta');
    expect(
      (
        message as {
          type: 'snapshot_delta';
          payload: { delta: { links: Record<string, LinkRuntimeDTO> } };
        }
      ).payload.delta.links['link-1'].source_if_name,
    ).toBe('');
    expect(
      (
        message as {
          type: 'snapshot_delta';
          payload: { delta: { links: Record<string, LinkRuntimeDTO> } };
        }
      ).payload.delta.links['link-1'].target_if_name,
    ).toBe('');
  });

  it('fails snapshot parsing when link map keys do not match link_id', () => {
    expect(() =>
      parseWSMessage({
        type: 'snapshot_delta',
        payload: {
          delta: {
            devices: {},
            links: {
              'link-1': makeLinkRuntime({ link_id: 'link-2' }),
            },
          },
        },
      }),
    ).toThrow('invalid snapshot payload');
  });
});
