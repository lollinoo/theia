import { useCallback, useEffect, useRef, useState } from 'react';

const canvasInteractionIdleDelayMs = 140;

interface UseCanvasInteractionStateParams {
  onInteractionActiveChange?: (active: boolean) => void;
}

export function useCanvasInteractionState({
  onInteractionActiveChange,
}: UseCanvasInteractionStateParams): {
  canvasInteractionActive: boolean;
  beginCanvasInteraction: () => void;
  endCanvasInteraction: () => void;
} {
  const [canvasInteractionActive, setCanvasInteractionActive] = useState(false);
  const interactionIdleTimerRef = useRef<number | null>(null);

  useEffect(() => {
    onInteractionActiveChange?.(canvasInteractionActive);
  }, [canvasInteractionActive, onInteractionActiveChange]);

  useEffect(() => () => onInteractionActiveChange?.(false), [onInteractionActiveChange]);

  useEffect(
    () => () => {
      if (interactionIdleTimerRef.current !== null) {
        window.clearTimeout(interactionIdleTimerRef.current);
      }
    },
    [],
  );

  const beginCanvasInteraction = useCallback(() => {
    if (interactionIdleTimerRef.current !== null) {
      window.clearTimeout(interactionIdleTimerRef.current);
      interactionIdleTimerRef.current = null;
    }
    setCanvasInteractionActive(true);
  }, []);

  const endCanvasInteraction = useCallback(() => {
    if (interactionIdleTimerRef.current !== null) {
      window.clearTimeout(interactionIdleTimerRef.current);
    }
    interactionIdleTimerRef.current = window.setTimeout(() => {
      interactionIdleTimerRef.current = null;
      setCanvasInteractionActive(false);
    }, canvasInteractionIdleDelayMs);
  }, []);

  return { canvasInteractionActive, beginCanvasInteraction, endCanvasInteraction };
}
