/**
 * Exercises link label registry behavior ensuring lazy snapshots and reference stability.
 */
import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  clearLinkLabelRegistry,
  clearRegisteredLinkLabels,
  getLinkLabelSnapshot,
  getRegisteredLinkLabelsSnapshot,
  type RegisteredLinkLabel,
  registerLinkLabel,
  subscribeLinkLabels,
  unregisterLinkLabel,
} from './linkLabelRegistry';

function createMockLabel(
  edgeId: string,
  bandwidth = '1 Gbps',
  throughput = 'TX: 100M / RX: 50M',
): RegisteredLinkLabel {
  return {
    edgeId,
    interactive: false,
    presentation: {
      anchor: { x: 100, y: 100 },
      items: [
        {
          key: 'rate',
          text: bandwidth,
          className: 'border-status-up/35 text-status-up',
        },
        {
          key: 'throughput',
          text: throughput,
          className: 'border-status-up/35 text-status-up',
        },
      ],
      opacity: 0.9,
      scale: 1,
      visibility: {
        zoomBand: 'high',
        showRate: true,
        showThroughput: true,
      },
      semanticState: 'up',
      semanticPriority: 'normal',
    },
  };
}

describe('linkLabelRegistry', () => {
  afterEach(() => {
    clearRegisteredLinkLabels();
  });

  it('preserves object references (Object.is) for unchanged labels when a new label is registered', () => {
    registerLinkLabel(createMockLabel('edge-1'));
    registerLinkLabel(createMockLabel('edge-2'));

    const snapshotBefore = getRegisteredLinkLabelsSnapshot();
    expect(snapshotBefore).toHaveLength(2);
    const edge1Before = snapshotBefore.find((l) => l.edgeId === 'edge-1');
    const edge2Before = snapshotBefore.find((l) => l.edgeId === 'edge-2');

    // Register a new edge label
    registerLinkLabel(createMockLabel('edge-3'));

    const snapshotAfter = getRegisteredLinkLabelsSnapshot();
    expect(snapshotAfter).toHaveLength(3);

    const edge1After = snapshotAfter.find((l) => l.edgeId === 'edge-1');
    const edge2After = snapshotAfter.find((l) => l.edgeId === 'edge-2');

    // Unchanged edges MUST preserve their exact object references
    expect(Object.is(edge1After, edge1Before)).toBe(true);
    expect(Object.is(edge2After, edge2Before)).toBe(true);
  });

  it('preserves object references for unchanged labels when another label is updated', () => {
    registerLinkLabel(createMockLabel('edge-1'));
    registerLinkLabel(createMockLabel('edge-2'));

    const snapshotBefore = getRegisteredLinkLabelsSnapshot();
    const edge1Before = snapshotBefore.find((l) => l.edgeId === 'edge-1');
    const edge2Before = snapshotBefore.find((l) => l.edgeId === 'edge-2');

    // Update edge-2 with different throughput telemetry
    registerLinkLabel(createMockLabel('edge-2', '1 Gbps', 'TX: 900M / RX: 800M'));

    const snapshotAfter = getRegisteredLinkLabelsSnapshot();
    expect(snapshotAfter).toHaveLength(2);

    const edge1After = snapshotAfter.find((l) => l.edgeId === 'edge-1');
    const edge2After = snapshotAfter.find((l) => l.edgeId === 'edge-2');

    // edge-1 was unchanged, so its reference is strictly preserved
    expect(Object.is(edge1After, edge1Before)).toBe(true);
    // edge-2 was updated, so its reference changed
    expect(Object.is(edge2After, edge2Before)).toBe(false);
    expect(edge2After?.presentation.items[1].text).toBe('TX: 900M / RX: 800M');
  });

  it('returns identical array reference on repeated snapshot reads when nothing changed', () => {
    registerLinkLabel(createMockLabel('edge-1'));
    registerLinkLabel(createMockLabel('edge-2'));

    const first = getRegisteredLinkLabelsSnapshot();
    const second = getRegisteredLinkLabelsSnapshot();
    const third = getLinkLabelSnapshot();

    expect(first).toBe(second);
    expect(first).toBe(third);
  });

  it('does not rebuild snapshot or notify listeners if registering identical label content', () => {
    const label = createMockLabel('edge-1');
    registerLinkLabel(label);

    const listener = vi.fn();
    const unsubscribe = subscribeLinkLabels(listener);

    const snapBefore = getRegisteredLinkLabelsSnapshot();

    // Register identical label content
    registerLinkLabel(createMockLabel('edge-1'));

    expect(listener).not.toHaveBeenCalled();
    const snapAfter = getRegisteredLinkLabelsSnapshot();
    expect(snapAfter).toBe(snapBefore);

    unsubscribe();
  });

  it('preserves unchanged label references across unregister operations', () => {
    registerLinkLabel(createMockLabel('edge-1'));
    registerLinkLabel(createMockLabel('edge-2'));
    registerLinkLabel(createMockLabel('edge-3'));

    const snapBefore = getRegisteredLinkLabelsSnapshot();
    const edge1Before = snapBefore.find((l) => l.edgeId === 'edge-1');
    const edge3Before = snapBefore.find((l) => l.edgeId === 'edge-3');

    unregisterLinkLabel('edge-2');

    const snapAfter = getRegisteredLinkLabelsSnapshot();
    expect(snapAfter).toHaveLength(2);

    const edge1After = snapAfter.find((l) => l.edgeId === 'edge-1');
    const edge3After = snapAfter.find((l) => l.edgeId === 'edge-3');

    expect(Object.is(edge1After, edge1Before)).toBe(true);
    expect(Object.is(edge3After, edge3Before)).toBe(true);
  });

  it('clears registry and resets snapshot to empty', () => {
    registerLinkLabel(createMockLabel('edge-1'));
    expect(getRegisteredLinkLabelsSnapshot()).toHaveLength(1);

    clearLinkLabelRegistry();

    expect(getRegisteredLinkLabelsSnapshot()).toEqual([]);
  });

  it('sorts labels deterministically by edgeId', () => {
    registerLinkLabel(createMockLabel('edge-z'));
    registerLinkLabel(createMockLabel('edge-a'));
    registerLinkLabel(createMockLabel('edge-m'));

    const snap = getRegisteredLinkLabelsSnapshot();
    expect(snap.map((l) => l.edgeId)).toEqual(['edge-a', 'edge-m', 'edge-z']);
  });
});
