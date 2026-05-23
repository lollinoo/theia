import type { LinkBadgePresentation } from './linkSemantics';

export interface RegisteredLinkLabel {
  edgeId: string;
  interactive: boolean;
  presentation: LinkBadgePresentation;
}

type RegisteredLinkLabelRecord = RegisteredLinkLabel & {
  signature: string;
};

const labelsByEdgeId = new Map<string, RegisteredLinkLabelRecord>();
const listeners = new Set<() => void>();
let snapshot: RegisteredLinkLabel[] = [];

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

function rebuildSnapshot(): void {
  snapshot = Array.from(labelsByEdgeId.values())
    .sort((a, b) => a.edgeId.localeCompare(b.edgeId))
    .map(({ signature: _signature, ...label }) => label);
}

function notifyLinkLabelListeners(): void {
  for (const listener of Array.from(listeners)) {
    listener();
  }
}

export function subscribeLinkLabels(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function getLinkLabelSnapshot(): RegisteredLinkLabel[] {
  return snapshot;
}

export function registerLinkLabel(label: RegisteredLinkLabel): void {
  const signature = serializeLabel(label);
  const existing = labelsByEdgeId.get(label.edgeId);
  if (existing?.signature === signature) {
    return;
  }

  labelsByEdgeId.set(label.edgeId, { ...label, signature });
  rebuildSnapshot();
  notifyLinkLabelListeners();
}

export function unregisterLinkLabel(edgeId: string): void {
  if (!labelsByEdgeId.delete(edgeId)) {
    return;
  }

  rebuildSnapshot();
  notifyLinkLabelListeners();
}

export function clearLinkLabelRegistry(): void {
  if (labelsByEdgeId.size === 0) {
    return;
  }

  labelsByEdgeId.clear();
  rebuildSnapshot();
  notifyLinkLabelListeners();
}
