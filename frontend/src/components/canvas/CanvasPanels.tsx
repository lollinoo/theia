import type { ReactFlowInstance } from '@xyflow/react';

import { fetchDevices } from '../../api/client';
import type { Device, Link } from '../../types/api';
import type { PrometheusStatusPayload, SnapshotPayload } from '../../types/metrics';
import type { DeviceNodeData } from '../DeviceCard';
import type { LinkEdgeData } from '../LinkEdge';
import { InterfaceStatsPanel, DeviceInterfaceStatsPanel } from '../InterfaceStatsPanel';
import { AlertsPanel } from '../AlertsPanel';
import { SettingsPanel } from '../SettingsPanel';
import { AddDevicePanel } from '../AddDevicePanel';
import { DeviceConfigPanel } from '../DeviceConfigPanel';
import { LinkCreatePanel } from '../LinkCreatePanel';
import { LinkDetailsPanel } from '../LinkDetailsPanel';
import { viewportSize } from './canvasHelpers';

interface CanvasPanelsProps {
  panelContent: { type: string; data?: unknown } | null;
  setPanelContent: (content: { type: string; data?: unknown } | null) => void;
  snapshot: SnapshotPayload | null;
  devices: Device[];
  topologyLinks: Link[];
  loadTopology: (silent?: boolean, pos?: { x: number; y: number }) => Promise<void>;
  setDevices: React.Dispatch<React.SetStateAction<Device[]>>;
  setNodes: React.Dispatch<React.SetStateAction<import('@xyflow/react').Node<DeviceNodeData>[]>>;
  reactFlow: ReactFlowInstance<DeviceNodeData, LinkEdgeData>;
  prometheusStatus: PrometheusStatusPayload | null;
}

export function CanvasPanels({
  panelContent,
  setPanelContent,
  snapshot,
  devices,
  topologyLinks,
  loadTopology,
  setDevices,
  setNodes,
  reactFlow,
  prometheusStatus,
}: CanvasPanelsProps) {
  return (
    <>
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
        return <div className="text-on-bg-secondary text-sm">No data available.</div>;
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
          initialSourceDeviceId={(panelContent.data as { initialSourceDeviceId?: string })?.initialSourceDeviceId}
          initialTargetDeviceId={(panelContent.data as { initialTargetDeviceId?: string })?.initialTargetDeviceId}
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
    </>
  );
}
