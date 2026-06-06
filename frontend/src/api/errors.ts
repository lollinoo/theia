/**
 * Provides frontend API helpers for errors endpoints.
 * Keeps request construction and backend response handling out of UI components.
 */
/**
 * Thrown by the API client when the backend returns a 400 Bad Request response.
 * The message contains the user-facing validation error from the backend.
 */
export class ValidationError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'ValidationError';
  }
}

/**
 * Thrown by the API client when the backend returns a 500 Internal Server Error.
 * Contains an optional correlation ID for log lookup — displayed to the user as
 * "Something went wrong (ref: <correlationId>)" when present.
 */
export class ServerError extends Error {
  public readonly correlationId: string | undefined;

  constructor(message: string, correlationId: string | undefined) {
    super(message);
    this.name = 'ServerError';
    this.correlationId = correlationId;
  }
}
