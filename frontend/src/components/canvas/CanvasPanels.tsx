import type { ReactFlowInstance } from '@xyflow/react';

import { fetchDevices } from '../../api/client';
import type { Device, Link } from '../../types/api';
import type { AlertDTO } from '../../types/metrics';
import { AddDevicePanel } from '../AddDevicePanel';
import { AlertsPanel } from '../AlertsPanel';
import { BulkEditPanel } from '../BulkEditPanel';
import type { DeviceNode } from '../DeviceCard';
import { DeviceConfigPanel } from '../DeviceConfigPanel';
import { LinkCreatePanel } from '../LinkCreatePanel';
import { LinkDetailsPanel } from '../LinkDetailsPanel';
import type { LinkEdgeType } from '../LinkEdge';
import { SettingsPanel } from '../SettingsPanel';
import {
  resolveDeviceMonitoringState,
  sanitizeDeviceMetricsForDisplay,
} from '../deviceVisualState';
import {
  DeviceInterfaceStatsPanelRoute,
  LinkInterfaceStatsPanelRoute,
} from './InterfaceStatsPanelRoutes';
import { viewportSize } from './canvasHelpers';
import { buildAlertsPanelModel } from './panelAdapters';
import type { RuntimeState } from './runtimeAdapters';

const emptyAlerts: AlertDTO[] = [];

interface CanvasPanelsProps {
  panelContent: { type: string; data?: unknown } | null;
  setPanelContent: (content: { type: string; data?: unknown } | null) => void;
  alerts?: AlertDTO[];
  devices: Device[];
  topologyLinks: Link[];
  loadTopology: (silent?: boolean, pos?: { x: number; y: number }) => Promise<void>;
  setDevices: React.Dispatch<React.SetStateAction<Device[]>>;
  setNodes: React.Dispatch<React.SetStateAction<DeviceNode[]>>;
  reactFlow: ReactFlowInstance<DeviceNode, LinkEdgeType>;
  runtimeState: RuntimeState;
  editMode?: boolean;
  onAreasChange?: () => void;
  onSettingsChange?: () => void;
  onWinBoxAvailabilityChange?: (deviceId: string, hasWinboxProfile: boolean) => void;
}

export function CanvasPanels({
  panelContent,
  setPanelContent,
  alerts = emptyAlerts,
  devices,
  topologyLinks,
  loadTopology,
  setDevices,
  setNodes,
  reactFlow,
  runtimeState,
  editMode = false,
  onAreasChange,
  onSettingsChange,
  onWinBoxAvailabilityChange,
}: CanvasPanelsProps) {
  return (
    <>
      {panelContent?.type === 'interfaceStats' &&
        (() => {
          const data = panelContent.data as { linkId?: string; deviceId?: string } | undefined;
          const link = data?.linkId
            ? topologyLinks.find((candidate) => candidate.id === data.linkId)
            : undefined;
          if (link) {
            const currentSource = devices.find((d) => d.id === link.source_device_id);
            const currentTarget = devices.find((d) => d.id === link.target_device_id);
            if (!currentSource || !currentTarget)
              return <div className="text-on-bg-secondary text-sm">No data available.</div>;
            return (
              <LinkInterfaceStatsPanelRoute
                link={link}
                sourceDevice={currentSource}
                targetDevice={currentTarget}
                runtimeState={runtimeState}
              />
            );
          }
          const currentDevice = data?.deviceId
            ? devices.find((d) => d.id === data.deviceId)
            : undefined;
          if (currentDevice) {
            return (
              <DeviceInterfaceStatsPanelRoute device={currentDevice} runtimeState={runtimeState} />
            );
          }
          return <div className="text-on-bg-secondary text-sm">No data available.</div>;
        })()}
      {panelContent?.type === 'alerts' && (
        <AlertsPanel model={buildAlertsPanelModel({ alerts, runtimeState })} />
      )}
      {panelContent?.type === 'settings' && (
        <SettingsPanel onAreasChange={onAreasChange} onSettingsChange={onSettingsChange} />
      )}
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
          initialSourceDeviceId={
            (panelContent.data as { initialSourceDeviceId?: string })?.initialSourceDeviceId
          }
          initialTargetDeviceId={
            (panelContent.data as { initialTargetDeviceId?: string })?.initialTargetDeviceId
          }
        />
      )}
      {panelContent?.type === 'link-details' &&
        (() => {
          const data = panelContent.data as { link?: Link; readOnly?: boolean } | undefined;
          if (data?.link) {
            const liveLink =
              topologyLinks.find((candidate) => candidate.id === data.link!.id) ?? data.link;
            return (
              <LinkDetailsPanel
                link={liveLink}
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
      {panelContent?.type === 'deviceConfig' &&
        (() => {
          const data = panelContent.data as { deviceId?: string } | undefined;
          const device = data?.deviceId
            ? devices.find((candidate) => candidate.id === data.deviceId)
            : undefined;
          if (device) {
            return (
              <DeviceConfigPanel
                device={device}
                detailMetrics={runtimeState.devicesById.get(device.id)?.metrics ?? null}
                readOnly={!editMode}
                isVirtual={device.device_type === 'virtual'}
                onDeviceUpdated={(updated) => {
                  setDevices((prev) => prev.map((d) => (d.id === updated.id ? updated : d)));
                  setNodes((prev) =>
                    prev.map((n) =>
                      n.id === updated.id
                        ? {
                            ...n,
                            data: {
                              ...n.data,
                              device: updated,
                              isVirtual: updated.device_type === 'virtual',
                              monitoringState: resolveDeviceMonitoringState(updated),
                              subtype:
                                updated.device_type === 'virtual'
                                  ? (updated.tags?.virtual_subtype ?? 'generic')
                                  : undefined,
                              metrics: sanitizeDeviceMetricsForDisplay(updated, n.data.metrics),
                            },
                          }
                        : n,
                    ),
                  );
                  setPanelContent({ type: 'deviceConfig', data: { deviceId: updated.id } });
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
      {panelContent?.type === 'bulkEdit' &&
        (() => {
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
