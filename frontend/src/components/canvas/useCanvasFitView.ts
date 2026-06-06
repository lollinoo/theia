/**
 * Coordinates canvas fit view state for the topology canvas.
 * Keeps canvas lifecycle, projected graph state, and cleanup behavior explicit for callers.
 */
import type { FitViewOptions, ReactFlowInstance } from '@xyflow/react';
import { useEffect, useRef } from 'react';

import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import { recordCanvasDiagnosticEvent } from './canvasDiagnostics';

/** Parameters that decide when React Flow can safely fit the currently rendered topology. */
interface UseCanvasFitViewParams {
  visible: boolean;
  flowViewportReady: boolean;
  nodesInitialized: boolean;
  displayNodeCount: number;
  renderedMapKey: string | null;
  selectedTopologyMapKey: string;
  effectiveAreaId: string | null;
  fitViewRevision?: number;
  fitViewPadding: NonNullable<FitViewOptions['padding']>;
  initialHiddenChromePadding: NonNullable<FitViewOptions['padding']>;
  effectiveChromeHidden: boolean;
  mapId: string | null;
  reactFlow: ReactFlowInstance<DeviceNode, LinkEdgeType>;
}

/**
 * Coordinates fit-view requests for area changes, explicit canvas activation, and hidden-chrome restoration.
 * Requests wait for viewport/node readiness so React Flow is not asked to fit stale or unmounted graph state.
 */
export function useCanvasFitView({
  visible,
  flowViewportReady,
  nodesInitialized,
  displayNodeCount,
  renderedMapKey,
  selectedTopologyMapKey,
  effectiveAreaId,
  fitViewRevision,
  fitViewPadding,
  initialHiddenChromePadding,
  effectiveChromeHidden,
  mapId,
  reactFlow,
}: UseCanvasFitViewParams) {
  const previousAreaRef = useRef<string | null>(null);
  const previousFitViewRevisionRef = useRef(fitViewRevision);
  const shouldApplyInitialChromeHiddenFitRef = useRef(effectiveChromeHidden);
  const initialChromeHiddenFitAppliedRef = useRef(false);
  const canFitVisibleTopology =
    visible &&
    flowViewportReady &&
    nodesInitialized &&
    displayNodeCount > 0 &&
    renderedMapKey === selectedTopologyMapKey;

  useEffect(() => {
    if (canFitVisibleTopology && previousAreaRef.current !== effectiveAreaId) {
      let canceled = false;
      const frameId = window.requestAnimationFrame(() => {
        if (canceled) {
          return;
        }
        previousAreaRef.current = effectiveAreaId;
        reactFlow.fitView({ padding: fitViewPadding, duration: 280 });
        recordCanvasDiagnosticEvent({
          level: 'debug',
          source: 'reactflow',
          event: 'reactflow.fit_view',
          message: 'React Flow fitView requested after area change',
          metadata: {
            selectedAreaId: effectiveAreaId,
            displayedNodeCount: displayNodeCount,
          },
        });
      });
      return () => {
        canceled = true;
        window.cancelAnimationFrame(frameId);
      };
    }
    return undefined;
  }, [canFitVisibleTopology, fitViewPadding, effectiveAreaId, displayNodeCount, reactFlow]);

  useEffect(() => {
    if (fitViewRevision === undefined) {
      return;
    }
    if (previousFitViewRevisionRef.current === fitViewRevision) {
      return;
    }
    if (!canFitVisibleTopology) {
      return;
    }

    let canceled = false;
    const frameId = window.requestAnimationFrame(() => {
      if (canceled) {
        return;
      }
      previousFitViewRevisionRef.current = fitViewRevision;
      reactFlow.fitView({ padding: fitViewPadding, duration: 280 });
      recordCanvasDiagnosticEvent({
        level: 'debug',
        source: 'reactflow',
        event: 'reactflow.fit_view',
        message: 'React Flow fitView requested after canvas context activation',
        metadata: {
          selectedAreaId: effectiveAreaId,
          mapId: mapId ?? 'default',
          displayedNodeCount: displayNodeCount,
        },
      });
    });
    return () => {
      canceled = true;
      window.cancelAnimationFrame(frameId);
    };
  }, [
    canFitVisibleTopology,
    displayNodeCount,
    effectiveAreaId,
    fitViewPadding,
    fitViewRevision,
    mapId,
    reactFlow,
  ]);

  useEffect(() => {
    if (
      !shouldApplyInitialChromeHiddenFitRef.current ||
      initialChromeHiddenFitAppliedRef.current ||
      !effectiveChromeHidden ||
      !canFitVisibleTopology
    ) {
      return undefined;
    }

    let canceled = false;
    const applyFitView = () => {
      if (canceled) {
        return;
      }
      initialChromeHiddenFitAppliedRef.current = true;
      reactFlow.fitView({ padding: initialHiddenChromePadding, duration: 280 });
      recordCanvasDiagnosticEvent({
        level: 'debug',
        source: 'reactflow',
        event: 'reactflow.fit_view',
        message: 'React Flow fitView requested after initial hidden canvas chrome restore',
        metadata: {
          mapId: mapId ?? 'default',
          displayedNodeCount: displayNodeCount,
        },
      });
    };

    if (typeof window.requestAnimationFrame === 'function') {
      const frameId = window.requestAnimationFrame(applyFitView);
      return () => {
        canceled = true;
        window.cancelAnimationFrame(frameId);
      };
    }

    applyFitView();
    return () => {
      canceled = true;
    };
  }, [
    canFitVisibleTopology,
    displayNodeCount,
    effectiveChromeHidden,
    initialHiddenChromePadding,
    mapId,
    reactFlow,
  ]);
}
