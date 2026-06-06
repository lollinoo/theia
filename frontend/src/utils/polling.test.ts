/**
 * Exercises polling utility behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';

import { getDefaultPollingIntervalSeconds, getEffectivePollingIntervalSeconds } from './polling';

describe('polling helpers', () => {
  it('returns class defaults when no override exists', () => {
    expect(getDefaultPollingIntervalSeconds('core')).toBe(30);
    expect(getDefaultPollingIntervalSeconds('standard')).toBe(60);
    expect(getDefaultPollingIntervalSeconds('low')).toBe(300);
  });

  it('prefers poll_interval_override over poll_class default', () => {
    expect(
      getEffectivePollingIntervalSeconds({
        poll_class: 'standard',
        poll_interval_override: 15,
      }),
    ).toBe(15);

    expect(
      getEffectivePollingIntervalSeconds({
        poll_class: 'low',
        poll_interval_override: null,
      }),
    ).toBe(300);
  });
});
