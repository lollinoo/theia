/**
 * Provides freshness utility behavior shared by frontend workflows.
 * Keeps non-UI policy and formatting rules reusable across components.
 */
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
