/**
 * Defines interface stats panel routes behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import { useEffect, useState } from 'react';

import { fetchDeviceInterfaces } from '../../api/client';
import type { Device, InterfaceInfo, Link } from '../../types/api';
import { DeviceInterfaceStatsPanel, InterfaceStatsPanel } from '../InterfaceStatsPanel';
import { buildDeviceInterfacePanelModel, buildLinkInterfacePanelModel } from './panelAdapters';
import type { RuntimeState } from './runtimeAdapters';

function useDeviceInterfaces(
  deviceId: string,
  enabled = true,
): {
  interfaces: InterfaceInfo[];
  loading: boolean;
  error: boolean;
} {
  const [state, setState] = useState({
    deviceId,
    enabled,
    interfaces: [] as InterfaceInfo[],
    loading: enabled,
    error: false,
  });
  const currentState =
    state.deviceId === deviceId && state.enabled === enabled
      ? state
      : { deviceId, enabled, interfaces: [] as InterfaceInfo[], loading: enabled, error: false };

  useEffect(() => {
    let stale = false;
    if (!enabled) {
      setState({ deviceId, enabled, interfaces: [], loading: false, error: false });
      return () => {
        stale = true;
      };
    }

    setState({ deviceId, enabled, interfaces: [], loading: true, error: false });
    fetchDeviceInterfaces(deviceId)
      .then((nextInterfaces) => {
        if (!stale) {
          setState({ deviceId, enabled, interfaces: nextInterfaces, loading: false, error: false });
        }
      })
      .catch(() => {
        if (!stale) {
          setState({ deviceId, enabled, interfaces: [], loading: false, error: true });
        }
      });

    return () => {
      stale = true;
    };
  }, [deviceId, enabled]);

  return {
    interfaces: currentState.interfaces,
    loading: currentState.loading,
    error: currentState.error,
  };
}

/** Renders the DeviceInterfaceStatsPanelRoute component within the topology canvas. */
export function DeviceInterfaceStatsPanelRoute({
  device,
  runtimeState,
  links = [],
}: {
  device: Device;
  runtimeState: RuntimeState;
  links?: Link[];
}) {
  const { interfaces, loading, error } = useDeviceInterfaces(
    device.id,
    device.polling_enabled !== false,
  );

  if (error) {
    return (
      <div className="p-4 text-sm text-on-bg-secondary">Unable to load interface details.</div>
    );
  }

  return (
    <DeviceInterfaceStatsPanel
      model={buildDeviceInterfacePanelModel({
        device,
        runtimeState,
        loadingInterfaces: loading,
        interfaces,
        links,
      })}
    />
  );
}

/** Renders the LinkInterfaceStatsPanelRoute component within the topology canvas. */
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
    return (
      <div className="p-4 text-sm text-on-bg-secondary">Unable to load interface details.</div>
    );
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
