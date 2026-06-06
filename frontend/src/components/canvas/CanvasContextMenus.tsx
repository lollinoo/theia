import type { Dispatch, SetStateAction } from 'react';

import type { Device } from '../../types/api';
import { ContextMenu } from '../ContextMenu';
import type { LinkEdgeType } from '../LinkEdge';
import { buildDeviceContextMenuItems } from './canvasHelpers';
import type { CanvasDeviceMenu, CanvasEdgeMenu, CanvasPanelContent } from './useCanvasMenus';

interface CanvasContextMenusProps {
  deviceMenu: CanvasDeviceMenu | null;
  edgeMenu: CanvasEdgeMenu | null;
  devices: Device[];
  edges: LinkEdgeType[];
  bridgeChecked: boolean;
  bridgeRunning: boolean;
  deviceWinboxState: Record<string, boolean>;
  launchWinbox: (deviceId: string) => Promise<void>;
  grafanaUrl: (device?: Device) => string;
  setDeviceMenu: Dispatch<SetStateAction<CanvasDeviceMenu | null>>;
  setEdgeMenu: Dispatch<SetStateAction<CanvasEdgeMenu | null>>;
  setPanelContent: Dispatch<SetStateAction<CanvasPanelContent | null>>;
}

export function CanvasContextMenus({
  deviceMenu,
  edgeMenu,
  devices,
  edges,
  bridgeChecked,
  bridgeRunning,
  deviceWinboxState,
  launchWinbox,
  grafanaUrl,
  setDeviceMenu,
  setEdgeMenu,
  setPanelContent,
}: CanvasContextMenusProps) {
  return (
    <>
      {deviceMenu &&
        (() => {
          const d = devices.find((dev) => dev.id === deviceMenu.deviceId);
          const gUrl = grafanaUrl(d);
          const isVirtual = d?.device_type === 'virtual';
          const hasWinboxProfile = deviceWinboxState[deviceMenu.deviceId];
          const winboxDisabled = hasWinboxProfile === false;
          const winboxTitle =
            hasWinboxProfile === false
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
              if (d) void launchWinbox(d.id);
              setDeviceMenu(null);
            },
            onOpenGrafana: () => {
              if (gUrl) window.open(gUrl, '_blank');
              setDeviceMenu(null);
            },
            onConfigure: () => {
              if (d) {
                setPanelContent({
                  type: 'deviceConfig',
                  data: { deviceId: d.id },
                });
              }
              setDeviceMenu(null);
            },
          });
          return (
            <ContextMenu
              position={{ x: deviceMenu.x, y: deviceMenu.y }}
              onClose={() => setDeviceMenu(null)}
              items={items}
            />
          );
        })()}

      {edgeMenu &&
        (() => {
          const me = edges.find((e) => e.id === edgeMenu.edgeID);
          const ml = me?.data?.link;
          const dMap = new Map(devices.map((d) => [d.id, d]));
          const sd = ml ? dMap.get(ml.source_device_id) : undefined;
          const gUrl = grafanaUrl(sd);
          return (
            <ContextMenu
              position={{ x: edgeMenu.x, y: edgeMenu.y }}
              onClose={() => setEdgeMenu(null)}
              items={[
                {
                  label: 'Per-Interface Stats',
                  icon: 'devices',
                  onClick: () => {
                    if (ml) {
                      setPanelContent({
                        type: 'interfaceStats',
                        data: { linkId: ml.id },
                      });
                    }
                    setEdgeMenu(null);
                  },
                },
                {
                  label: gUrl ? 'Open in Grafana' : 'Open in Grafana (not configured)',
                  icon: 'hub',
                  onClick: () => {
                    if (gUrl) window.open(gUrl, '_blank');
                    setEdgeMenu(null);
                  },
                },
                {
                  label: 'View Details',
                  icon: 'search',
                  onClick: () => {
                    const el = edges.find((e) => e.id === edgeMenu.edgeID)?.data?.link;
                    if (el) {
                      setPanelContent({
                        type: 'link-details',
                        data: { link: el },
                      });
                    }
                    setEdgeMenu(null);
                  },
                },
              ]}
            />
          );
        })()}
    </>
  );
}
