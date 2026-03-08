import { startTransition, useCallback, useEffect, useMemo, useRef, useState } from 'react';
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
import { fetchDevices, fetchLinks, fetchSettings } from '../api/client';
import { computeForceLayout } from '../hooks/useAutoLayout';
import { usePositions, type PositionPayload } from '../hooks/usePositions';
import { useWebSocket } from '../hooks/useWebSocket';
import type { Device, Link } from '../types/api';
import { alertStatusForDevice, formatThroughput, type AlertDTO, type AlertStatus, type LinkMetricsDTO, type SnapshotPayload } from '../types/metrics';
import DeviceCard, { type DeviceNodeData } from './DeviceCard';
import LinkEdge, { formatBandwidth, type LinkEdgeData } from './LinkEdge';
import { ReconnectBanner } from './ReconnectBanner';
import SearchOverlay from './SearchOverlay';
import ZoomControls from './ZoomControls';
import { ContextMenu } from './ContextMenu';
import { SidePanel } from './SidePanel';
import { ShortcutHelp } from './ShortcutHelp';
import { Toolbar } from './Toolbar';
import { useKeyboardShortcuts } from '../hooks/useKeyboardShortcuts';
import { InterfaceStatsPanel } from './InterfaceStatsPanel';
import { SettingsPanel } from './SettingsPanel';
import { AddDevicePanel } from './AddDevicePanel';
import { DeviceConfigPanel } from './DeviceConfigPanel';

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



const defaultPollingIntervalMs = 60_000;
const staleThresholdMs = defaultPollingIntervalMs * 2;
const canvasBackgroundImageKey = 'canvas_background_image';

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

function alertStatusForLink(link: Link, alerts: AlertDTO[]): AlertStatus {
  const deviceIds = new Set([link.source_device_id, link.target_device_id]);
  const sourceIfName = (link.source_if_name ?? '').toLowerCase();
  const targetIfName = (link.target_if_name ?? '').toLowerCase();

  const relevantAlerts = alerts.filter((alert) => {
    if (!deviceIds.has(alert.device_id)) return false;
    if (alert.state !== 'firing') return false;
    // Interface-specific alerts: check if the summary references the interface name
    const summary = alert.summary.toLowerCase();
    const isLinkAlert = alert.alert_name === 'LinkDown' || alert.alert_name === 'HighLinkUtilization';
    if (!isLinkAlert) return false;
    // Best-effort interface name matching
    if (sourceIfName && summary.includes(sourceIfName)) return true;
    if (targetIfName && summary.includes(targetIfName)) return true;
    // If no interface names to match, fall back to device-level match
    if (!sourceIfName && !targetIfName) return true;
    return false;
  });

  if (relevantAlerts.some((alert) => alert.severity === 'critical')) {
    return 'down';
  }
  if (relevantAlerts.some((alert) => alert.severity === 'warning')) {
    return 'degraded';
  }
  return 'normal';
}

function viewportSize() {
  return {
    width: typeof window === 'undefined' ? 1440 : window.innerWidth,
    height: typeof window === 'undefined' ? 900 : window.innerHeight,
  };
}

function buildGrafanaSlug(hostname: string): string {
  return hostname.toLowerCase().replace(/\s+/g, '-');
}

export default function Canvas() {
  const [nodes, setNodes, onNodesChange] = useNodesState<DeviceNodeData>([]);
  const [edges, setEdges] = useState<Edge<LinkEdgeData>[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [topologyLinks, setTopologyLinks] = useState<Link[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [deviceMenu, setDeviceMenu] = useState<{ deviceId: string, x: number, y: number } | null>(null);
  const [edgeMenu, setEdgeMenu] = useState<{ edgeID: string, x: number, y: number } | null>(null);
  const [panelContent, setPanelContent] = useState<{ type: string, data?: unknown } | null>(null);
  const [showShortcuts, setShowShortcuts] = useState(false);
  const [showSearch, setShowSearch] = useState(false);
  const [canvasBgImage, setCanvasBgImage] = useState<string>('');

  const highlightTimerRef = useRef<number | null>(null);
  const layoutInitializedRef = useRef(false);
  const grafanaUrlRef = useRef<string>('');
  const lastSnapshotTimeRef = useRef<number | null>(null);
  const staleAppliedRef = useRef(false);
  const reactFlow = useReactFlow<DeviceNodeData, LinkEdgeData>();
  const { fetchPositions, savePositions } = usePositions();
  const { snapshot, reconnecting } = useWebSocket('/api/v1/ws');

  const openEdgeMenu = useCallback(
    (event: MouseEvent | React.MouseEvent<SVGPathElement>, edgeID: string) => {
      setEdgeMenu({
        edgeID,
        x: event.clientX,
        y: event.clientY,
      });
    },
    [],
  );

  const openDeviceMenu = useCallback(
    (event: React.MouseEvent, deviceId: string) => {
      setDeviceMenu({
        deviceId,
        x: event.clientX,
        y: event.clientY,
      });
    },
    [],
  );

  const shortcuts = useMemo(() => ({
    search: {
      key: 'k',
      ctrl: true,
      description: 'Search devices',
      handler: () => setShowSearch(s => !s),
    },
    addDevice: {
      key: 'n',
      ctrl: true,
      description: 'Add device',
      handler: () => setPanelContent({ type: 'addDevice' }),
    },
    settings: {
      key: ',',
      ctrl: true,
      description: 'Settings',
      handler: () => setPanelContent({ type: 'settings' }),
    },
    zoomIn: {
      key: '+',
      description: 'Zoom in',
      handler: () => { void reactFlow.zoomIn({ duration: 200 }); },
    },
    zoomOut: {
      key: '-',
      description: 'Zoom out',
      handler: () => { void reactFlow.zoomOut({ duration: 200 }); },
    },
    zoomFit: {
      key: '0',
      description: 'Fit view',
      handler: () => { void reactFlow.fitView({ padding: 0.18, duration: 280 }); },
    },
    help: {
      key: '?',
      description: 'Shortcuts help',
      handler: () => setShowShortcuts(s => !s),
    },
    escape: {
      key: 'escape',
      description: 'Close panels',
      handler: () => {
        if (deviceMenu) setDeviceMenu(null);
        else if (edgeMenu) setEdgeMenu(null);
        else if (panelContent) setPanelContent(null);
        else if (showSearch) setShowSearch(false);
        else if (showShortcuts) setShowShortcuts(false);
      },
    },
  }), [reactFlow, deviceMenu, edgeMenu, panelContent, showSearch, showShortcuts]);

  useKeyboardShortcuts(shortcuts);

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
            onContextMenu: openDeviceMenu,
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
    fetchSettings()
      .then((settings) => {
        grafanaUrlRef.current = settings['grafana_url'] ?? '';
        setCanvasBgImage(settings[canvasBackgroundImageKey] ?? '');
      })
      .catch(() => {
        // Settings fetch failure is non-fatal; Grafana links will be disabled.
      });
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

          const linkAlertStatus = alertStatusForLink(link, snapshot.alerts);
          const metrics = findLinkMetrics(snapshot.link_metrics, link);
          if (metrics === null) {
            return {
              ...edge,
              data: {
                ...edge.data,
                metrics: null,
                throughputLabel: undefined,
                utilization: null,
                alertStatus: linkAlertStatus === 'normal' ? undefined : linkAlertStatus,
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
              alertStatus: linkAlertStatus === 'normal' ? undefined : linkAlertStatus,
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
              alertStatus: undefined,
            },
          })),
        );
      });
    }, 10_000);

    return () => {
      window.clearInterval(interval);
    };
  }, [setNodes]);

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

  // Derive panel title from panelContent
  function getPanelTitle(): string {
    if (!panelContent) return '';
    if (panelContent.type === 'settings') return 'Settings';
    if (panelContent.type === 'addDevice') return 'Add Device';
    if (panelContent.type === 'deviceConfig') {
      const data = panelContent.data as { device?: Device } | undefined;
      return data?.device ? data.device.hostname : 'Configure Device';
    }
    if (panelContent.type === 'interfaceStats') {
      const data = panelContent.data as { link?: Link; sourceDevice?: Device; targetDevice?: Device } | undefined;
      if (data?.link && data.sourceDevice && data.targetDevice) {
        return `${data.sourceDevice.hostname} — ${data.targetDevice.hostname}`;
      }
      return 'Interface Stats';
    }
    return '';
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
      {canvasBgImage && (
        <div
          aria-hidden="true"
          style={{
            position: 'absolute',
            inset: 0,
            zIndex: 0,
            backgroundImage: `url(${canvasBgImage})`,
            backgroundSize: 'contain',
            backgroundPosition: 'center',
            backgroundRepeat: 'no-repeat',
            opacity: 0.15,
            pointerEvents: 'none',
          }}
        />
      )}
      {showSearch && <SearchOverlay devices={devices} onSelectDevice={focusOnDevice} />}
      <Toolbar
        onSearch={() => setShowSearch(s => !s)}
        onAddDevice={() => setPanelContent({ type: 'addDevice' })}
        onSettings={() => setPanelContent({ type: 'settings' })}
      />

      {deviceMenu && (() => {
        const menuDevice = devices.find((d) => d.id === deviceMenu.deviceId);
        const grafanaUrl = grafanaUrlRef.current;
        const grafanaLabel = grafanaUrl ? 'Open in Grafana' : 'Open in Grafana (not configured)';
        return (
          <ContextMenu
            position={{ x: deviceMenu.x, y: deviceMenu.y }}
            onClose={() => setDeviceMenu(null)}
            items={[
              {
                label: grafanaLabel,
                onClick: () => {
                  if (grafanaUrl && menuDevice) {
                    const slug = buildGrafanaSlug(menuDevice.hostname);
                    window.open(`${grafanaUrl}/d/device-${slug}`, '_blank');
                  }
                  setDeviceMenu(null);
                },
              },
              {
                label: 'Per-Interface Stats',
                onClick: () => {
                  if (menuDevice) {
                    setPanelContent({
                      type: 'interfaceStats',
                      data: { deviceId: menuDevice.id, device: menuDevice },
                    });
                  }
                  setDeviceMenu(null);
                },
              },
              {
                label: 'Configure',
                onClick: () => {
                  if (menuDevice) {
                    setPanelContent({ type: 'deviceConfig', data: { device: menuDevice } });
                  }
                  setDeviceMenu(null);
                },
              },
            ]}
          />
        );
      })()}

      {edgeMenu && (() => {
        const menuEdge = edges.find((e) => e.id === edgeMenu.edgeID);
        const menuLink = menuEdge?.data?.link;
        const devicesByID = new Map(devices.map((d) => [d.id, d]));
        const menuSourceDevice = menuLink ? devicesByID.get(menuLink.source_device_id) : undefined;
        const menuTargetDevice = menuLink ? devicesByID.get(menuLink.target_device_id) : undefined;
        const grafanaUrl = grafanaUrlRef.current;
        const grafanaLabel = grafanaUrl ? 'Open in Grafana' : 'Open in Grafana (not configured)';
        return (
          <ContextMenu
            position={{ x: edgeMenu.x, y: edgeMenu.y }}
            onClose={() => setEdgeMenu(null)}
            items={[
              {
                label: 'Per-Interface Stats',
                onClick: () => {
                  if (menuLink && menuSourceDevice && menuTargetDevice) {
                    setPanelContent({
                      type: 'interfaceStats',
                      data: {
                        linkId: menuLink.id,
                        link: menuLink,
                        sourceDevice: menuSourceDevice,
                        targetDevice: menuTargetDevice,
                      },
                    });
                  }
                  setEdgeMenu(null);
                },
              },
              {
                label: grafanaLabel,
                onClick: () => {
                  if (grafanaUrl && menuSourceDevice) {
                    const slug = buildGrafanaSlug(menuSourceDevice.hostname);
                    window.open(`${grafanaUrl}/d/device-${slug}`, '_blank');
                  }
                  setEdgeMenu(null);
                },
              },
              {
                label: 'Remove',
                variant: 'danger',
                onClick: () => {
                  setEdges((currentEdges) => {
                    const nextEdges = currentEdges.filter((edge) => edge.id !== edgeMenu.edgeID);
                    serializeManualEdges(nextEdges);
                    return nextEdges;
                  });
                  setEdgeMenu(null);
                },
              },
            ]}
          />
        );
      })()}

      <SidePanel
        open={!!panelContent}
        onClose={() => setPanelContent(null)}
        title={getPanelTitle()}
      >
        {panelContent?.type === 'interfaceStats' && (() => {
          const data = panelContent.data as { link?: Link; sourceDevice?: Device; targetDevice?: Device } | undefined;
          if (data?.link && data.sourceDevice && data.targetDevice) {
            return (
              <InterfaceStatsPanel
                link={data.link}
                sourceDevice={data.sourceDevice}
                targetDevice={data.targetDevice}
                snapshot={snapshot as SnapshotPayload | null}
              />
            );
          }
          return <div className="text-text-secondary text-sm">No link data available.</div>;
        })()}
        {panelContent?.type === 'settings' && <SettingsPanel />}
        {panelContent?.type === 'addDevice' && (
          <AddDevicePanel
            onDeviceAdded={() => {
              setPanelContent(null);
              void loadTopology();
            }}
          />
        )}
        {panelContent?.type === 'deviceConfig' && (() => {
          const data = panelContent.data as { device?: Device } | undefined;
          if (data?.device) {
            return (
              <DeviceConfigPanel
                device={data.device}
                onDeviceUpdated={() => { void loadTopology(); }}
                onDeviceDeleted={() => {
                  setPanelContent(null);
                  void loadTopology();
                }}
              />
            );
          }
          return null;
        })()}
      </SidePanel>

      <ShortcutHelp open={showShortcuts} onClose={() => setShowShortcuts(false)} />

      <ReconnectBanner visible={reconnecting} />
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
          setDeviceMenu(null);
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
