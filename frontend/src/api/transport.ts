/**
 * Provides frontend API helpers for transport endpoints.
 * Keeps request construction and backend response handling out of UI components.
 */
import { ServerError, ValidationError } from './errors';

/** Describes the error payload contract used by the frontend API boundary. */
export type ErrorPayload = {
  error?: string;
};

const multipartJSONErrorPayloads = new WeakMap<Error, unknown>();

function retainMultipartJSONErrorPayload<T extends Error>(error: T, payload: unknown): T {
  multipartJSONErrorPayloads.set(error, payload);
  return error;
}

/** Returns the decoded response body retained for a multipart request error. */
export function multipartJSONErrorPayload(error: unknown): unknown | undefined {
  return error instanceof Error ? multipartJSONErrorPayloads.get(error) : undefined;
}

// csrfTokenFromCookie extracts the CSRF cookie without throwing during SSR or malformed encoding.
function csrfTokenFromCookie(): string | null {
  if (typeof document === 'undefined') {
    return null;
  }
  const csrfCookie = document.cookie
    .split(';')
    .map((part) => part.trim())
    .find((part) => part.startsWith('theia_csrf='));
  if (!csrfCookie) {
    return null;
  }
  try {
    return decodeURIComponent(csrfCookie.slice('theia_csrf='.length));
  } catch {
    return null;
  }
}

// headersWithCsrf appends the browser CSRF token to mutating JSON requests when available.
export function headersWithCsrf(headers: Record<string, string>): Record<string, string> {
  const csrfToken = csrfTokenFromCookie();
  if (!csrfToken) {
    return headers;
  }
  return { ...headers, 'X-CSRF-Token': csrfToken };
}

// errorMessageFromPayload preserves backend error text while falling back to response status text.
function errorMessageFromPayload(payload: ErrorPayload | unknown, fallback: string): string {
  return typeof payload === 'object' &&
    payload !== null &&
    'error' in payload &&
    typeof payload.error === 'string'
    ? payload.error
    : fallback;
}

// serverErrorFromMessage hides raw 500 details and preserves correlation references.
function serverErrorFromMessage(errorMessage: string): ServerError {
  const refMatch = /ref:\s*([a-zA-Z0-9-]+)/.exec(errorMessage);
  const correlationId = refMatch ? refMatch[1] : undefined;
  const userMessage = correlationId
    ? `Something went wrong (ref: ${correlationId})`
    : 'Something went wrong';
  return new ServerError(userMessage, correlationId);
}

// requestJSON performs a GET JSON request and preserves the legacy generic Error shape.
export async function requestJSON(
  path: string,
  options: Pick<RequestInit, 'cache' | 'signal'> = {},
): Promise<unknown> {
  const response = await fetch(path, {
    ...options,
    headers: {
      Accept: 'application/json',
    },
  });

  const payload = (await response.json().catch(() => null)) as ErrorPayload | unknown;

  if (!response.ok) {
    const errorMessage = errorMessageFromPayload(payload, response.statusText);

    if (response.status === 500) {
      throw serverErrorFromMessage(errorMessage);
    }

    throw new Error(`${path} failed: ${response.status} ${errorMessage}`);
  }

  return payload;
}

// requestJSONWithBody sends JSON with CSRF headers and maps validation/server failures to typed errors.
export async function requestJSONWithBody(
  path: string,
  method: string,
  body?: unknown,
): Promise<unknown> {
  const response = await fetch(path, {
    method,
    headers: headersWithCsrf({
      Accept: 'application/json',
      'Content-Type': 'application/json',
    }),
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });

  if (response.status === 204) {
    return null;
  }

  const payload = (await response.json().catch(() => null)) as ErrorPayload | unknown;

  if (!response.ok) {
    const errorMessage = errorMessageFromPayload(payload, response.statusText);

    if (response.status === 400 || response.status === 409) {
      throw new ValidationError(errorMessage);
    }

    if (response.status === 500) {
      throw serverErrorFromMessage(errorMessage);
    }

    throw new Error(`${path} failed: ${response.status} ${errorMessage}`);
  }

  return payload;
}

/** Sends a same-origin multipart POST without overriding the browser-generated boundary. */
export async function requestMultipartJSON(path: string, body: FormData): Promise<unknown> {
  const response = await fetch(path, {
    method: 'POST',
    headers: headersWithCsrf({ Accept: 'application/json' }),
    body,
  });

  const payload = (await response.json().catch(() => null)) as ErrorPayload | unknown;
  if (!response.ok) {
    const errorMessage = errorMessageFromPayload(payload, response.statusText);

    if (response.status === 400 || response.status === 409) {
      throw retainMultipartJSONErrorPayload(new ValidationError(errorMessage), payload);
    }
    if (response.status === 413) {
      throw retainMultipartJSONErrorPayload(
        new ValidationError('Import files are limited to 2 MiB and 5,000 targets'),
        payload,
      );
    }
    if (response.status === 500) {
      throw retainMultipartJSONErrorPayload(serverErrorFromMessage(errorMessage), payload);
    }

    throw retainMultipartJSONErrorPayload(
      new Error(`${path} failed: ${response.status} ${errorMessage}`),
      payload,
    );
  }

  return payload;
}
