import type { Device } from '../../types/api';
import type { SnapshotPayload } from '../../types/metrics';
import { alertStatusForDevice } from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';

export function buildTopologyNodes(
  devices: Device[],
  savedPositions: Map<string, { x: number; y: number; pinned?: boolean }>,
  computedPositions: Map<string, { x: number; y: number }>,
  defaultPosition: { x: number; y: number } | undefined,
  editMode: boolean,
  openDeviceMenu: (event: React.MouseEvent, deviceId: string) => void,
  pendingSnapshot: SnapshotPayload | null,
): DeviceNode[] {
  return devices.map((device) => {
    const saved = savedPositions.get(device.id);
    const position = saved ?? defaultPosition ?? computedPositions.get(device.id) ?? { x: 0, y: 0 };

    // Merge snapshot status/hostname into device if available
    let deviceData = device;
    if (pendingSnapshot) {
      const snapStatus = pendingSnapshot.device_statuses[device.id];
      const snapHostname = pendingSnapshot.device_hostnames[device.id];
      if (snapStatus || snapHostname) {
        deviceData = {
          ...device,
          ...(snapStatus ? { status: snapStatus as Device['status'] } : {}),
          ...(snapHostname ? { sys_name: snapHostname } : {}),
        };
      }
    }

    return {
      id: device.id,
      type: 'device',
      position: {
        x: position.x,
        y: position.y,
      },
      data: {
        device: deviceData,
        pinned: saved?.pinned ?? false,
        highlighted: false,
        editMode,
        onContextMenu: openDeviceMenu,
        metrics: pendingSnapshot?.device_metrics[device.id] ?? null,
        alertStatus: pendingSnapshot
          ? alertStatusForDevice(device.id, pendingSnapshot.alerts)
          : undefined,
      },
    };
  });
}
