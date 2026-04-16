import type { ReactFlowInstance } from '@xyflow/react';

import { fetchDevices } from '../../api/client';
import type { Device, Link } from '../../types/api';
import type { PrometheusStatusPayload, SnapshotPayload } from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import { InterfaceStatsPanel, DeviceInterfaceStatsPanel } from '../InterfaceStatsPanel';
import { AlertsPanel } from '../AlertsPanel';
import { SettingsPanel } from '../SettingsPanel';
import { AddDevicePanel } from '../AddDevicePanel';
import { DeviceConfigPanel } from '../DeviceConfigPanel';
import { BulkEditPanel } from '../BulkEditPanel';
import { LinkCreatePanel } from '../LinkCreatePanel';
import { LinkDetailsPanel } from '../LinkDetailsPanel';
import { viewportSize } from './canvasHelpers';
import { getEffectivePollingIntervalSeconds } from '../../utils/polling';
import {
  resolveDeviceMonitoringState,
  sanitizeDeviceMetricsForDisplay,
} from '../deviceVisualState';

interface CanvasPanelsProps {
  panelContent: { type: string; data?: unknown } | null;
  setPanelContent: (content: { type: string; data?: unknown } | null) => void;
  snapshot: SnapshotPayload | null;
  devices: Device[];
  loadTopology: (silent?: boolean, pos?: { x: number; y: number }) => Promise<void>;
  setDevices: React.Dispatch<React.SetStateAction<Device[]>>;
  setNodes: React.Dispatch<React.SetStateAction<DeviceNode[]>>;
  reactFlow: ReactFlowInstance<DeviceNode, LinkEdgeType>;
  prometheusStatus: PrometheusStatusPayload | null;
  onAreasChange?: () => void;
  onSettingsChange?: () => void;
  onWinBoxAvailabilityChange?: (deviceId: string, hasWinboxProfile: boolean) => void;
}

export function CanvasPanels({
  panelContent,
  setPanelContent,
  snapshot,
  devices,
  loadTopology,
  setDevices,
  setNodes,
  reactFlow,
  prometheusStatus,
  onAreasChange,
  onSettingsChange,
  onWinBoxAvailabilityChange,
}: CanvasPanelsProps) {
  return (
    <>
      {panelContent?.type === 'interfaceStats' && (() => {
        const data = panelContent.data as { link?: Link; sourceDevice?: Device; targetDevice?: Device; device?: Device } | undefined;
        if (data?.link && data.sourceDevice && data.targetDevice) {
          // Look up live device state so promDown overrides are reflected
          const currentSource = devices.find((d) => d.id === data.sourceDevice!.id) ?? data.sourceDevice;
          const currentTarget = devices.find((d) => d.id === data.targetDevice!.id) ?? data.targetDevice;
          return (
            <InterfaceStatsPanel
              link={data.link}
              sourceDevice={currentSource}
              targetDevice={currentTarget}
              snapshot={snapshot as SnapshotPayload | null}
              prometheusStatus={prometheusStatus}
            />
          );
        }
        if (data?.device) {
          const currentDevice = devices.find((d) => d.id === data.device!.id) ?? data.device;
          return (
            <DeviceInterfaceStatsPanel
              device={currentDevice}
              snapshot={snapshot as SnapshotPayload | null}
              prometheusStatus={prometheusStatus}
            />
          );
        }
        return <div className="text-on-bg-secondary text-sm">No data available.</div>;
      })()}
      {panelContent?.type === 'alerts' && (
        <AlertsPanel
          alerts={snapshot?.alerts ?? []}
          devices={devices}
          prometheusStatus={prometheusStatus}
        />
      )}
      {panelContent?.type === 'settings' && <SettingsPanel onAreasChange={onAreasChange} onSettingsChange={onSettingsChange} />}
      {panelContent?.type === 'addDevice' && (
        <AddDevicePanel
          onDeviceAdded={() => {
            const { width, height } = viewportSize();
            const center = reactFlow.screenToFlowPosition({
              x: width / 2,
              y: height / 2,
            });
            setPanelContent(null);
            void loadTopology(true, center);
          }}
        />
      )}
      {panelContent?.type === 'create-link' && (
        <LinkCreatePanel
          devices={devices}
          onCreated={() => {
            setPanelContent(null);
            void loadTopology(true);
          }}
          onClose={() => setPanelContent(null)}
          onRefreshDevices={async () => {
            const refreshedDevices = await fetchDevices();
            setDevices(refreshedDevices);
          }}
          initialSourceDeviceId={(panelContent.data as { initialSourceDeviceId?: string })?.initialSourceDeviceId}
          initialTargetDeviceId={(panelContent.data as { initialTargetDeviceId?: string })?.initialTargetDeviceId}
        />
      )}
      {panelContent?.type === 'link-details' && (() => {
        const data = panelContent.data as { link?: Link; readOnly?: boolean } | undefined;
        if (data?.link) {
          return (
            <LinkDetailsPanel
              link={data.link}
              readOnly={data.readOnly === true}
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
        const device = data?.device;
        if (device) {
          return (
            <DeviceConfigPanel
              device={device}
              isVirtual={device.device_type === 'virtual'}
              onDeviceUpdated={(updated) => {
                setDevices((prev) => prev.map((d) => d.id === updated.id ? updated : d));
                setNodes((prev) => prev.map((n) => n.id === updated.id
                  ? {
                      ...n,
                      data: {
                        ...n.data,
                        device: updated,
                        isVirtual: updated.device_type === 'virtual',
                        monitoringState: resolveDeviceMonitoringState(updated),
                        subtype: updated.device_type === 'virtual'
                          ? (updated.tags?.virtual_subtype ?? 'generic')
                          : undefined,
                        metrics: sanitizeDeviceMetricsForDisplay(
                          updated,
                          n.data.metrics
                            ? {
                                ...n.data.metrics,
                                expected_poll_interval_seconds: getEffectivePollingIntervalSeconds(updated),
                              }
                            : n.data.metrics,
                        ),
                      },
                    }
                  : n,
                ));
                setPanelContent({ type: 'deviceConfig', data: { device: updated } });
              }}
              onDeviceDeleted={() => {
                setPanelContent(null);
                void loadTopology(true);
              }}
              onSettingsChange={onSettingsChange}
              onWinBoxAvailabilityChange={(hasWinboxProfile) => {
                onWinBoxAvailabilityChange?.(device.id, hasWinboxProfile);
              }}
            />
          );
        }
        return null;
      })()}
      {panelContent?.type === 'bulkEdit' && (() => {
        const data = panelContent.data as { deviceIds?: string[] } | undefined;
        if (data?.deviceIds && data.deviceIds.length > 1) {
          const selectedDevices = data.deviceIds
            .map((id) => devices.find((d) => d.id === id))
            .filter((d): d is Device => d !== undefined);
          if (selectedDevices.length < 2) return null;
          return (
            <BulkEditPanel
              devices={selectedDevices}
              onDevicesUpdated={(updatedDevices) => {
                setDevices((prev) => {
                  const updatedMap = new Map(updatedDevices.map((d) => [d.id, d]));
                  return prev.map((d) => updatedMap.get(d.id) ?? d);
                });
                setNodes((prev) => {
                  const updatedMap = new Map(updatedDevices.map((d) => [d.id, d]));
                  return prev.map((n) => {
                    const updated = updatedMap.get(n.id);
                    return updated ? { ...n, data: { ...n.data, device: updated } } : n;
                  });
                });
                // Re-open bulk panel with fresh device data
                setPanelContent({ type: 'bulkEdit', data: { deviceIds: data.deviceIds } });
              }}
              onDevicesDeleted={() => {
                setPanelContent(null);
                void loadTopology(true);
              }}
            />
          );
        }
        return null;
      })()}
    </>
  );
}
