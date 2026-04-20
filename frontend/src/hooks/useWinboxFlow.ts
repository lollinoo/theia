import { useCallback, useEffect, useRef, useState } from 'react';

import { fetchBridgeToken, fetchSettings } from '../api/client';
import { fetchBridgeWithTimeout, getBridgeLaunchErrorMessage } from '../utils/bridgeRequests';
import { useBridgeHealth } from './useBridgeHealth';
import { useDeviceWinboxAvailability } from './useDeviceWinboxAvailability';

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
  const bridgeSecretRef = useRef('');
  const settingsLoadedRef = useRef(false);
  const settingsLoadPromiseRef = useRef<Promise<void> | null>(null);
  const { bridgeRunning, bridgeChecked, bridgeError, checkBridgeHealth } = useBridgeHealth(bridgePort);
  const {
    deviceWinboxState,
    refreshDeviceWinboxAvailability,
    setDeviceWinboxAvailability,
  } = useDeviceWinboxAvailability();

  useEffect(() => {
    settingsLoadPromiseRef.current = fetchSettings().then((settings) => {
      const nextBridgeSecret = settings.bridge_secret ?? '';
      const nextBridgePort = settings.bridge_port ?? '1337';
      bridgeSecretRef.current = nextBridgeSecret;
      bridgePortRef.current = nextBridgePort;
      setBridgePort(nextBridgePort);
    }).catch(() => {}).finally(() => {
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

  const openDeviceMenu = useCallback(async (deviceId: string) => {
    refreshDeviceWinboxAvailability(deviceId);

    if (!settingsLoadedRef.current) {
      await settingsLoadPromiseRef.current;
    }

    checkBridgeHealth(bridgePortRef.current);
  }, [checkBridgeHealth, refreshDeviceWinboxAvailability]);

  const launchWinbox = useCallback(async (deviceId: string) => {
    if (!settingsLoadedRef.current) {
      await settingsLoadPromiseRef.current;
    }

    if (!bridgeSecretRef.current) {
      setWinboxError('Bridge secret not configured');
      return;
    }

    let token: string;
    try {
      token = await fetchBridgeToken(deviceId, bridgeSecretRef.current);
    } catch (error) {
      setWinboxError(error instanceof Error ? error.message : 'Failed to launch WinBox');
      return;
    }

    try {
      const response = await fetchBridgeWithTimeout(`http://localhost:${bridgePortRef.current}/launch`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token }),
      });

      if (!response.ok) {
        const data = await response.json().catch(() => ({})) as { error?: string };
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
