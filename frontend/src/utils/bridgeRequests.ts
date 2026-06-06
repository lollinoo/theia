/**
 * Provides bridge requests utility behavior shared by frontend workflows.
 * Keeps non-UI policy and formatting rules reusable across components.
 */
export const BRIDGE_REQUEST_TIMEOUT_MS = 5_000;

const BRIDGE_TIMEOUT_SENTINEL = '__winbox_bridge_timeout__';

/** Defines bridge health timeout message constants and helper contracts for the shared frontend utility layer. */
export const BRIDGE_HEALTH_TIMEOUT_MESSAGE =
  'WinBox bridge health check timed out. Check that the local bridge is running.';
/** Defines bridge health unreachable message constants and helper contracts for the shared frontend utility layer. */
export const BRIDGE_HEALTH_UNREACHABLE_MESSAGE =
  'WinBox bridge is unreachable. Check that the local bridge is running.';
/** Defines bridge launch timeout message constants and helper contracts for the shared frontend utility layer. */
export const BRIDGE_LAUNCH_TIMEOUT_MESSAGE =
  'WinBox launch request timed out. Check that the local bridge is running.';
/** Defines bridge launch unreachable message constants and helper contracts for the shared frontend utility layer. */
export const BRIDGE_LAUNCH_UNREACHABLE_MESSAGE =
  'WinBox bridge is unreachable. Check that the local bridge is running.';

/** Fetches bridge with timeout for the shared frontend utility layer. */
export async function fetchBridgeWithTimeout(
  url: string,
  init?: RequestInit,
  timeoutMs: number = BRIDGE_REQUEST_TIMEOUT_MS,
): Promise<Response> {
  const controller = new AbortController();
  let timeoutId: number | undefined;

  const fetchPromise = fetch(url, { ...init, signal: controller.signal });
  const timeoutPromise = new Promise<never>((_, reject) => {
    timeoutId = window.setTimeout(() => {
      controller.abort();
      reject(new Error(BRIDGE_TIMEOUT_SENTINEL));
    }, timeoutMs);
  });

  try {
    return await Promise.race([fetchPromise, timeoutPromise]);
  } finally {
    if (timeoutId !== undefined) {
      window.clearTimeout(timeoutId);
    }
  }
}

function isBridgeTimeoutError(error: unknown): boolean {
  return (
    error instanceof Error &&
    (error.message === BRIDGE_TIMEOUT_SENTINEL || error.name === 'AbortError')
  );
}

/** Returns bridge health error message for the shared frontend utility layer. */
export function getBridgeHealthErrorMessage(error: unknown): string {
  if (isBridgeTimeoutError(error)) {
    return BRIDGE_HEALTH_TIMEOUT_MESSAGE;
  }
  return BRIDGE_HEALTH_UNREACHABLE_MESSAGE;
}

/** Returns bridge launch error message for the shared frontend utility layer. */
export function getBridgeLaunchErrorMessage(error: unknown): string {
  if (isBridgeTimeoutError(error)) {
    return BRIDGE_LAUNCH_TIMEOUT_MESSAGE;
  }
  return BRIDGE_LAUNCH_UNREACHABLE_MESSAGE;
}
