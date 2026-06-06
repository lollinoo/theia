/**
 * Provides freshness utility behavior shared by frontend workflows.
 * Keeps non-UI policy and formatting rules reusable across components.
 */
export type FreshnessTier = 'Fresh' | 'Stale' | 'Dead';

function clampAgeSeconds(ageSeconds: number): number {
  if (!Number.isFinite(ageSeconds) || ageSeconds <= 0) {
    return 0;
  }
  return Math.floor(ageSeconds);
}

/** Formats relative age for the shared frontend utility layer. */
export function formatRelativeAge(ageSeconds: number): string {
  const safeAgeSeconds = clampAgeSeconds(ageSeconds);

  if (safeAgeSeconds < 60) {
    return `${safeAgeSeconds}s ago`;
  }

  if (safeAgeSeconds < 3_600) {
    return `${Math.floor(safeAgeSeconds / 60)}m ago`;
  }

  return `${Math.floor(safeAgeSeconds / 3_600)}h ago`;
}

/** Next freshness update delay ms for the shared frontend utility layer. */
export function nextFreshnessUpdateDelayMs(
  lastPolledAt: string | undefined,
  expectedIntervalSeconds: number | null | undefined,
  nowMs: number,
): number | null {
  if (!lastPolledAt) {
    return null;
  }

  const lastPolledAtMs = Date.parse(lastPolledAt);
  const intervalSeconds = expectedIntervalSeconds ?? 0;

  if (!Number.isFinite(lastPolledAtMs) || intervalSeconds <= 0) {
    return null;
  }

  const elapsedMs = Math.max(0, nowMs - lastPolledAtMs);
  const ageSeconds = clampAgeSeconds(elapsedMs / 1_000);

  const nextDisplayBoundaryMs =
    ageSeconds < 60
      ? (ageSeconds + 1) * 1_000
      : ageSeconds < 3_600
        ? (Math.floor(ageSeconds / 60) + 1) * 60 * 1_000
        : (Math.floor(ageSeconds / 3_600) + 1) * 3_600 * 1_000;

  const nextTransitionBoundariesMs = [
    (intervalSeconds * 2 + 1) * 1_000,
    (intervalSeconds * 5 + 1) * 1_000,
  ].filter((boundaryMs) => boundaryMs > elapsedMs);

  const nextUpdateAtMs = Math.min(nextDisplayBoundaryMs, ...nextTransitionBoundariesMs);
  return Math.max(1, nextUpdateAtMs - elapsedMs);
}

/** Formats polling every for the shared frontend utility layer. */
export function formatPollingEvery(seconds: number | null | undefined): string {
  if (seconds === null || seconds === undefined || seconds <= 0) {
    return 'Polling every --';
  }

  if (seconds < 60) {
    return `Polling every ${seconds}s`;
  }

  if (seconds % 60 === 0) {
    return `Polling every ${seconds / 60}m`;
  }

  return `Polling every ${seconds}s`;
}

/** Formats freshness for the shared frontend utility layer. */
export function formatFreshness(
  lastPolledAt: string | undefined,
  expectedIntervalSeconds: number | null | undefined,
  nowMs: number,
): { tier: FreshnessTier; text: string } {
  if (!lastPolledAt) {
    return { tier: 'Dead', text: 'Dead · Waiting for first poll' };
  }

  const lastPolledAtMs = Date.parse(lastPolledAt);
  const intervalSeconds = expectedIntervalSeconds ?? 0;

  if (!Number.isFinite(lastPolledAtMs) || intervalSeconds <= 0) {
    return { tier: 'Dead', text: 'Dead · Waiting for first poll' };
  }

  const ageSeconds = clampAgeSeconds((nowMs - lastPolledAtMs) / 1_000);
  const tier: FreshnessTier =
    ageSeconds <= intervalSeconds * 2
      ? 'Fresh'
      : ageSeconds <= intervalSeconds * 5
        ? 'Stale'
        : 'Dead';

  return {
    tier,
    text: `${tier} · ${formatRelativeAge(ageSeconds)}`,
  };
}

/** Formats health label for the shared frontend utility layer. */
export function formatHealthLabel(
  health: string | undefined,
): 'Healthy' | 'Warning' | 'Critical' | 'Unknown' {
  switch (health) {
    case 'healthy':
      return 'Healthy';
    case 'warning':
      return 'Warning';
    case 'critical':
      return 'Critical';
    default:
      return 'Unknown';
  }
}
