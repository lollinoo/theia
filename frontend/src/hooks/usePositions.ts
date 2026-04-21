import { useCallback, useEffect, useRef, useState } from 'react';
import { type DevicePosition, parsePositionsResponse } from '../types/api';

export interface PositionState {
  x: number;
  y: number;
  pinned: boolean;
}

export interface PositionPayload {
  device_id: string;
  x: number;
  y: number;
  pinned: boolean;
}

function toPositionMap(
  positions: DevicePosition[] | PositionPayload[],
): Map<string, PositionState> {
  return new Map(
    positions.map((position) => [
      position.device_id,
      {
        x: position.x,
        y: position.y,
        pinned: position.pinned,
      },
    ]),
  );
}

export function usePositions() {
  const [positions, setPositions] = useState<Map<string, PositionState>>(new Map());
  const [loading, setLoading] = useState(false);
  const timerRef = useRef<number | null>(null);
  const pendingRef = useRef<PositionPayload[] | null>(null);

  const fetchPositions = useCallback(async () => {
    setLoading(true);
    try {
      const response = await fetch('/api/v1/positions', {
        headers: {
          Accept: 'application/json',
        },
      });
      const payload = await response.json().catch(() => null);

      if (!response.ok) {
        const message =
          typeof payload === 'object' &&
          payload !== null &&
          'error' in payload &&
          typeof payload.error === 'string'
            ? payload.error
            : response.statusText;
        throw new Error(`Failed to fetch positions: ${response.status} ${message}`);
      }

      const parsed = parsePositionsResponse(payload);
      const nextPositions = toPositionMap(parsed);
      setPositions(nextPositions);
      return nextPositions;
    } finally {
      setLoading(false);
    }
  }, []);

  const commitPositions = useCallback(async (nextPositions: PositionPayload[]) => {
    const response = await fetch('/api/v1/positions', {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'application/json',
      },
      body: JSON.stringify({ positions: nextPositions }),
    });

    if (!response.ok) {
      const payload = await response.json().catch(() => null);
      const message =
        typeof payload === 'object' &&
        payload !== null &&
        'error' in payload &&
        typeof payload.error === 'string'
          ? payload.error
          : response.statusText;
      throw new Error(`Failed to save positions: ${response.status} ${message}`);
    }
  }, []);

  const savePositions = useCallback(
    async (nextPositions: PositionPayload[]) => {
      setPositions(toPositionMap(nextPositions));
      pendingRef.current = nextPositions;

      if (timerRef.current !== null) {
        window.clearTimeout(timerRef.current);
      }

      timerRef.current = window.setTimeout(() => {
        const payload = pendingRef.current;
        pendingRef.current = null;

        if (!payload) {
          return;
        }

        void commitPositions(payload).catch((error) => {
          console.error(error);
        });
      }, 1000);
    },
    [commitPositions],
  );

  useEffect(() => {
    return () => {
      if (timerRef.current !== null) {
        window.clearTimeout(timerRef.current);
      }
    };
  }, []);

  return {
    positions,
    loading,
    fetchPositions,
    savePositions,
  };
}
