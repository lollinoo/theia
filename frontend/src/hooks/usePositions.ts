import { useCallback, useEffect, useRef, useState } from 'react';
import {
  recordCanvasDiagnosticEvent,
  updateCanvasDiagnosticsState,
} from '../components/canvas/canvasDiagnostics';
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
  const pendingSaveCountRef = useRef(0);

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
    const startedAt = performance.now();
    recordCanvasDiagnosticEvent({
      level: 'debug',
      source: 'positions',
      event: 'positions.save.started',
      message: 'Canvas position save started',
      metadata: {
        positionCount: nextPositions.length,
      },
    });

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

    pendingSaveCountRef.current = 0;
    updateCanvasDiagnosticsState({
      positions: {
        pendingSaveCount: 0,
        lastSaveAt: new Date().toISOString(),
        lastSaveDurationMs: Number((performance.now() - startedAt).toFixed(3)),
        lastSaveStatus: 'success',
        lastSaveError: undefined,
      },
    });
    recordCanvasDiagnosticEvent({
      level: 'info',
      source: 'positions',
      event: 'positions.save.succeeded',
      message: 'Canvas positions saved',
      metadata: {
        positionCount: nextPositions.length,
      },
    });
  }, []);

  const savePositions = useCallback(
    async (nextPositions: PositionPayload[]) => {
      setPositions(toPositionMap(nextPositions));
      pendingRef.current = nextPositions;
      pendingSaveCountRef.current = 1;
      updateCanvasDiagnosticsState({
        positions: {
          pendingSaveCount: pendingSaveCountRef.current,
          lastSaveStatus: 'pending',
          lastSaveError: undefined,
        },
      });
      recordCanvasDiagnosticEvent({
        level: 'debug',
        source: 'positions',
        event: 'positions.save.queued',
        message: 'Canvas position save queued',
        metadata: {
          positionCount: nextPositions.length,
        },
      });

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
          pendingSaveCountRef.current = 0;
          const saveError = error instanceof Error ? error.message : 'Failed to save positions';
          updateCanvasDiagnosticsState({
            positions: {
              pendingSaveCount: 0,
              lastSaveAt: new Date().toISOString(),
              lastSaveStatus: 'error',
              lastSaveError: saveError,
            },
          });
          recordCanvasDiagnosticEvent({
            level: 'error',
            source: 'positions',
            event: 'positions.save.failed',
            message: 'Canvas position save failed',
            metadata: {
              error: saveError,
              positionCount: payload.length,
            },
          });
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
