/**
 * Coordinates winbox flow state and side effects for consuming components.
 * Owns cleanup-sensitive lifecycle work so callers receive stable state and actions.
 */
import { useCallback, useEffect, useRef, useState } from 'react';

import { createBridgeLaunchRequest, fetchUserSettings } from '../api/client';
import { fetchBridgeWithTimeout, getBridgeLaunchErrorMessage } from '../utils/bridgeRequests';
import { useBridgeHealth } from './useBridgeHealth';
import { useDeviceWinboxAvailability } from './useDeviceWinboxAvailability';

/** Coordinates winbox flow behavior for the React hook lifecycle. */
export function useWinboxFlow(): {
  bridgeChecked: boolean;
  bridgeRunning: boolean;
  deviceWinboxState: Record<string, boolean>;
  winboxError: string | null;
  openDeviceMenu: (deviceId: string) => Promise<void>;
  launchWinbox: (deviceId: string) => Promise<void>;
  clearWinboxError: () => void;
  setDeviceWinboxAvailability: (deviceId: string, hasWinboxProfile: boolean) => void;
} {
  const [bridgePort, setBridgePort] = useState('1337');
  const [winboxError, setWinboxError] = useState<string | null>(null);
  const bridgePortRef = useRef('1337');
  const settingsLoadedRef = useRef(false);
  const settingsLoadPromiseRef = useRef<Promise<void> | null>(null);
  const { bridgeRunning, bridgeChecked, bridgeError, checkBridgeHealth } =
    useBridgeHealth(bridgePort);
  const { deviceWinboxState, refreshDeviceWinboxAvailability, setDeviceWinboxAvailability } =
    useDeviceWinboxAvailability();

  useEffect(() => {
    settingsLoadPromiseRef.current = fetchUserSettings()
      .then((settings) => {
        const nextBridgePort = String(settings.preferences.bridge_port ?? 1337);
        bridgePortRef.current = nextBridgePort;
        setBridgePort(nextBridgePort);
      })
      .catch(() => {})
      .finally(() => {
        settingsLoadedRef.current = true;
      });
  }, []);

  useEffect(() => {
    if (!winboxError) return;

    const timeoutId = window.setTimeout(() => {
      setWinboxError(null);
    }, 5000);

    return () => window.clearTimeout(timeoutId);
  }, [winboxError]);

  useEffect(() => {
    if (bridgeError) {
      setWinboxError(bridgeError);
    }
  }, [bridgeError]);

  const openDeviceMenu = useCallback(
    async (deviceId: string) => {
      refreshDeviceWinboxAvailability(deviceId);

      if (!settingsLoadedRef.current) {
        await settingsLoadPromiseRef.current;
      }

      checkBridgeHealth(bridgePortRef.current);
    },
    [checkBridgeHealth, refreshDeviceWinboxAvailability],
  );

  const launchWinbox = useCallback(async (deviceId: string) => {
    if (!settingsLoadedRef.current) {
      await settingsLoadPromiseRef.current;
    }

    let launchToken: string;
    try {
      const launch = await createBridgeLaunchRequest(deviceId);
      launchToken = launch.launch_token;
    } catch (error) {
      setWinboxError(error instanceof Error ? error.message : 'Failed to launch WinBox');
      return;
    }

    try {
      const response = await fetchBridgeWithTimeout(
        `http://localhost:${bridgePortRef.current}/launch`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ launch_token: launchToken }),
        },
      );

      if (!response.ok) {
        const data = (await response.json().catch(() => ({}))) as { error?: string };
        setWinboxError(data.error ?? `Bridge error (${response.status})`);
      }
    } catch (error) {
      setWinboxError(getBridgeLaunchErrorMessage(error));
    }
  }, []);

  const clearWinboxError = useCallback(() => {
    setWinboxError(null);
  }, []);

  return {
    bridgeChecked,
    bridgeRunning,
    deviceWinboxState,
    winboxError,
    openDeviceMenu,
    launchWinbox,
    clearWinboxError,
    setDeviceWinboxAvailability,
  };
}
