/**
 * Defines canvas panels behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import type { ReactFlowInstance } from '@xyflow/react';

import {
  checkDeviceAddressReachability,
  type DeviceAddressPayload,
  fetchDevices,
  updateDevice,
} from '../../api/client';
import type { Area, Device, Link } from '../../types/api';
import type { AlertDTO } from '../../types/metrics';
import { AddDevicePanel } from '../AddDevicePanel';
import { AlertsPanel } from '../AlertsPanel';
import { BulkEditPanel } from '../BulkEditPanel';
import type { DeviceNode } from '../DeviceCard';
import { DeviceConfigPanel } from '../DeviceConfigPanel';
import { DeviceDetailsPanel } from '../DeviceDetailsPanel';
import {
  resolveDeviceMonitoringState,
  sanitizeDeviceMetricsForDisplay,
} from '../deviceVisualState';
import { LinkCreatePanel } from '../LinkCreatePanel';
import { LinkDetailsPanel } from '../LinkDetailsPanel';
import type { LinkEdgeType } from '../LinkEdge';
import { viewportSize } from './canvasHelpers';
import {
  DeviceInterfaceStatsPanelRoute,
  LinkInterfaceStatsPanelRoute,
} from './InterfaceStatsPanelRoutes';
import { buildAlertsPanelModel } from './panelAdapters';
import type { RuntimeState } from './runtimeAdapters';

const emptyAlerts: AlertDTO[] = [];

function sameAreaIds(first: string[] = [], second: string[] = []): boolean {
  if (first.length !== second.length) return false;
  const sortedFirst = [...first].sort();
  const sortedSecond = [...second].sort();
  return sortedFirst.every((value, index) => value === sortedSecond[index]);
}

function buildPromotedAddressPayloads(device: Device, addressId: string): DeviceAddressPayload[] {
  return device.addresses.map((address) => {
    const promoted = address.id === addressId;
    return {
      address: address.address,
      label: address.label,
      role: promoted ? 'primary' : address.is_primary ? 'other' : address.role,
      is_primary: promoted,
      priority: promoted ? 0 : address.priority,
      probe_ports: address.probe_ports,
    };
  });
}

interface CanvasPanelsProps {
  panelContent: { type: string; data?: unknown } | null;
  setPanelContent: (content: { type: string; data?: unknown } | null) => void;
  alerts?: AlertDTO[];
  devices: Device[];
  topologyLinks: Link[];
  topologyAreas?: Area[];
  loadTopology: (silent?: boolean, pos?: { x: number; y: number }) => Promise<void>;
  setDevices: React.Dispatch<React.SetStateAction<Device[]>>;
  setNodes: React.Dispatch<React.SetStateAction<DeviceNode[]>>;
  reactFlow: ReactFlowInstance<DeviceNode, LinkEdgeType>;
  runtimeState: RuntimeState;
  mapId?: string | null;
  mapName?: string;
  editMode?: boolean;
  onRemoveDeviceFromMap?: (deviceId: string) => void | Promise<void>;
  onSettingsChange?: () => void;
  onWinBoxAvailabilityChange?: (deviceId: string, hasWinboxProfile: boolean) => void;
}

/** Renders the CanvasPanels component within the topology canvas. */
export function CanvasPanels({
  panelContent,
  setPanelContent,
  alerts = emptyAlerts,
  devices,
  topologyLinks,
  topologyAreas = [],
  loadTopology,
  setDevices,
  setNodes,
  reactFlow,
  runtimeState,
  mapId = null,
  mapName = 'Default',
  editMode = false,
  onRemoveDeviceFromMap,
  onSettingsChange,
  onWinBoxAvailabilityChange,
}: CanvasPanelsProps) {
  return (
    <>
      {panelContent?.type === 'interfaceStats' &&
        (() => {
          const data = panelContent.data as { linkId?: string } | undefined;
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
          return <div className="text-on-bg-secondary text-sm">No data available.</div>;
        })()}
      {panelContent?.type === 'alerts' && (
        <AlertsPanel model={buildAlertsPanelModel({ alerts, runtimeState })} />
      )}
      {panelContent?.type === 'deviceDetails' &&
        (() => {
          const data = panelContent.data as { deviceId?: string } | undefined;
          const device = data?.deviceId
            ? devices.find((candidate) => candidate.id === data.deviceId)
            : undefined;
          if (!device) return null;
          return (
            <DeviceDetailsPanel
              device={device}
              detailMetrics={runtimeState.devicesById.get(device.id)?.metrics ?? null}
              onCheckAddressReachability={checkDeviceAddressReachability}
              onPromoteAddress={async (addressId) => {
                const address = device.addresses.find((candidate) => candidate.id === addressId);
                if (!address) return;
                await updateDevice(device.id, {
                  ip: address.address,
                  addresses: buildPromotedAddressPayloads(device, addressId),
                });
                await loadTopology(true);
              }}
              interfaceStats={
                device.device_type !== 'virtual' ? (
                  <DeviceInterfaceStatsPanelRoute
                    device={device}
                    runtimeState={runtimeState}
                    links={topologyLinks}
                  />
                ) : undefined
              }
            />
          );
        })()}
      {panelContent?.type === 'addDevice' && (
        <AddDevicePanel
          areas={topologyAreas}
          devices={devices}
          mapContext={mapId ? { mapId } : undefined}
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
                readOnly={!editMode}
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
                readOnly={!editMode}
                isVirtual={device.device_type === 'virtual'}
                areas={topologyAreas}
                mapContext={mapId && onRemoveDeviceFromMap ? { mapId, mapName } : undefined}
                onRemoveFromMap={onRemoveDeviceFromMap}
                onDeviceUpdated={(updated) => {
                  const ipChanged = device.ip !== updated.ip;
                  const mapScopedAreaChanged =
                    Boolean(mapId && onRemoveDeviceFromMap) &&
                    !sameAreaIds(device.area_ids ?? [], updated.area_ids ?? []);
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
                              runtime: {
                                ...n.data.runtime,
                                status: ipChanged ? updated.status : n.data.runtime.status,
                                monitoringState: resolveDeviceMonitoringState(updated),
                                metrics: ipChanged
                                  ? null
                                  : sanitizeDeviceMetricsForDisplay(
                                      updated,
                                      n.data.runtime.metrics,
                                      resolveDeviceMonitoringState(updated),
                                    ),
                              },
                            },
                          }
                        : n,
                    ),
                  );
                  setPanelContent({
                    type: 'deviceConfig',
                    data: { deviceId: updated.id },
                  });
                  if (mapScopedAreaChanged) {
                    void loadTopology(true);
                  }
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
                areas={topologyAreas}
                mapContext={mapId ? { mapId, mapName } : undefined}
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
                  setPanelContent({
                    type: 'bulkEdit',
                    data: { deviceIds: data.deviceIds },
                  });
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
