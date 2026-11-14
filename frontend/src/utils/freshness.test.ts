/**
 * Exercises freshness utility behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';

import { formatPollingEvery, formatRelativeAge } from './freshness';

describe('freshness helpers', () => {
  it('formats relative age in seconds minutes and hours', () => {
    expect(formatRelativeAge(12)).toBe('12s ago');
    expect(formatRelativeAge(120)).toBe('2m ago');
    expect(formatRelativeAge(7_200)).toBe('2h ago');
  });

  it('formats polling cadence copy for seconds and whole minutes', () => {
    expect(formatPollingEvery(30)).toBe('Polling every 30s');
    expect(formatPollingEvery(60)).toBe('Polling every 1m');
    expect(formatPollingEvery(300)).toBe('Polling every 5m');
  });
});
