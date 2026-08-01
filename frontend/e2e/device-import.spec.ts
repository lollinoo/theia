/**
 * Exercises the one-time Admin Area node import against the real backend and PostgreSQL store.
 */
import { fileURLToPath } from 'node:url';
import { expect, type Page, test } from '@playwright/test';

const TEST_ADDRESSES: string[] = ['192.0.2.241', '192.0.2.242', '192.0.2.243', '192.0.2.244'];
const TEST_TARGETS = TEST_ADDRESSES.map((address) => `${address}:9100`);
const TEST_MAP_NAME = 'Device import e2e map';
const TEST_PROFILE_NAME = 'Device import e2e SNMP profile';
const LINKED_LAYOUT_MAP_NAME = 'Device import linked layout e2e map';
const LINKED_LAYOUT_PROFILE_NAME = 'Device import linked layout e2e SNMP profile';
const LINKED_LAYOUT_ADDRESSES = [
  '192.0.2.221',
  '192.0.2.222',
  '192.0.2.223',
  '192.0.2.224',
  '192.0.2.225',
];
const LINKED_LAYOUT_IMPORT_ADDRESS = '192.0.2.226';
const DISABLED_FALLBACK_IMPORT_ADDRESS = '192.0.2.227';
const ALL_TEST_ADDRESSES = [
  ...TEST_ADDRESSES,
  ...LINKED_LAYOUT_ADDRESSES,
  LINKED_LAYOUT_IMPORT_ADDRESS,
  DISABLED_FALLBACK_IMPORT_ADDRESS,
];
const TEST_MAP_NAMES = [TEST_MAP_NAME, LINKED_LAYOUT_MAP_NAME];
const TEST_PROFILE_NAMES = [TEST_PROFILE_NAME, LINKED_LAYOUT_PROFILE_NAME];
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
    topology_discovery_mode?: unknown;
    effective_topology_discovery_mode?: unknown;
    topology_bootstrap_state?: unknown;
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
  links?: Array<{
    id?: unknown;
    source_device_id?: unknown;
    target_device_id?: unknown;
  }>;
  positions?: Record<string, CanvasPositionResource>;
}

interface LinkedLayoutFixture {
  mapId: string;
  deviceIds: string[];
  links: Array<{ id: string; source: string; target: string }>;
  profileId: string;
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
      !ALL_TEST_ADDRESSES.includes(device.attributes.ip) ||
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
    if (
      typeof map.name !== 'string' ||
      !TEST_MAP_NAMES.includes(map.name) ||
      map.is_default !== false ||
      typeof map.id !== 'string'
    ) {
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
    if (
      typeof profile.name !== 'string' ||
      !TEST_PROFILE_NAMES.includes(profile.name) ||
      typeof profile.id !== 'string'
    ) {
      continue;
    }
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

async function createRedactedSNMPProfile(
  page: Page,
  profileName = TEST_PROFILE_NAME,
): Promise<string> {
  const response = await page.request.post('/api/v1/snmp-profiles', {
    headers: await csrfHeaders(page),
    data: {
      name: profileName,
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

async function createLinkedLayoutFixture(page: Page): Promise<LinkedLayoutFixture> {
  const headers = await csrfHeaders(page);
  const deviceIds: string[] = [];

  for (const [index, address] of LINKED_LAYOUT_ADDRESSES.entries()) {
    const response = await page.request.post('/api/v1/devices', {
      headers,
      data: {
        hostname: `layout-node-${index + 1}`,
        ip: address,
        metrics_source: 'prometheus',
        prometheus_label_name: 'instance',
        prometheus_label_value: `${address}:9100`,
        skip_primary_map_membership: true,
      },
    });
    expect(response.ok(), `linked device creation returned ${response.status()}`).toBe(true);
    const payload = (await response.json()) as APIDataResponse<DeviceResource>;
    expect(payload.data?.id).toEqual(expect.any(String));
    deviceIds.push(payload.data?.id as string);
  }

  const mapResponse = await page.request.post('/api/v1/canvas/maps', {
    headers,
    data: {
      name: LINKED_LAYOUT_MAP_NAME,
      description: 'Dense linked map for Bootstrap-Once layout browser coverage',
      filter: { device_ids: deviceIds },
    },
  });
  expect(mapResponse.ok(), `linked map creation returned ${mapResponse.status()}`).toBe(true);
  const mapPayload = (await mapResponse.json()) as APIDataResponse<CanvasMapResource>;
  expect(mapPayload.data?.id).toEqual(expect.any(String));

  const links: LinkedLayoutFixture['links'] = [];
  for (let index = 1; index < deviceIds.length; index += 1) {
    const response = await page.request.post('/api/v1/links', {
      headers,
      data: {
        source_device_id: deviceIds[0],
        source_if_name: `ether${index}`,
        target_device_id: deviceIds[index],
        target_if_name: 'ether1',
      },
    });
    expect(response.ok(), `linked topology creation returned ${response.status()}`).toBe(true);
    const payload = (await response.json()) as APIDataResponse<{
      id?: unknown;
      source_device_id?: unknown;
      target_device_id?: unknown;
    }>;
    expect(payload.data?.id).toEqual(expect.any(String));
    links.push({
      id: payload.data?.id as string,
      source: deviceIds[0],
      target: deviceIds[index],
    });
  }

  return {
    mapId: mapPayload.data?.id as string,
    deviceIds,
    links,
    profileId: await createRedactedSNMPProfile(page, LINKED_LAYOUT_PROFILE_NAME),
  };
}

async function deviceIdsForAddresses(page: Page, addresses: string[]): Promise<string[]> {
  const response = await page.request.get('/api/v1/devices');
  expect(response.ok(), `device lookup returned ${response.status()}`).toBe(true);
  const payload = (await response.json()) as APIListResponse<DeviceResource>;

  return addresses.map((address) => {
    const device = (payload.data ?? []).find(
      (candidate) => candidate.attributes?.ip === address && typeof candidate.id === 'string',
    );
    expect(device?.id).toEqual(expect.any(String));
    return device?.id as string;
  });
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

async function expectLinksDoNotCrossUnrelatedNodes(
  page: Page,
  links: LinkedLayoutFixture['links'],
  deviceIds: string[],
): Promise<void> {
  await expect
    .poll(
      () =>
        page.evaluate(
          ({ evaluatedLinks, evaluatedDeviceIds }) => {
            const nodeRects = new Map<string, DOMRect>();
            for (const deviceId of evaluatedDeviceIds) {
              const node = document.querySelector<HTMLElement>(
                `.react-flow__node[data-id="${CSS.escape(deviceId)}"]`,
              );
              if (!node) return [`missing-node:${deviceId}`];
              nodeRects.set(deviceId, node.getBoundingClientRect());
            }

            const conflicts: string[] = [];
            for (const link of evaluatedLinks) {
              const path = document.getElementById(link.id) as SVGPathElement | null;
              const matrix = path?.getScreenCTM();
              if (!path || !matrix) return [`missing-link:${link.id}`];
              const length = path.getTotalLength();
              const sampleCount = Math.max(2, Math.ceil(length / 4));

              for (let sample = 1; sample < sampleCount; sample += 1) {
                const local = path.getPointAtLength((length * sample) / sampleCount);
                const svgPoint = path.ownerSVGElement!.createSVGPoint();
                svgPoint.x = local.x;
                svgPoint.y = local.y;
                const screen = svgPoint.matrixTransform(matrix);

                for (const [deviceId, rect] of nodeRects) {
                  if (deviceId === link.source || deviceId === link.target) continue;
                  if (
                    screen.x > rect.left + 2 &&
                    screen.x < rect.right - 2 &&
                    screen.y > rect.top + 2 &&
                    screen.y < rect.bottom - 2
                  ) {
                    conflicts.push(`${link.id}:${deviceId}`);
                    break;
                  }
                }
                if (conflicts.length > 0) break;
              }
            }
            return conflicts;
          },
          { evaluatedLinks: links, evaluatedDeviceIds: deviceIds },
        ),
      { timeout: 15_000 },
    )
    .toEqual([]);
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
  let releasePositionSave: (() => void) | undefined;
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

    await page.getByRole('radio', { name: 'SNMP Direct', exact: true }).check();
    const topologyBootstrap = page.getByRole('checkbox', {
      name: 'Discover links with LLDP/CDP (Bootstrap-Once)',
    });
    await expect(topologyBootstrap).toBeChecked();
    // This scenario isolates the legacy collision-free placement path; Bootstrap-Once
    // orchestration is covered independently because TEST-NET targets cannot answer SNMP.
    await topologyBootstrap.uncheck();
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

    const positionPath = `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/positions`;
    const positionSaveGate = new Promise<void>((resolve) => {
      releasePositionSave = resolve;
    });
    await page.route(`**${positionPath}`, async (route) => {
      if (route.request().method() !== 'PUT') {
        await route.continue();
        return;
      }
      await positionSaveGate;
      await route.continue();
    });
    const positionSaveRequest = page.waitForRequest(
      (request) => request.method() === 'PUT' && new URL(request.url()).pathname === positionPath,
    );

    await page.getByRole('button', { name: 'Commit import' }).click();
    await expect(page.getByLabel(/Select topology map/)).toContainText(TEST_MAP_NAME);
    const canvasRoot = page.getByTestId('topology-canvas-root');
    const placementOverlay = page.getByTestId('imported-node-placement-overlay');
    await expect(placementOverlay).toBeVisible();
    await positionSaveRequest;

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
    await expect(canvasRoot).toHaveCount(1);
    const allDeviceIds = [seedDeviceId, ...devices.map((device) => device.id)];
    await expect(placementOverlay).toContainText(`Arranging ${devices.length} imported nodes…`);
    await expect(canvasRoot).toHaveAttribute('aria-busy', 'true');
    const canvasGraph = page.getByTestId('topology-canvas');
    await expect(canvasGraph).toHaveCSS('opacity', '0');
    await expect(canvasGraph).toHaveCSS('visibility', 'hidden');
    await expect(canvasGraph).toHaveCSS('pointer-events', 'none');
    for (const deviceId of allDeviceIds) {
      const node = page.locator(`.react-flow__node[data-id="${deviceId}"]`);
      await expect(node).toHaveCount(1);
    }

    releasePositionSave();
    releasePositionSave = undefined;
    await expect(placementOverlay).toBeHidden();
    await expect(canvasRoot).toBeVisible();
    await expect(canvasRoot).not.toHaveAttribute('aria-busy');
    await expect(canvasGraph).toHaveCSS('opacity', '1');
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
    releasePositionSave?.();
    await cleanupTestFixtures(page);
  }
});

const linkedLayoutImportCases = [
  {
    name: 'SNMP Direct',
    radioName: 'SNMP Direct',
    metricsMode: 'snmp',
    metricsSource: 'snmp',
  },
  {
    name: 'Prometheus SNMP fallback',
    radioName: 'Prometheus with SNMP fallback',
    metricsMode: 'prometheus_snmp_fallback',
    metricsSource: 'prometheus_snmp_fallback',
  },
] as const;

for (const importCase of linkedLayoutImportCases) {
  test(`${importCase.name} Bootstrap-Once keeps links clear after layout`, async ({ page }) => {
    test.setTimeout(180_000);
    await cleanupTestFixtures(page);

    try {
      const fixture = await createLinkedLayoutFixture(page);

      await page.goto('/');
      await page.getByRole('button', { name: /User menu for/ }).click();
      await page.getByRole('menuitem', { name: 'Admin Area' }).click();
      await page.getByRole('tab', { name: 'Node Import' }).click();
      await expect(page.getByRole('heading', { name: 'One-time node import' })).toBeVisible();

      await page.getByRole('radio', { name: importCase.radioName, exact: true }).check();
      await expect(
        page.getByRole('checkbox', {
          name: 'Discover links with LLDP/CDP (Bootstrap-Once)',
        }),
      ).toBeChecked();
      await page.getByRole('combobox', { name: 'SNMP Profile' }).selectOption(fixture.profileId);
      await page.getByRole('combobox', { name: 'Destination map' }).selectOption(fixture.mapId);
      await page.getByRole('radio', { name: 'Reorganize entire map' }).check();
      await page.getByLabel('Prometheus file-SD YAML').setInputFiles({
        name: 'bootstrap-linked-layout.yml',
        mimeType: 'application/yaml',
        buffer: Buffer.from(`- targets:\n    - ${LINKED_LAYOUT_IMPORT_ADDRESS}\n`),
      });
      await page.getByRole('button', { name: 'Preview import' }).click();
      await expect(page.getByTestId('device-import-preview-row')).toHaveCount(1);
      const commitResponsePromise = page.waitForResponse(
        (response) =>
          new URL(response.url()).pathname === '/api/v1/admin/device-imports/commit' &&
          response.request().method() === 'POST',
      );
      await page.getByRole('button', { name: 'Commit import' }).click();
      const commitResponse = await commitResponsePromise;
      expect(commitResponse.ok(), `device import commit returned ${commitResponse.status()}`).toBe(
        true,
      );
      const commitPayload = (await commitResponse.json()) as {
        configuration?: {
          metrics_mode?: unknown;
          topology_bootstrap_enabled?: unknown;
          topology_layout_scope?: unknown;
        };
        topology_run_id?: unknown;
      };
      expect(commitPayload.configuration).toMatchObject({
        metrics_mode: importCase.metricsMode,
        topology_bootstrap_enabled: true,
        topology_layout_scope: 'reorganize',
      });
      expect(commitPayload.topology_run_id).toEqual(expect.any(String));
      const topologyRunId = commitPayload.topology_run_id as string;
      await expect(page.getByLabel(/Select topology map/)).toContainText(LINKED_LAYOUT_MAP_NAME);

      await expect
        .poll(() => deviceIdsForAddresses(page, [LINKED_LAYOUT_IMPORT_ADDRESS]), {
          timeout: 30_000,
        })
        .toEqual([expect.any(String)]);
      const importedDeviceIds = await deviceIdsForAddresses(page, [LINKED_LAYOUT_IMPORT_ADDRESS]);
      const allDeviceIds = [...fixture.deviceIds, ...importedDeviceIds];

      await expect
        .poll(
          async () => {
            const topology = await mapTopology(page, fixture.mapId);
            return allDeviceIds.every((deviceId) => {
              const position = topology.positions?.[deviceId];
              return (
                typeof position?.x === 'number' &&
                Number.isFinite(position.x) &&
                typeof position.y === 'number' &&
                Number.isFinite(position.y)
              );
            });
          },
          { timeout: 120_000 },
        )
        .toBe(true);
      await expect(page.getByTestId('topology-bootstrap-overlay')).toBeHidden({ timeout: 120_000 });

      const runResponse = await page.request.get(
        `/api/v1/admin/device-imports/topology-runs/${encodeURIComponent(topologyRunId)}`,
      );
      expect(runResponse.ok(), `topology run verification returned ${runResponse.status()}`).toBe(
        true,
      );
      const runSnapshot = (await runResponse.json()) as {
        run?: {
          id?: unknown;
          map_id?: unknown;
          layout_scope?: unknown;
          state?: unknown;
        };
        items?: Array<{ device_id?: unknown }>;
      };
      expect(runSnapshot.run).toMatchObject({
        id: topologyRunId,
        map_id: fixture.mapId,
        layout_scope: 'reorganize',
        state: 'completed',
      });
      expect(runSnapshot.items).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            device_id: importedDeviceIds[0],
          }),
        ]),
      );

      const devicesResponse = await page.request.get('/api/v1/devices');
      expect(devicesResponse.ok(), `device verification returned ${devicesResponse.status()}`).toBe(
        true,
      );
      const devicesPayload = (await devicesResponse.json()) as APIListResponse<DeviceResource>;
      const importedDevice = (devicesPayload.data ?? []).find(
        (device) => device.attributes?.ip === LINKED_LAYOUT_IMPORT_ADDRESS,
      );
      expect(importedDevice?.attributes).toMatchObject({
        metrics_source: importCase.metricsSource,
        topology_discovery_mode: 'bootstrap_once',
        topology_bootstrap_state: 'completed',
        ...(importCase.metricsMode === 'prometheus_snmp_fallback'
          ? {
              prometheus_label_name: 'instance',
              prometheus_label_value: LINKED_LAYOUT_IMPORT_ADDRESS,
            }
          : {}),
      });

      await page.getByRole('button', { name: 'Fit view' }).click();
      for (const deviceId of allDeviceIds) {
        await expect(page.locator(`.react-flow__node[data-id="${deviceId}"]`)).toBeVisible();
      }
      await expectNodesDoNotOverlap(page, allDeviceIds);
      await expectLinksDoNotCrossUnrelatedNodes(page, fixture.links, allDeviceIds);

      const positionedTopology = await mapTopology(page, fixture.mapId);
      const positionSnapshot = Object.fromEntries(
        allDeviceIds.map((deviceId) => [deviceId, positionedTopology.positions?.[deviceId]]),
      );
      await page.reload();
      await openSavedMap(page, LINKED_LAYOUT_MAP_NAME);
      await page.getByRole('button', { name: 'Fit view' }).click();
      const reloadedTopology = await mapTopology(page, fixture.mapId);
      for (const deviceId of allDeviceIds) {
        expect(reloadedTopology.positions?.[deviceId]).toEqual(positionSnapshot[deviceId]);
      }
      await expectNodesDoNotOverlap(page, allDeviceIds);
      await expectLinksDoNotCrossUnrelatedNodes(page, fixture.links, allDeviceIds);
    } finally {
      await cleanupTestFixtures(page);
    }
  });
}

test('Prometheus SNMP fallback opt-out keeps topology off and creates no bootstrap run', async ({
  page,
}) => {
  test.setTimeout(120_000);
  await cleanupTestFixtures(page);

  try {
    const seedDeviceId = await findSeedDeviceId(page);
    const mapId = await createTestMap(page, seedDeviceId);
    const profileId = await createRedactedSNMPProfile(page);

    await page.goto('/');
    await page.getByRole('button', { name: /User menu for/ }).click();
    await page.getByRole('menuitem', { name: 'Admin Area' }).click();
    await page.getByRole('tab', { name: 'Node Import' }).click();
    await page.getByRole('radio', { name: 'Prometheus with SNMP fallback', exact: true }).check();
    await page
      .getByRole('checkbox', { name: 'Discover links with LLDP/CDP (Bootstrap-Once)' })
      .uncheck();
    await page.getByRole('combobox', { name: 'SNMP Profile' }).selectOption(profileId);
    await page.getByRole('combobox', { name: 'Destination map' }).selectOption(mapId);
    await page.getByLabel('Prometheus file-SD YAML').setInputFiles({
      name: 'fallback-topology-disabled.yml',
      mimeType: 'application/yaml',
      buffer: Buffer.from(`- targets:\n    - ${DISABLED_FALLBACK_IMPORT_ADDRESS}\n`),
    });
    await page.getByRole('button', { name: 'Preview import' }).click();
    await expect(page.getByTestId('device-import-preview-row')).toHaveCount(1);

    const commitResponsePromise = page.waitForResponse(
      (response) =>
        new URL(response.url()).pathname === '/api/v1/admin/device-imports/commit' &&
        response.request().method() === 'POST',
    );
    await page.getByRole('button', { name: 'Commit import' }).click();
    const commitResponse = await commitResponsePromise;
    expect(commitResponse.ok(), `device import commit returned ${commitResponse.status()}`).toBe(
      true,
    );
    const commitPayload = (await commitResponse.json()) as {
      configuration?: {
        metrics_mode?: unknown;
        topology_bootstrap_enabled?: unknown;
      };
      topology_run_id?: unknown;
    };
    expect(commitPayload.configuration).toMatchObject({
      metrics_mode: 'prometheus_snmp_fallback',
      topology_bootstrap_enabled: false,
    });
    expect(commitPayload).not.toHaveProperty('topology_run_id');
    await expect(page.getByLabel(/Select topology map/)).toContainText(TEST_MAP_NAME);

    await expect
      .poll(() => deviceIdsForAddresses(page, [DISABLED_FALLBACK_IMPORT_ADDRESS]), {
        timeout: 30_000,
      })
      .toEqual([expect.any(String)]);
    const [importedDeviceId] = await deviceIdsForAddresses(page, [
      DISABLED_FALLBACK_IMPORT_ADDRESS,
    ]);

    const devicesResponse = await page.request.get('/api/v1/devices');
    expect(devicesResponse.ok(), `device verification returned ${devicesResponse.status()}`).toBe(
      true,
    );
    const devicesPayload = (await devicesResponse.json()) as APIListResponse<DeviceResource>;
    const importedDevice = (devicesPayload.data ?? []).find(
      (device) => device.attributes?.ip === DISABLED_FALLBACK_IMPORT_ADDRESS,
    );
    expect(importedDevice?.attributes).toMatchObject({
      metrics_source: 'prometheus_snmp_fallback',
      topology_discovery_mode: 'off',
      effective_topology_discovery_mode: 'off',
    });

    const activeRunResponse = await page.request.get(
      `/api/v1/admin/device-imports/topology-runs/active?map_id=${encodeURIComponent(mapId)}`,
    );
    expect(activeRunResponse.status()).toBe(204);
    await expect(page.getByTestId('topology-bootstrap-overlay')).toBeHidden();

    await expect
      .poll(async () => {
        const topology = await mapTopology(page, mapId);
        const position = importedDeviceId ? topology.positions?.[importedDeviceId] : undefined;
        return {
          positioned:
            typeof position?.x === 'number' &&
            Number.isFinite(position.x) &&
            typeof position.y === 'number' &&
            Number.isFinite(position.y),
          importedLinks: (topology.links ?? []).filter(
            (link) =>
              link.source_device_id === importedDeviceId ||
              link.target_device_id === importedDeviceId,
          ).length,
        };
      })
      .toEqual({ positioned: true, importedLinks: 0 });
  } finally {
    await cleanupTestFixtures(page);
  }
});
