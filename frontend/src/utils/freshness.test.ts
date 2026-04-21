import { describe, expect, it } from 'vitest';

import {
  formatFreshness,
  formatHealthLabel,
  formatPollingEvery,
  formatRelativeAge,
  nextFreshnessUpdateDelayMs,
} from './freshness';

describe('freshness helpers', () => {
  it('applies the 2x and 5x freshness thresholds', () => {
    const nowMs = Date.parse('2026-04-13T12:00:00Z');

    expect(formatFreshness('2026-04-13T11:59:10Z', 30, nowMs)).toEqual({
      tier: 'Fresh',
      text: 'Fresh · 50s ago',
    });
    expect(formatFreshness('2026-04-13T11:58:30Z', 30, nowMs)).toEqual({
      tier: 'Stale',
      text: 'Stale · 1m ago',
    });
    expect(formatFreshness('2026-04-13T11:57:00Z', 30, nowMs)).toEqual({
      tier: 'Dead',
      text: 'Dead · 3m ago',
    });
  });

  it('returns waiting-for-first-poll copy when no timestamp exists', () => {
    expect(formatFreshness(undefined, 30, Date.now())).toEqual({
      tier: 'Dead',
      text: 'Dead · Waiting for first poll',
    });
  });

  it('formats relative age in seconds minutes and hours', () => {
    expect(formatRelativeAge(12)).toBe('12s ago');
    expect(formatRelativeAge(120)).toBe('2m ago');
    expect(formatRelativeAge(7_200)).toBe('2h ago');
  });

  it('schedules freshness updates only when the visible text or tier changes', () => {
    const lastPolledAt = '2026-04-13T12:00:00Z';

    expect(nextFreshnessUpdateDelayMs(lastPolledAt, 30, Date.parse('2026-04-13T12:00:00Z'))).toBe(
      1_000,
    );
    expect(nextFreshnessUpdateDelayMs(lastPolledAt, 30, Date.parse('2026-04-13T12:00:59Z'))).toBe(
      1_000,
    );
    expect(nextFreshnessUpdateDelayMs(lastPolledAt, 30, Date.parse('2026-04-13T12:01:01Z'))).toBe(
      59_000,
    );
    expect(nextFreshnessUpdateDelayMs(lastPolledAt, 30, Date.parse('2026-04-13T12:02:00Z'))).toBe(
      31_000,
    );
  });

  it('formats polling cadence copy for seconds and whole minutes', () => {
    expect(formatPollingEvery(30)).toBe('Polling every 30s');
    expect(formatPollingEvery(60)).toBe('Polling every 1m');
    expect(formatPollingEvery(300)).toBe('Polling every 5m');
  });

  it('maps backend health labels to card copy', () => {
    expect(formatHealthLabel('healthy')).toBe('Healthy');
    expect(formatHealthLabel('warning')).toBe('Warning');
    expect(formatHealthLabel('critical')).toBe('Critical');
    expect(formatHealthLabel('weird')).toBe('Unknown');
    expect(formatHealthLabel(undefined)).toBe('Unknown');
  });
});
