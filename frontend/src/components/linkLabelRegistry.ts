/**
 * Renders link label registry UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
import type { LinkBadgePresentation } from './linkSemantics';

/** Describes the registered link label contract used by the UI component boundary. */
export interface RegisteredLinkLabel {
  edgeId: string;
  interactive: boolean;
  presentation: LinkBadgePresentation;
}

const EMPTY_LABELS: RegisteredLinkLabel[] = [];
const labelsByEdgeId = new Map<string, RegisteredLinkLabel>();
const signaturesByEdgeId = new Map<string, string>();
const listeners = new Set<() => void>();
let snapshotDirty = false;
let snapshot: RegisteredLinkLabel[] = EMPTY_LABELS;

function serializeLabel(label: RegisteredLinkLabel): string {
  return JSON.stringify({
    edgeId: label.edgeId,
    interactive: label.interactive,
    anchor: label.presentation.anchor,
    opacity: label.presentation.opacity,
    scale: label.presentation.scale,
    visibility: label.presentation.visibility,
    semanticState: label.presentation.semanticState,
    semanticPriority: label.presentation.semanticPriority,
    items: label.presentation.items,
  });
}

function notifyLinkLabelListeners(): void {
  for (const listener of Array.from(listeners)) {
    listener();
  }
}

/** Subscribes to link labels for the UI component boundary. */
export function subscribeLinkLabels(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** Returns link label snapshot for the UI component boundary. */
export function getRegisteredLinkLabelsSnapshot(): RegisteredLinkLabel[] {
  if (snapshotDirty) {
    snapshot =
      labelsByEdgeId.size === 0
        ? EMPTY_LABELS
        : [...labelsByEdgeId.values()].sort((a, b) => a.edgeId.localeCompare(b.edgeId));
    snapshotDirty = false;
  }
  return snapshot;
}

/** Returns link label snapshot for the UI component boundary. */
export const getLinkLabelSnapshot = getRegisteredLinkLabelsSnapshot;

/** Registers link label for the UI component boundary. */
export function registerLinkLabel(label: RegisteredLinkLabel): void {
  const signature = serializeLabel(label);
  const existingSignature = signaturesByEdgeId.get(label.edgeId);
  if (existingSignature === signature) {
    return;
  }

  signaturesByEdgeId.set(label.edgeId, signature);
  labelsByEdgeId.set(label.edgeId, { ...label });
  snapshotDirty = true;
  notifyLinkLabelListeners();
}

/** Unregisters link label for the UI component boundary. */
export function unregisterLinkLabel(edgeId: string): void {
  signaturesByEdgeId.delete(edgeId);
  if (!labelsByEdgeId.delete(edgeId)) {
    return;
  }

  snapshotDirty = true;
  notifyLinkLabelListeners();
}

/** Clears link label registry for the UI component boundary. */
export function clearRegisteredLinkLabels(): void {
  if (labelsByEdgeId.size === 0) {
    snapshot = EMPTY_LABELS;
    snapshotDirty = false;
    return;
  }

  signaturesByEdgeId.clear();
  labelsByEdgeId.clear();
  snapshot = EMPTY_LABELS;
  snapshotDirty = false;
  notifyLinkLabelListeners();
}

/** Clears link label registry for the UI component boundary. */
export const clearLinkLabelRegistry = clearRegisteredLinkLabels;
