/**
 * Exercises canvas perf scenarios topology canvas behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';

import {
  CANVAS_PERF_SCENARIOS,
  type CanvasPerfScenarioName,
  generateCanvasPerfScenario,
} from './canvasPerfScenarios';

describe('canvasPerfScenarios', () => {
  it.each([
    ['small', 25, 40],
    ['medium', 100, 180],
    ['large', 300, 600],
    ['stress', 700, 1500],
  ] satisfies Array<[CanvasPerfScenarioName, number, number]>)(
    'generates exact cardinality for %s',
    (scenarioName, deviceCount, linkCount) => {
      const scenario = generateCanvasPerfScenario(scenarioName);

      expect(CANVAS_PERF_SCENARIOS[scenarioName]).toEqual({ deviceCount, linkCount });
      expect(scenario.devices).toHaveLength(deviceCount);
      expect(scenario.links).toHaveLength(linkCount);
      expect(Object.keys(scenario.runtimeSnapshot.devices).length).toBeGreaterThan(0);
      expect(Object.keys(scenario.runtimeSnapshot.links).length).toBeGreaterThan(0);
      expect(scenario.selectedAreaId).toMatch(/^area-/);
    },
  );

  it('is deterministic for the same seed', () => {
    const first = generateCanvasPerfScenario('medium', { seed: 1234 });
    const second = generateCanvasPerfScenario('medium', { seed: 1234 });

    expect(second).toEqual(first);
  });

  it('connects every link to valid devices and includes realistic topology variance', () => {
    const scenario = generateCanvasPerfScenario('large', { seed: 99 });
    const deviceIds = new Set(scenario.devices.map((device) => device.id));

    expect(
      scenario.links.every(
        (link) => deviceIds.has(link.source_device_id) && deviceIds.has(link.target_device_id),
      ),
    ).toBe(true);
    expect(scenario.links.some((link) => link.source_device_id === link.target_device_id)).toBe(
      true,
    );
    expect(scenario.links.some((link) => link.source_if_speed !== link.target_if_speed)).toBe(true);
    expect(scenario.devices.some((device) => device.device_type === 'virtual')).toBe(true);
    expect(scenario.positions.size).toBeGreaterThan(0);
    expect(scenario.positions.size).toBeLessThan(scenario.devices.length);
  });
});
