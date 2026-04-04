import { useCallback, useEffect, useRef, useState } from 'react';
import type { ReactFlowInstance } from '@xyflow/react';

import { fetchDevices, fetchLinks, fetchSettings, createLink } from '../../api/client';
import { computeForceLayout } from '../../hooks/useAutoLayout';
import { usePositions } from '../../hooks/usePositions';
import type { Device, Link } from '../../types/api';
import {
  alertStatusForDevice,
  type PrometheusStatusPayload,
  type SnapshotPayload,
} from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import {
  buildPositionPayload,
  buildThroughputLabel,
  findLinkMetrics,
  manualEdgeStorageKey,
  staleThresholdMs,
  viewportSize,
} from './canvasHelpers';
import { alertStatusForLink, buildTopologyEdges } from './edgeBuilder';
import { buildTopologyNodes } from './nodeBuilder';

interface UseCanvasDataParams {
  snapshot: SnapshotPayload | null;
  reconnecting: boolean;
  prometheusStatus: PrometheusStatusPayload | null;
  editMode: boolean;
  openDeviceMenu: (event: React.MouseEvent, deviceId: string) => void;
  openEdgeMenu: (event: MouseEvent | React.MouseEvent<SVGPathElement>, edgeID: string) => void;
  reactFlow: ReactFlowInstance<DeviceNode, LinkEdgeType>;
  nodes: DeviceNode[];
  setNodes: React.Dispatch<React.SetStateAction<DeviceNode[]>>;
  setEdges: React.Dispatch<React.SetStateAction<LinkEdgeType[]>>;
  onDevicesChange?: (devices: Device[]) => void;
  onLinksChange?: (links: Link[]) => void;
}

interface UseCanvasDataReturn {
  devices: Device[];
  setDevices: React.Dispatch<React.SetStateAction<Device[]>>;
  topologyLinks: Link[];
  loading: boolean;
  error: string | null;
  loadTopology: (isSilentRefresh?: boolean, defaultPosition?: { x: number; y: number }) => Promise<void>;
  grafanaUrlRef: React.MutableRefObject<string>;
  deviceGrafanaUrlsRef: React.MutableRefObject<Map<string, string>>;
  refreshSettings: () => void;
  prometheusAlertDismissed: boolean;
  setPrometheusAlertDismissed: React.Dispatch<React.SetStateAction<boolean>>;
  showRecoveryToast: boolean;
  setShowRecoveryToast: React.Dispatch<React.SetStateAction<boolean>>;
}

export function useCanvasData({
  snapshot,
  prometheusStatus,
  editMode,
  openDeviceMenu,
  openEdgeMenu,
  reactFlow,
  nodes: _nodes,
  setNodes,
  setEdges,
  onDevicesChange,
  onLinksChange,
}: UseCanvasDataParams): UseCanvasDataReturn {
  const [devices, setDevices] = useState<Device[]>([]);
  const [topologyLinks, setTopologyLinks] = useState<Link[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const snapshotRef = useRef<SnapshotPayload | null>(null);
  const prometheusStatusRef = useRef<PrometheusStatusPayload | null>(null);
  const devicesRef = useRef<Device[]>([]);
  const lastSnapshotTimeRef = useRef<number | null>(null);
  const staleAppliedRef = useRef(false);
  const layoutInitializedRef = useRef(false);
  const grafanaUrlRef = useRef<string>('');
  const deviceGrafanaUrlsRef = useRef<Map<string, string>>(new Map());
  const [prometheusAlertDismissed, setPrometheusAlertDismissed] = useState(false);

  const { fetchPositions, savePositions } = usePositions();

  // Track Prometheus recovery transition and auto-dismiss recovery toast.
  const prevPromAvailableRef = useRef<boolean | null>(null);
  const [showRecoveryToast, setShowRecoveryToast] = useState(false);

  // Keep refs in sync so async loadTopology and snapshot effect can read the latest state
  useEffect(() => {
    snapshotRef.current = snapshot;
  }, [snapshot]);
  useEffect(() => {
    prometheusStatusRef.current = prometheusStatus;
  }, [prometheusStatus]);
  devicesRef.current = devices;

  // Propagate device state changes to parent (for Dashboard view)
  useEffect(() => {
    onDevicesChange?.(devices);
  }, [devices, onDevicesChange]);

  // Propagate link state changes to parent (for Hub view)
  useEffect(() => {
    onLinksChange?.(topologyLinks);
  }, [topologyLinks, onLinksChange]);

  // Stable reference to devices.length for loadTopology closure
  const devicesLengthRef = useRef(0);
  devicesLengthRef.current = devices.length;

  const loadTopology = useCallback(
    async (isSilentRefresh = false, defaultPosition?: { x: number; y: number }) => {
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

        // Read any pending snapshot so first-load metrics are included in the
        // initial node/edge data -- eliminates the race where the WS snapshot
        // arrives before loadTopology resolves and the snapshot effect maps over
        // an empty node array.
        const pendingSnapshot = snapshotRef.current;

        // When Prometheus is down, override prometheus-sourced devices to 'down'
        // so the initial render shows error styling immediately (without waiting
        // for the snapshot effect to re-apply the override).
        let effectiveSnapshot = pendingSnapshot;
        const promStatus = prometheusStatusRef.current;
        if (pendingSnapshot && promStatus !== null && !promStatus.available) {
          const effectiveStatuses = { ...pendingSnapshot.device_statuses };
          for (const d of fetchedDevices) {
            const src = d.metrics_source || 'prometheus';
            if (src === 'prometheus' || src === 'prometheus_snmp_fallback') {
              effectiveStatuses[d.id] = 'down';
            }
          }
          effectiveSnapshot = { ...pendingSnapshot, device_statuses: effectiveStatuses };
        }

        const nextNodes = buildTopologyNodes(
          fetchedDevices,
          savedPositions,
          computedPositions,
          defaultPosition,
          editMode,
          openDeviceMenu,
          effectiveSnapshot,
        );

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

        let nextEdges = buildTopologyEdges(fetchedLinks, devicesByID, nextNodes, undefined, openEdgeMenu);

        // Merge pending snapshot link metrics into edges and mark snapshot time
        // so the stale-data timer doesn't immediately clear the metrics.
        if (effectiveSnapshot) {
          lastSnapshotTimeRef.current = Date.now();
          nextEdges = nextEdges.map((edge) => {
            const link = edge.data?.link;
            if (!link) return edge;
            const linkAlertStatus = alertStatusForLink(link, effectiveSnapshot.alerts);
            const srcStatus = effectiveSnapshot.device_statuses[link.source_device_id];
            const tgtStatus = effectiveSnapshot.device_statuses[link.target_device_id];
            const bothDown = srcStatus === 'down' && tgtStatus === 'down';
            const metrics = bothDown ? null : findLinkMetrics(effectiveSnapshot.link_metrics, link);
            return {
              ...edge,
              data: {
                ...edge.data!,
                alertStatus: linkAlertStatus === 'normal' ? undefined : linkAlertStatus,
                sourceDeviceStatus: srcStatus,
                targetDeviceStatus: tgtStatus,
                metrics: metrics,
                throughputLabel: metrics ? buildThroughputLabel(metrics) : undefined,
                utilization: metrics?.utilization ?? null,
              },
            };
          });
        }

        // Apply all state updates together as urgent (not in startTransition).
        // Previously these were wrapped in startTransition which made them
        // low-priority, allowing WebSocket snapshot effects (which depend on
        // devices.length) to interrupt and race with the transition's setNodes,
        // sometimes causing all canvas nodes to vanish after a device delete.
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

        if (!layoutInitializedRef.current || fetchedDevices.length !== devicesLengthRef.current) {
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
    },
    [editMode, openDeviceMenu, openEdgeMenu, reactFlow, setNodes, setEdges, fetchPositions, savePositions],
  );

  // Initial load
  useEffect(() => {
    void loadTopology();
  }, []);

  // Re-fetch settings (Grafana URLs) on demand; called on mount and after
  // any settings panel or device config panel saves Grafana URL changes.
  const refreshSettings = useCallback(() => {
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

  // Fetch settings on mount
  useEffect(() => {
    refreshSettings();
  }, [refreshSettings]);

  // Prometheus recovery toast effect
  useEffect(() => {
    if (prometheusStatus === null) return;

    // Detect recovery: was unavailable, now available
    if (prevPromAvailableRef.current === false && prometheusStatus.available) {
      setShowRecoveryToast(true);
      setPrometheusAlertDismissed(false);
      const timer = window.setTimeout(() => setShowRecoveryToast(false), 8000);
      prevPromAvailableRef.current = prometheusStatus.available;
      return () => { window.clearTimeout(timer); };
    }

    if (prometheusStatus.available) {
      setPrometheusAlertDismissed(false);
    }

    prevPromAvailableRef.current = prometheusStatus.available;
  }, [prometheusStatus?.available]);

  // Apply snapshot data to nodes and edges.
  //
  // IMPORTANT: This effect intentionally does NOT depend on devices.length or
  // topologyLinks.length. Previously those were included to re-apply snapshot
  // data after topology changes (add/delete device or link), but loadTopology
  // already bakes snapshot data into the nodes/edges it builds (via
  // snapshotRef.current). The redundant re-trigger caused a cascade of
  // competing setNodes/setEdges calls that produced rendering glitches:
  //   loadTopology setNodes -> devices.length changes -> snapshot effect
  //   setNodes again -> displayNodes new refs -> StoreUpdater replace cycle.
  // By only reacting to actual snapshot or Prometheus status changes, we
  // eliminate the double-update after add/delete operations.
  //
  // The promDown override uses devicesRef (always current) instead of the
  // devices state variable so it doesn't need devices.length as a dependency.
  useEffect(() => {
    if (snapshot === null) {
      return;
    }

    lastSnapshotTimeRef.current = Date.now();
    staleAppliedRef.current = false;

    // Compute effective device statuses: override prometheus-only devices to 'down'
    // when Prometheus is unreachable (no probe_success data available).
    const promDown = prometheusStatus !== null && !prometheusStatus.available;
    const effectiveStatuses = { ...snapshot.device_statuses };

    if (promDown) {
      // Use ref to always read the latest devices without adding devices.length
      // to the dependency array (which would cause redundant re-triggers after
      // loadTopology).
      for (const d of devicesRef.current) {
        const src = d.metrics_source || 'prometheus';
        if (src === 'prometheus' || src === 'prometheus_snmp_fallback') {
          effectiveStatuses[d.id] = 'down';
        }
        // 'none' and 'snmp' sources are unaffected by Prometheus availability
      }
    }

    // Apply all snapshot data (status, hostname, alerts, metrics) in a single
    // urgent update so the first snapshot on page load renders immediately
    // instead of being split across an urgent + deferred transition pass.
    setNodes((currentNodes) =>
      currentNodes.map((node) => {
        const newStatus = effectiveStatuses[node.id];
        const newHostname = snapshot.device_hostnames[node.id];
        const updatedDevice = newStatus || newHostname
          ? {
              ...node.data.device,
              ...(newStatus ? { status: newStatus as Device['status'] } : {}),
              ...(newHostname ? { sys_name: newHostname } : {}),
            }
          : node.data.device;

        // When a device is confirmed down, null out metrics so the card shows
        // error styling rather than potentially stale last-known values.
        const isDown = newStatus === 'down' || (!newStatus && node.data.device.status === 'down');
        const nodeMetrics = isDown ? null : (snapshot.device_metrics[node.id] ?? null);

        return {
          ...node,
          data: {
            ...node.data,
            alertStatus: alertStatusForDevice(node.id, snapshot.alerts),
            device: updatedDevice,
            metrics: nodeMetrics,
          },
        };
      }),
    );

    // Sync devices state with hostnames/statuses so panels (e.g. LinkCreatePanel) see them
    if (Object.keys(snapshot.device_hostnames).length > 0 || Object.keys(effectiveStatuses).length > 0) {
      setDevices((prev) =>
        prev.map((d) => {
          const newHostname = snapshot.device_hostnames[d.id];
          const newStatus = effectiveStatuses[d.id];
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
        const srcStatus = effectiveStatuses[link.source_device_id];
        const tgtStatus = effectiveStatuses[link.target_device_id];

        // When both link endpoints are down, null out link metrics so throughput
        // labels don't show stale last-known values.
        const bothDown = srcStatus === 'down' && tgtStatus === 'down';
        const metrics = bothDown ? null : findLinkMetrics(snapshot.link_metrics, link);

        return {
          ...edge,
          data: {
            ...edge.data,
            alertStatus: linkAlertStatus === 'normal' ? undefined : linkAlertStatus,
            sourceDeviceStatus: srcStatus,
            targetDeviceStatus: tgtStatus,
            metrics: metrics,
            throughputLabel: metrics ? buildThroughputLabel(metrics) : undefined,
            utilization: metrics?.utilization ?? null,
          },
        };
      }),
    );
  }, [snapshot, setNodes, prometheusStatus?.available]);

  // Stale data timer
  useEffect(() => {
    const interval = window.setInterval(() => {
      if (lastSnapshotTimeRef.current === null || staleAppliedRef.current) {
        return;
      }

      if (Date.now() - lastSnapshotTimeRef.current <= staleThresholdMs) {
        return;
      }

      staleAppliedRef.current = true;

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
    }, 10_000);

    return () => {
      window.clearInterval(interval);
    };
  }, [setNodes]);

  return {
    devices,
    setDevices,
    topologyLinks,
    loading,
    error,
    loadTopology,
    grafanaUrlRef,
    deviceGrafanaUrlsRef,
    refreshSettings,
    prometheusAlertDismissed,
    setPrometheusAlertDismissed,
    showRecoveryToast,
    setShowRecoveryToast,
  };
}
