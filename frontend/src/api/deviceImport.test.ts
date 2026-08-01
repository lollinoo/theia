/**
 * Exercises the typed one-time device import client and its label-blind response boundary.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setDocumentCookie } from '../test/documentCookie';
import {
  applyDeviceImportTopologyLayout,
  commitDeviceImport,
  continueDeviceImportTopologyRun,
  DeviceImportPartialCommitError,
  fetchActiveDeviceImportTopologyRun,
  fetchDeviceImportTopologyRun,
  parseDeviceImportCommitResult,
  parseDeviceImportPreview,
  previewDeviceImport,
  retryDeviceImportTopologyRun,
} from './deviceImport';
import { ValidationError } from './errors';

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

function validPreviewPayload(): Record<string, unknown> {
  return {
    file_digest: 'sha256:preview',
    configuration: {
      metrics_mode: 'prometheus',
      snmp_profile_id: null,
      map_id: '11111111-1111-1111-1111-111111111111',
      area_id: '22222222-2222-2222-2222-222222222222',
      topology_bootstrap_enabled: false,
      topology_layout_scope: 'preserve',
    },
    summary: {
      total: 3,
      ready: 1,
      invalid: 1,
      invalid_groups: 1,
      skipped_existing: 1,
      skipped_duplicate_in_file: 0,
    },
    targets: [
      {
        group_index: 0,
        item_index: 0,
        target: 'router.example.net:9116',
        address: 'router.example.net',
        status: 'ready',
      },
      {
        group_index: 0,
        item_index: 1,
        target: 'bad host',
        address: '',
        status: 'invalid',
        message: 'invalid target',
      },
      {
        group_index: 1,
        item_index: 0,
        target: 'existing.example.net',
        address: 'existing.example.net',
        status: 'skipped_existing',
      },
    ],
    diagnostics: [{ group_index: 2, message: 'file-SD group is missing targets' }],
  };
}

function validCommitPayload(): Record<string, unknown> {
  return {
    file_digest: 'sha256:commit',
    configuration: {
      metrics_mode: 'snmp',
      snmp_profile_id: '33333333-3333-3333-3333-333333333333',
      map_id: '11111111-1111-1111-1111-111111111111',
      area_id: null,
      topology_bootstrap_enabled: true,
      topology_layout_scope: 'reorganize',
    },
    summary: {
      total: 3,
      created: 1,
      skipped: 1,
      failed: 0,
      not_processed: 1,
    },
    results: [
      {
        group_index: 0,
        item_index: 0,
        target: 'created.example.net',
        address: 'created.example.net',
        status: 'created',
        device_id: '44444444-4444-4444-4444-444444444444',
      },
      {
        group_index: 0,
        item_index: 1,
        target: 'existing.example.net',
        address: 'existing.example.net',
        status: 'skipped_existing',
      },
      {
        group_index: 0,
        item_index: 2,
        target: 'pending.example.net',
        address: 'pending.example.net',
        status: 'not_processed',
        message: 'not processed because the import stopped',
      },
    ],
    diagnostics: [],
    incomplete: true,
    topology_run_id: '55555555-5555-5555-5555-555555555555',
  };
}

beforeEach(() => {
  vi.restoreAllMocks();
  setDocumentCookie('theia_csrf=device-import-csrf');
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('device import client', () => {
  it('treats an empty active-run response as a normal completed workflow', async () => {
    const fetchMock = vi.fn().mockResolvedValue(mockResponse(null, { status: 204 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchActiveDeviceImportTopologyRun('map-1')).resolves.toBeNull();
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/admin/device-imports/topology-runs/active?map_id=map-1',
      expect.objectContaining({ headers: { Accept: 'application/json' } }),
    );
  });

  it('keeps treating a legacy missing active-run response as a completed workflow', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        mockResponse(
          { error: 'device import topology run not found' },
          { ok: false, status: 404, statusText: 'Not Found' },
        ),
      );
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchActiveDeviceImportTopologyRun('map-1')).resolves.toBeNull();
  });

  it('sends preview with only the approved multipart fields', async () => {
    const file = new File(['- targets: ["router.example.net"]\n'], 'targets.yml', {
      type: 'application/yaml',
    });
    const fetchMock = vi.fn().mockResolvedValue(mockResponse(validPreviewPayload()));
    vi.stubGlobal('fetch', fetchMock);

    const preview = await previewDeviceImport({
      file,
      metrics_mode: 'prometheus',
      map_id: '11111111-1111-1111-1111-111111111111',
      area_id: '22222222-2222-2222-2222-222222222222',
    });

    expect(preview.targets.map((target) => target.status)).toEqual([
      'ready',
      'invalid',
      'skipped_existing',
    ]);
    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(path).toBe('/api/v1/admin/device-imports/preview');
    const form = init.body as FormData;
    expect([...form.keys()]).toEqual(['file', 'metrics_mode', 'map_id', 'area_id']);
    expect(form.get('file')).toBe(file);
    expect(form.get('metrics_mode')).toBe('prometheus');
    expect(form.get('map_id')).toBe('11111111-1111-1111-1111-111111111111');
    expect(form.get('area_id')).toBe('22222222-2222-2222-2222-222222222222');
    expect(form.has('snmp_profile_id')).toBe(false);
    expect(form.has('expected_file_digest')).toBe(false);
  });

  it('defaults Prometheus SNMP fallback to the explicit Bootstrap-Once fields', async () => {
    const file = new File(['- targets: ["router.example.net:161"]\n'], 'targets.yml');
    const response = validPreviewPayload();
    response.configuration = {
      metrics_mode: 'prometheus_snmp_fallback',
      snmp_profile_id: '33333333-3333-3333-3333-333333333333',
      map_id: '11111111-1111-1111-1111-111111111111',
      area_id: null,
      topology_bootstrap_enabled: true,
      topology_layout_scope: 'preserve',
    };
    const fetchMock = vi.fn().mockResolvedValue(mockResponse(response));
    vi.stubGlobal('fetch', fetchMock);

    await previewDeviceImport({
      file,
      metrics_mode: 'prometheus_snmp_fallback',
      map_id: '11111111-1111-1111-1111-111111111111',
      snmp_profile_id: '33333333-3333-3333-3333-333333333333',
    });

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    const form = init.body as FormData;
    expect([...form.keys()]).toEqual([
      'file',
      'metrics_mode',
      'snmp_profile_id',
      'map_id',
      'topology_bootstrap_enabled',
      'topology_layout_scope',
    ]);
    expect(form.get('topology_bootstrap_enabled')).toBe('true');
    expect(form.get('topology_layout_scope')).toBe('preserve');
  });

  it('sends an explicitly disabled Bootstrap-Once setting for Prometheus SNMP fallback', async () => {
    const file = new File(['- targets: ["router.example.net:161"]\n'], 'targets.yml');
    const fetchMock = vi.fn().mockResolvedValue(mockResponse(validPreviewPayload()));
    vi.stubGlobal('fetch', fetchMock);

    await previewDeviceImport({
      file,
      metrics_mode: 'prometheus_snmp_fallback',
      map_id: '11111111-1111-1111-1111-111111111111',
      snmp_profile_id: '33333333-3333-3333-3333-333333333333',
      topology_bootstrap_enabled: false,
      topology_layout_scope: 'preserve',
    });

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    const form = init.body as FormData;
    expect(form.get('topology_bootstrap_enabled')).toBe('false');
    expect(form.get('topology_layout_scope')).toBe('preserve');
  });

  it('resends the same File for commit with profile and expected digest', async () => {
    const file = new File(['- targets: ["router.example.net:161"]\n'], 'targets.yml');
    const fetchMock = vi.fn().mockResolvedValue(mockResponse(validCommitPayload()));
    vi.stubGlobal('fetch', fetchMock);

    const result = await commitDeviceImport(
      {
        file,
        metrics_mode: 'snmp',
        map_id: '11111111-1111-1111-1111-111111111111',
        snmp_profile_id: '33333333-3333-3333-3333-333333333333',
        topology_bootstrap_enabled: true,
        topology_layout_scope: 'reorganize',
      },
      'sha256:preview',
    );

    expect(result.results.map((row) => row.status)).toEqual([
      'created',
      'skipped_existing',
      'not_processed',
    ]);
    expect(result.incomplete).toBe(true);
    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(path).toBe('/api/v1/admin/device-imports/commit');
    const form = init.body as FormData;
    expect([...form.keys()]).toEqual([
      'file',
      'metrics_mode',
      'snmp_profile_id',
      'map_id',
      'topology_bootstrap_enabled',
      'topology_layout_scope',
      'expected_file_digest',
    ]);
    expect(form.get('file')).toBe(file);
    expect(form.get('snmp_profile_id')).toBe('33333333-3333-3333-3333-333333333333');
    expect(form.get('topology_bootstrap_enabled')).toBe('true');
    expect(form.get('topology_layout_scope')).toBe('reorganize');
    expect(form.get('expected_file_digest')).toBe('sha256:preview');
  });

  it.each([
    {
      status: 409,
      statusText: 'Conflict',
      backendError: 'the uploaded file changed after preview',
      expectedMessage: 'the uploaded file changed after preview',
    },
    {
      status: 500,
      statusText: 'Internal Server Error',
      backendError: 'device import failed (ref: import-abc123)',
      expectedMessage: 'Something went wrong (ref: import-abc123)',
    },
    {
      status: 503,
      statusText: 'Service Unavailable',
      backendError: 'device import store unavailable',
      expectedMessage:
        '/api/v1/admin/device-imports/commit failed: 503 device import store unavailable',
    },
  ])(
    'preserves ordered partial commit results from an HTTP $status response',
    async ({ status, statusText, backendError, expectedMessage }) => {
      const file = new File(['- targets: ["router.example.net:161"]\n'], 'targets.yml');
      const fetchMock = vi.fn().mockResolvedValue(
        mockResponse(
          {
            ...validCommitPayload(),
            error: backendError,
          },
          { ok: false, status, statusText },
        ),
      );
      vi.stubGlobal('fetch', fetchMock);

      let caught: unknown;
      try {
        await commitDeviceImport(
          {
            file,
            metrics_mode: 'snmp',
            map_id: '11111111-1111-1111-1111-111111111111',
            snmp_profile_id: '33333333-3333-3333-3333-333333333333',
          },
          'sha256:preview',
        );
      } catch (error) {
        caught = error;
      }

      expect(caught).toBeInstanceOf(DeviceImportPartialCommitError);
      expect(caught).toMatchObject({
        message: expectedMessage,
        result: {
          incomplete: true,
          summary: {
            created: 1,
            skipped: 1,
            not_processed: 1,
          },
        },
      });
      expect(
        (caught as DeviceImportPartialCommitError).result.results.map((row) => row.status),
      ).toEqual(['created', 'skipped_existing', 'not_processed']);
    },
  );

  it('preserves target and diagnostic order while dropping unapproved response fields', () => {
    const payload = validPreviewPayload();
    const targets = payload.targets as Array<Record<string, unknown>>;
    targets[0] = {
      ...targets[0],
      labels: { identity: 'SHOULD_NOT_SURVIVE' },
      vendor: 'mikrotik',
      community: 'secret',
    };

    const parsed = parseDeviceImportPreview(payload);

    expect(parsed.targets.map((target) => target.target)).toEqual([
      'router.example.net:9116',
      'bad host',
      'existing.example.net',
    ]);
    expect(parsed.diagnostics).toEqual([
      { group_index: 2, message: 'file-SD group is missing targets' },
    ]);
    expect(parsed.targets[0]).not.toHaveProperty('labels');
    expect(parsed.targets[0]).not.toHaveProperty('vendor');
    expect(parsed.targets[0]).not.toHaveProperty('community');
  });

  it('parses ordered commit outcomes and optional device IDs', () => {
    const parsed = parseDeviceImportCommitResult(validCommitPayload());

    expect(parsed.results).toHaveLength(3);
    expect(parsed.results[0].device_id).toBe('44444444-4444-4444-4444-444444444444');
    expect(parsed.results[1].device_id).toBeUndefined();
    expect(parsed.summary).toEqual({
      total: 3,
      created: 1,
      skipped: 1,
      failed: 0,
      not_processed: 1,
    });
    expect(parsed.topology_run_id).toBe('55555555-5555-5555-5555-555555555555');
  });

  it('parses run progress and sends actor-scoped topology controls', async () => {
    const runID = '55555555-5555-5555-5555-555555555555';
    const mapID = '11111111-1111-1111-1111-111111111111';
    const deviceID = '44444444-4444-4444-4444-444444444444';
    const snapshotPayload = {
      run: {
        id: runID,
        map_id: mapID,
        file_digest: 'sha256:commit',
        layout_scope: 'reorganize',
        state: 'ready_for_layout',
        auto_layout_allowed: true,
        backgrounded: false,
        layout_input_token: 'sha256:layout',
        reconcile_attempts: 0,
        created_at: '2026-07-31T10:00:00Z',
        updated_at: '2026-07-31T10:00:02Z',
      },
      items: [
        {
          device_id: deviceID,
          state: 'warning',
          attempt: 1,
          result_code: 'unresolved_neighbors',
          message: 'Some discovered neighbors could not be linked automatically.',
          neighbor_count: 2,
          links_created: 1,
          unresolved_neighbors: 1,
          updated_at: '2026-07-31T10:00:02Z',
        },
      ],
    };
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(mockResponse(snapshotPayload))
      .mockResolvedValue(mockResponse(null, { status: 204 }));
    vi.stubGlobal('fetch', fetchMock);

    const snapshot = await fetchDeviceImportTopologyRun(runID);
    await retryDeviceImportTopologyRun(runID, [deviceID]);
    await continueDeviceImportTopologyRun(runID);
    await applyDeviceImportTopologyLayout(runID, {
      input_token: 'sha256:layout',
      positions: [{ device_id: deviceID, x: 120, y: 240, pinned: false }],
      reset_link_route_ids: [],
    });

    expect(snapshot.run.state).toBe('ready_for_layout');
    expect(snapshot.items[0].result_code).toBe('unresolved_neighbors');
    expect(fetchMock.mock.calls.map(([path]) => path)).toEqual([
      `/api/v1/admin/device-imports/topology-runs/${runID}`,
      `/api/v1/admin/device-imports/topology-runs/${runID}/retry`,
      `/api/v1/admin/device-imports/topology-runs/${runID}/continue`,
      `/api/v1/admin/device-imports/topology-runs/${runID}/layout`,
    ]);
    expect(JSON.parse(fetchMock.mock.calls[3][1].body as string)).toEqual({
      input_token: 'sha256:layout',
      positions: [{ device_id: deviceID, x: 120, y: 240, pinned: false }],
      reset_link_route_ids: [],
    });
  });

  it.each([
    { name: 'non-object preview', parse: () => parseDeviceImportPreview(null) },
    {
      name: 'invalid preview status',
      parse: () => {
        const payload = validPreviewPayload();
        (payload.targets as Array<Record<string, unknown>>)[0].status = 'unknown';
        return parseDeviceImportPreview(payload);
      },
    },
    {
      name: 'missing commit incomplete flag',
      parse: () => {
        const payload = validCommitPayload();
        delete payload.incomplete;
        return parseDeviceImportCommitResult(payload);
      },
    },
  ])('rejects malformed response: $name', ({ parse }) => {
    expect(parse).toThrow(ValidationError);
    expect(parse).toThrow(/Invalid device import response/);
  });
});
