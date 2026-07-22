/**
 * Exercises the one-time node import workflow at the Admin Area component boundary.
 */
import { act, fireEvent, render, screen, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  type DeviceImportCommitResult,
  type DeviceImportConfiguration,
  DeviceImportPartialCommitError,
  type DeviceImportPreview,
} from '../api/deviceImport';
import type { Area, CanvasMap, SNMPProfile } from '../types/api';
import { DeviceImportPanel } from './DeviceImportPanel';

const fetchCanvasMapsMock = vi.hoisted(() => vi.fn<() => Promise<CanvasMap[]>>());
const fetchCanvasMapAreasMock = vi.hoisted(() => vi.fn<(mapId: string) => Promise<Area[]>>());
const fetchSNMPProfilesMock = vi.hoisted(() => vi.fn<() => Promise<SNMPProfile[]>>());
const previewDeviceImportMock = vi.hoisted(() =>
  vi.fn<(configuration: unknown) => Promise<DeviceImportPreview>>(),
);
const commitDeviceImportMock = vi.hoisted(() =>
  vi.fn<
    (configuration: unknown, expectedFileDigest: string) => Promise<DeviceImportCommitResult>
  >(),
);

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>();
  return {
    ...actual,
    fetchCanvasMaps: () => fetchCanvasMapsMock(),
    fetchCanvasMapAreas: (mapId: string) => fetchCanvasMapAreasMock(mapId),
    fetchSNMPProfiles: () => fetchSNMPProfilesMock(),
    previewDeviceImport: (configuration: unknown) => previewDeviceImportMock(configuration),
    commitDeviceImport: (configuration: unknown, expectedFileDigest: string) =>
      commitDeviceImportMock(configuration, expectedFileDigest),
  };
});

const primaryMap: CanvasMap = {
  id: 'map-primary',
  name: 'Primary Backbone',
  description: '',
  source_area_id: null,
  filter: {},
  is_default: true,
  device_count: 4,
  link_count: 3,
  position_count: 4,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-02T00:00:00Z',
};

const secondaryMap: CanvasMap = {
  ...primaryMap,
  id: 'map-secondary',
  name: 'Edge Sites',
  is_default: false,
};

const primaryArea: Area = {
  id: 'area-primary',
  name: 'Core',
  description: '',
  color: '#2979FF',
  device_count: 2,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-02T00:00:00Z',
};

const secondaryArea: Area = {
  ...primaryArea,
  id: 'area-secondary',
  name: 'Branches',
};

const profiles: SNMPProfile[] = [
  {
    id: 'profile-v3',
    name: 'Core SNMPv3',
    description: 'Redacted profile',
    snmp: {
      version: '3',
      username: 'monitor',
      auth_password_set: true,
      priv_password_set: true,
    },
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
  },
  {
    id: 'profile-v2',
    name: 'Branches SNMPv2',
    description: '',
    snmp: { version: '2c', community_set: true },
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
  },
];

function previewFixture(overrides: Partial<DeviceImportPreview> = {}): DeviceImportPreview {
  return {
    file_digest: 'sha256:preview-digest',
    configuration: {
      metrics_mode: 'prometheus',
      snmp_profile_id: null,
      map_id: primaryMap.id,
      area_id: primaryArea.id,
    },
    summary: {
      total: 4,
      ready: 1,
      invalid: 1,
      invalid_groups: 1,
      skipped_existing: 1,
      skipped_duplicate_in_file: 1,
    },
    targets: [
      {
        group_index: 0,
        item_index: 0,
        target: 'ready.example.net:9100',
        address: 'ready.example.net',
        status: 'ready',
      },
      {
        group_index: 0,
        item_index: 1,
        target: 'existing.example.net:9100',
        address: 'existing.example.net',
        status: 'skipped_existing',
      },
      {
        group_index: 1,
        item_index: 0,
        target: 'bad target',
        address: '',
        status: 'invalid',
        message: 'invalid target',
      },
      {
        group_index: 1,
        item_index: 1,
        target: 'ready.example.net:9100',
        address: 'ready.example.net',
        status: 'skipped_duplicate_in_file',
      },
    ],
    diagnostics: [{ group_index: 2, message: 'file-SD group is missing targets' }],
    ...overrides,
  };
}

function commitFixture(): DeviceImportCommitResult {
  return {
    file_digest: 'sha256:preview-digest',
    configuration: {
      metrics_mode: 'prometheus',
      snmp_profile_id: null,
      map_id: primaryMap.id,
      area_id: primaryArea.id,
    },
    summary: {
      total: 4,
      created: 1,
      skipped: 1,
      failed: 1,
      not_processed: 1,
    },
    results: [
      {
        group_index: 0,
        item_index: 0,
        target: 'ready.example.net:9100',
        address: 'ready.example.net',
        status: 'created',
        device_id: 'device-created',
      },
      {
        group_index: 0,
        item_index: 1,
        target: 'existing.example.net:9100',
        address: 'existing.example.net',
        status: 'skipped_existing',
      },
      {
        group_index: 1,
        item_index: 0,
        target: 'failed.example.net:9100',
        address: 'failed.example.net',
        status: 'failed',
        message: 'target failed',
      },
      {
        group_index: 1,
        item_index: 1,
        target: 'pending.example.net:9100',
        address: 'pending.example.net',
        status: 'not_processed',
        message: 'import stopped',
      },
    ],
    diagnostics: [{ group_index: 3, message: 'ignored malformed group' }],
    incomplete: true,
  };
}

function uploadFile(name = 'targets.yml'): File {
  return new File(['- targets: ["ready.example.net:9100"]\n'], name, {
    type: 'application/yaml',
  });
}

async function renderPanel(
  props: { canReadCredentials?: boolean; onOpenMap?: (map: CanvasMap) => void } = {},
) {
  let rendered: ReturnType<typeof render> | undefined;
  await act(async () => {
    rendered = render(
      <DeviceImportPanel
        canReadCredentials={props.canReadCredentials ?? true}
        onOpenMap={props.onOpenMap}
      />,
    );
  });
  return rendered as ReturnType<typeof render>;
}

async function change(element: Element, value: string) {
  await act(async () => {
    fireEvent.change(element, { target: { value } });
  });
}

async function click(element: Element) {
  await act(async () => {
    fireEvent.click(element);
  });
}

async function chooseFile(file = uploadFile()) {
  await act(async () => {
    fireEvent.change(screen.getByLabelText('Prometheus file-SD YAML'), {
      target: { files: [file] },
    });
  });
  return file;
}

beforeEach(() => {
  fetchCanvasMapsMock.mockReset();
  fetchCanvasMapAreasMock.mockReset();
  fetchSNMPProfilesMock.mockReset();
  previewDeviceImportMock.mockReset();
  commitDeviceImportMock.mockReset();

  fetchCanvasMapsMock.mockResolvedValue([secondaryMap, primaryMap]);
  fetchCanvasMapAreasMock.mockImplementation(async (mapId) =>
    mapId === secondaryMap.id ? [secondaryArea] : [primaryArea],
  );
  fetchSNMPProfilesMock.mockResolvedValue(profiles);
  previewDeviceImportMock.mockResolvedValue(previewFixture());
  commitDeviceImportMock.mockResolvedValue(commitFixture());
});

describe('DeviceImportPanel', () => {
  it('loads maps and redacted profile summaries, defaults the accessible primary map, and lists all modes', async () => {
    await renderPanel();

    expect(screen.getByRole('radio', { name: 'Prometheus' })).toBeChecked();
    expect(screen.getByRole('radio', { name: 'Prometheus with SNMP fallback' })).toBeEnabled();
    expect(screen.getByRole('radio', { name: 'SNMP' })).toBeEnabled();
    expect(await screen.findByRole('option', { name: 'Primary Backbone (primary)' })).toBeVisible();
    expect(screen.getByRole('combobox', { name: 'Destination map' })).toHaveValue(primaryMap.id);
    expect(fetchCanvasMapAreasMock).toHaveBeenCalledWith(primaryMap.id);
    expect(fetchSNMPProfilesMock).toHaveBeenCalledTimes(1);

    await click(screen.getByRole('radio', { name: 'SNMP' }));
    const profileSelect = screen.getByRole('combobox', { name: 'SNMP Profile' });
    expect(within(profileSelect).getByRole('option', { name: 'Core SNMPv3 (v3)' })).toBeVisible();
    expect(
      within(profileSelect).getByRole('option', { name: 'Branches SNMPv2 (v2c)' }),
    ).toBeVisible();
    expect(screen.queryByText(/auth_password|priv_password|community/i)).not.toBeInTheDocument();
  });

  it('disables SNMP modes with an explanation and never loads profiles without credentials read', async () => {
    await renderPanel({ canReadCredentials: false });

    expect(screen.getByRole('radio', { name: 'Prometheus' })).toBeChecked();
    expect(screen.getByRole('radio', { name: 'Prometheus with SNMP fallback' })).toBeDisabled();
    expect(screen.getByRole('radio', { name: 'SNMP' })).toBeDisabled();
    expect(screen.getByText(/credentials:read permission is required/i)).toBeVisible();
    expect(fetchSNMPProfilesMock).not.toHaveBeenCalled();
  });

  it('invalidates an SNMP configuration when credentials read is revoked', async () => {
    const rendered = await renderPanel();
    await chooseFile();
    await click(screen.getByRole('radio', { name: 'SNMP' }));
    await change(screen.getByRole('combobox', { name: 'SNMP Profile' }), profiles[0].id);
    expect(screen.getByRole('button', { name: 'Preview import' })).toBeEnabled();

    await act(async () => {
      rendered.rerender(<DeviceImportPanel canReadCredentials={false} />);
    });

    expect(screen.getByRole('radio', { name: 'SNMP' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Preview import' })).toBeDisabled();
  });

  it('requires a file and accessible saved map, leaving map empty when no primary map is available', async () => {
    fetchCanvasMapsMock.mockResolvedValue([secondaryMap]);
    await renderPanel();

    const previewButton = screen.getByRole('button', { name: 'Preview import' });
    expect(screen.getByRole('combobox', { name: 'Destination map' })).toHaveValue('');
    expect(previewButton).toBeDisabled();

    await chooseFile();
    expect(previewButton).toBeDisabled();
    await change(screen.getByRole('combobox', { name: 'Destination map' }), secondaryMap.id);
    expect(previewButton).toBeEnabled();
  });

  it('clears and reloads the optional map-local area when destination map changes', async () => {
    await renderPanel();
    const areaSelect = await screen.findByRole('combobox', { name: 'Map area (optional)' });
    await change(areaSelect, primaryArea.id);
    expect(areaSelect).toHaveValue(primaryArea.id);

    await change(screen.getByRole('combobox', { name: 'Destination map' }), secondaryMap.id);
    expect(areaSelect).toHaveValue('');
    expect(fetchCanvasMapAreasMock).toHaveBeenCalledWith(secondaryMap.id);
    expect(await screen.findByRole('option', { name: 'Branches' })).toBeVisible();
  });

  it('requires a profile only for fallback and pure SNMP modes', async () => {
    await renderPanel();
    await chooseFile();
    const previewButton = screen.getByRole('button', { name: 'Preview import' });
    expect(previewButton).toBeEnabled();

    await click(screen.getByRole('radio', { name: 'Prometheus with SNMP fallback' }));
    expect(previewButton).toBeDisabled();
    await change(screen.getByRole('combobox', { name: 'SNMP Profile' }), profiles[0].id);
    expect(previewButton).toBeEnabled();

    await click(screen.getByRole('radio', { name: 'SNMP' }));
    expect(previewButton).toBeEnabled();
    await click(screen.getByRole('radio', { name: 'Prometheus' }));
    expect(screen.queryByRole('combobox', { name: 'SNMP Profile' })).not.toBeInTheDocument();
    expect(previewButton).toBeEnabled();
  });

  it('prevents duplicate previews and renders ordered counters, diagnostics, and target rows', async () => {
    let resolvePreview: ((value: DeviceImportPreview) => void) | undefined;
    previewDeviceImportMock.mockReturnValue(
      new Promise((resolve) => {
        resolvePreview = resolve;
      }),
    );
    await renderPanel();
    await chooseFile();
    const previewButton = screen.getByRole('button', { name: 'Preview import' });

    await act(async () => {
      fireEvent.click(previewButton);
      fireEvent.click(previewButton);
    });
    expect(previewDeviceImportMock).toHaveBeenCalledTimes(1);
    expect(previewButton).toBeDisabled();
    expect(previewButton).toHaveTextContent('Previewing');

    await act(async () => {
      resolvePreview?.(previewFixture());
    });

    const summary = screen.getByTestId('device-import-preview-summary');
    expect(summary).toHaveTextContent('Ready1');
    expect(summary).toHaveTextContent('Invalid1');
    expect(screen.getByText('file-SD group is missing targets')).toBeVisible();
    const rows = screen.getAllByTestId('device-import-preview-row');
    expect(rows.map((row) => within(row).getByTestId('target-value').textContent)).toEqual([
      'ready.example.net:9100',
      'existing.example.net:9100',
      'bad target',
      'ready.example.net:9100',
    ]);
    expect(screen.getByRole('button', { name: 'Commit import' })).toBeEnabled();
  });

  it.each([
    'file',
    'mode',
    'profile',
    'map',
    'area',
  ] as const)('invalidates a preview when the %s configuration changes', async (field) => {
    await renderPanel();
    await chooseFile();
    await click(screen.getByRole('radio', { name: 'Prometheus with SNMP fallback' }));
    await change(screen.getByRole('combobox', { name: 'SNMP Profile' }), profiles[0].id);
    await change(screen.getByRole('combobox', { name: 'Map area (optional)' }), primaryArea.id);
    await click(screen.getByRole('button', { name: 'Preview import' }));
    expect(await screen.findByTestId('device-import-preview-summary')).toBeVisible();
    await click(screen.getByRole('button', { name: 'Back to configuration' }));

    if (field === 'file') {
      await chooseFile(uploadFile('replacement.yml'));
    } else if (field === 'mode') {
      await click(screen.getByRole('radio', { name: 'SNMP' }));
    } else if (field === 'profile') {
      await change(screen.getByRole('combobox', { name: 'SNMP Profile' }), profiles[1].id);
    } else if (field === 'map') {
      await change(screen.getByRole('combobox', { name: 'Destination map' }), secondaryMap.id);
    } else {
      await change(screen.getByRole('combobox', { name: 'Map area (optional)' }), '');
    }

    expect(screen.queryByTestId('device-import-preview-summary')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Preview import' })).toBeVisible();
  });

  it('disables commit without ready rows', async () => {
    previewDeviceImportMock.mockResolvedValue(
      previewFixture({
        summary: {
          ...previewFixture().summary,
          ready: 0,
        },
        targets: previewFixture().targets.map((target) => ({
          ...target,
          status: 'skipped_existing',
        })),
      }),
    );
    await renderPanel();
    await chooseFile();
    await click(screen.getByRole('button', { name: 'Preview import' }));

    expect(await screen.findByRole('button', { name: 'Commit import' })).toBeDisabled();
    expect(screen.getByText(/No ready targets can be committed/i)).toBeVisible();
  });

  it('keeps partial error rows, uses the same File and digest, opens the map, and resets', async () => {
    const onOpenMap = vi.fn();
    commitDeviceImportMock.mockRejectedValueOnce(
      new DeviceImportPartialCommitError('device import store unavailable', commitFixture()),
    );
    await renderPanel({ onOpenMap });
    const file = await chooseFile();
    await change(screen.getByRole('combobox', { name: 'Map area (optional)' }), primaryArea.id);
    await click(screen.getByRole('button', { name: 'Preview import' }));
    await click(await screen.findByRole('button', { name: 'Commit import' }));

    expect(commitDeviceImportMock).toHaveBeenCalledTimes(1);
    const [configuration, digest] = commitDeviceImportMock.mock.calls[0];
    expect(configuration).toMatchObject({
      metrics_mode: 'prometheus',
      map_id: primaryMap.id,
      area_id: primaryArea.id,
    });
    expect((configuration as DeviceImportConfiguration).file).toBe(file);
    expect(digest).toBe('sha256:preview-digest');

    expect(await screen.findByRole('alert')).toHaveTextContent('device import store unavailable');
    expect(await screen.findByText(/Import incomplete/i)).toBeVisible();
    const rows = screen.getAllByTestId('device-import-result-row');
    expect(rows.map((row) => within(row).getByTestId('result-status').textContent)).toEqual([
      'Created',
      'Skipped existing',
      'Failed',
      'Not processed',
    ]);
    expect(screen.getByText('target failed')).toBeVisible();
    expect(screen.getByText('import stopped')).toBeVisible();
    expect(screen.getByText('ignored malformed group')).toBeVisible();

    await click(screen.getByRole('button', { name: 'Open destination map' }));
    expect(onOpenMap).toHaveBeenCalledWith(primaryMap);

    await click(screen.getByRole('button', { name: 'Reset import' }));
    expect(screen.queryByTestId('device-import-result-summary')).not.toBeInTheDocument();
    expect(screen.getByLabelText('Prometheus file-SD YAML')).toHaveValue('');
    expect(screen.getByRole('radio', { name: 'Prometheus' })).toBeChecked();
    expect(screen.getByRole('combobox', { name: 'Destination map' })).toHaveValue(primaryMap.id);
  });
});
