import { ServerError, ValidationError } from './errors';

export type ErrorPayload = {
  error?: string;
};

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

export function headersWithCsrf(headers: Record<string, string>): Record<string, string> {
  const csrfToken = csrfTokenFromCookie();
  if (!csrfToken) {
    return headers;
  }
  return { ...headers, 'X-CSRF-Token': csrfToken };
}

function errorMessageFromPayload(payload: ErrorPayload | unknown, fallback: string): string {
  return typeof payload === 'object' &&
    payload !== null &&
    'error' in payload &&
    typeof payload.error === 'string'
    ? payload.error
    : fallback;
}

function serverErrorFromMessage(errorMessage: string): ServerError {
  const refMatch = /ref:\s*([a-zA-Z0-9-]+)/.exec(errorMessage);
  const correlationId = refMatch ? refMatch[1] : undefined;
  const userMessage = correlationId
    ? `Something went wrong (ref: ${correlationId})`
    : 'Something went wrong';
  return new ServerError(userMessage, correlationId);
}

export async function requestJSON(path: string): Promise<unknown> {
  const response = await fetch(path, {
    headers: {
      Accept: 'application/json',
    },
  });

  const payload = (await response.json().catch(() => null)) as ErrorPayload | unknown;

  if (!response.ok) {
    const errorMessage = errorMessageFromPayload(payload, response.statusText);
    throw new Error(`${path} failed: ${response.status} ${errorMessage}`);
  }

  return payload;
}

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
