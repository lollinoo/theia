/**
 * Exercises the one-time Admin Area node import against the real backend and PostgreSQL store.
 */
import { fileURLToPath } from 'node:url';
import { expect, type Page, test } from '@playwright/test';

const TEST_ADDRESSES: string[] = ['192.0.2.241', '192.0.2.242', '192.0.2.243', '192.0.2.244'];
const TEST_TARGETS = TEST_ADDRESSES.map((address) => `${address}:9100`);
const TEST_MAP_NAME = 'Device import e2e map';
const TEST_PROFILE_NAME = 'Device import e2e SNMP profile';
const TEST_SNMP_COMMUNITY = 'device-import-e2e-community';
const IGNORED_LABEL_VALUE = 'MUST_NOT_BE_IMPORTED';
const EXISTING_NODE_POSITION = { x: 80, y: 80, pinned: true };
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

interface CanvasPositionResource {
  device_id?: unknown;
  x?: unknown;
  y?: unknown;
  pinned?: unknown;
}

interface CanvasTopologyResource {
  devices?: Array<{ id?: unknown }>;
  positions?: Record<string, CanvasPositionResource>;
}

interface ImportedDeviceFixture {
  id: string;
  address: string;
  target: string;
  attributes: NonNullable<DeviceResource['attributes']>;
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
    if (
      typeof device.attributes?.ip !== 'string' ||
      !TEST_ADDRESSES.includes(device.attributes.ip) ||
      typeof device.id !== 'string'
    ) {
      continue;
    }
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

async function findSeedDeviceId(page: Page): Promise<string> {
  const response = await page.request.get('/api/v1/devices');
  expect(response.ok(), `seed device list returned ${response.status()}`).toBe(true);
  const payload = (await response.json()) as APIListResponse<DeviceResource>;
  const seedDevice = (payload.data ?? []).find(
    (device) => device.attributes?.hostname === 'router-a' && typeof device.id === 'string',
  );
  expect(seedDevice?.id).toEqual(expect.any(String));
  return seedDevice?.id as string;
}

async function createTestMap(page: Page, seedDeviceId: string): Promise<string> {
  const response = await page.request.post('/api/v1/canvas/maps', {
    headers: await csrfHeaders(page),
    data: {
      name: TEST_MAP_NAME,
      description: 'Dedicated saved map for node import browser coverage',
      filter: { device_ids: [seedDeviceId] },
    },
  });
  expect(response.ok(), `map creation returned ${response.status()}`).toBe(true);
  const payload = (await response.json()) as APIDataResponse<CanvasMapResource>;
  expect(payload.data?.id).toEqual(expect.any(String));
  const mapId = payload.data?.id as string;
  const positionResponse = await page.request.put(
    `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/positions`,
    {
      headers: await csrfHeaders(page),
      data: {
        positions: [{ device_id: seedDeviceId, ...EXISTING_NODE_POSITION }],
      },
    },
  );
  expect(
    positionResponse.ok(),
    `seed position creation returned ${positionResponse.status()}`,
  ).toBe(true);
  return mapId;
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

async function importedDevices(page: Page): Promise<ImportedDeviceFixture[]> {
  const response = await page.request.get('/api/v1/devices');
  expect(response.ok(), `device verification returned ${response.status()}`).toBe(true);
  const payload = (await response.json()) as APIListResponse<DeviceResource>;
  return TEST_ADDRESSES.map((address, index) => {
    const device = (payload.data ?? []).find(
      (candidate) => candidate.attributes?.ip === address && typeof candidate.id === 'string',
    );
    expect(device?.id).toEqual(expect.any(String));
    expect(device?.attributes).toBeDefined();
    return {
      id: device?.id as string,
      address,
      target: TEST_TARGETS[index],
      attributes: device?.attributes as NonNullable<DeviceResource['attributes']>,
    };
  });
}

async function mapTopology(page: Page, mapId: string): Promise<CanvasTopologyResource> {
  const response = await page.request.get(
    `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/topology`,
  );
  expect(response.ok(), `map topology returned ${response.status()}`).toBe(true);
  return (await response.json()) as CanvasTopologyResource;
}

async function expectNodesDoNotOverlap(page: Page, deviceIds: string[]): Promise<void> {
  await expect
    .poll(
      async () => {
        const boxes = await Promise.all(
          deviceIds.map(async (deviceId) => ({
            deviceId,
            box: await page.locator(`.react-flow__node[data-id="${deviceId}"]`).boundingBox(),
          })),
        );
        if (boxes.some(({ box }) => box === null)) {
          return 'missing-node';
        }

        for (let leftIndex = 0; leftIndex < boxes.length; leftIndex += 1) {
          for (let rightIndex = leftIndex + 1; rightIndex < boxes.length; rightIndex += 1) {
            const left = boxes[leftIndex];
            const right = boxes[rightIndex];
            if (!left.box || !right.box) continue;
            const overlapWidth =
              Math.min(left.box.x + left.box.width, right.box.x + right.box.width) -
              Math.max(left.box.x, right.box.x);
            const overlapHeight =
              Math.min(left.box.y + left.box.height, right.box.y + right.box.height) -
              Math.max(left.box.y, right.box.y);
            if (overlapWidth > 0.5 && overlapHeight > 0.5) {
              return `${left.deviceId}:${right.deviceId}`;
            }
          }
        }
        return 'none';
      },
      { timeout: 10_000 },
    )
    .toBe('none');
}

async function openSavedMap(page: Page, mapName: string): Promise<void> {
  const mapSelector = page.getByLabel(/Select topology map/);
  await expect(mapSelector).toBeVisible();
  await mapSelector.click();
  await page.getByRole('option', { name: mapName, exact: true }).click();
  await expect(mapSelector).toContainText(mapName);
}

test('imports and persists a collision-free file-SD batch in an occupied saved map', async ({
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
    const seedDeviceId = await findSeedDeviceId(page);
    const mapId = await createTestMap(page, seedDeviceId);
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

    const previewRows = page.getByTestId('device-import-preview-row');
    await expect(previewRows).toHaveCount(TEST_TARGETS.length);
    for (const target of TEST_TARGETS) {
      const previewRow = previewRows.filter({ hasText: target });
      await expect(previewRow).toHaveCount(1);
      await expect(previewRow).toContainText('Ready');
    }
    await expect(page.getByText(IGNORED_LABEL_VALUE)).toHaveCount(0);

    await page.getByRole('button', { name: 'Commit import' }).click();
    await expect(page.getByRole('heading', { name: 'Import completed' })).toBeVisible();
    const resultRows = page.getByTestId('device-import-result-row');
    await expect(resultRows).toHaveCount(TEST_TARGETS.length);
    for (const target of TEST_TARGETS) {
      const resultRow = resultRows.filter({ hasText: target });
      await expect(resultRow).toHaveCount(1);
      await expect(resultRow).toContainText('Created');
    }

    const devices = await importedDevices(page);
    for (const device of devices) {
      expect(device.attributes).toMatchObject({
        hostname: '',
        ip: device.address,
        vendor: 'default',
        tags: {},
        area_ids: [],
        metrics_source: 'prometheus',
        prometheus_label_name: 'instance',
        prometheus_label_value: device.target,
      });
    }

    const topology = await mapTopology(page, mapId);
    const topologyDeviceIds = topology.devices?.map((candidate) => candidate.id);
    expect(topologyDeviceIds).toContain(seedDeviceId);
    expect(topology.positions?.[seedDeviceId]).toMatchObject(EXISTING_NODE_POSITION);
    for (const device of devices) {
      expect(topologyDeviceIds).toContain(device.id);
      expect(topology.positions).not.toHaveProperty(device.id);
    }

    expect(revealRequests).toEqual([]);
    await page.getByRole('button', { name: 'Open destination map' }).click();
    await expect(page.getByLabel(/Select topology map/)).toContainText(TEST_MAP_NAME);
    await expect(page.getByTestId('topology-canvas-root')).toBeVisible();
    const allDeviceIds = [seedDeviceId, ...devices.map((device) => device.id)];
    for (const deviceId of allDeviceIds) {
      await expect(page.locator(`.react-flow__node[data-id="${deviceId}"]`)).toBeVisible();
    }

    await expect
      .poll(
        async () => {
          const positionedTopology = await mapTopology(page, mapId);
          return devices.every((device) => {
            const position = positionedTopology.positions?.[device.id];
            return (
              typeof position?.x === 'number' &&
              Number.isFinite(position.x) &&
              typeof position.y === 'number' &&
              Number.isFinite(position.y)
            );
          });
        },
        { timeout: 10_000 },
      )
      .toBe(true);

    const positionedTopology = await mapTopology(page, mapId);
    expect(positionedTopology.positions?.[seedDeviceId]).toMatchObject(EXISTING_NODE_POSITION);
    const importedPositionSnapshot = Object.fromEntries(
      devices.map((device) => [device.id, positionedTopology.positions?.[device.id]]),
    );
    await expectNodesDoNotOverlap(page, allDeviceIds);

    await page.reload();
    await openSavedMap(page, TEST_MAP_NAME);
    await expect(page.getByTestId('topology-canvas-root')).toBeVisible();
    for (const deviceId of allDeviceIds) {
      await expect(page.locator(`.react-flow__node[data-id="${deviceId}"]`)).toBeVisible();
    }
    const reloadedTopology = await mapTopology(page, mapId);
    expect(reloadedTopology.positions?.[seedDeviceId]).toMatchObject(EXISTING_NODE_POSITION);
    for (const device of devices) {
      expect(reloadedTopology.positions?.[device.id]).toEqual(importedPositionSnapshot[device.id]);
    }
    await expectNodesDoNotOverlap(page, allDeviceIds);
  } finally {
    await cleanupTestFixtures(page);
  }
});
