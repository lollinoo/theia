import { useCallback, useRef, useState } from 'react';
import { fetchDeviceCredentialProfiles } from '../api/client';

export function useDeviceWinboxAvailability(): {
  deviceWinboxState: Record<string, boolean>;
  refreshDeviceWinboxAvailability: (deviceId: string) => void;
  setDeviceWinboxAvailability: (deviceId: string, hasWinboxProfile: boolean) => void;
} {
  const [deviceWinboxState, setDeviceWinboxState] = useState<Record<string, boolean>>({});
  const latestRequestRef = useRef<Record<string, number>>({});

  const refreshDeviceWinboxAvailability = useCallback((deviceId: string) => {
    const requestId = (latestRequestRef.current[deviceId] ?? 0) + 1;
    latestRequestRef.current[deviceId] = requestId;

    void (async () => {
      try {
        const profiles = await fetchDeviceCredentialProfiles(deviceId);
        if (latestRequestRef.current[deviceId] !== requestId) return;
        setDeviceWinboxState((prev) => ({
          ...prev,
          [deviceId]: profiles.some((profile) => profile.is_winbox),
        }));
      } catch {
        if (latestRequestRef.current[deviceId] !== requestId) return;
        setDeviceWinboxState((prev) => ({ ...prev, [deviceId]: false }));
      }
    })();
  }, []);

  const setDeviceWinboxAvailability = useCallback((deviceId: string, hasWinboxProfile: boolean) => {
    latestRequestRef.current[deviceId] = (latestRequestRef.current[deviceId] ?? 0) + 1;
    setDeviceWinboxState((prev) => ({ ...prev, [deviceId]: hasWinboxProfile }));
  }, []);

  return {
    deviceWinboxState,
    refreshDeviceWinboxAvailability,
    setDeviceWinboxAvailability,
  };
}
