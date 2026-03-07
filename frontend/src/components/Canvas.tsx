import { startTransition, useEffect, useRef, useState } from 'react';
import {
  ConnectionMode,
  Background,
  MiniMap,
  ReactFlow,
  addEdge,
  applyEdgeChanges,
  useNodesState,
  useReactFlow,
  type Connection,
  type Edge,
  type EdgeChange,
  type Node,
} from 'reactflow';
import { fetchDevices, fetchLinks } from '../api/client';
import { computeForceLayout } from '../hooks/useAutoLayout';
import { usePositions, type PositionPayload } from '../hooks/usePositions';
import { useWebSocket } from '../hooks/useWebSocket';
import type { Device, Link } from '../types/api';
import { alertStatusForDevice, formatThroughput, type LinkMetricsDTO } from '../types/metrics';
import DeviceCard, { type DeviceNodeData } from './DeviceCard';
import LinkEdge, { formatBandwidth, type LinkEdgeData } from './LinkEdge';
import { ReconnectBanner } from './ReconnectBanner';
import SearchOverlay from './SearchOverlay';
import ZoomControls from './ZoomControls';

const nodeTypes = {
  device: DeviceCard,
};

const edgeTypes = {
  link: LinkEdge,
};

const manualEdgeStorageKey = 'theia-manual-edges';

type HandleSide = 'top' | 'right' | 'bottom' | 'left';

interface StoredManualEdge {
  id: string;
  source: string;
  target: string;
  sourceHandle?: string | null;
  targetHandle?: string | null;
}

interface EdgeMenuState {
  edgeID: string;
  x: number;
  y: number;
}

const defaultPollingIntervalMs = 60_000;
const staleThresholdMs = defaultPollingIntervalMs * 2;

function buildPositionPayload(nodes: Node<DeviceNodeData>[]): PositionPayload[] {
  return nodes.map((node) => ({
    device_id: node.id,
    x: node.position.x,
    y: node.position.y,
    pinned: node.data.pinned,
  }));
}

function inferSpeedLabel(
  sourceDevice: Device | undefined,
  targetDevice?: Device,
): string | undefined {
  const speeds = [
    ...(sourceDevice?.interfaces ?? []).map((iface) => iface.speed),
    ...(targetDevice?.interfaces ?? []).map((iface) => iface.speed),
  ].filter((speed) => speed > 0);

  if (speeds.length === 0) {
    return undefined;
  }

  return formatBandwidth(Math.max(...speeds));
}

function compactThroughput(bps: number): string {
  return formatThroughput(bps)
    .replace(' Gbps', 'G')
    .replace(' Mbps', 'M')
    .replace(' Kbps', 'K')
    .replace(' bps', 'b');
}

function normalizeInterfaceName(name: string | undefined): string {
  return (name ?? '').trim().toLowerCase();
}

function buildThroughputLabel(metrics: LinkMetricsDTO): string | undefined {
  if (metrics.tx_bps === null && metrics.rx_bps === null) {
    return undefined;
  }

  const txLabel = metrics.tx_bps === null ? '--' : compactThroughput(metrics.tx_bps);
  const rxLabel = metrics.rx_bps === null ? '--' : compactThroughput(metrics.rx_bps);
  return `TX: ${txLabel} / RX: ${rxLabel}`;
}

function findLinkMetrics(
  snapshotMetrics: Record<string, LinkMetricsDTO[]>,
  link: Link,
): LinkMetricsDTO | null {
  const deviceMetrics = snapshotMetrics[link.source_device_id];
  if (!deviceMetrics) {
    return null;
  }

  const sourceIfName = normalizeInterfaceName(link.source_if_name);
  return (
    deviceMetrics.find(
      (metric) => normalizeInterfaceName(metric.if_name) === sourceIfName,
    ) ?? null
  );
}

function buildEdgeData(
  link: Link,
  devicesByID: Map<string, Device>,
  existingData?: LinkEdgeData,
  onContextMenu?: (event: MouseEvent | React.MouseEvent<SVGPathElement>, edgeID: string) => void,
): LinkEdgeData {
  const sourceDevice = devicesByID.get(link.source_device_id);
  const targetDevice = devicesByID.get(link.target_device_id);
  const sourceInterface = sourceDevice?.interfaces.find(
    (iface) => iface.if_name === link.source_if_name,
  );

  return {
    link,
    bandwidthLabel:
      sourceInterface?.speed && sourceInterface.speed > 0
        ? formatBandwidth(sourceInterface.speed)
        : inferSpeedLabel(sourceDevice, targetDevice),
    onContextMenu,
    metrics: existingData?.metrics,
    throughputLabel: existingData?.throughputLabel,
    utilization: existingData?.utilization,
  };
}

function getHandleSide(
  sourcePosition: { x: number; y: number },
  targetPosition: { x: number; y: number },
): { sourceHandle: HandleSide; targetHandle: HandleSide } {
  const dx = targetPosition.x - sourcePosition.x;
  const dy = targetPosition.y - sourcePosition.y;

  if (Math.abs(dx) >= Math.abs(dy)) {
    return dx >= 0
      ? { sourceHandle: 'right', targetHandle: 'left' }
      : { sourceHandle: 'left', targetHandle: 'right' };
  }

  return dy >= 0
    ? { sourceHandle: 'bottom', targetHandle: 'top' }
    : { sourceHandle: 'top', targetHandle: 'bottom' };
}

function serializeManualEdges(edges: Edge<LinkEdgeData>[]) {
  if (typeof window === 'undefined') {
    return;
  }

  const payload: StoredManualEdge[] = edges
    .filter((edge) => edge.data?.manual)
    .map((edge) => ({
      id: edge.id,
      source: edge.source,
      target: edge.target,
      sourceHandle: edge.sourceHandle,
      targetHandle: edge.targetHandle,
    }));

  window.localStorage.setItem(manualEdgeStorageKey, JSON.stringify(payload));
}

function loadManualEdges() {
  if (typeof window === 'undefined') {
    return [] as StoredManualEdge[];
  }

  const payload = window.localStorage.getItem(manualEdgeStorageKey);
  if (!payload) {
    return [] as StoredManualEdge[];
  }

  try {
    const parsed = JSON.parse(payload);
    return Array.isArray(parsed) ? (parsed as StoredManualEdge[]) : [];
  } catch {
    return [] as StoredManualEdge[];
  }
}

function buildTopologyEdges(
  links: Link[],
  devicesByID: Map<string, Device>,
  nodes: Node<DeviceNodeData>[],
  existingEdgeDataByID?: Map<string, LinkEdgeData>,
  onContextMenu?: (event: MouseEvent | React.MouseEvent<SVGPathElement>, edgeID: string) => void,
): Edge<LinkEdgeData>[] {
  const nodesByID = new Map(nodes.map((node) => [node.id, node]));
  const pairCounts = new Map<string, number>();

  return links
    .filter(
      (link) =>
        nodesByID.has(link.source_device_id) && nodesByID.has(link.target_device_id),
    )
    .map((link) => {
      const sourceNode = nodesByID.get(link.source_device_id)!;
      const targetNode = nodesByID.get(link.target_device_id)!;
      const { sourceHandle, targetHandle } = getHandleSide(
        sourceNode.position,
        targetNode.position,
      );

      const pairKey = [link.source_device_id, link.target_device_id].sort().join('-');
      const parallelIndex = pairCounts.get(pairKey) || 0;
      pairCounts.set(pairKey, parallelIndex + 1);

      const data = buildEdgeData(
        link,
        devicesByID,
        existingEdgeDataByID?.get(link.id),
        onContextMenu,
      );
      data.parallelIndex = parallelIndex;

      return {
        id: link.id,
        source: link.source_device_id,
        target: link.target_device_id,
        sourceHandle,
        targetHandle,
        type: 'link',
        selectable: false,
        data,
      };
    });
}

function buildManualEdges(
  storedEdges: StoredManualEdge[],
  nodes: Node<DeviceNodeData>[],
  devicesByID: Map<string, Device>,
  existingEdgeDataByID?: Map<string, LinkEdgeData>,
  onContextMenu?: (event: MouseEvent | React.MouseEvent<SVGPathElement>, edgeID: string) => void,
): Edge<LinkEdgeData>[] {
  const nodeIDs = new Set(nodes.map((node) => node.id));

  return storedEdges
    .filter((edge) => nodeIDs.has(edge.source) && nodeIDs.has(edge.target))
    .map((edge) => ({
      id: edge.id,
      source: edge.source,
      target: edge.target,
      sourceHandle: edge.sourceHandle ?? undefined,
      targetHandle: edge.targetHandle ?? undefined,
      type: 'link',
      data: {
        bandwidthLabel: inferSpeedLabel(
          devicesByID.get(edge.source),
          devicesByID.get(edge.target),
        ),
        manual: true,
        onContextMenu,
        metrics: existingEdgeDataByID?.get(edge.id)?.metrics,
        throughputLabel: existingEdgeDataByID?.get(edge.id)?.throughputLabel,
        utilization: existingEdgeDataByID?.get(edge.id)?.utilization,
      },
    }));
}

function statusColor(status: Device['status']): string {
  switch (status) {
    case 'up':
      return '#00c853';
    case 'down':
      return '#ff1744';
    case 'probing':
      return '#ffc107';
    default:
      return '#657786';
  }
}

function viewportSize() {
  return {
    width: typeof window === 'undefined' ? 1440 : window.innerWidth,
    height: typeof window === 'undefined' ? 900 : window.innerHeight,
  };
}

export default function Canvas() {
  const [nodes, setNodes, onNodesChange] = useNodesState<DeviceNodeData>([]);
  const [edges, setEdges] = useState<Edge<LinkEdgeData>[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [topologyLinks, setTopologyLinks] = useState<Link[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [edgeMenu, setEdgeMenu] = useState<EdgeMenuState | null>(null);
  const highlightTimerRef = useRef<number | null>(null);
  const layoutInitializedRef = useRef(false);
  const lastSnapshotTimeRef = useRef<number | null>(null);
  const staleAppliedRef = useRef(false);
  const reactFlow = useReactFlow<DeviceNodeData, LinkEdgeData>();
  const { fetchPositions, savePositions } = usePositions();
  const { snapshot, reconnecting } = useWebSocket('/api/v1/ws');

  function openEdgeMenu(
    event: MouseEvent | React.MouseEvent<SVGPathElement>,
    edgeID: string,
  ) {
    setEdgeMenu({
      edgeID,
      x: event.clientX,
      y: event.clientY,
    });
  }

  async function loadTopology() {
    setLoading(true);
    setError(null);

    try {
      const [fetchedDevices, fetchedLinks, savedPositions] = await Promise.all([
        fetchDevices(),
        fetchLinks(),
        fetchPositions(),
      ]);

      const devicesByID = new Map(fetchedDevices.map((device) => [device.id, device]));
      const { width, height } = viewportSize();

      const computedPositions = computeForceLayout(
        fetchedDevices.map((device) => {
          const saved = savedPositions.get(device.id);
          return {
            id: device.id,
            x: saved?.x,
            y: saved?.y,
            pinned: saved?.pinned,
          };
        }),
        fetchedLinks.map((link) => ({
          source: link.source_device_id,
          target: link.target_device_id,
        })),
        width,
        height,
      );

      const nextNodes: Node<DeviceNodeData>[] = fetchedDevices.map((device) => {
        const saved = savedPositions.get(device.id);
        const position = saved ?? computedPositions.get(device.id) ?? { x: 0, y: 0 };

        return {
          id: device.id,
          type: 'device',
          position: {
            x: position.x,
            y: position.y,
          },
          data: {
            device,
            pinned: saved?.pinned ?? false,
            highlighted: false,
          },
        };
      });

      const nextEdges = [
        ...buildTopologyEdges(fetchedLinks, devicesByID, nextNodes, undefined, openEdgeMenu),
        ...buildManualEdges(
          loadManualEdges(),
          nextNodes,
          devicesByID,
          undefined,
          openEdgeMenu,
        ),
      ];

      startTransition(() => {
        setDevices(fetchedDevices);
        setTopologyLinks(fetchedLinks);
        setNodes(nextNodes);
        setEdges(nextEdges);
      });

      if (!layoutInitializedRef.current || fetchedDevices.length !== devices.length) {
        layoutInitializedRef.current = true;
        void savePositions(buildPositionPayload(nextNodes));
      }

      window.requestAnimationFrame(() => {
        reactFlow.fitView({ padding: 0.18, duration: 320 });
      });
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : 'Failed to load topology');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void loadTopology();

    return () => {
      if (highlightTimerRef.current !== null) {
        window.clearTimeout(highlightTimerRef.current);
      }
    };
  }, []);

  useEffect(() => {
    if (snapshot === null) {
      return;
    }

    lastSnapshotTimeRef.current = Date.now();
    staleAppliedRef.current = false;

    startTransition(() => {
      setNodes((currentNodes) =>
        currentNodes.map((node) => ({
          ...node,
          data: {
            ...node.data,
            metrics: snapshot.device_metrics[node.id] ?? null,
            alertStatus: alertStatusForDevice(node.id, snapshot.alerts),
          },
        })),
      );

      setEdges((currentEdges) =>
        currentEdges.map((edge) => {
          const link = edge.data?.link;
          if (!link) {
            return edge;
          }

          const metrics = findLinkMetrics(snapshot.link_metrics, link);
          if (metrics === null) {
            return {
              ...edge,
              data: {
                ...edge.data,
                metrics: null,
                throughputLabel: undefined,
                utilization: null,
              },
            };
          }

          return {
            ...edge,
            data: {
              ...edge.data,
              metrics,
              throughputLabel: buildThroughputLabel(metrics),
              utilization: metrics.utilization,
            },
          };
        }),
      );
    });
  }, [snapshot, devices.length, topologyLinks.length, setNodes]);

  useEffect(() => {
    const interval = window.setInterval(() => {
      if (lastSnapshotTimeRef.current === null || staleAppliedRef.current) {
        return;
      }

      if (Date.now() - lastSnapshotTimeRef.current <= staleThresholdMs) {
        return;
      }

      staleAppliedRef.current = true;

      startTransition(() => {
        setNodes((currentNodes) =>
          currentNodes.map((node) => ({
            ...node,
            data: {
              ...node.data,
              metrics: null,
              alertStatus: undefined,
            },
          })),
        );

        setEdges((currentEdges) =>
          currentEdges.map((edge) => ({
            ...edge,
            data: {
              ...edge.data,
              metrics: null,
              throughputLabel: undefined,
              utilization: null,
            },
          })),
        );
      });
    }, 10_000);

    return () => {
      window.clearInterval(interval);
    };
  }, [setNodes]);

  useEffect(() => {
    if (!edgeMenu) {
      return;
    }

    function closeMenu() {
      setEdgeMenu(null);
    }

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        setEdgeMenu(null);
      }
    }

    window.addEventListener('click', closeMenu);
    window.addEventListener('keydown', handleKeyDown);

    return () => {
      window.removeEventListener('click', closeMenu);
      window.removeEventListener('keydown', handleKeyDown);
    };
  }, [edgeMenu]);

  function handleEdgesChange(changes: EdgeChange[]) {
    setEdges((currentEdges) => {
      const nextEdges = applyEdgeChanges(changes, currentEdges);
      serializeManualEdges(nextEdges);
      return nextEdges;
    });
  }

  function focusOnDevice(deviceID: string) {
    const targetNode = reactFlow.getNodes().find((node) => node.id === deviceID);
    if (!targetNode) {
      return;
    }

    reactFlow.setCenter(targetNode.position.x + 110, targetNode.position.y + 44, {
      zoom: 1.2,
      duration: 500,
    });

    setNodes((currentNodes) =>
      currentNodes.map((node) => ({
        ...node,
        data: {
          ...node.data,
          highlighted: node.id === deviceID,
        },
      })),
    );

    if (highlightTimerRef.current !== null) {
      window.clearTimeout(highlightTimerRef.current);
    }

    highlightTimerRef.current = window.setTimeout(() => {
      setNodes((currentNodes) =>
        currentNodes.map((node) =>
          node.id === deviceID
            ? {
              ...node,
              data: {
                ...node.data,
                highlighted: false,
              },
            }
            : node,
        ),
      );
    }, 2000);
  }

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center bg-bg-canvas">
        <div className="rounded-3xl border border-border-subtle bg-bg-surface/85 px-6 py-5 text-center shadow-canvas">
          <div className="mx-auto mb-3 h-10 w-10 animate-spin rounded-full border-2 border-border-subtle border-t-accent" />
          <p className="text-sm uppercase tracking-[0.28em] text-text-secondary">
            Loading topology...
          </p>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex h-full items-center justify-center bg-bg-canvas px-6">
        <div className="max-w-md rounded-3xl border border-border-subtle bg-bg-surface/85 px-6 py-6 text-center shadow-canvas">
          <p className="text-sm uppercase tracking-[0.28em] text-status-down">Topology Error</p>
          <h2 className="mt-3 text-2xl font-semibold tracking-tight text-text-primary">
            Canvas data could not load
          </h2>
          <p className="mt-3 text-sm text-text-secondary">{error}</p>
          <button
            type="button"
            onClick={() => {
              void loadTopology();
            }}
            className="mt-6 rounded-full border border-accent/40 bg-accent/10 px-5 py-2 text-sm font-medium text-accent transition-colors duration-150 hover:bg-accent/20"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="relative h-full w-full bg-bg-canvas">
      <SearchOverlay devices={devices} onSelectDevice={focusOnDevice} />
      <ReconnectBanner visible={reconnecting} />
      {edgeMenu ? (
        <div
          className="fixed z-30 min-w-[140px] rounded-xl border border-border-subtle bg-bg-surface/95 p-2 shadow-[0_18px_40px_rgba(0,0,0,0.45)] backdrop-blur-xl"
          style={{
            left: edgeMenu.x + 8,
            top: edgeMenu.y + 8,
          }}
          onClick={(event) => {
            event.stopPropagation();
          }}
        >
          <button
            type="button"
            className="w-full rounded-lg px-3 py-2 text-left text-sm text-status-down transition-colors duration-150 hover:bg-bg-elevated"
            onClick={() => {
              setEdges((currentEdges) => {
                const nextEdges = currentEdges.filter((edge) => edge.id !== edgeMenu.edgeID);
                serializeManualEdges(nextEdges);
                return nextEdges;
              });
              setEdgeMenu(null);
            }}
          >
            Remove
          </button>
        </div>
      ) : null}
      <ZoomControls
        onZoomIn={() => {
          void reactFlow.zoomIn({ duration: 200 });
        }}
        onZoomOut={() => {
          void reactFlow.zoomOut({ duration: 200 });
        }}
        onFitView={() => {
          void reactFlow.fitView({ padding: 0.18, duration: 280 });
        }}
      />
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        onNodesChange={onNodesChange}
        onEdgesChange={handleEdgesChange}
        onPaneClick={() => {
          setEdgeMenu(null);
        }}
        onConnect={(connection: Connection) => {
          if (!connection.source || !connection.target || connection.source === connection.target) {
            return;
          }
          const source = connection.source;
          const target = connection.target;

          setEdges((currentEdges) => {
            const duplicate = currentEdges.some(
              (edge) =>
                edge.source === source &&
                edge.target === target &&
                edge.sourceHandle === connection.sourceHandle &&
                edge.targetHandle === connection.targetHandle,
            );
            if (duplicate) {
              return currentEdges;
            }

            const nextEdges = addEdge(
              {
                id: `manual:${source}:${connection.sourceHandle ?? 'auto'}:${target}:${connection.targetHandle ?? 'auto'}`,
                source,
                target,
                sourceHandle: connection.sourceHandle ?? undefined,
                targetHandle: connection.targetHandle ?? undefined,
                type: 'link',
                data: {
                  bandwidthLabel: inferSpeedLabel(
                    devices.find((device) => device.id === source),
                    devices.find((device) => device.id === target),
                  ),
                  manual: true,
                  onContextMenu: openEdgeMenu,
                },
              },
              currentEdges,
            );
            serializeManualEdges(nextEdges);
            return nextEdges;
          });
        }}
        onNodeDragStop={(_event, node) => {
          const updatedNodes = reactFlow.getNodes().map((currentNode) =>
            currentNode.id === node.id
              ? {
                ...currentNode,
                position: node.position,
                data: {
                  ...currentNode.data,
                  pinned: true,
                },
              }
              : currentNode,
          );
          const devicesByID = new Map(devices.map((device) => [device.id, device]));
          const existingEdgeDataByID = new Map(
            edges.map((currentEdge) => [currentEdge.id, currentEdge.data ?? {}]),
          );
          const repositionedEdges = [
            ...buildTopologyEdges(
              topologyLinks,
              devicesByID,
              updatedNodes,
              existingEdgeDataByID,
              openEdgeMenu,
            ),
            ...buildManualEdges(
              loadManualEdges(),
              updatedNodes,
              devicesByID,
              existingEdgeDataByID,
              openEdgeMenu,
            ),
          ];

          setNodes(updatedNodes);
          setEdges(repositionedEdges);
          serializeManualEdges(repositionedEdges);
          void savePositions(buildPositionPayload(updatedNodes));
        }}
        connectionMode={ConnectionMode.Loose}
        minZoom={0.1}
        maxZoom={2}
        fitView
        nodesDraggable
        panOnDrag
        zoomOnScroll
        zoomOnDoubleClick={false}
        connectionLineStyle={{ stroke: '#4a4a5e', strokeWidth: 2 }}
        proOptions={{ hideAttribution: false }}
        className="bg-bg-canvas"
      >
        <Background color="#3f3f53" gap={28} size={1.2} />
        <MiniMap
          pannable
          zoomable
          nodeColor={(node) => statusColor(node.data.device.status)}
          style={{
            backgroundColor: '#363647',
            border: '1px solid #4a4a5e',
          }}
          maskColor="rgba(45, 45, 61, 0.55)"
        />
      </ReactFlow>
    </div>
  );
}
