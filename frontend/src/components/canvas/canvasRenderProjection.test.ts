import { describe, expect, it, vi } from 'vitest';

import type { Device, Link } from '../../types/api';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import {
  type CanvasRenderProjectionNodeCacheEntry,
  type ProjectCanvasRenderGraphInput,
  projectCanvasRenderGraph,
} from './canvasRenderProjection';
import { buildRuntimeState } from './runtimeAdapters';

function device(overrides: Partial<Device> & Pick<Device, 'id'>): Device {
  return {
    id: overrides.id,
    hostname: overrides.hostname ?? overrides.id,
    ip: overrides.ip ?? `192.0.2.${overrides.id.length}`,
    notes: null,
    device_type: overrides.device_type ?? 'switch',
    poll_class: overrides.poll_class ?? 'standard',
    poll_interval_override: null,
    polling_enabled: true,
    status: overrides.status ?? 'up',
    sys_name: overrides.sys_name ?? overrides.hostname ?? overrides.id,
    sys_descr: overrides.sys_descr ?? '',
    hardware_model: overrides.hardware_model ?? '',
    os_version: overrides.os_version,
    vendor: overrides.vendor ?? '',
    managed: overrides.managed ?? true,
    tags: overrides.tags ?? {},
    interfaces: overrides.interfaces ?? [],
    area_ids: overrides.area_ids ?? [],
    backup_supported: overrides.backup_supported ?? false,
    metrics_source: overrides.metrics_source ?? 'none',
    prometheus_label_name: overrides.prometheus_label_name ?? '',
    prometheus_label_value: overrides.prometheus_label_value ?? '',
    topology_discovery_mode: overrides.topology_discovery_mode,
    effective_topology_discovery_mode: overrides.effective_topology_discovery_mode,
    topology_bootstrap_state: overrides.topology_bootstrap_state,
    last_topology_discovery_at: overrides.last_topology_discovery_at,
    last_topology_discovery_result: overrides.last_topology_discovery_result,
    map_visual_color: overrides.map_visual_color,
  };
}

function link(id: string, source: Device, target: Device): Link {
  return {
    id,
    source_device_id: source.id,
    source_if_name: 'eth0',
    target_device_id: target.id,
    target_if_name: 'eth1',
    discovery_protocol: 'lldp',
    source_if_speed: 1_000_000_000,
    source_if_oper_status: 'up',
    target_if_speed: 1_000_000_000,
    target_if_oper_status: 'up',
  };
}

function nodeFor(
  sourceDevice: Device,
  position: DeviceNode['position'],
  data: Partial<DeviceNode['data']> = {},
): DeviceNode {
  return {
    id: sourceDevice.id,
    type: 'device',
    position,
    data: {
      device: sourceDevice,
      runtime: {
        status: sourceDevice.status,
        metrics: null,
        alertStatus: 'normal',
        monitoringState: 'monitorable',
      },
      pinned: false,
      ...data,
    },
  };
}

function edgeFor(sourceLink: Link, data: Partial<NonNullable<LinkEdgeType['data']>> = {}) {
  return {
    id: sourceLink.id,
    type: 'link',
    source: sourceLink.source_device_id,
    target: sourceLink.target_device_id,
    data: {
      link: sourceLink,
      ...data,
    },
  } satisfies LinkEdgeType;
}

function inputFor(
  overrides: Partial<ProjectCanvasRenderGraphInput> & {
    devices: Device[];
    edges: LinkEdgeType[];
    filteredDevices: Device[];
    filteredLinks: Link[];
    links: Link[];
    nodes: DeviceNode[];
  },
): ProjectCanvasRenderGraphInput {
  return {
    nodes: overrides.nodes,
    edges: overrides.edges,
    devices: overrides.devices,
    filteredDevices: overrides.filteredDevices,
    filteredLinks: overrides.filteredLinks,
    ghostDevices: overrides.ghostDevices ?? [],
    runtimeState:
      overrides.runtimeState ??
      buildRuntimeState({
        devices: overrides.devices,
        links: overrides.links,
        snapshot: null,
        alerts: [],
        prometheusStatus: { available: true },
      }),
    areaColorMap: overrides.areaColorMap ?? new Map(),
    effectiveAreaId: overrides.effectiveAreaId ?? null,
    selectedRealNodeIds: overrides.selectedRealNodeIds ?? new Set(),
    ghostMeasurements: overrides.ghostMeasurements ?? new Map(),
    areaColorNodeCache: overrides.areaColorNodeCache ?? new Map(),
    onGhostClick: overrides.onGhostClick ?? vi.fn(),
  };
}

describe('projectCanvasRenderGraph', () => {
  it('returns canonical global nodes and selected-edge emphasis without ghost nodes', () => {
    const alpha = device({ id: 'alpha' });
    const beta = device({ id: 'beta' });
    const gamma = device({ id: 'gamma' });
    const alphaBeta = link('alpha-beta', alpha, beta);
    const alphaGamma = link('alpha-gamma', alpha, gamma);
    const nodes = [
      nodeFor(alpha, { x: 0, y: 0 }),
      nodeFor(beta, { x: 120, y: 0 }),
      nodeFor(gamma, { x: 240, y: 0 }),
    ];
    const edges = [edgeFor(alphaBeta), edgeFor(alphaGamma)];

    const result = projectCanvasRenderGraph(
      inputFor({
        nodes,
        edges,
        devices: [alpha, beta, gamma],
        links: [alphaBeta, alphaGamma],
        filteredDevices: [alpha, beta, gamma],
        filteredLinks: [alphaBeta, alphaGamma],
        ghostDevices: [device({ id: 'remote', area_ids: ['remote-area'] })],
        selectedRealNodeIds: new Set(['beta']),
      }),
    );

    expect(result.displayNodes.map((node) => node.id)).toEqual(['alpha', 'beta', 'gamma']);
    expect(result.displayNodes.every((node) => node.data.kind !== 'ghost-device')).toBe(true);
    expect(result.displayEdges.map((edge) => [edge.id, edge.data?.emphasis])).toEqual([
      ['alpha-beta', 'connected'],
      ['alpha-gamma', 'muted'],
    ]);
  });

  it('returns area nodes plus cross-area ghosts and reuses ghost measurements', () => {
    const alpha = device({ id: 'alpha', area_ids: ['area-a'] });
    const beta = device({ id: 'beta', area_ids: ['area-a'] });
    const remote = device({ id: 'remote', area_ids: ['area-b'] });
    const alphaBeta = link('alpha-beta', alpha, beta);
    const alphaRemote = link('alpha-remote', alpha, remote);
    const nodes = [
      nodeFor(alpha, { x: 0, y: 0 }),
      nodeFor(beta, { x: 120, y: 0 }),
      nodeFor(remote, { x: 480, y: 0 }),
    ];
    const edges = [edgeFor(alphaBeta), edgeFor(alphaRemote)];
    const remoteMeasurement = { width: 172, height: 88 };
    const onGhostClick = vi.fn();

    const result = projectCanvasRenderGraph(
      inputFor({
        nodes,
        edges,
        devices: [alpha, beta, remote],
        links: [alphaBeta, alphaRemote],
        filteredDevices: [alpha, beta],
        filteredLinks: [alphaBeta, alphaRemote],
        ghostDevices: [remote],
        effectiveAreaId: 'area-a',
        ghostMeasurements: new Map([['remote', remoteMeasurement]]),
        onGhostClick,
      }),
    );

    expect(result.displayNodes.map((node) => node.id)).toEqual(['alpha', 'beta', 'remote']);
    expect(result.displayEdges.map((edge) => edge.id)).toEqual(['alpha-beta', 'alpha-remote']);

    const ghostNode = result.displayNodes.find((node) => node.id === 'remote');
    expect(ghostNode?.data.kind).toBe('ghost-device');
    expect(ghostNode?.data.isGhost).toBe(true);
    expect(ghostNode?.draggable).toBe(false);
    expect(ghostNode?.position).toEqual({ x: 480, y: 0 });
    expect(ghostNode?.measured).toBe(remoteMeasurement);

    ghostNode?.data.onGhostClick?.('remote');
    expect(onGhostClick).toHaveBeenCalledWith('remote');
  });

  it('preserves area-color and virtual visual-color node references through an explicit cache', () => {
    const physical = device({ id: 'physical', area_ids: ['area-a'] });
    const virtual = device({
      id: 'virtual',
      area_ids: ['area-a'],
      device_type: 'virtual',
      map_visual_color: '#ff33aa',
    });
    const nodes = [nodeFor(physical, { x: 0, y: 0 }), nodeFor(virtual, { x: 120, y: 0 })];
    const initialCache = new Map<string, CanvasRenderProjectionNodeCacheEntry>();
    const baseInput = inputFor({
      nodes,
      edges: [],
      devices: [physical, virtual],
      links: [],
      filteredDevices: [physical, virtual],
      filteredLinks: [],
      areaColorMap: new Map([['area-a', '#2288ff']]),
      areaColorNodeCache: initialCache,
    });

    const first = projectCanvasRenderGraph(baseInput);
    const second = projectCanvasRenderGraph({
      ...baseInput,
      areaColorNodeCache: first.areaColorNodeCache,
    });

    expect(first.nodesWithAreaColor[0]).not.toBe(nodes[0]);
    expect(first.nodesWithAreaColor[1]).not.toBe(nodes[1]);
    expect(first.nodesWithAreaColor[0]?.data.areaColors).toEqual(['#2288ff']);
    expect(first.nodesWithAreaColor[1]?.data.visualColor).toBe('#ff33aa');
    expect(second.nodesWithAreaColor[0]).toBe(first.nodesWithAreaColor[0]);
    expect(second.nodesWithAreaColor[1]).toBe(first.nodesWithAreaColor[1]);
    expect(initialCache.size).toBe(0);
    expect(second.areaColorNodeCache).not.toBe(first.areaColorNodeCache);
  });

  it('keeps edge objects whose selection emphasis is already correct', () => {
    const alpha = device({ id: 'alpha' });
    const beta = device({ id: 'beta' });
    const gamma = device({ id: 'gamma' });
    const delta = device({ id: 'delta' });
    const alphaBeta = link('alpha-beta', alpha, beta);
    const gammaDelta = link('gamma-delta', gamma, delta);
    const alphaGamma = link('alpha-gamma', alpha, gamma);
    const alreadyConnected = edgeFor(alphaBeta, { emphasis: 'connected' });
    const alreadyMuted = edgeFor(gammaDelta, { emphasis: 'muted' });
    const needsChange = edgeFor(alphaGamma, { emphasis: 'muted' });

    const result = projectCanvasRenderGraph(
      inputFor({
        nodes: [
          nodeFor(alpha, { x: 0, y: 0 }),
          nodeFor(beta, { x: 120, y: 0 }),
          nodeFor(gamma, { x: 240, y: 0 }),
          nodeFor(delta, { x: 360, y: 0 }),
        ],
        edges: [alreadyConnected, alreadyMuted, needsChange],
        devices: [alpha, beta, gamma, delta],
        links: [alphaBeta, gammaDelta, alphaGamma],
        filteredDevices: [alpha, beta, gamma, delta],
        filteredLinks: [alphaBeta, gammaDelta, alphaGamma],
        selectedRealNodeIds: new Set(['alpha']),
      }),
    );

    expect(result.displayEdges[0]).toBe(alreadyConnected);
    expect(result.displayEdges[1]).toBe(alreadyMuted);
    expect(result.displayEdges[2]).not.toBe(needsChange);
    expect(result.displayEdges[2]?.data).not.toBe(needsChange.data);
    expect(result.displayEdges.map((edge) => [edge.id, edge.data?.emphasis])).toEqual([
      ['alpha-beta', 'connected'],
      ['gamma-delta', 'muted'],
      ['alpha-gamma', 'connected'],
    ]);
  });

  it('keeps default-emphasis edge data references when selection is empty', () => {
    const alpha = device({ id: 'alpha' });
    const beta = device({ id: 'beta' });
    const alphaBeta = link('alpha-beta', alpha, beta);
    const alreadyDefault = edgeFor(alphaBeta, { emphasis: 'default' });

    const result = projectCanvasRenderGraph(
      inputFor({
        nodes: [nodeFor(alpha, { x: 0, y: 0 }), nodeFor(beta, { x: 120, y: 0 })],
        edges: [alreadyDefault],
        devices: [alpha, beta],
        links: [alphaBeta],
        filteredDevices: [alpha, beta],
        filteredLinks: [alphaBeta],
        selectedRealNodeIds: new Set(),
      }),
    );

    expect(result.displayEdges[0]).toBe(alreadyDefault);
    expect(result.displayEdges[0]?.data).toBe(alreadyDefault.data);
    expect(result.displayEdges[0]?.data?.emphasis).toBe('default');
  });
});
