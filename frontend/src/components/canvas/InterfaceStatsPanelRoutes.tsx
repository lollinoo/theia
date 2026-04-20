import { useEffect, useState } from 'react';

import { fetchDeviceInterfaces } from '../../api/client';
import type { Device, InterfaceInfo, Link } from '../../types/api';
import { DeviceInterfaceStatsPanel, InterfaceStatsPanel } from '../InterfaceStatsPanel';
import {
  buildDeviceInterfacePanelModel,
  buildLinkInterfacePanelModel,
} from './panelAdapters';
import type { RuntimeState } from './runtimeAdapters';

function useDeviceInterfaces(deviceId: string): {
  interfaces: InterfaceInfo[];
  loading: boolean;
  error: boolean;
} {
  const [state, setState] = useState({
    deviceId,
    interfaces: [] as InterfaceInfo[],
    loading: true,
    error: false,
  });
  const currentState = state.deviceId === deviceId
    ? state
    : { deviceId, interfaces: [] as InterfaceInfo[], loading: true, error: false };

  useEffect(() => {
    let stale = false;
    setState({ deviceId, interfaces: [], loading: true, error: false });
    fetchDeviceInterfaces(deviceId)
      .then((nextInterfaces) => {
        if (!stale) {
          setState({ deviceId, interfaces: nextInterfaces, loading: false, error: false });
        }
      })
      .catch(() => {
        if (!stale) {
          setState({ deviceId, interfaces: [], loading: false, error: true });
        }
      });

    return () => {
      stale = true;
    };
  }, [deviceId]);

  return {
    interfaces: currentState.interfaces,
    loading: currentState.loading,
    error: currentState.error,
  };
}

export function DeviceInterfaceStatsPanelRoute({
  device,
  runtimeState,
}: {
  device: Device;
  runtimeState: RuntimeState;
}) {
  const { interfaces, loading, error } = useDeviceInterfaces(device.id);

  if (error) {
    return <div className="p-4 text-sm text-on-bg-secondary">Unable to load interface details.</div>;
  }

  return (
    <DeviceInterfaceStatsPanel
      model={buildDeviceInterfacePanelModel({
        device,
        runtimeState,
        loadingInterfaces: loading,
        interfaces,
      })}
    />
  );
}

export function LinkInterfaceStatsPanelRoute({
  link,
  sourceDevice,
  targetDevice,
  runtimeState,
}: {
  link: Link;
  sourceDevice: Device;
  targetDevice: Device;
  runtimeState: RuntimeState;
}) {
  const source = useDeviceInterfaces(sourceDevice.id);
  const target = useDeviceInterfaces(targetDevice.id);

  if (source.error || target.error) {
    return <div className="p-4 text-sm text-on-bg-secondary">Unable to load interface details.</div>;
  }

  if (source.loading || target.loading) {
    return <div className="p-4 text-sm text-on-bg-secondary">Loading interface details...</div>;
  }

  return (
    <InterfaceStatsPanel
      model={buildLinkInterfacePanelModel({
        link,
        sourceDevice,
        targetDevice,
        sourceInterfaces: source.interfaces,
        targetInterfaces: target.interfaces,
        runtimeState,
      })}
    />
  );
}
