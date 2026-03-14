import { startTransition, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  ConnectionMode,
  Background,
  MiniMap,
  ReactFlow,
  applyEdgeChanges,
  useNodesState,
  useReactFlow,
  type Edge,
  type EdgeChange,
  type Node,
} from 'reactflow';
import { fetchDevices, fetchLinks, fetchSettings, createLink } from '../api/client';
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
import { InterfaceStatsPanel, DeviceInterfaceStatsPanel } from './InterfaceStatsPanel';
import { AlertsPanel } from './AlertsPanel';
import { SettingsPanel } from './SettingsPanel';
import { AddDevicePanel } from './AddDevicePanel';
import { DeviceConfigPanel } from './DeviceConfigPanel';
import { LinkCreatePanel } from './LinkCreatePanel';
import { LinkDetailsPanel } from './LinkDetailsPanel';

const nodeTypes = {
  device: DeviceCard,
};

const edgeTypes = {
  link: LinkEdge,
};

const manualEdgeStorageKey = 'theia-manual-edges';

type HandleSide = 'top' | 'right' | 'bottom' | 'left';

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
  const targetInterface = targetDevice?.interfaces.find(
    (iface) => iface.if_name === link.target_if_name,
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
    sourceIfStatus: sourceInterface?.oper_status,
    targetIfStatus: targetInterface?.oper_status,
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

function buildTopologyEdges(
  links: Link[],
  devicesByID: Map<string, Device>,
  nodes: Node<DeviceNodeData>[],
  existingEdgeDataByID?: Map<string, LinkEdgeData>,
  onContextMenu?: (event: MouseEvent | React.MouseEvent<SVGPathElement>, edgeID: string) => void,
): Edge<LinkEdgeData>[] {
  const nodesByID = new Map(nodes.map((node) => [node.id, node]));
  const pairCounts = new Map<string, number>();
  const seenPairs = new Set<string>();

  return links
    .filter((link) => {
      if (!nodesByID.has(link.source_device_id) || !nodesByID.has(link.target_device_id)) {
        return false;
      }
      // Deduplicate bidirectional links: A→B and B→A for the same port pair are the same physical link
      const pairKey = [link.source_device_id, link.target_device_id].sort().join('-');
      if (seenPairs.has(pairKey)) {
        return false;
      }
      seenPairs.add(pairKey);
      return true;
    })
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
        selectable: true,
        data,
      };
    });
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

  const highlightTimerRef = useRef<number | null>(null);
  const layoutInitializedRef = useRef(false);
  const grafanaUrlRef = useRef<string>('');
  const deviceGrafanaUrlsRef = useRef<Map<string, string>>(new Map());
  const lastSnapshotTimeRef = useRef<number | null>(null);
  const staleAppliedRef = useRef(false);
  const reactFlow = useReactFlow<DeviceNodeData, LinkEdgeData>();
  const { fetchPositions, savePositions } = usePositions();
  const { snapshot, reconnecting, prometheusStatus } = useWebSocket('/api/v1/ws');
  const [prometheusAlertDismissed, setPrometheusAlertDismissed] = useState(false);

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
      key: 'a',
      description: 'Add device',
      handler: () => setPanelContent({ type: 'addDevice' }),
    },
    createLink: {
      key: 'l',
      description: 'Create link',
      handler: () => setPanelContent({ type: 'create-link' }),
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

  async function loadTopology(isSilentRefresh = false) {
    if (!isSilentRefresh) {
      setLoading(true);
    }
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

      // Migrate localStorage manual edges to backend on first load (best-effort)
      const storedManualRaw = window.localStorage.getItem(manualEdgeStorageKey);
      if (storedManualRaw) {
        try {
          const storedManual = JSON.parse(storedManualRaw) as Array<{
            id: string;
            source: string;
            target: string;
          }>;
          if (Array.isArray(storedManual) && storedManual.length > 0) {
            const migrations = storedManual.map((edge) =>
              createLink({
                source_device_id: edge.source,
                source_if_name: '',
                target_device_id: edge.target,
                target_if_name: '',
              }).catch(() => {
                // Best-effort: ignore individual migration failures
              }),
            );
            await Promise.allSettled(migrations);
          }
        } catch {
          // Ignore parse errors during migration
        }
        window.localStorage.removeItem(manualEdgeStorageKey);
      }

      const nextEdges = buildTopologyEdges(fetchedLinks, devicesByID, nextNodes, undefined, openEdgeMenu);

      startTransition(() => {
        setDevices(fetchedDevices);
        setTopologyLinks(fetchedLinks);
        if (isSilentRefresh) {
          // Preserve alertStatus so a topology refresh doesn't momentarily clear
          // alert rings while the next snapshot hasn't yet re-applied them.
          setNodes((currentNodes) => {
            const prevDataByID = new Map(currentNodes.map((n) => [n.id, n.data]));
            return nextNodes.map((n) => ({
              ...n,
              data: {
                ...n.data,
                alertStatus: prevDataByID.get(n.id)?.alertStatus,
              },
            }));
          });
        } else {
          setNodes(nextNodes);
        }
        setEdges(nextEdges);
      });

      if (!layoutInitializedRef.current || fetchedDevices.length !== devices.length) {
        layoutInitializedRef.current = true;
        void savePositions(buildPositionPayload(nextNodes));
      }

      if (!isSilentRefresh) {
        window.requestAnimationFrame(() => {
          reactFlow.fitView({ padding: 0.18, duration: 320 });
        });
      }
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : 'Failed to load topology');
    } finally {
      if (!isSilentRefresh) {
        setLoading(false);
      }
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
        // Parse per-device Grafana dashboard URLs stored as grafana_dashboard_url:<device_id>
        const perDeviceUrls = new Map<string, string>();
        for (const [key, value] of Object.entries(settings)) {
          if (key.startsWith('grafana_dashboard_url:') && value) {
            const deviceId = key.slice('grafana_dashboard_url:'.length);
            perDeviceUrls.set(deviceId, value);
          }
        }
        deviceGrafanaUrlsRef.current = perDeviceUrls;
      })
      .catch(() => {
        // Settings fetch failure is non-fatal; Grafana links will be disabled.
      });
  }, []);

  // Reset prometheus alert dismissed state when prometheus recovers.
  useEffect(() => {
    if (prometheusStatus?.available) {
      setPrometheusAlertDismissed(false);
    }
  }, [prometheusStatus?.available]);

  useEffect(() => {
    if (snapshot === null) {
      return;
    }

    lastSnapshotTimeRef.current = Date.now();
    staleAppliedRef.current = false;

    // Apply alert status, device status, and discovered hostname immediately (not deferred)
    // so clearing is instant and not interrupted by concurrent UI interactions.
    setNodes((currentNodes) =>
      currentNodes.map((node) => {
        const newStatus = snapshot.device_statuses[node.id];
        const newHostname = snapshot.device_hostnames[node.id];
        const updatedDevice = newStatus || newHostname
          ? {
              ...node.data.device,
              ...(newStatus ? { status: newStatus as Device['status'] } : {}),
              ...(newHostname ? { sys_name: newHostname } : {}),
            }
          : node.data.device;
        return {
          ...node,
          data: {
            ...node.data,
            alertStatus: alertStatusForDevice(node.id, snapshot.alerts),
            device: updatedDevice,
          },
        };
      }),
    );

    // Sync devices state with hostnames/statuses so panels (e.g. LinkCreatePanel) see them
    if (Object.keys(snapshot.device_hostnames).length > 0 || Object.keys(snapshot.device_statuses).length > 0) {
      setDevices((prev) =>
        prev.map((d) => {
          const newHostname = snapshot.device_hostnames[d.id];
          const newStatus = snapshot.device_statuses[d.id];
          if (!newHostname && !newStatus) return d;
          return {
            ...d,
            ...(newHostname ? { sys_name: newHostname } : {}),
            ...(newStatus ? { status: newStatus as Device['status'] } : {}),
          };
        }),
      );
    }

    setEdges((currentEdges) =>
      currentEdges.map((edge) => {
        const link = edge.data?.link;
        if (!link) return edge;
        const linkAlertStatus = alertStatusForLink(link, snapshot.alerts);
        return {
          ...edge,
          data: {
            ...edge.data,
            alertStatus: linkAlertStatus === 'normal' ? undefined : linkAlertStatus,
          },
        };
      }),
    );

    // Metrics are non-urgent display data — defer so alert updates are always
    // applied first and never delayed by heavy node/edge re-renders.
    startTransition(() => {
      setNodes((currentNodes) =>
        currentNodes.map((node) => ({
          ...node,
          data: {
            ...node.data,
            metrics: snapshot.device_metrics[node.id] ?? null,
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
    setEdges((currentEdges) => applyEdgeChanges(changes, currentEdges));
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
    if (panelContent.type === 'alerts') return 'Alerts';
    if (panelContent.type === 'settings') return 'Settings';
    if (panelContent.type === 'addDevice') return 'Add Device';
    if (panelContent.type === 'create-link') return 'Create Link';
    if (panelContent.type === 'link-details') return 'Link Details';
    if (panelContent.type === 'deviceConfig') {
      const data = panelContent.data as { device?: Device } | undefined;
      if (data?.device) {
        const d = data.device;
        return d.tags?.display_name || d.sys_name || d.hostname || 'Configure Device';
      }
      return 'Configure Device';
    }
    if (panelContent.type === 'interfaceStats') {
      const data = panelContent.data as { link?: Link; sourceDevice?: Device; targetDevice?: Device; device?: Device } | undefined;
      if (data?.link && data.sourceDevice && data.targetDevice) {
        const srcName = data.sourceDevice.tags?.display_name || data.sourceDevice.sys_name || data.sourceDevice.hostname;
        const dstName = data.targetDevice.tags?.display_name || data.targetDevice.sys_name || data.targetDevice.hostname;
        return `${srcName} — ${dstName}`;
      }
      if (data?.device) {
        return data.device.tags?.display_name || data.device.sys_name || data.device.ip;
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
      {showSearch && <SearchOverlay devices={devices} onSelectDevice={focusOnDevice} />}
      <Toolbar
        onSearch={() => setShowSearch(s => !s)}
        onAddDevice={() => setPanelContent({ type: 'addDevice' })}
        onCreateLink={() => setPanelContent({ type: 'create-link' })}
        onAlerts={() => setPanelContent({ type: 'alerts' })}
        onSettings={() => setPanelContent({ type: 'settings' })}
        alertCount={
          (snapshot?.alerts.filter((a) => a.state === 'firing').length ?? 0) +
          (prometheusStatus !== null && !prometheusStatus.available ? 1 : 0)
        }
      />

      {deviceMenu && (() => {
        const menuDevice = devices.find((d) => d.id === deviceMenu.deviceId);
        const globalGrafanaUrl = grafanaUrlRef.current;
        // Per-device URL takes priority; fall back to global base URL
        const effectiveGrafanaUrl = menuDevice
          ? (deviceGrafanaUrlsRef.current.get(menuDevice.id) || globalGrafanaUrl)
          : globalGrafanaUrl;
        const grafanaLabel = effectiveGrafanaUrl ? 'Open in Grafana' : 'Open in Grafana (not configured)';
        return (
          <ContextMenu
            position={{ x: deviceMenu.x, y: deviceMenu.y }}
            onClose={() => setDeviceMenu(null)}
            items={[
              {
                label: 'Open WebFig',
                onClick: () => {
                  if (menuDevice) {
                    window.open(`http://${menuDevice.ip}/webfig/`, '_blank');
                  }
                  setDeviceMenu(null);
                },
              },
              {
                label: grafanaLabel,
                onClick: () => {
                  if (effectiveGrafanaUrl) {
                    window.open(effectiveGrafanaUrl, '_blank');
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
                      data: { device: menuDevice },
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
        const globalGrafanaUrl = grafanaUrlRef.current;
        // Use per-device URL for source device, or fall back to global base URL
        const effectiveGrafanaUrl = menuSourceDevice
          ? (deviceGrafanaUrlsRef.current.get(menuSourceDevice.id) || globalGrafanaUrl)
          : globalGrafanaUrl;
        const grafanaLabel = effectiveGrafanaUrl ? 'Open in Grafana' : 'Open in Grafana (not configured)';
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
                  if (effectiveGrafanaUrl) {
                    window.open(effectiveGrafanaUrl, '_blank');
                  }
                  setEdgeMenu(null);
                },
              },
              {
                label: 'View Details',
                onClick: () => {
                  const menuEdgeObj = edges.find((e) => e.id === edgeMenu.edgeID);
                  const menuLinkObj = menuEdgeObj?.data?.link;
                  if (menuLinkObj) {
                    setPanelContent({ type: 'link-details', data: { link: menuLinkObj } });
                  }
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
          const data = panelContent.data as { link?: Link; sourceDevice?: Device; targetDevice?: Device; device?: Device } | undefined;
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
          if (data?.device) {
            return (
              <DeviceInterfaceStatsPanel
                device={data.device}
                snapshot={snapshot as SnapshotPayload | null}
              />
            );
          }
          return <div className="text-text-secondary text-sm">No data available.</div>;
        })()}
        {panelContent?.type === 'alerts' && (
          <AlertsPanel
            alerts={snapshot?.alerts ?? []}
            devices={devices}
            prometheusStatus={prometheusStatus}
          />
        )}
        {panelContent?.type === 'settings' && <SettingsPanel />}
        {panelContent?.type === 'addDevice' && (
          <AddDevicePanel
            onDeviceAdded={() => {
              setPanelContent(null);
              void loadTopology(true);
            }}
          />
        )}
        {panelContent?.type === 'create-link' && (
          <LinkCreatePanel
            devices={devices}
            links={topologyLinks}
            onCreated={() => {
              setPanelContent(null);
              void loadTopology(true);
            }}
            onClose={() => setPanelContent(null)}
            onRefreshDevices={async () => {
              const refreshedDevices = await fetchDevices();
              setDevices(refreshedDevices);
            }}
          />
        )}
        {panelContent?.type === 'link-details' && (() => {
          const data = panelContent.data as { link?: Link } | undefined;
          if (data?.link) {
            return (
              <LinkDetailsPanel
                link={data.link}
                devices={devices}
                onUpdated={() => {
                  setPanelContent(null);
                  void loadTopology(true);
                }}
                onDeleted={() => {
                  setPanelContent(null);
                  void loadTopology(true);
                }}
                onClose={() => setPanelContent(null)}
              />
            );
          }
          return null;
        })()}
        {panelContent?.type === 'deviceConfig' && (() => {
          const data = panelContent.data as { device?: Device } | undefined;
          if (data?.device) {
            return (
              <DeviceConfigPanel
                device={data.device}
                onDeviceUpdated={(updated) => {
                  setDevices((prev) => prev.map((d) => d.id === updated.id ? updated : d));
                  setNodes((prev) => prev.map((n) => n.id === updated.id
                    ? { ...n, data: { ...n.data, device: updated } }
                    : n,
                  ));
                  setPanelContent({ type: 'deviceConfig', data: { device: updated } });
                }}
                onDeviceDeleted={() => {
                  setPanelContent(null);
                  void loadTopology(true);
                }}
              />
            );
          }
          return null;
        })()}
      </SidePanel>

      <ShortcutHelp open={showShortcuts} onClose={() => setShowShortcuts(false)} />

      <ReconnectBanner visible={reconnecting} />
      {prometheusStatus !== null && !prometheusStatus.available && !prometheusAlertDismissed && (
        <div className="absolute bottom-16 left-1/2 z-50 -translate-x-1/2 flex items-center gap-2.5 rounded-xl border border-yellow-500/30 bg-bg-surface/95 px-4 py-2.5 shadow-canvas backdrop-blur-sm">
          <span className="h-2 w-2 flex-none rounded-full bg-yellow-400 animate-pulse" />
          <p className="text-sm text-yellow-300">Prometheus unreachable</p>
          <button
            type="button"
            onClick={() => {
              setPanelContent({ type: 'alerts' });
              setPrometheusAlertDismissed(true);
            }}
            className="text-xs font-medium text-yellow-400 hover:text-yellow-300"
          >
            Details
          </button>
          <button
            type="button"
            onClick={() => setPrometheusAlertDismissed(true)}
            className="text-text-secondary hover:text-text-primary"
            title="Dismiss"
          >
            <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
      )}
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
        onNodeClick={(_event, node) => {
          const clickedDevice = devices.find((d) => d.id === node.id);
          if (clickedDevice) {
            setPanelContent({ type: 'deviceConfig', data: { device: clickedDevice } });
          }
        }}
        onEdgeClick={(_event, edge) => {
          // Only open details for backend-persisted links (edges with link data)
          const link = edge.data?.link;
          if (link) {
            setPanelContent({ type: 'link-details', data: { link } });
          }
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
          const repositionedEdges = buildTopologyEdges(
            topologyLinks,
            devicesByID,
            updatedNodes,
            existingEdgeDataByID,
            openEdgeMenu,
          );

          setNodes(updatedNodes);
          setEdges(repositionedEdges);
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
          nodeColor={(node) => {
            const alertStatus = node.data.alertStatus as string | undefined;
            if (alertStatus === 'down') return '#ff1744';
            if (alertStatus === 'degraded') return '#ffc107';
            return statusColor(node.data.device.status);
          }}
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
