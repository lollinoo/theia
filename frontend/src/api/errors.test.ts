/**
 * Exercises errors API boundary behavior so refactors preserve the documented contract.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { type CreateDevicePayload, createDevice } from './client';
import { ServerError, ValidationError } from './errors';

// Helper to create a mock Response
function mockResponse(
  body: unknown,
  init: { ok?: boolean; status?: number; statusText?: string } = {},
) {
  const { ok = true, status = 200, statusText = 'OK' } = init;
  return {
    ok,
    status,
    statusText,
    json: () => Promise.resolve(body),
    headers: new Headers(),
  } as unknown as Response;
}

beforeEach(() => {
  vi.restoreAllMocks();
});

// --- ValidationError class ---

describe('ValidationError', () => {
  it('is an instance of Error', () => {
    const err = new ValidationError('field is required');
    expect(err instanceof Error).toBe(true);
  });

  it('is an instance of ValidationError', () => {
    const err = new ValidationError('field is required');
    expect(err instanceof ValidationError).toBe(true);
  });

  it('.message equals the constructor argument', () => {
    const err = new ValidationError('field is required');
    expect(err.message).toBe('field is required');
  });

  it('.name equals ValidationError', () => {
    const err = new ValidationError('field is required');
    expect(err.name).toBe('ValidationError');
  });
});

// --- ServerError class ---

describe('ServerError', () => {
  it('is an instance of Error', () => {
    const err = new ServerError('Something went wrong', 'abc123');
    expect(err instanceof Error).toBe(true);
  });

  it('is an instance of ServerError', () => {
    const err = new ServerError('Something went wrong', 'abc123');
    expect(err instanceof ServerError).toBe(true);
  });

  it('.message equals the constructor message', () => {
    const err = new ServerError('Something went wrong', 'abc123');
    expect(err.message).toBe('Something went wrong');
  });

  it('.correlationId equals the constructor correlationId', () => {
    const err = new ServerError('Something went wrong', 'abc123');
    expect(err.correlationId).toBe('abc123');
  });

  it('.name equals ServerError', () => {
    const err = new ServerError('Something went wrong', 'abc123');
    expect(err.name).toBe('ServerError');
  });

  it('.correlationId is undefined when not provided', () => {
    const err = new ServerError('error', undefined);
    expect(err.correlationId).toBeUndefined();
  });
});

// --- Client integration: typed error classes ---

describe('client typed errors', () => {
  // Valid device resource for success cases
  function deviceResource(id: string, hostname: string, ip: string) {
    return {
      id,
      attributes: {
        hostname,
        ip,
        device_type: 'router',
        status: 'up',
        sys_name: hostname,
        sys_descr: 'RouterOS',
        hardware_model: 'RB4011',
        vendor: 'mikrotik',
        managed: true,
        backup_supported: true,
        metrics_source: 'prometheus',
        prometheus_label_name: 'instance',
        prometheus_label_value: `${ip}:9100`,
      },
      relationships: { interfaces: { data: [] } },
    };
  }

  const payload: CreateDevicePayload = {
    hostname: 'new-router',
    ip: '10.0.0.2',
    snmp: { version: '2c', community: 'public' },
  };

  it('throws ValidationError for 400 response', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          mockResponse(
            { error: 'hostname too long' },
            { ok: false, status: 400, statusText: 'Bad Request' },
          ),
        ),
    );

    await expect(createDevice(payload)).rejects.toThrow(ValidationError);
    await expect(createDevice(payload)).rejects.toThrow('hostname too long');
  });

  it('throws ValidationError for 409 response', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          mockResponse(
            { error: 'a device with IP/host "10.0.0.2" already exists' },
            { ok: false, status: 409, statusText: 'Conflict' },
          ),
        ),
    );

    await expect(createDevice(payload)).rejects.toThrow(ValidationError);
    await expect(createDevice(payload)).rejects.toThrow(
      'a device with IP/host "10.0.0.2" already exists',
    );
  });

  it('throws ServerError for 500 response with correlation ID', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          mockResponse(
            { error: 'internal error, ref: xyz789' },
            { ok: false, status: 500, statusText: 'Internal Server Error' },
          ),
        ),
    );

    let caughtError: unknown;
    try {
      await createDevice(payload);
    } catch (err) {
      caughtError = err;
    }

    expect(caughtError instanceof ServerError).toBe(true);
    expect((caughtError as ServerError).correlationId).toBe('xyz789');
  });

  it('throws ServerError for 500 response without correlation ID', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          mockResponse(
            { error: 'internal error' },
            { ok: false, status: 500, statusText: 'Internal Server Error' },
          ),
        ),
    );

    let caughtError: unknown;
    try {
      await createDevice(payload);
    } catch (err) {
      caughtError = err;
    }

    expect(caughtError instanceof ServerError).toBe(true);
    expect((caughtError as ServerError).correlationId).toBeUndefined();
  });

  it('throws plain Error for 401 response (not ValidationError or ServerError)', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          mockResponse(
            { error: 'Unauthorized' },
            { ok: false, status: 401, statusText: 'Unauthorized' },
          ),
        ),
    );

    let caughtError: unknown;
    try {
      await createDevice(payload);
    } catch (err) {
      caughtError = err;
    }

    expect(caughtError instanceof Error).toBe(true);
    expect(caughtError instanceof ValidationError).toBe(false);
    expect(caughtError instanceof ServerError).toBe(false);
  });

  it('throws plain Error for 403 response (not ValidationError or ServerError)', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          mockResponse({ error: 'Forbidden' }, { ok: false, status: 403, statusText: 'Forbidden' }),
        ),
    );

    let caughtError: unknown;
    try {
      await createDevice(payload);
    } catch (err) {
      caughtError = err;
    }

    expect(caughtError instanceof Error).toBe(true);
    expect(caughtError instanceof ValidationError).toBe(false);
    expect(caughtError instanceof ServerError).toBe(false);
  });

  it('resolves successfully on 200 with valid device payload', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          mockResponse({ data: deviceResource('uuid-99', 'new-router', '10.0.0.2') }),
        ),
    );

    const result = await createDevice(payload);
    expect(result.hostname).toBe('new-router');
  });
});
