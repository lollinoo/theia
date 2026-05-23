import { EdgeLabelRenderer } from '@xyflow/react';
import { useSyncExternalStore } from 'react';
import {
  type RegisteredLinkLabel,
  getLinkLabelSnapshot,
  subscribeLinkLabels,
} from './linkLabelRegistry';

const linkBadgeReadabilityScaleCssVar = 'var(--theia-link-badge-readability-scale, 1)';

function LinkLabelStack({ label }: { label: RegisteredLinkLabel }) {
  const { edgeId, interactive, presentation } = label;

  return (
    <div
      data-testid={`${label.edgeId}-badge-stack`}
      data-link-edge-state={presentation.semanticState}
      data-link-badge-priority={presentation.semanticPriority}
      className={`topology-link-badge-stack topology-render-contained pointer-events-none absolute top-0 left-0 z-10 flex flex-col items-center gap-1.5 ${
        label.interactive ? 'transition-none' : 'transition-[opacity,transform] duration-150'
      }`}
      style={{
        position: 'absolute',
        transform: `translate(-50%, -50%) translate(${presentation.anchor.x}px, ${presentation.anchor.y}px) scale(${linkBadgeReadabilityScaleCssVar})`,
        opacity: presentation.opacity,
      }}
    >
      {presentation.items.map((badge) => (
        <span
          key={`${edgeId}-${badge.key}`}
          data-testid={`${edgeId}-badge-${badge.key}`}
          title={badge.title}
          className={`topology-link-badge topology-render-contained inline-flex min-h-7 items-center gap-2 whitespace-nowrap rounded-full border bg-surface-container-high px-2.5 py-1.5 font-mono text-[11px] font-bold leading-none tracking-[0.06em] ${
            interactive ? 'transition-none' : 'transition-[border-color,color] duration-150'
          } ${badge.className}`}
          style={badge.style}
        >
          <span data-testid={`${edgeId}-badge-${badge.key}-text`}>{badge.text}</span>
          {badge.warningIndicator ? (
            <span
              data-testid={`${edgeId}-badge-${badge.key}-warning`}
              title={badge.warningIndicator.title}
              className={`inline-flex h-4 min-w-4 items-center justify-center rounded-full border text-[10px] font-bold leading-none ${badge.warningIndicator.className}`}
            >
              {badge.warningIndicator.text}
            </span>
          ) : null}
        </span>
      ))}
    </div>
  );
}

export function LinkLabelLayer() {
  const labels = useSyncExternalStore(
    subscribeLinkLabels,
    getLinkLabelSnapshot,
    getLinkLabelSnapshot,
  );

  if (labels.length === 0) {
    return null;
  }

  return (
    <EdgeLabelRenderer>
      {labels.map((label) => (
        <LinkLabelStack key={label.edgeId} label={label} />
      ))}
    </EdgeLabelRenderer>
  );
}
