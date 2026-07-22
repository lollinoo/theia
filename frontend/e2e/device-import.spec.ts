/**
 * Exercises the one-time Admin Area node import against the real backend and PostgreSQL store.
 */
import { fileURLToPath } from 'node:url';
import { expect, type Page, test } from '@playwright/test';

const TEST_ADDRESS = '192.0.2.241';
const TEST_TARGET = `${TEST_ADDRESS}:9100`;
const TEST_MAP_NAME = 'Device import e2e map';
const TEST_PROFILE_NAME = 'Device import e2e SNMP profile';
const TEST_SNMP_COMMUNITY = 'device-import-e2e-community';
const IGNORED_LABEL_VALUE = 'MUST_NOT_BE_IMPORTED';
const IMPORT_FIXTURE_PATH = fileURLToPath(
  new URL('./fixtures/prometheus-file-sd.yml', import.meta.url),
);

interface APIListResponse<T> {
  data?: T[];
}

interface APIDataResponse<T> {
  data?: T;
}

interface DeviceResource {
  id?: unknown;
  attributes?: {
    hostname?: unknown;
    ip?: unknown;
    vendor?: unknown;
    tags?: unknown;
    area_ids?: unknown;
    metrics_source?: unknown;
    prometheus_label_name?: unknown;
    prometheus_label_value?: unknown;
  };
}

interface CanvasMapResource {
  id?: unknown;
  name?: unknown;
  is_default?: unknown;
}

interface SNMPProfileResource {
  id?: unknown;
  name?: unknown;
  snmp?: {
    community?: unknown;
    community_set?: unknown;
  };
}

async function csrfHeaders(page: Page): Promise<Record<string, string>> {
  const cookies = await page.context().cookies('http://127.0.0.1');
  const csrfCookie = cookies.find((cookie) => cookie.name === 'theia_csrf');
  expect(csrfCookie?.value).toBeTruthy();
  return { 'X-CSRF-Token': csrfCookie?.value ?? '' };
}

async function cleanupTestFixtures(page: Page): Promise<void> {
  const headers = await csrfHeaders(page);

  const devicesResponse = await page.request.get('/api/v1/devices');
  expect(devicesResponse.ok(), `device cleanup list returned ${devicesResponse.status()}`).toBe(
    true,
  );
  const devices = (await devicesResponse.json()) as APIListResponse<DeviceResource>;
  for (const device of devices.data ?? []) {
    if (device.attributes?.ip !== TEST_ADDRESS || typeof device.id !== 'string') continue;
    const response = await page.request.delete(`/api/v1/devices/${encodeURIComponent(device.id)}`, {
      headers,
    });
    expect(response.ok(), `device cleanup returned ${response.status()}`).toBe(true);
  }

  const mapsResponse = await page.request.get('/api/v1/canvas/maps');
  expect(mapsResponse.ok(), `map cleanup list returned ${mapsResponse.status()}`).toBe(true);
  const maps = (await mapsResponse.json()) as APIListResponse<CanvasMapResource>;
  for (const map of maps.data ?? []) {
    if (map.name !== TEST_MAP_NAME || map.is_default !== false || typeof map.id !== 'string') {
      continue;
    }
    const response = await page.request.delete(
      `/api/v1/canvas/maps/${encodeURIComponent(map.id)}`,
      { headers },
    );
    expect(response.ok(), `map cleanup returned ${response.status()}`).toBe(true);
  }

  const profilesResponse = await page.request.get('/api/v1/snmp-profiles');
  expect(
    profilesResponse.ok(),
    `SNMP profile cleanup list returned ${profilesResponse.status()}`,
  ).toBe(true);
  const profiles = (await profilesResponse.json()) as APIListResponse<SNMPProfileResource>;
  for (const profile of profiles.data ?? []) {
    if (profile.name !== TEST_PROFILE_NAME || typeof profile.id !== 'string') continue;
    const response = await page.request.delete(
      `/api/v1/snmp-profiles/${encodeURIComponent(profile.id)}`,
      { headers },
    );
    expect(response.ok(), `SNMP profile cleanup returned ${response.status()}`).toBe(true);
  }
}

async function createTestMap(page: Page): Promise<string> {
  const response = await page.request.post('/api/v1/canvas/maps', {
    headers: await csrfHeaders(page),
    data: {
      name: TEST_MAP_NAME,
      description: 'Dedicated saved map for node import browser coverage',
      filter: { device_ids: [] },
    },
  });
  expect(response.ok(), `map creation returned ${response.status()}`).toBe(true);
  const payload = (await response.json()) as APIDataResponse<CanvasMapResource>;
  expect(payload.data?.id).toEqual(expect.any(String));
  return payload.data?.id as string;
}

async function createRedactedSNMPProfile(page: Page): Promise<string> {
  const response = await page.request.post('/api/v1/snmp-profiles', {
    headers: await csrfHeaders(page),
    data: {
      name: TEST_PROFILE_NAME,
      description: 'Redacted profile used by node import browser coverage',
      snmp: { version: '2c', community: TEST_SNMP_COMMUNITY },
    },
  });
  expect(response.ok(), `SNMP profile creation returned ${response.status()}`).toBe(true);
  const payload = (await response.json()) as APIDataResponse<SNMPProfileResource>;
  expect(payload.data?.id).toEqual(expect.any(String));
  expect(payload.data?.snmp?.community).toBeUndefined();
  expect(payload.data?.snmp?.community_set).toBe(true);
  return payload.data?.id as string;
}

async function importedDevice(
  page: Page,
): Promise<{ id: string; attributes: NonNullable<DeviceResource['attributes']> }> {
  const response = await page.request.get('/api/v1/devices');
  expect(response.ok(), `device verification returned ${response.status()}`).toBe(true);
  const payload = (await response.json()) as APIListResponse<DeviceResource>;
  const device = (payload.data ?? []).find(
    (candidate) => candidate.attributes?.ip === TEST_ADDRESS && typeof candidate.id === 'string',
  );
  expect(device?.id).toEqual(expect.any(String));
  expect(device?.attributes).toBeDefined();
  return {
    id: device?.id as string,
    attributes: device?.attributes as NonNullable<DeviceResource['attributes']>,
  };
}

test('imports only file-SD targets from Admin Area into a dedicated saved map', async ({
  page,
}) => {
  await cleanupTestFixtures(page);
  const revealRequests: string[] = [];
  page.on('request', (request) => {
    if (new URL(request.url()).pathname.endsWith('/reveal')) {
      revealRequests.push(request.url());
    }
  });

  try {
    const mapId = await createTestMap(page);
    const profileId = await createRedactedSNMPProfile(page);

    await page.goto('/');
    await page.getByRole('button', { name: /User menu for/ }).click();
    await page.getByRole('menuitem', { name: 'Admin Area' }).click();
    await expect(page.getByRole('heading', { name: 'Admin', exact: true })).toBeVisible();
    await page.getByRole('tab', { name: 'Node Import' }).click();
    await expect(page.getByRole('heading', { name: 'One-time node import' })).toBeVisible();

    await page.getByRole('radio', { name: 'SNMP', exact: true }).check();
    const profileSelect = page.getByRole('combobox', { name: 'SNMP Profile' });
    const profileOption = profileSelect.getByRole('option', {
      name: `${TEST_PROFILE_NAME} (v2c)`,
    });
    await expect(profileOption).toHaveCount(1);
    await expect(profileOption).toHaveAttribute('value', profileId);
    await profileSelect.selectOption(profileId);
    await expect(page.getByText(TEST_SNMP_COMMUNITY)).toHaveCount(0);

    await page.getByRole('radio', { name: 'Prometheus', exact: true }).check();
    await page.getByRole('combobox', { name: 'Destination map' }).selectOption(mapId);
    await page.getByLabel('Prometheus file-SD YAML').setInputFiles(IMPORT_FIXTURE_PATH);
    await page.getByRole('button', { name: 'Preview import' }).click();

    const previewRow = page.getByTestId('device-import-preview-row');
    await expect(previewRow).toContainText(TEST_TARGET);
    await expect(previewRow).toContainText(TEST_ADDRESS);
    await expect(previewRow).toContainText('Ready');
    await expect(page.getByText(IGNORED_LABEL_VALUE)).toHaveCount(0);

    await page.getByRole('button', { name: 'Commit import' }).click();
    await expect(page.getByRole('heading', { name: 'Import completed' })).toBeVisible();
    const resultRow = page.getByTestId('device-import-result-row');
    await expect(resultRow).toContainText(TEST_TARGET);
    await expect(resultRow).toContainText('Created');

    const device = await importedDevice(page);
    expect(device.attributes).toMatchObject({
      hostname: '',
      ip: TEST_ADDRESS,
      vendor: 'default',
      tags: {},
      area_ids: [],
      metrics_source: 'prometheus',
      prometheus_label_name: 'instance',
      prometheus_label_value: TEST_TARGET,
    });

    const topologyResponse = await page.request.get(
      `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/topology`,
    );
    expect(topologyResponse.ok(), `map topology returned ${topologyResponse.status()}`).toBe(true);
    const topology = (await topologyResponse.json()) as {
      devices?: Array<{ id?: unknown }>;
      positions?: Record<string, unknown>;
    };
    expect(topology.devices?.map((candidate) => candidate.id)).toContain(device.id);
    expect(topology.positions).not.toHaveProperty(device.id);

    expect(revealRequests).toEqual([]);
    await page.getByRole('button', { name: 'Open destination map' }).click();
    await expect(page.getByLabel(/Select topology map/)).toContainText(TEST_MAP_NAME);
    await expect(page.getByTestId('topology-canvas-root')).toBeVisible();
    await expect(page.locator(`.react-flow__node[data-id="${device.id}"]`)).toBeVisible();
  } finally {
    await cleanupTestFixtures(page);
  }
});
