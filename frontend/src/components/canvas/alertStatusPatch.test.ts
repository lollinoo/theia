/**
 * Exercises alert status patch topology canvas behavior so refactors preserve the documented contract.
 */
import { describe, expect, it, vi } from 'vitest';

import type { SnapshotPayload } from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import { applyAlertStatusPatch } from './alertStatusPatch';

describe('applyAlertStatusPatch', () => {
  it('schedules node and edge alert patch updates', () => {
    const snapshot: SnapshotPayload = { devices: {}, links: {} };
    const setNodes = vi.fn((updater: (nodes: DeviceNode[]) => DeviceNode[]) => {
      expect(updater([])).toEqual([]);
    });
    const setEdges = vi.fn((updater: (edges: LinkEdgeType[]) => LinkEdgeType[]) => {
      expect(updater([])).toEqual([]);
    });

    applyAlertStatusPatch({
      snapshot,
      alerts: [],
      setNodes,
      setEdges,
    });

    expect(setNodes).toHaveBeenCalledTimes(1);
    expect(setEdges).toHaveBeenCalledTimes(1);
  });
});
