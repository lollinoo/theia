import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  ConnectionMode, Background, MiniMap, ReactFlow, SelectionMode,
  applyEdgeChanges, useNodesState, useReactFlow,
  type Connection, type EdgeChange,
} from '@xyflow/react';
import type { Area, Device, Link } from '../types/api';
import { isPrometheusUnavailable, type AlertDTO, type PrometheusStatusPayload, type SnapshotPayload } from '../types/metrics';
import DeviceCard, { type DeviceNode } from './DeviceCard';
import LinkEdge, { type LinkEdgeType } from './LinkEdge';
import SearchOverlay from './SearchOverlay';
import ZoomControls from './ZoomControls';
import { ContextMenu } from './ContextMenu';
import { SidePanel } from './SidePanel';
import { ShortcutHelp } from './ShortcutHelp';
import { Toolbar } from './Toolbar';
import { useKeyboardShortcuts } from '../hooks/useKeyboardShortcuts';
import { buildDeviceContextMenuItems, buildPositionPayload } from './canvas/canvasHelpers';
import { buildTopologyEdges } from './canvas/edgeBuilder';
import { useCanvasData } from './canvas/useCanvasData';
import { useCanvasMenus } from './canvas/useCanvasMenus';
import { CanvasPanels } from './canvas/CanvasPanels';
import { CanvasOverlays } from './canvas/CanvasOverlays';
import { getCanvasDetailDeviceId } from './canvas/detailSubscription';
import { useAreaFilteredTopology } from './canvas/useAreaFilteredTopology';
import { usePositions } from '../hooks/usePositions';
import { useTheme, adaptAreaColor } from '../contexts/ThemeContext';
import { useBridgeHealth } from '../hooks/useBridgeHealth';
import { useDeviceWinboxAvailability } from '../hooks/useDeviceWinboxAvailability';
import { minimapColorForDevice } from './deviceVisualState';
import { fetchBridgeToken, fetchSettings } from '../api/client';
import { fetchBridgeWithTimeout, getBridgeLaunchErrorMessage } from '../utils/bridgeRequests';

const nodeTypes = { device: DeviceCard };
const edgeTypes = { link: LinkEdge };
const emptyAlerts: AlertDTO[] = [];

interface CanvasProps {
  snapshot: SnapshotPayload | null;
  alerts?: AlertDTO[];
  reconnecting: boolean;
  prometheusStatus: PrometheusStatusPayload | null;
  selectedAreaId: string | null;
  areas?: Area[];
  onDevicesChange?: (devices: Device[]) => void;
  onLinksChange?: (links: Link[]) => void;
  onAreaSelect?: (areaId: string | null) => void;
  onAreasChange?: () => void;
  onDetailDeviceChange?: (deviceId: string | null) => void;
}

export default function Canvas({ snapshot, alerts = emptyAlerts, reconnecting, prometheusStatus, selectedAreaId, areas, onDevicesChange, onLinksChange, onAreaSelect, onAreasChange, onDetailDeviceChange }: CanvasProps) {
  const [nodes, setNodes, onNodesChange] = useNodesState<DeviceNode>([]);
  const [edges, setEdges] = useState<LinkEdgeType[]>([]);
  const [selectedNodeCount, setSelectedNodeCount] = useState(0);
  const highlightTimerRef = useRef<number | null>(null);
  const reactFlow = useReactFlow<DeviceNode, LinkEdgeType>();
  const { savePositions } = usePositions();
  const { resolvedTheme } = useTheme();
  const [bridgePort, setBridgePort] = useState('1337');
  const { bridgeRunning, bridgeChecked, bridgeError, checkBridgeHealth } = useBridgeHealth(bridgePort);
  const {
    deviceWinboxState,
    refreshDeviceWinboxAvailability,
    setDeviceWinboxAvailability,
  } = useDeviceWinboxAvailability();
  const [bridgeSecret, setBridgeSecret] = useState('');
  const [winboxError, setWinboxError] = useState<string | null>(null);
  useEffect(() => {
    fetchSettings().then((s) => {
      setBridgeSecret(s['bridge_secret'] ?? '');
      setBridgePort(s['bridge_port'] ?? '1337');
    }).catch(() => {});
  }, []);
  useEffect(() => {
    if (!winboxError) return;
    const t = window.setTimeout(() => setWinboxError(null), 5000);
    return () => window.clearTimeout(t);
  }, [winboxError]);
  useEffect(() => {
    if (bridgeError) {
      setWinboxError(bridgeError);
    }
  }, [bridgeError]);
  const {
    deviceMenu, setDeviceMenu, edgeMenu, setEdgeMenu,
    panelContent, setPanelContent, showShortcuts, setShowShortcuts,
    showSearch, setShowSearch, editMode, setEditMode,
    shortcuts, getPanelTitle,
  } = useCanvasMenus({ reactFlow });

  useEffect(() => {
    onDetailDeviceChange?.(getCanvasDetailDeviceId(panelContent));
    return () => onDetailDeviceChange?.(null);
  }, [panelContent, onDetailDeviceChange]);

  const handleSelectionChange = useCallback(
    ({ nodes: selectedNodes }: { nodes: DeviceNode[] }) => {
      setSelectedNodeCount(selectedNodes.length);
      if (selectedNodes.length > 1 && editMode) {
        setPanelContent({ type: 'bulkEdit', data: { deviceIds: selectedNodes.map((n) => n.id) } });
      }
    },
    [editMode, setPanelContent],
  );

  useKeyboardShortcuts(shortcuts);

  const openEdgeMenu = useCallback(
    (event: MouseEvent | React.MouseEvent<SVGPathElement>, edgeID: string) => {
      // Bridge health is only checked for device menus because WinBox launch
      // is not available from edge menus.
      setEdgeMenu({ edgeID, x: event.clientX, y: event.clientY });
    }, [setEdgeMenu],
  );
  const openSelfLinkDetails = useCallback(
    (link: Link) => {
      setPanelContent({ type: 'link-details', data: { link, readOnly: true } });
    },
    [setPanelContent],
  );
  const openDeviceMenu = useCallback(
    (event: React.MouseEvent, deviceId: string) => {
      checkBridgeHealth();
      refreshDeviceWinboxAvailability(deviceId);
      setDeviceMenu({ deviceId, x: event.clientX, y: event.clientY });
    }, [checkBridgeHealth, refreshDeviceWinboxAvailability, setDeviceMenu],
  );

  const {
    devices, setDevices, topologyLinks, loading, error, loadTopology,
    grafanaUrlRef, deviceGrafanaUrlsRef, refreshSettings,
    prometheusAlertDismissed, setPrometheusAlertDismissed,
    showRecoveryToast, setShowRecoveryToast,
    topologyRecoveryNotice, dismissTopologyRecoveryNotice, retryTopologyRefresh,
  } = useCanvasData({
    snapshot, alerts, reconnecting, prometheusStatus, editMode,
    openDeviceMenu, openEdgeMenu, openSelfLinkDetails,
    reactFlow, nodes, setNodes, setEdges, onDevicesChange, onLinksChange,
  });

  const alertCount = alerts.filter((alert) => alert.state === 'firing').length
    + (isPrometheusUnavailable(prometheusStatus) ? 1 : 0);

  // Area filtering: derive filtered devices/links and ghost devices
  const { filteredDevices, filteredLinks, ghostDevices } = useAreaFilteredTopology(
    devices, topologyLinks, selectedAreaId,
  );

  // Build a lookup for area colors (adapted for current theme)
  const areaColorMap = useMemo(() => {
    const map = new Map<string, string>();
    if (areas) {
      for (const area of areas) {
        map.set(area.id, adaptAreaColor(area.color, resolvedTheme));
      }
    }
    return map;
  }, [areas, resolvedTheme]);

  // Inject areaColors into node data based on device.area_ids
  const nodesWithAreaColor = useMemo(() => {
    if (areaColorMap.size === 0) return nodes;
    return nodes.map((n) => {
      const colors = (n.data.device.area_ids ?? [])
        .map((id) => areaColorMap.get(id))
        .filter((c): c is string => !!c);
      const newColors = colors.length > 0 ? colors : undefined;
      const prev = n.data.areaColors;
      if (prev?.length === newColors?.length && (prev ?? []).every((c, i) => c === newColors?.[i])) return n;
      return { ...n, data: { ...n.data, areaColors: newColors } };
    });
  }, [nodes, areaColorMap]);

  // Inject areaColor into edge data when both endpoints share at least one area
  const edgesWithAreaColor = useMemo(() => {
    if (areaColorMap.size === 0) return edges;
    const deviceAreaMap = new Map<string, string[]>();
    for (const n of nodesWithAreaColor) {
      deviceAreaMap.set(n.id, n.data.device.area_ids ?? []);
    }
    return edges.map((e) => {
      const srcAreas = new Set(deviceAreaMap.get(e.source) ?? []);
      const tgtAreas = deviceAreaMap.get(e.target) ?? [];
      const sharedArea = tgtAreas.find((a) => srcAreas.has(a));
      const color = sharedArea ? areaColorMap.get(sharedArea) : undefined;
      if (color === e.data?.areaColor) return e;
      return { ...e, data: { ...e.data!, areaColor: color } };
    });
  }, [edges, areaColorMap, nodesWithAreaColor]);

  // Build display nodes/edges by filtering the full node/edge set and adding ghost nodes
  const { displayNodes, displayEdges } = useMemo(() => {
    if (!selectedAreaId) {
      const selectedIds = new Set(nodesWithAreaColor.filter((node) => node.selected && !node.data.isGhost).map((node) => node.id));
      const emphasizedEdges = edgesWithAreaColor.map((edge) => {
        if (selectedIds.size === 0) {
          if (!edge.data?.emphasis) return edge;
          return { ...edge, data: { ...edge.data!, emphasis: 'default' } };
        }

        const emphasis = selectedIds.has(edge.source) || selectedIds.has(edge.target)
          ? 'connected'
          : 'muted';
        if (edge.data?.emphasis === emphasis) return edge;
        return { ...edge, data: { ...edge.data!, emphasis } };
      });

      return { displayNodes: nodesWithAreaColor, displayEdges: emphasizedEdges };
    }

    const filteredDeviceIds = new Set(filteredDevices.map((d) => d.id));
    // Keep existing nodes that are in the filtered area
    const areaNodes = nodesWithAreaColor.filter((n) => filteredDeviceIds.has(n.id));

    // Create ghost nodes for cross-area link endpoints
    const ghostNodes: DeviceNode[] = ghostDevices.map((device) => {
      const existingNode = nodesWithAreaColor.find((n) => n.id === device.id);

      // Position ghost near its connected real node (offset by 200px per RESEARCH recommendation)
      const connectedLink = filteredLinks.find(
        (l) => l.source_device_id === device.id || l.target_device_id === device.id,
      );
      const connectedRealDeviceId = connectedLink
        ? (connectedLink.source_device_id === device.id
            ? connectedLink.target_device_id
            : connectedLink.source_device_id)
        : null;
      const connectedRealNode = connectedRealDeviceId
        ? areaNodes.find((n) => n.id === connectedRealDeviceId)
        : null;

      const basePos = existingNode?.position
        ?? (connectedRealNode
          ? { x: connectedRealNode.position.x + 200, y: connectedRealNode.position.y }
          : { x: 0, y: 0 });

      return {
        id: device.id,
        type: 'device',
        position: basePos,
        draggable: false,
        data: {
          device,
          pinned: false,
          isGhost: true,
          onGhostClick: (deviceId: string) => {
            const clickedDevice = devices.find((d) => d.id === deviceId);
            if (clickedDevice?.area_ids?.[0] && onAreaSelect) {
              onAreaSelect(clickedDevice.area_ids[0]);
            }
          },
        },
      };
    });

    const allDisplayNodes = [...areaNodes, ...ghostNodes];

    // Filter edges to only include filtered links
    const filteredLinkIds = new Set(filteredLinks.map((l) => l.id));
    const areaEdges = edgesWithAreaColor.filter((e) => filteredLinkIds.has(e.id));
    const selectedIds = new Set(allDisplayNodes.filter((node) => node.selected && !node.data.isGhost).map((node) => node.id));
    const emphasizedEdges = areaEdges.map((edge) => {
      if (selectedIds.size === 0) {
        if (!edge.data?.emphasis) return edge;
        return { ...edge, data: { ...edge.data!, emphasis: 'default' } };
      }

      const emphasis = selectedIds.has(edge.source) || selectedIds.has(edge.target)
        ? 'connected'
        : 'muted';
      if (edge.data?.emphasis === emphasis) return edge;
      return { ...edge, data: { ...edge.data!, emphasis } };
    });

    return { displayNodes: allDisplayNodes, displayEdges: emphasizedEdges };
  }, [selectedAreaId, nodesWithAreaColor, edgesWithAreaColor, filteredDevices, filteredLinks, ghostDevices, devices, onAreaSelect]);

  // fitView when selectedAreaId changes to re-center on filtered subset
  const prevAreaRef = useRef<string | null>(null);
  useEffect(() => {
    if (prevAreaRef.current !== selectedAreaId && displayNodes.length > 0) {
      prevAreaRef.current = selectedAreaId;
      window.requestAnimationFrame(() => {
        reactFlow.fitView({ padding: 0.18, duration: 280 });
      });
    }
  }, [selectedAreaId, displayNodes.length, reactFlow]);

  useEffect(() => {
    setNodes((prev) => prev.map((n) => ({ ...n, data: { ...n.data, editMode } })));
    if (!editMode) setSelectedNodeCount(0);
  }, [editMode, setNodes]);
  useEffect(() => () => { if (highlightTimerRef.current !== null) window.clearTimeout(highlightTimerRef.current); }, []);

  async function handleLaunchWinBox(deviceId: string) {
    if (!bridgeSecret) {
      setWinboxError('Bridge secret not configured');
      return;
    }
    let token: string;
    try {
      token = await fetchBridgeToken(deviceId, bridgeSecret);
    } catch (error) {
      setWinboxError(error instanceof Error ? error.message : 'Failed to launch WinBox');
      return;
    }

    try {
      const res = await fetchBridgeWithTimeout(`http://localhost:${bridgePort}/launch`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({})) as { error?: string };
        setWinboxError(data.error ?? `Bridge error (${res.status})`);
      }
    } catch (error) {
      setWinboxError(getBridgeLaunchErrorMessage(error));
    }
  }

  const handleEdgesChange = useCallback((changes: EdgeChange[]) => {
    setEdges((cur) => applyEdgeChanges(changes, cur));
  }, []);
  const handleConnect = useCallback((connection: Connection) => {
    if (!editMode || !connection.source || !connection.target || connection.source === connection.target) return;
    setPanelContent({ type: 'create-link', data: { initialSourceDeviceId: connection.source, initialTargetDeviceId: connection.target } });
  }, [editMode, setPanelContent]);

  function focusOnDevice(deviceID: string) {
    // If the target device is not in the current area view, switch to its area first
    const device = devices.find((d) => d.id === deviceID);
    if (device && selectedAreaId && !device.area_ids?.includes(selectedAreaId)) {
      // Switch to the device's area (or global if unassigned)
      onAreaSelect?.(device.area_ids?.[0] ?? null);
      // Defer the focus/highlight until after the area switch triggers a re-render
      window.requestAnimationFrame(() => {
        window.requestAnimationFrame(() => {
          const target = reactFlow.getNodes().find((n) => n.id === deviceID);
          if (!target) return;
          reactFlow.setCenter(target.position.x + 110, target.position.y + 44, { zoom: 1.2, duration: 500 });
          setNodes((cur) => cur.map((n) => ({ ...n, data: { ...n.data, highlighted: n.id === deviceID } })));
          if (highlightTimerRef.current !== null) window.clearTimeout(highlightTimerRef.current);
          highlightTimerRef.current = window.setTimeout(() => {
            setNodes((cur) => cur.map((n) => n.id === deviceID ? { ...n, data: { ...n.data, highlighted: false } } : n));
          }, 2000);
        });
      });
      return;
    }

    const target = reactFlow.getNodes().find((n) => n.id === deviceID);
    if (!target) return;
    reactFlow.setCenter(target.position.x + 110, target.position.y + 44, { zoom: 1.2, duration: 500 });
    setNodes((cur) => cur.map((n) => ({ ...n, data: { ...n.data, highlighted: n.id === deviceID } })));
    if (highlightTimerRef.current !== null) window.clearTimeout(highlightTimerRef.current);
    highlightTimerRef.current = window.setTimeout(() => {
      setNodes((cur) => cur.map((n) => n.id === deviceID ? { ...n, data: { ...n.data, highlighted: false } } : n));
    }, 2000);
  }

  // Resolve Grafana URL for a device (per-device override or global)
  function grafanaUrl(deviceId?: string): string {
    if (deviceId) return deviceGrafanaUrlsRef.current.get(deviceId) || grafanaUrlRef.current;
    return grafanaUrlRef.current;
  }

  if (loading) {
    return (
      <div className="topology-backdrop flex h-full items-center justify-center bg-bg">
        <div className="rounded-[28px] border border-outline bg-surface/88 px-6 py-5 text-center shadow-canvas backdrop-blur-sm">
          <div className="mx-auto mb-3 h-10 w-10 animate-spin rounded-full border-2 border-outline-subtle border-t-primary" />
          <p className="text-sm uppercase tracking-[0.28em] text-on-bg-secondary">Loading topology...</p>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="topology-backdrop flex h-full items-center justify-center bg-bg px-6">
        <div className="max-w-md rounded-[28px] border border-outline bg-surface/88 px-6 py-6 text-center shadow-canvas backdrop-blur-sm">
          <p className="text-sm uppercase tracking-[0.28em] text-status-down">Topology Error</p>
          <h2 className="mt-3 text-2xl font-semibold tracking-tight text-on-bg">Canvas data could not load</h2>
          <p className="mt-3 text-sm text-on-bg-secondary">{error}</p>
          <button type="button" onClick={() => { void loadTopology(); }}
            className="mt-6 rounded-full border border-primary/40 bg-primary/10 px-5 py-2 text-sm font-medium text-primary transition-colors duration-150 hover:bg-primary/20">
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="topology-backdrop relative h-full w-full bg-bg">
      {showSearch && <SearchOverlay devices={devices} onSelectDevice={focusOnDevice} />}
      <Toolbar onSearch={() => setShowSearch((s) => !s)} onAddDevice={() => setPanelContent({ type: 'addDevice' })}
        onCreateLink={() => setPanelContent({ type: 'create-link' })} onAlerts={() => setPanelContent({ type: 'alerts' })}
        onSettings={() => setPanelContent({ type: 'settings' })} onToggleEditMode={() => setEditMode((m) => !m)}
        editMode={editMode}
        alertCount={alertCount} />

      {deviceMenu && (() => {
        const d = devices.find((dev) => dev.id === deviceMenu.deviceId);
        const gUrl = grafanaUrl(d?.id);
        const isVirtual = d?.device_type === 'virtual';
        const hasWinboxProfile = deviceWinboxState[deviceMenu.deviceId];
        const winboxDisabled = hasWinboxProfile === false;
        const winboxTitle = hasWinboxProfile === false
          ? 'No WinBox profile designated'
          : bridgeChecked && !bridgeRunning
            ? 'WinBox bridge appears unavailable - click to try launch anyway'
            : undefined;
        const items = buildDeviceContextMenuItems({
          isVirtual,
          grafanaEnabled: Boolean(gUrl),
          winboxDisabled,
          winboxTitle,
          onOpenWinbox: () => {
            if (d) void handleLaunchWinBox(d.id);
            setDeviceMenu(null);
          },
          onOpenGrafana: () => {
            if (gUrl) window.open(gUrl, '_blank');
            setDeviceMenu(null);
          },
          onOpenInterfaceStats: () => {
            if (d) setPanelContent({ type: 'interfaceStats', data: { device: d } });
            setDeviceMenu(null);
          },
          onConfigure: () => {
            if (d) setPanelContent({ type: 'deviceConfig', data: { device: d } });
            setDeviceMenu(null);
          },
        });
        return (
          <ContextMenu position={{ x: deviceMenu.x, y: deviceMenu.y }} onClose={() => setDeviceMenu(null)} items={items} />
        );
      })()}

      {edgeMenu && (() => {
        const me = edges.find((e) => e.id === edgeMenu.edgeID);
        const ml = me?.data?.link;
        const dMap = new Map(devices.map((d) => [d.id, d]));
        const sd = ml ? dMap.get(ml.source_device_id) : undefined;
        const td = ml ? dMap.get(ml.target_device_id) : undefined;
        const gUrl = grafanaUrl(sd?.id);
        return (
          <ContextMenu position={{ x: edgeMenu.x, y: edgeMenu.y }} onClose={() => setEdgeMenu(null)} items={[
            { label: 'Per-Interface Stats', icon: 'devices', onClick: () => { if (ml && sd && td) setPanelContent({ type: 'interfaceStats', data: { linkId: ml.id, link: ml, sourceDevice: sd, targetDevice: td } }); setEdgeMenu(null); } },
            { label: gUrl ? 'Open in Grafana' : 'Open in Grafana (not configured)', icon: 'hub', onClick: () => { if (gUrl) window.open(gUrl, '_blank'); setEdgeMenu(null); } },
            { label: 'View Details', icon: 'search', onClick: () => { const el = edges.find((e) => e.id === edgeMenu.edgeID)?.data?.link; if (el) setPanelContent({ type: 'link-details', data: { link: el, readOnly: true } }); setEdgeMenu(null); } },
          ]} />
        );
      })()}

      <SidePanel open={!!panelContent} onClose={() => setPanelContent(null)} title={getPanelTitle()}>
        <CanvasPanels panelContent={panelContent} setPanelContent={setPanelContent} snapshot={snapshot} alerts={alerts}
          devices={devices} topologyLinks={topologyLinks} loadTopology={loadTopology}
          setDevices={setDevices} setNodes={setNodes} reactFlow={reactFlow} prometheusStatus={prometheusStatus}
          onAreasChange={onAreasChange} onSettingsChange={refreshSettings}
          onWinBoxAvailabilityChange={(deviceId, hasWinboxProfile) => {
            setDeviceWinboxAvailability(deviceId, hasWinboxProfile);
          }} />
      </SidePanel>

      <ShortcutHelp open={showShortcuts} onClose={() => setShowShortcuts(false)} />
      <CanvasOverlays editMode={editMode} reconnecting={reconnecting}
        showRecoveryToast={showRecoveryToast} setShowRecoveryToast={setShowRecoveryToast}
        topologyRecoveryNotice={topologyRecoveryNotice}
        dismissTopologyRecoveryNotice={dismissTopologyRecoveryNotice}
        retryTopologyRefresh={retryTopologyRefresh}
        prometheusStatus={prometheusStatus} prometheusAlertDismissed={prometheusAlertDismissed}
        setPrometheusAlertDismissed={setPrometheusAlertDismissed} setPanelContent={setPanelContent}
        selectedNodeCount={selectedNodeCount}
        onBulkEditClick={() => {
          const selectedNodes = reactFlow.getNodes().filter((n) => n.selected);
          if (selectedNodes.length > 1) {
            setPanelContent({ type: 'bulkEdit', data: { deviceIds: selectedNodes.map((n) => n.id) } });
          }
        }} />
      {/* WinBox launch error toast */}
      {winboxError && (
        <div className="absolute bottom-16 left-1/2 z-50 flex -translate-x-1/2 items-center gap-2 rounded-full border border-status-down/35 bg-surface-container-high px-4 py-2.5 text-xs text-status-down shadow-floating">
          <span>{winboxError}</span>
          <button type="button" onClick={() => setWinboxError(null)} className="ml-1 hover:opacity-70">&times;</button>
        </div>
      )}

      <ZoomControls onZoomIn={() => { void reactFlow.zoomIn({ duration: 200 }); }}
        onZoomOut={() => { void reactFlow.zoomOut({ duration: 200 }); }}
        onFitView={() => { void reactFlow.fitView({ padding: 0.18, duration: 280 }); }} />

      <ReactFlow nodes={displayNodes} edges={displayEdges} nodeTypes={nodeTypes} edgeTypes={edgeTypes}
        onNodesChange={onNodesChange} onEdgesChange={handleEdgesChange} onConnect={handleConnect}
        onSelectionChange={handleSelectionChange}
        onPaneClick={() => { setEdgeMenu(null); setDeviceMenu(null); setPanelContent(null); setShowSearch(false); setShowShortcuts(false); }}
        onNodeClick={(_ev, node) => {
          if (node.data.isGhost) return;
          // Check if multiple nodes are selected (including the just-clicked one)
          const selectedNodes = reactFlow.getNodes().filter((n) => n.selected);
          if (selectedNodes.length > 1 && editMode) {
            setPanelContent({ type: 'bulkEdit', data: { deviceIds: selectedNodes.map((n) => n.id) } });
          } else {
            const cd = devices.find((d) => d.id === node.id);
            if (cd) setPanelContent({ type: 'deviceConfig', data: { device: cd } });
          }
        }}
        onEdgeClick={(_ev, edge) => {
          const lk = edge.data?.link;
          if (!lk) return;
          setPanelContent({
            type: 'link-details',
            data: { link: lk, readOnly: !editMode },
          });
        }}
        onNodeDragStop={(_ev, node) => {
          if (node.data.isGhost) return;
          const updated = reactFlow.getNodes().map((cn) => cn.id === node.id ? { ...cn, position: node.position, data: { ...cn.data, pinned: true } } : cn);
          const dMap = new Map(devices.map((d) => [d.id, d]));
          const eMap = new Map(edges.map((e) => [e.id, e.data ?? {}]));
          setNodes(updated); setEdges(buildTopologyEdges(topologyLinks, dMap, updated, eMap, openEdgeMenu));
          void savePositions(buildPositionPayload(updated));
        }}
        selectionOnDrag={editMode} selectionMode={SelectionMode.Partial} selectionKeyCode="Shift"
        connectionMode={ConnectionMode.Loose} minZoom={0.1} maxZoom={2} fitView
        nodesDraggable={editMode} panOnDrag zoomOnScroll zoomOnDoubleClick={false}
        connectionLineStyle={{ stroke: 'var(--color-edge-default)', strokeWidth: 2 }} proOptions={{ hideAttribution: false }} className="bg-transparent">
        <Background color="var(--color-edge-muted)" gap={30} size={1.1} />
        <MiniMap pannable zoomable
          nodeColor={(n) => {
            const d = (n as DeviceNode).data;
            return minimapColorForDevice({
              device: d.device,
              metrics: d.metrics,
              isGhost: !!d.isGhost,
            });
          }}
          style={{ backgroundColor: 'var(--nt-surface-container)', border: '1px solid var(--nt-outline)', borderRadius: 16, boxShadow: 'var(--nt-shadow-floating)' }} maskColor="var(--nt-minimap-mask, rgba(45, 45, 61, 0.55))" />
      </ReactFlow>
    </div>
  );
}
