/**
 * Exercises runtime snapshot patch topology canvas behavior so refactors preserve the documented contract.
 */
import { describe, expect, it, vi } from 'vitest';

import type { Device, Link } from '../../types/api';
import type { SnapshotPayload } from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import type { RuntimeState } from './runtimeAdapters';
import { applyRuntimeSnapshotPatch } from './runtimeSnapshotPatch';

function device(id: string, status: Device['status'] = 'up'): Device {
  return {
    id,
    hostname: id,
    ip: '',
    device_type: 'router',
    poll_class: 'standard',
    poll_interval_override: null,
    status,
    sys_name: id,
    sys_descr: '',
    hardware_model: '',
    vendor: '',
    managed: true,
    interfaces: [],
    area_ids: [],
    backup_supported: false,
    metrics_source: 'prometheus',
    prometheus_label_name: '',
    prometheus_label_value: '',
  };
}

function snapshot(status: Device['status']): SnapshotPayload {
  return {
    devices: {
      'dev-1': {
        device_id: 'dev-1',
        operational_status: status,
        reachability: status,
        health: status === 'down' ? 'critical' : 'ok',
        freshness: 'fresh',
        primary_reason: 'ok',
        metrics_status: 'available',
        metrics_reason: 'ok',
        alert_status: 'normal',
        firing_alert_count: 0,
        last_collected_at: '',
        last_polled_at: '',
        expected_poll_interval_seconds: 60,
        cpu_percent: null,
        mem_percent: null,
        temp_celsius: null,
        uptime_secs: null,
      },
    },
    links: {},
  };
}

function node(): DeviceNode {
  return {
    id: 'dev-1',
    type: 'device',
    position: { x: 0, y: 0 },
    data: {
      device: device('dev-1'),
      pinned: false,
      runtime: {
        status: 'up',
        metrics: null,
        alertStatus: 'normal',
        monitoringState: 'monitored',
      },
    },
  } as DeviceNode;
}

describe('applyRuntimeSnapshotPatch', () => {
  it('leaves last-applied snapshot untouched when no devices are loaded', () => {
    const previous = snapshot('up');
    const setNodes = vi.fn();
    const setEdges = vi.fn();

    const result = applyRuntimeSnapshotPatch({
      previousSnapshot: previous,
      snapshot: snapshot('down'),
      devices: [],
      links: [],
      alerts: [],
      prometheusStatus: null,
      setNodes,
      setEdges,
      openEdgeMenu: vi.fn(),
    });

    expect(result).toBe(previous);
    expect(setNodes).not.toHaveBeenCalled();
    expect(setEdges).not.toHaveBeenCalled();
  });

  it('updates last-applied snapshot and schedules node/edge runtime patches', () => {
    const nextSnapshot = snapshot('down');
    const setNodes = vi.fn((updater: (nodes: DeviceNode[]) => DeviceNode[]) => {
      const patched = updater([node()]);
      expect(patched[0].data.runtime.status).toBe('down');
    });
    const setEdges = vi.fn((updater: (edges: LinkEdgeType[]) => LinkEdgeType[]) => {
      expect(updater([])).toEqual([]);
    });

    const result = applyRuntimeSnapshotPatch({
      previousSnapshot: snapshot('up'),
      snapshot: nextSnapshot,
      devices: [device('dev-1')],
      links: [] as Link[],
      alerts: [],
      prometheusStatus: null,
      setNodes,
      setEdges,
      openEdgeMenu: vi.fn(),
    });

    expect(result).toBe(nextSnapshot);
    expect(setNodes).toHaveBeenCalledTimes(1);
    expect(setEdges).toHaveBeenCalledTimes(1);
  });

  it('reuses a provided runtimeState instead of computing a new one', () => {
    const nextSnapshot = snapshot('down');
    const customRuntimeState: RuntimeState = {
      prometheusDown: false,
      firingAlerts: [],
      devicesById: new Map([
        [
          'dev-1',
          {
            device: device('dev-1', 'probing'),
            monitoringState: 'monitored',
            metrics: null,
            alertStatus: 'down',
            runtimeStatus: 'probing',
            primaryHealth: null,
            runtimeFlags: [],
          },
        ],
      ]),
      linksById: new Map(),
      interfaceMetricsByDeviceId: new Map(),
    };

    const setNodes = vi.fn((updater: (nodes: DeviceNode[]) => DeviceNode[]) => {
      const patched = updater([node()]);
      expect(patched[0].data.runtime.status).toBe('probing');
      expect(patched[0].data.runtime.alertStatus).toBe('down');
    });
    const setEdges = vi.fn();

    const result = applyRuntimeSnapshotPatch({
      previousSnapshot: snapshot('up'),
      snapshot: nextSnapshot,
      devices: [device('dev-1')],
      links: [] as Link[],
      alerts: [],
      prometheusStatus: null,
      setNodes,
      setEdges,
      openEdgeMenu: vi.fn(),
      runtimeState: customRuntimeState,
    });

    expect(result).toBe(nextSnapshot);
    expect(setNodes).toHaveBeenCalledTimes(1);
  });

  it('invokes setDevices via patchRuntimeDevices when device runtime statuses change', () => {
    const nextSnapshot = snapshot('down');
    const setNodes = vi.fn();
    const setEdges = vi.fn();
    const setDevices = vi.fn((updater: (devices: Device[]) => Device[]) => {
      const patched = updater([device('dev-1', 'up')]);
      expect(patched[0].status).toBe('down');
      return patched;
    });

    const result = applyRuntimeSnapshotPatch({
      previousSnapshot: snapshot('up'),
      snapshot: nextSnapshot,
      devices: [device('dev-1')],
      links: [] as Link[],
      alerts: [],
      prometheusStatus: null,
      setNodes,
      setEdges,
      openEdgeMenu: vi.fn(),
      setDevices,
    });

    expect(result).toBe(nextSnapshot);
    expect(setDevices).toHaveBeenCalledTimes(1);
  });

  it('does not invoke setDevices when device runtime statuses do not change', () => {
    const nextSnapshot = snapshot('up');
    const setNodes = vi.fn();
    const setEdges = vi.fn();
    const setDevices = vi.fn();

    const result = applyRuntimeSnapshotPatch({
      previousSnapshot: snapshot('up'),
      snapshot: nextSnapshot,
      devices: [device('dev-1')],
      links: [] as Link[],
      alerts: [],
      prometheusStatus: null,
      setNodes,
      setEdges,
      openEdgeMenu: vi.fn(),
      setDevices,
    });

    expect(result).toBe(nextSnapshot);
    expect(setDevices).not.toHaveBeenCalled();
  });
});
