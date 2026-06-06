import type { FitViewOptions } from '@xyflow/react';
import { useCallback, useState } from 'react';

import { recordCanvasDiagnosticEvent } from './canvasDiagnostics';

type FitViewPadding = NonNullable<FitViewOptions['padding']>;

/** Inputs for controlled/uncontrolled canvas chrome visibility and related fit-view padding. */
interface UseCanvasChromeParams {
  chromeHidden?: boolean;
  onChromeHiddenChange?: (hidden: boolean) => void;
  mapId: string | null;
  normalPadding: FitViewPadding;
  hiddenPadding: FitViewPadding;
  fitTopologyView: (padding: FitViewPadding) => void;
  closeCanvasOverlays: () => void;
}

/**
 * Coordinates canvas chrome visibility, overlay cleanup, and post-layout fit-view updates.
 * The hook supports a controlled App-owned preference while preserving an internal fallback for tests.
 */
export function useCanvasChrome({
  chromeHidden,
  onChromeHiddenChange,
  mapId,
  normalPadding,
  hiddenPadding,
  fitTopologyView,
  closeCanvasOverlays,
}: UseCanvasChromeParams): {
  effectiveChromeHidden: boolean;
  currentTopologyFitViewPadding: FitViewPadding;
  handleToggleChrome: () => void;
} {
  const [internalChromeHidden, setInternalChromeHidden] = useState(false);
  const effectiveChromeHidden = chromeHidden ?? internalChromeHidden;
  const currentTopologyFitViewPadding = effectiveChromeHidden ? hiddenPadding : normalPadding;

  const handleToggleChrome = useCallback(() => {
    const nextHidden = !effectiveChromeHidden;
    setInternalChromeHidden(nextHidden);
    onChromeHiddenChange?.(nextHidden);

    if (nextHidden) {
      closeCanvasOverlays();
    }

    const fitAfterLayout = () => {
      fitTopologyView(nextHidden ? hiddenPadding : normalPadding);
      recordCanvasDiagnosticEvent({
        level: 'debug',
        source: 'reactflow',
        event: 'reactflow.fit_view',
        message: 'React Flow fitView requested after canvas chrome toggle',
        metadata: {
          chromeHidden: nextHidden,
          mapId: mapId ?? 'default',
        },
      });
    };

    if (typeof window.requestAnimationFrame === 'function') {
      window.requestAnimationFrame(fitAfterLayout);
    } else {
      fitAfterLayout();
    }
  }, [
    closeCanvasOverlays,
    effectiveChromeHidden,
    fitTopologyView,
    hiddenPadding,
    mapId,
    normalPadding,
    onChromeHiddenChange,
  ]);

  return { effectiveChromeHidden, currentTopologyFitViewPadding, handleToggleChrome };
}
