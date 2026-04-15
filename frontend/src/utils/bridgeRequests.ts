export const BRIDGE_REQUEST_TIMEOUT_MS = 5_000;

const BRIDGE_TIMEOUT_SENTINEL = '__winbox_bridge_timeout__';

export const BRIDGE_HEALTH_TIMEOUT_MESSAGE = 'WinBox bridge health check timed out. Check that the local bridge is running.';
export const BRIDGE_HEALTH_UNREACHABLE_MESSAGE = 'WinBox bridge is unreachable. Check that the local bridge is running.';
export const BRIDGE_LAUNCH_TIMEOUT_MESSAGE = 'WinBox launch request timed out. Check that the local bridge is running.';
export const BRIDGE_LAUNCH_UNREACHABLE_MESSAGE = 'WinBox bridge is unreachable. Check that the local bridge is running.';

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
  return error instanceof Error && (error.message === BRIDGE_TIMEOUT_SENTINEL || error.name === 'AbortError');
}

export function getBridgeHealthErrorMessage(error: unknown): string {
  if (isBridgeTimeoutError(error)) {
    return BRIDGE_HEALTH_TIMEOUT_MESSAGE;
  }
  return BRIDGE_HEALTH_UNREACHABLE_MESSAGE;
}

export function getBridgeLaunchErrorMessage(error: unknown): string {
  if (isBridgeTimeoutError(error)) {
    return BRIDGE_LAUNCH_TIMEOUT_MESSAGE;
  }
  return BRIDGE_LAUNCH_UNREACHABLE_MESSAGE;
}
