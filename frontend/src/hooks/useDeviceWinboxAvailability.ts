import { useCallback, useRef, useState } from 'react';
import { fetchDeviceCredentialProfiles } from '../api/client';

export function useDeviceWinboxAvailability(): {
  deviceWinboxState: Record<string, boolean>;
  refreshDeviceWinboxAvailability: (deviceId: string) => void;
  setDeviceWinboxAvailability: (deviceId: string, hasWinboxProfile: boolean) => void;
} {
  const [deviceWinboxState, setDeviceWinboxState] = useState<Record<string, boolean>>({});
  const latestRequestRef = useRef(0);

  const refreshDeviceWinboxAvailability = useCallback((deviceId: string) => {
    const requestId = latestRequestRef.current + 1;
    latestRequestRef.current = requestId;

    void (async () => {
      try {
        const profiles = await fetchDeviceCredentialProfiles(deviceId);
        if (latestRequestRef.current !== requestId) return;
        setDeviceWinboxState((prev) => ({
          ...prev,
          [deviceId]: profiles.some((profile) => profile.is_winbox),
        }));
      } catch {
        if (latestRequestRef.current !== requestId) return;
        setDeviceWinboxState((prev) => ({ ...prev, [deviceId]: false }));
      }
    })();
  }, []);

  const setDeviceWinboxAvailability = useCallback((deviceId: string, hasWinboxProfile: boolean) => {
    setDeviceWinboxState((prev) => ({ ...prev, [deviceId]: hasWinboxProfile }));
  }, []);

  return {
    deviceWinboxState,
    refreshDeviceWinboxAvailability,
    setDeviceWinboxAvailability,
  };
}
