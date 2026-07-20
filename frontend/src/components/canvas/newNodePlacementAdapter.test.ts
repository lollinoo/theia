import type { ReactFlowInstance } from '@xyflow/react';
import { describe, expect, it, vi } from 'vitest';

import type { Device, Link } from '../../types/api';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import {
  NEW_NODE_PREFERRED_GAP_PX,
  NEW_NODE_VIEWPORT_MARGIN_PX,
  type ScreenRect,
} from './newNodePlacement';
import { buildExplicitNodePlacements } from './newNodePlacementAdapter';

interface ReactFlowStubOptions {
  canvasRect: ScreenRect;
  viewport?: { x: number; y: number; zoom: number };
  nodes?: DeviceNode[];
  screenToFlowPosition?: (
    point: { x: number; y: number },
    options?: { snapToGrid?: boolean; snapGrid?: [number, number] },
  ) => { x: number; y: number };
}

function device(overrides: Partial<Device> & Pick<Device, 'id'>): Device {
  return {
    id: overrides.id,
    hostname: overrides.hostname ?? overrides.id,
    ip: overrides.ip ?? '192.0.2.1',
    addresses: overrides.addresses ?? [],
    probe_ports: overrides.probe_ports ?? null,
    notes: overrides.notes ?? null,
    device_type: overrides.device_type ?? 'router',
    poll_class: overrides.poll_class ?? 'standard',
    poll_interval_override: overrides.poll_interval_override ?? null,
    polling_enabled: overrides.polling_enabled ?? true,
    status: overrides.status ?? 'up',
    sys_name: overrides.sys_name ?? overrides.id,
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

function node(
  sourceDevice: Device,
  position: { x: number; y: number },
  overrides: Partial<DeviceNode> = {},
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
        monitoringState:
          sourceDevice.device_type === 'virtual' && sourceDevice.ip === ''
            ? 'unmonitored'
            : 'monitorable',
      },
      pinned: false,
    },
    ...overrides,
  };
}

function link(id: string, sourceDeviceId: string, targetDeviceId: string): Link {
  return {
    id,
    source_device_id: sourceDeviceId,
    source_if_name: 'eth0',
    target_device_id: targetDeviceId,
    target_if_name: 'eth1',
    discovery_protocol: 'lldp',
    source_if_speed: 1_000_000_000,
    source_if_oper_status: 'up',
    target_if_speed: 1_000_000_000,
    target_if_oper_status: 'up',
  };
}

function reactFlowStub({
  canvasRect,
  viewport = { x: 0, y: 0, zoom: 1 },
  nodes = [],
  screenToFlowPosition: screenToFlowOverride,
}: ReactFlowStubOptions): {
  reactFlow: ReactFlowInstance<DeviceNode, LinkEdgeType>;
  flowToScreenPosition: ReturnType<typeof vi.fn>;
  screenToFlowPosition: ReturnType<typeof vi.fn>;
} {
  const flowToScreenPosition = vi.fn(({ x, y }: { x: number; y: number }) => ({
    x: canvasRect.x + viewport.x + x * viewport.zoom,
    y: canvasRect.y + viewport.y + y * viewport.zoom,
  }));
  const screenToFlowPosition = vi.fn(
    screenToFlowOverride ??
      ((
        { x, y }: { x: number; y: number },
        options?: { snapToGrid?: boolean; snapGrid?: [number, number] },
      ) => {
        const position = {
          x: (x - canvasRect.x - viewport.x) / viewport.zoom,
          y: (y - canvasRect.y - viewport.y) / viewport.zoom,
        };
        if (!options?.snapToGrid || !options.snapGrid) return position;
        return {
          x: Math.round(position.x / options.snapGrid[0]) * options.snapGrid[0],
          y: Math.round(position.y / options.snapGrid[1]) * options.snapGrid[1],
        };
      }),
  );
  const reactFlow = {
    flowToScreenPosition,
    getNodes: vi.fn(() => nodes),
    getViewport: vi.fn(() => viewport),
    screenToFlowPosition,
  } as unknown as ReactFlowInstance<DeviceNode, LinkEdgeType>;

  return { reactFlow, flowToScreenPosition, screenToFlowPosition };
}

function intersectionArea(left: ScreenRect, right: ScreenRect): number {
  const overlapWidth = Math.max(
    0,
    Math.min(left.x + left.width, right.x + right.width) - Math.max(left.x, right.x),
  );
  const overlapHeight = Math.max(
    0,
    Math.min(left.y + left.height, right.y + right.height) - Math.max(left.y, right.y),
  );
  return overlapWidth * overlapHeight;
}

function expectContainedInCanvasInset(screenRect: ScreenRect, canvasRect: ScreenRect): void {
  expect(screenRect.x).toBeGreaterThanOrEqual(canvasRect.x + NEW_NODE_VIEWPORT_MARGIN_PX);
  expect(screenRect.y).toBeGreaterThanOrEqual(canvasRect.y + NEW_NODE_VIEWPORT_MARGIN_PX);
  expect(screenRect.x + screenRect.width).toBeLessThanOrEqual(
    canvasRect.x + canvasRect.width - NEW_NODE_VIEWPORT_MARGIN_PX,
  );
  expect(screenRect.y + screenRect.height).toBeLessThanOrEqual(
    canvasRect.y + canvasRect.height - NEW_NODE_VIEWPORT_MARGIN_PX,
  );
}

describe('buildExplicitNodePlacements', () => {
  it('round-trips a pan-and-zoom placement into the inset client-space viewport', () => {
    const canvasRect = { x: 80, y: 40, width: 1000, height: 700 };
    const viewport = { x: -320, y: 140, zoom: 0.5 };
    const { reactFlow, flowToScreenPosition, screenToFlowPosition } = reactFlowStub({
      canvasRect,
      viewport,
    });

    const result = buildExplicitNodePlacements({
      reactFlow,
      canvasRect,
      devices: [device({ id: 'target' })],
      links: [],
      deviceIds: new Set(['target']),
    });

    expect(result.placedDeviceIds).toEqual(new Set(['target']));
    const flowTopLeft = result.positions.get('target');
    expect(flowTopLeft).toBeDefined();
    if (!flowTopLeft) return;

    const screenTopLeft = flowToScreenPosition(flowTopLeft);
    const screenRect: ScreenRect = {
      ...screenTopLeft,
      width: 370 * viewport.zoom,
      height: 140 * viewport.zoom,
    };
    expectContainedInCanvasInset(screenRect, canvasRect);
    expect(screenToFlowPosition).toHaveBeenCalledWith(screenTopLeft, { snapToGrid: false });
  });

  it('delegates enabled snapping to React Flow and projects the snapped point for obstacles', () => {
    const canvasRect = { x: 0, y: 0, width: 1000, height: 700 };
    const snappedFlowPosition = { x: 330, y: 270 };
    const { reactFlow, flowToScreenPosition, screenToFlowPosition } = reactFlowStub({
      canvasRect,
      screenToFlowPosition: () => snappedFlowPosition,
    });

    const result = buildExplicitNodePlacements({
      reactFlow,
      canvasRect,
      devices: [device({ id: 'target' })],
      links: [],
      deviceIds: new Set(['target']),
      snapGrid: [30, 30],
    });

    expect(screenToFlowPosition).toHaveBeenCalledWith(
      { x: 315, y: 280 },
      { snapToGrid: true, snapGrid: [30, 30] },
    );
    expect(result.positions.get('target')).toEqual(snappedFlowPosition);
    expect(flowToScreenPosition).toHaveBeenCalledWith(snappedFlowPosition);
  });

  it.each([
    {
      zoom: 0.1,
      canvasRect: { x: 0, y: 0, width: 500, height: 300 },
      viewport: { x: 2.1, y: 0.6, zoom: 0.1 },
      obstacle: { x: 205, y: 116, width: 2, height: 2 },
      expected: { x: 2310, y: 1410 },
    },
    {
      zoom: 1,
      canvasRect: { x: 0, y: 0, width: 1000, height: 700 },
      viewport: { x: 1, y: 0, zoom: 1 },
      obstacle: { x: 260, y: 230, width: 20, height: 20 },
      expected: { x: 330, y: 270 },
    },
    {
      zoom: 2,
      canvasRect: { x: 0, y: 0, width: 1400, height: 900 },
      viewport: { x: 6, y: 42, zoom: 2 },
      obstacle: { x: 260, y: 230, width: 30, height: 30 },
      expected: { x: 150, y: 150 },
    },
  ])('uses a nearby safe grid point when base snapping collides at zoom $zoom', ({
    canvasRect,
    viewport,
    obstacle,
    expected,
  }) => {
    const target = device({ id: 'target' });
    const existing = device({ id: 'existing' });
    const { reactFlow, flowToScreenPosition } = reactFlowStub({
      canvasRect,
      viewport,
      nodes: [
        node(
          existing,
          {
            x: (obstacle.x - canvasRect.x - viewport.x) / viewport.zoom,
            y: (obstacle.y - canvasRect.y - viewport.y) / viewport.zoom,
          },
          {
            measured: {
              width: obstacle.width / viewport.zoom,
              height: obstacle.height / viewport.zoom,
            },
          },
        ),
      ],
    });

    const result = buildExplicitNodePlacements({
      reactFlow,
      canvasRect,
      devices: [target, existing],
      links: [],
      deviceIds: new Set([target.id]),
      snapGrid: [30, 30],
    });

    expect(result.positions.get(target.id)).toEqual(expected);
    const selected = result.positions.get(target.id);
    if (!selected) return;
    const screenRect = {
      ...flowToScreenPosition(selected),
      width: 370 * viewport.zoom,
      height: 140 * viewport.zoom,
    };
    expectContainedInCanvasInset(screenRect, canvasRect);
    expect(
      intersectionArea(screenRect, {
        x: obstacle.x - NEW_NODE_PREFERRED_GAP_PX,
        y: obstacle.y - NEW_NODE_PREFERRED_GAP_PX,
        width: obstacle.width + NEW_NODE_PREFERRED_GAP_PX * 2,
        height: obstacle.height + NEW_NODE_PREFERRED_GAP_PX * 2,
      }),
    ).toBe(0);
  });

  it.each([
    { zoom: 0.1, canvasRect: { x: 0, y: 0, width: 70, height: 100 } },
    { zoom: 1, canvasRect: { x: 0, y: 0, width: 403, height: 300 } },
    { zoom: 2, canvasRect: { x: 0, y: 0, width: 773, height: 400 } },
  ])('rejects explicit placement when no local grid point fits at zoom $zoom', ({
    zoom,
    canvasRect,
  }) => {
    const target = device({ id: 'target' });
    const { reactFlow } = reactFlowStub({
      canvasRect,
      viewport: { x: 0, y: 0, zoom },
    });

    const result = buildExplicitNodePlacements({
      reactFlow,
      canvasRect,
      devices: [target],
      links: [],
      deviceIds: new Set([target.id]),
      snapGrid: [30, 30],
    });

    expect(result.positions).toEqual(new Map());
    expect(result.placedDeviceIds).toEqual(new Set());
  });

  it('keeps no-gap snapped placement from overlapping an obstacle', () => {
    const canvasRect = { x: 0, y: 0, width: 500, height: 220 };
    const viewport = { x: 10, y: 20, zoom: 1 };
    const obstacle = { x: 90, y: 50, width: 10, height: 10 };
    const target = device({ id: 'target' });
    const existing = device({ id: 'existing' });
    const { reactFlow, flowToScreenPosition } = reactFlowStub({
      canvasRect,
      viewport,
      nodes: [node(existing, { x: 80, y: 30 }, { measured: { width: 10, height: 10 } })],
    });

    const result = buildExplicitNodePlacements({
      reactFlow,
      canvasRect,
      devices: [target, existing],
      links: [],
      deviceIds: new Set([target.id]),
      snapGrid: [30, 30],
    });

    expect(result.positions.get(target.id)).toEqual({ x: 90, y: 30 });
    const selected = result.positions.get(target.id);
    if (!selected) return;
    expect(
      intersectionArea({ ...flowToScreenPosition(selected), width: 370, height: 140 }, obstacle),
    ).toBe(0);
  });

  it('selects the least-overlap local grid point when overlap is unavoidable', () => {
    const canvasRect = { x: 0, y: 0, width: 500, height: 220 };
    const viewport = { x: 10, y: 1, zoom: 1 };
    const target = device({ id: 'target' });
    const existing = device({ id: 'existing' });
    const { reactFlow } = reactFlowStub({
      canvasRect,
      viewport,
      nodes: [node(existing, { x: 60, y: 59 }, { measured: { width: 50, height: 100 } })],
    });

    const result = buildExplicitNodePlacements({
      reactFlow,
      canvasRect,
      devices: [target, existing],
      links: [],
      deviceIds: new Set([target.id]),
      snapGrid: [30, 30],
    });

    expect(result.positions.get(target.id)).toEqual({ x: 90, y: 60 });
  });

  it('uses measured node dimensions before the rendered-card fallback', () => {
    const canvasRect = { x: 0, y: 0, width: 1000, height: 700 };
    const target = device({ id: 'target' });
    const existing = device({
      id: 'existing',
      device_type: 'virtual',
      ip: '192.0.2.2',
    });
    const { reactFlow } = reactFlowStub({
      canvasRect,
      nodes: [node(existing, { x: 500, y: 300 }, { measured: { width: 40, height: 40 } })],
    });

    const result = buildExplicitNodePlacements({
      reactFlow,
      canvasRect,
      devices: [target, existing],
      links: [],
      deviceIds: new Set([target.id]),
    });

    expect(result.positions.get(target.id)).toEqual({ x: 335, y: 364 });
  });

  it('uses public node dimensions before the rendered-card fallback', () => {
    const canvasRect = { x: 0, y: 0, width: 1000, height: 700 };
    const target = device({ id: 'target' });
    const existing = device({ id: 'existing' });
    const { reactFlow } = reactFlowStub({
      canvasRect,
      nodes: [node(existing, { x: 500, y: 300 }, { width: 60, height: 50 })],
    });

    const result = buildExplicitNodePlacements({
      reactFlow,
      canvasRect,
      devices: [target, existing],
      links: [],
      deviceIds: new Set([target.id]),
    });

    expect(result.positions.get(target.id)).toEqual({ x: 345, y: 374 });
  });

  it.each([
    {
      name: 'physical for an unmatched ghost',
      existingDevice: null,
      nodeDevice: device({ id: 'existing', device_type: 'virtual', ip: '' }),
      expected: { x: 208, y: 208 },
    },
    {
      name: 'virtual monitorable',
      existingDevice: device({
        id: 'existing',
        device_type: 'virtual',
        ip: '192.0.2.2',
      }),
      nodeDevice: device({
        id: 'existing',
        device_type: 'virtual',
        ip: '192.0.2.2',
      }),
      expected: { x: 208, y: 192 },
    },
    {
      name: 'virtual unmonitored',
      existingDevice: device({ id: 'existing', device_type: 'virtual', ip: '' }),
      nodeDevice: device({ id: 'existing', device_type: 'virtual', ip: '' }),
      expected: { x: 215, y: 180 },
    },
  ])('uses the $name conservative fallback dimensions', ({
    existingDevice,
    nodeDevice,
    expected,
  }) => {
    const canvasRect = { x: 0, y: 0, width: 800, height: 500 };
    const target = device({ id: 'target' });
    const { reactFlow } = reactFlowStub({
      canvasRect,
      nodes: [node(nodeDevice, { x: 0, y: 40 }, existingDevice ? {} : { type: 'ghost-device' })],
    });

    const result = buildExplicitNodePlacements({
      reactFlow,
      canvasRect,
      devices: existingDevice ? [target, existingDevice] : [target],
      links: [],
      deviceIds: new Set([target.id]),
    });

    expect(result.positions.get(target.id)).toEqual(expected);
  });

  it('subtracts an explicit node origin to build the rendered top-left obstacle', () => {
    const canvasRect = { x: 0, y: 0, width: 1000, height: 700 };
    const target = device({ id: 'target' });
    const existing = device({ id: 'existing' });
    const { reactFlow } = reactFlowStub({
      canvasRect,
      nodes: [
        node(
          existing,
          { x: 500, y: 300 },
          {
            measured: { width: 100, height: 80 },
            origin: [0.5, 0.5],
          },
        ),
      ],
    });

    const result = buildExplicitNodePlacements({
      reactFlow,
      canvasRect,
      devices: [target, existing],
      links: [],
      deviceIds: new Set([target.id]),
    });

    expect(result.positions.get(target.id)).toEqual({ x: 315, y: 364 });
  });

  it('excludes a pending target already returned by React Flow before obstacle projection', () => {
    const canvasRect = { x: 0, y: 0, width: 1000, height: 700 };
    const target = device({ id: 'target' });
    const { reactFlow, flowToScreenPosition } = reactFlowStub({
      canvasRect,
      nodes: [node(target, { x: 10_000, y: 10_000 }, { measured: { width: 370, height: 140 } })],
    });

    const result = buildExplicitNodePlacements({
      reactFlow,
      canvasRect,
      devices: [target],
      links: [],
      deviceIds: new Set([target.id]),
    });

    expect(result.positions.get(target.id)).toEqual({ x: 315, y: 280 });
    expect(flowToScreenPosition).toHaveBeenCalledOnce();
    expect(flowToScreenPosition).toHaveBeenCalledWith({ x: 315, y: 280 });
  });

  it('excludes hidden existing nodes from placement obstacles', () => {
    const canvasRect = { x: 0, y: 0, width: 1000, height: 700 };
    const target = device({ id: 'target' });
    const existing = device({ id: 'hidden-existing' });
    const { reactFlow, flowToScreenPosition } = reactFlowStub({
      canvasRect,
      nodes: [
        node(
          existing,
          { x: 500, y: 300 },
          {
            hidden: true,
            measured: { width: 40, height: 40 },
          },
        ),
      ],
    });

    const result = buildExplicitNodePlacements({
      reactFlow,
      canvasRect,
      devices: [target, existing],
      links: [],
      deviceIds: new Set([target.id]),
    });

    expect(result.positions.get(target.id)).toEqual({ x: 315, y: 280 });
    expect(flowToScreenPosition).toHaveBeenCalledOnce();
    expect(flowToScreenPosition).toHaveBeenCalledWith({ x: 315, y: 280 });
  });

  it('keeps a measured ghost node as an obstacle without a fetched device match', () => {
    const canvasRect = { x: 0, y: 0, width: 1000, height: 700 };
    const target = device({ id: 'target' });
    const ghost = device({ id: 'ghost', device_type: 'virtual', ip: '' });
    const { reactFlow } = reactFlowStub({
      canvasRect,
      nodes: [
        node(
          ghost,
          { x: 500, y: 300 },
          {
            type: 'ghost-device',
            measured: { width: 40, height: 40 },
          },
        ),
      ],
    });

    const result = buildExplicitNodePlacements({
      reactFlow,
      canvasRect,
      devices: [target],
      links: [],
      deviceIds: new Set([target.id]),
    });

    expect(result.positions.get(target.id)).toEqual({ x: 335, y: 364 });
  });

  it('retains a near-offscreen obstacle that can affect preferred clearance', () => {
    const canvasRect = { x: 0, y: 0, width: 410, height: 300 };
    const target = device({ id: 'target' });
    const existing = device({ id: 'existing' });
    const { reactFlow } = reactFlowStub({
      canvasRect,
      nodes: [node(existing, { x: 410, y: 80 }, { measured: { width: 20, height: 140 } })],
    });

    const result = buildExplicitNodePlacements({
      reactFlow,
      canvasRect,
      devices: [target, existing],
      links: [],
      deviceIds: new Set([target.id]),
    });

    expect(result.positions.get(target.id)).toEqual({ x: 16, y: 80 });
  });

  it('uses only visible connected-node centers to break an equal placement rank', () => {
    const canvasRect = { x: 0, y: 0, width: 804, height: 172 };
    const target = device({ id: 'target' });
    const barrier = device({ id: 'barrier' });
    const secondVisibleNeighbor = device({ id: 'a-visible-neighbor' });
    const visibleNeighbor = device({ id: 'visible-neighbor' });
    const offscreenNeighbor = device({ id: 'offscreen-neighbor' });
    const nodes = [
      node(barrier, { x: 386, y: 16 }, { measured: { width: 32, height: 140 } }),
      node(visibleNeighbor, { x: 415, y: 85.5 }, { measured: { width: 1, height: 1 } }),
      node(secondVisibleNeighbor, { x: 400, y: 85.5 }, { measured: { width: 1, height: 1 } }),
      node(offscreenNeighbor, { x: 804, y: 85.5 }, { measured: { width: 1, height: 1 } }),
    ];
    const devices = [target, barrier, visibleNeighbor, secondVisibleNeighbor, offscreenNeighbor];
    const { reactFlow: withVisibleNeighbor } = reactFlowStub({ canvasRect, nodes });
    const { reactFlow: withReversedLinks } = reactFlowStub({ canvasRect, nodes });
    const { reactFlow: withOffscreenNeighbor } = reactFlowStub({ canvasRect, nodes });
    const visibleLinks = [
      link('self', target.id, target.id),
      link('visible', visibleNeighbor.id, target.id),
      link('second-visible', target.id, secondVisibleNeighbor.id),
    ];

    const visibleResult = buildExplicitNodePlacements({
      reactFlow: withVisibleNeighbor,
      canvasRect,
      devices,
      links: visibleLinks,
      deviceIds: new Set([target.id]),
    });
    const reversedLinksResult = buildExplicitNodePlacements({
      reactFlow: withReversedLinks,
      canvasRect,
      devices,
      links: [...visibleLinks].reverse(),
      deviceIds: new Set([target.id]),
    });
    const offscreenResult = buildExplicitNodePlacements({
      reactFlow: withOffscreenNeighbor,
      canvasRect,
      devices,
      links: [link('offscreen', target.id, offscreenNeighbor.id)],
      deviceIds: new Set([target.id]),
    });

    expect(visibleResult.positions.get(target.id)).toEqual({ x: 418, y: 16 });
    expect(reversedLinksResult.positions).toEqual(visibleResult.positions);
    expect(offscreenResult.positions.get(target.id)).toEqual({ x: 16, y: 16 });
  });

  it.each([
    {
      zoom: 0.4,
      canvasRect: { x: 37, y: 61, width: 1000, height: 700 },
      viewport: { x: -180, y: 95, zoom: 0.4 },
      target: device({ id: 'physical-target' }),
      flowSize: { width: 370, height: 140 },
    },
    {
      zoom: 1,
      canvasRect: { x: 80, y: 40, width: 1000, height: 700 },
      viewport: { x: -320, y: 140, zoom: 1 },
      target: device({
        id: 'monitorable-target',
        device_type: 'virtual',
        ip: '192.0.2.20',
      }),
      flowSize: { width: 430, height: 128 },
    },
    {
      zoom: 1.8,
      canvasRect: { x: 123, y: 77, width: 1000, height: 700 },
      viewport: { x: 210, y: -165, zoom: 1.8 },
      target: device({ id: 'unmonitored-target', device_type: 'virtual', ip: '' }),
      flowSize: { width: 350, height: 102 },
    },
  ])('contains a round-tripped target at zoom $zoom with canvas and pan offsets', ({
    canvasRect,
    viewport,
    target,
    flowSize,
  }) => {
    const { reactFlow, flowToScreenPosition } = reactFlowStub({ canvasRect, viewport });

    const result = buildExplicitNodePlacements({
      reactFlow,
      canvasRect,
      devices: [target],
      links: [],
      deviceIds: new Set([target.id]),
    });

    const flowTopLeft = result.positions.get(target.id);
    expect(flowTopLeft).toBeDefined();
    if (!flowTopLeft) return;
    expectContainedInCanvasInset(
      {
        ...flowToScreenPosition(flowTopLeft),
        width: flowSize.width * viewport.zoom,
        height: flowSize.height * viewport.zoom,
      },
      canvasRect,
    );
  });

  it('places simultaneous IDs without overlap and independently of set insertion order', () => {
    const canvasRect = { x: 55, y: 35, width: 1200, height: 800 };
    const viewport = { x: -240, y: 110, zoom: 1 };
    const first = device({ id: 'a-target' });
    const second = device({ id: 'z-target' });
    const devices = [second, first];
    const reverseStub = reactFlowStub({ canvasRect, viewport });
    const forwardStub = reactFlowStub({ canvasRect, viewport });

    const reverseResult = buildExplicitNodePlacements({
      reactFlow: reverseStub.reactFlow,
      canvasRect,
      devices,
      links: [],
      deviceIds: new Set([second.id, 'missing-target', first.id]),
    });
    const forwardResult = buildExplicitNodePlacements({
      reactFlow: forwardStub.reactFlow,
      canvasRect,
      devices,
      links: [],
      deviceIds: new Set([first.id, 'missing-target', second.id]),
    });

    expect(reverseResult.placedDeviceIds).toEqual(new Set([first.id, second.id]));
    expect(reverseResult.positions).toEqual(forwardResult.positions);
    expect(reverseResult.placedDeviceIds).toEqual(forwardResult.placedDeviceIds);

    const firstPosition = reverseResult.positions.get(first.id);
    const secondPosition = reverseResult.positions.get(second.id);
    expect(firstPosition).toBeDefined();
    expect(secondPosition).toBeDefined();
    if (!firstPosition || !secondPosition) return;

    const firstRect = {
      ...reverseStub.flowToScreenPosition(firstPosition),
      width: 370,
      height: 140,
    };
    const secondRect = {
      ...reverseStub.flowToScreenPosition(secondPosition),
      width: 370,
      height: 140,
    };
    expect(intersectionArea(firstRect, secondRect)).toBe(0);
    expectContainedInCanvasInset(firstRect, canvasRect);
    expectContainedInCanvasInset(secondRect, canvasRect);
  });

  it('does not mark a target placed when client-to-flow conversion is non-finite', () => {
    const canvasRect = { x: 0, y: 0, width: 1000, height: 700 };
    const target = device({ id: 'target' });
    const { reactFlow, screenToFlowPosition } = reactFlowStub({
      canvasRect,
      screenToFlowPosition: () => ({ x: Number.NaN, y: 12 }),
    });

    const result = buildExplicitNodePlacements({
      reactFlow,
      canvasRect,
      devices: [target],
      links: [],
      deviceIds: new Set([target.id]),
    });

    expect(screenToFlowPosition).toHaveBeenCalledWith({ x: 315, y: 280 }, { snapToGrid: false });
    expect(result.positions).toEqual(new Map());
    expect(result.placedDeviceIds).toEqual(new Set());
  });

  it.each([
    {
      name: 'zero zoom',
      canvasRect: { x: 0, y: 0, width: 800, height: 500 },
      viewport: { x: 0, y: 0, zoom: 0 },
    },
    {
      name: 'negative zoom',
      canvasRect: { x: 0, y: 0, width: 800, height: 500 },
      viewport: { x: 0, y: 0, zoom: -1 },
    },
    {
      name: 'non-finite zoom',
      canvasRect: { x: 0, y: 0, width: 800, height: 500 },
      viewport: { x: 0, y: 0, zoom: Number.NaN },
    },
    {
      name: 'zero canvas width',
      canvasRect: { x: 0, y: 0, width: 0, height: 500 },
      viewport: { x: 0, y: 0, zoom: 1 },
    },
    {
      name: 'non-finite canvas height',
      canvasRect: { x: 0, y: 0, width: 800, height: Number.POSITIVE_INFINITY },
      viewport: { x: 0, y: 0, zoom: 1 },
    },
    {
      name: 'non-finite canvas coordinate',
      canvasRect: { x: Number.NaN, y: 0, width: 800, height: 500 },
      viewport: { x: 0, y: 0, zoom: 1 },
    },
  ])('returns an empty result for $name', ({ canvasRect, viewport }) => {
    const target = device({ id: 'target' });
    const { reactFlow } = reactFlowStub({ canvasRect, viewport });

    const result = buildExplicitNodePlacements({
      reactFlow,
      canvasRect,
      devices: [target],
      links: [],
      deviceIds: new Set([target.id]),
    });

    expect(result.positions).toEqual(new Map());
    expect(result.placedDeviceIds).toEqual(new Set());
  });
});
