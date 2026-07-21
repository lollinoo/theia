/**
 * Exercises saved maps browser workflow behavior so refactors preserve the documented contract.
 */
import {
  type APIRequestContext,
  expect,
  type Locator,
  type Page,
  type Response,
  test,
} from '@playwright/test';

const TEST_MAP_NAME = 'Backbone e2e';
const DUPLICATE_TEST_MAP_NAME = `Copy of ${TEST_MAP_NAME}`;
const TEST_MAP_NAMES = new Set([TEST_MAP_NAME, DUPLICATE_TEST_MAP_NAME]);
const ROUTE_TEST_DEVICE_NAME = 'Editable route e2e target';

interface EditableRouteFixture {
  sourceDeviceId: string;
  targetDeviceId: string;
  linkId: string;
  routeMapId: string;
  isolationMapId: string;
}

interface ScreenPoint {
  x: number;
  y: number;
}

async function getTestMaps(request: APIRequestContext) {
  const response = await request.get('/api/v1/canvas/maps');
  expect(response.ok()).toBeTruthy();
  const payload = (await response.json()) as {
    data?: Array<{ id?: unknown; name?: unknown; is_default?: unknown }>;
  };

  return (payload.data ?? []).filter(
    (map): map is { id: string; name: string; is_default: false } =>
      typeof map.id === 'string' &&
      typeof map.name === 'string' &&
      map.is_default === false &&
      TEST_MAP_NAMES.has(map.name),
  );
}

async function csrfHeaders(page: Page) {
  const cookies = await page.context().cookies('http://127.0.0.1');
  const csrfCookie = cookies.find((cookie) => cookie.name === 'theia_csrf');
  expect(csrfCookie?.value).toBeTruthy();
  return { 'X-CSRF-Token': csrfCookie?.value ?? '' };
}

async function getRouteTestDevices(request: APIRequestContext) {
  const response = await request.get('/api/v1/devices');
  expect(response.ok()).toBeTruthy();
  const payload = (await response.json()) as {
    data?: Array<{
      id?: unknown;
      attributes?: { tags?: { display_name?: unknown } };
    }>;
  };

  return (payload.data ?? []).filter(
    (device): device is { id: string; attributes: { tags: { display_name: string } } } =>
      typeof device.id === 'string' &&
      device.attributes?.tags?.display_name === ROUTE_TEST_DEVICE_NAME,
  );
}

async function cleanupTestMaps(page: Page) {
  const maps = await getTestMaps(page.request);
  const headers = await csrfHeaders(page);

  for (const map of maps) {
    const response = await page.request.delete(
      `/api/v1/canvas/maps/${encodeURIComponent(map.id)}`,
      {
        headers,
      },
    );
    expect(response.ok()).toBeTruthy();
  }

  await expect.poll(async () => getTestMaps(page.request)).toEqual([]);
}

async function cleanupRouteTestDevices(page: Page) {
  const devices = await getRouteTestDevices(page.request);
  const headers = await csrfHeaders(page);

  for (const device of devices) {
    const response = await page.request.delete(`/api/v1/devices/${encodeURIComponent(device.id)}`, {
      headers,
    });
    expect(response.ok()).toBeTruthy();
  }

  await expect.poll(async () => getRouteTestDevices(page.request)).toEqual([]);
}

async function cleanupTestFixtures(page: Page) {
  await cleanupTestMaps(page);
  await cleanupRouteTestDevices(page);
}

async function seedDeviceId(page: Page): Promise<string> {
  const response = await page.request.get('/api/v1/devices');
  expect(response.ok()).toBeTruthy();
  const payload = (await response.json()) as {
    data?: Array<{ id?: unknown; attributes?: { hostname?: unknown } }>;
  };
  const seed = (payload.data ?? []).find(
    (device) => device.attributes?.hostname === 'router-a' && typeof device.id === 'string',
  );
  expect(seed?.id).toBeTruthy();
  return seed?.id as string;
}

async function createFixtureMap(page: Page, name: string, deviceIds: string[]): Promise<string> {
  const response = await page.request.post('/api/v1/canvas/maps', {
    headers: await csrfHeaders(page),
    data: {
      name,
      filter: { device_ids: deviceIds },
    },
  });
  expect(response.ok(), `map creation returned ${response.status()}`).toBe(true);
  const payload = (await response.json()) as { data?: { id?: unknown } };
  expect(payload.data?.id).toEqual(expect.any(String));
  return payload.data?.id as string;
}

async function createEditableRouteFixture(page: Page): Promise<EditableRouteFixture> {
  const sourceDeviceId = await seedDeviceId(page);
  const headers = await csrfHeaders(page);
  const deviceResponse = await page.request.post('/api/v1/devices', {
    headers,
    data: {
      hostname: ROUTE_TEST_DEVICE_NAME,
      ip: '127.0.10.22',
      snmp: { version: '2c', community: 'public' },
      tags: { display_name: ROUTE_TEST_DEVICE_NAME },
      skip_primary_map_membership: true,
    },
  });
  expect(deviceResponse.ok(), `device creation returned ${deviceResponse.status()}`).toBe(true);
  const devicePayload = (await deviceResponse.json()) as { data?: { id?: unknown } };
  expect(devicePayload.data?.id).toEqual(expect.any(String));
  const targetDeviceId = devicePayload.data?.id as string;

  const linkResponse = await page.request.post('/api/v1/links', {
    headers,
    data: {
      source_device_id: sourceDeviceId,
      source_if_name: 'e2e0',
      target_device_id: targetDeviceId,
      target_if_name: 'e2e1',
    },
  });
  expect(linkResponse.ok(), `link creation returned ${linkResponse.status()}`).toBe(true);
  const linkPayload = (await linkResponse.json()) as { data?: { id?: unknown } };
  expect(linkPayload.data?.id).toEqual(expect.any(String));
  const linkId = linkPayload.data?.id as string;

  const deviceIds = [sourceDeviceId, targetDeviceId];
  const routeMapId = await createFixtureMap(page, TEST_MAP_NAME, deviceIds);
  const isolationMapId = await createFixtureMap(page, DUPLICATE_TEST_MAP_NAME, deviceIds);

  for (const mapId of [routeMapId, isolationMapId]) {
    const positionsResponse = await page.request.put(
      `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/positions`,
      {
        headers,
        data: {
          positions: [
            { device_id: sourceDeviceId, x: 100, y: 180, pinned: true },
            { device_id: targetDeviceId, x: 900, y: 420, pinned: true },
          ],
        },
      },
    );
    expect(positionsResponse.ok(), `position save returned ${positionsResponse.status()}`).toBe(
      true,
    );
    const topologyResponse = await page.request.get(
      `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/topology`,
    );
    expect(topologyResponse.ok()).toBeTruthy();
    const topology = (await topologyResponse.json()) as {
      devices?: Array<{ id?: unknown }>;
      links?: Array<{ id?: unknown }>;
    };
    expect(topology.devices?.map((device) => device.id)).toEqual(expect.arrayContaining(deviceIds));
    expect(topology.links?.map((link) => link.id)).toContain(linkId);
  }

  return { sourceDeviceId, targetDeviceId, linkId, routeMapId, isolationMapId };
}

async function openMap(page: Page, mapName: string) {
  const mapSelector = page.getByLabel(/Select topology map/);
  await expect(mapSelector).toBeVisible();
  await mapSelector.click();
  await page.getByRole('option', { name: mapName, exact: true }).click();
  await expect(mapSelector).toContainText(mapName);
  await expect(page.getByTestId('topology-canvas-root')).toBeVisible();
}

function visibleLinkHitPath(page: Page): Locator {
  return page.locator('.react-flow__edge path.cursor-pointer:visible').first();
}

async function pathScreenPoint(path: Locator, ratio: number): Promise<ScreenPoint> {
  return path.evaluate((element, requestedRatio) => {
    const svgPath = element as SVGPathElement;
    const matrix = svgPath.getScreenCTM();
    if (!matrix) {
      throw new Error('The link path did not expose a screen transform');
    }
    const point = svgPath.getPointAtLength(svgPath.getTotalLength() * requestedRatio);
    const screenPoint = new DOMPoint(point.x, point.y).matrixTransform(matrix);
    return { x: screenPoint.x, y: screenPoint.y };
  }, ratio);
}

async function selectLinkAtMidpoint(page: Page, path: Locator) {
  const midpoint = await pathScreenPoint(path, 0.5);
  await page.mouse.click(midpoint.x, midpoint.y);
  const edge = path.locator(
    'xpath=ancestor::*[contains(concat(" ", normalize-space(@class), " "), " react-flow__edge ")][1]',
  );
  await expect(edge).toHaveClass(/selected/);
}

async function waitForPathToSettle(path: Locator): Promise<string> {
  let previousPath: string | null = null;
  let stableReads = 0;
  await expect
    .poll(
      async () => {
        const currentPath = await path.getAttribute('d');
        if (currentPath !== null && currentPath === previousPath) {
          stableReads += 1;
        } else {
          previousPath = currentPath;
          stableReads = 0;
        }
        return stableReads;
      },
      { intervals: [100], timeout: 5_000 },
    )
    .toBeGreaterThanOrEqual(2);

  expect(previousPath).toBeTruthy();
  return previousPath as string;
}

async function waitForPersistedPositions(
  page: Page,
  mapId: string,
  deviceIds: string[],
): Promise<void> {
  await expect
    .poll(async () => {
      const response = await page.request.get(
        `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/topology`,
      );
      if (!response.ok()) return [];
      const payload = (await response.json()) as { positions?: Record<string, unknown> };
      return deviceIds.filter((deviceId) => payload.positions?.[deviceId] !== undefined);
    })
    .toEqual(deviceIds);
}

function linkRouteMutation(response: Response, mapId: string, linkId: string, method: string) {
  const url = new URL(response.url());
  return (
    response.request().method() === method &&
    url.pathname ===
      `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/link-routes/${encodeURIComponent(linkId)}`
  );
}

function savesDevicePosition(response: Response, mapId: string, deviceId: string) {
  const url = new URL(response.url());
  if (
    response.request().method() !== 'PUT' ||
    url.pathname !== `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/positions`
  ) {
    return false;
  }
  try {
    const payload = response.request().postDataJSON() as {
      positions?: Array<{ device_id?: unknown }>;
    };
    return payload.positions?.some((position) => position.device_id === deviceId) === true;
  } catch {
    return false;
  }
}

function expectOnRoundedBorder(
  point: ScreenPoint,
  box: { x: number; y: number; width: number; height: number },
  radius: number,
) {
  const right = box.x + box.width;
  const bottom = box.y + box.height;
  expect(point.x).toBeGreaterThanOrEqual(box.x - 2);
  expect(point.x).toBeLessThanOrEqual(right + 2);
  expect(point.y).toBeGreaterThanOrEqual(box.y - 2);
  expect(point.y).toBeLessThanOrEqual(bottom + 2);

  const nearestX = Math.max(box.x + radius, Math.min(point.x, right - radius));
  const nearestY = Math.max(box.y + radius, Math.min(point.y, bottom - radius));
  const cornerDistance = Math.hypot(point.x - nearestX, point.y - nearestY);
  const onStraightSide =
    Math.abs(point.x - box.x) < 2 ||
    Math.abs(point.x - right) < 2 ||
    Math.abs(point.y - box.y) < 2 ||
    Math.abs(point.y - bottom) < 2;
  expect(onStraightSide || Math.abs(cornerDistance - radius) < 2).toBe(true);
}

async function expectPathAnchoredToNodeBorders(
  page: Page,
  path: Locator,
  sourceNode: Locator,
  targetNode: Locator,
) {
  const [source, target, scale] = await path.evaluate((element) => {
    const svgPath = element as SVGPathElement;
    const matrix = svgPath.getScreenCTM();
    if (!matrix) throw new Error('The link path did not expose a screen transform');
    const length = svgPath.getTotalLength();
    const transform = (point: DOMPoint) => point.matrixTransform(matrix);
    const sourcePoint = transform(svgPath.getPointAtLength(0));
    const targetPoint = transform(svgPath.getPointAtLength(length));
    return [
      { x: sourcePoint.x, y: sourcePoint.y },
      { x: targetPoint.x, y: targetPoint.y },
      Math.hypot(matrix.a, matrix.b),
    ] as const;
  });
  const [sourceBox, targetBox] = await Promise.all([
    sourceNode.boundingBox(),
    targetNode.boundingBox(),
  ]);
  expect(sourceBox).not.toBeNull();
  expect(targetBox).not.toBeNull();
  if (!sourceBox || !targetBox) throw new Error('Endpoint nodes did not expose bounding boxes');

  expectOnRoundedBorder(source, sourceBox, 20 * scale);
  expectOnRoundedBorder(target, targetBox, 20 * scale);
  await expect(page.getByTestId('topology-canvas-root')).toBeVisible();
}

test.beforeEach(async ({ page }) => {
  await cleanupTestFixtures(page);
});

test.afterEach(async ({ page }) => {
  await cleanupTestFixtures(page);
});

test('creates, opens, duplicates, and deletes a saved map', async ({ page }) => {
  await page.goto('/');

  await page.getByLabel('Topology Hub').click();
  await page.getByRole('button', { name: 'Create map from area Backbone', exact: true }).click();
  const createMapDialog = page.getByRole('dialog', { name: 'Create map' });
  await createMapDialog.getByLabel('Map name').fill(TEST_MAP_NAME);
  await createMapDialog.getByRole('button', { name: 'Create map', exact: true }).click();
  await expect(page.getByLabel(/Select topology map/)).toContainText(TEST_MAP_NAME);

  await page.getByLabel(/Select topology map/).click();
  await page.getByRole('button', { name: 'Manage maps' }).click();
  await page.getByRole('button', { name: `Duplicate ${TEST_MAP_NAME}`, exact: true }).click();
  const duplicateMapDialog = page.getByRole('dialog', { name: 'Duplicate map' });
  await duplicateMapDialog.getByLabel('Map name').fill(DUPLICATE_TEST_MAP_NAME);
  await duplicateMapDialog.getByRole('button', { name: 'Duplicate map', exact: true }).click();
  await expect(page.getByLabel(/Select topology map/)).toContainText(DUPLICATE_TEST_MAP_NAME);

  await page.getByLabel(/Select topology map/).click();
  await page.getByRole('button', { name: 'Manage maps' }).click();
  await page.getByRole('button', { name: `Delete ${DUPLICATE_TEST_MAP_NAME}` }).click();
  const deleteMapDialog = page.getByRole('dialog', { name: 'Delete map' });
  await expect(deleteMapDialog).toContainText(DUPLICATE_TEST_MAP_NAME);
  await deleteMapDialog.getByRole('button', { name: 'Delete map', exact: true }).click();
  await expect(page.getByText(DUPLICATE_TEST_MAP_NAME)).toHaveCount(0);
});

test('edits and persists a map-local link route', async ({ page }) => {
  const fixture = await createEditableRouteFixture(page);
  await page.goto('/');
  await openMap(page, TEST_MAP_NAME);
  await waitForPersistedPositions(page, fixture.routeMapId, [
    fixture.sourceDeviceId,
    fixture.targetDeviceId,
  ]);

  const sourceNode = page.locator(`.react-flow__node[data-id="${fixture.sourceDeviceId}"]`);
  const targetNode = page.locator(`.react-flow__node[data-id="${fixture.targetDeviceId}"]`);
  await expect(sourceNode).toBeVisible();
  await expect(targetNode).toBeVisible();
  let hitPath = visibleLinkHitPath(page);
  await expect(hitPath).toBeVisible();
  await waitForPathToSettle(hitPath);
  await expectPathAnchoredToNodeBorders(page, hitPath, sourceNode, targetNode);

  const snapToggle = page.getByRole('button', { name: /Snap to grid: (On|Off)/ });
  const snapIcon = snapToggle.locator('.material-symbols-rounded', { hasText: 'grid_4x4' });
  await expect(snapIcon).toHaveCount(1);
  await expect(snapIcon).toHaveText('grid_4x4');
  expect(await snapIcon.evaluate((element) => getComputedStyle(element).fontFamily)).toContain(
    'Material Symbols Rounded',
  );
  expect(
    await page.evaluate(async () => {
      await document.fonts.ready;
      return document.fonts.check('18px "Material Symbols Rounded"');
    }),
  ).toBe(true);
  const initialSnapState = await snapToggle.getAttribute('aria-pressed');
  expect(initialSnapState === 'true' || initialSnapState === 'false').toBe(true);
  await snapToggle.click();
  await expect(snapToggle).toHaveAttribute(
    'aria-pressed',
    initialSnapState === 'true' ? 'false' : 'true',
  );
  await snapToggle.click();
  await expect(snapToggle).toHaveAttribute('aria-pressed', initialSnapState as string);

  const editMode = page.getByTitle('Edit Mode (E)');
  await editMode.click();
  await expect(editMode).toHaveClass(/bg-primary\/12/);
  await selectLinkAtMidpoint(page, hitPath);

  const firstRouteSave = page.waitForResponse((response) =>
    linkRouteMutation(response, fixture.routeMapId, fixture.linkId, 'PUT'),
  );
  const pathMidpoint = await pathScreenPoint(hitPath, 0.5);
  await page.mouse.move(pathMidpoint.x, pathMidpoint.y);
  await page.mouse.down();
  await page.mouse.move(pathMidpoint.x + 3, pathMidpoint.y);
  await page.mouse.move(pathMidpoint.x + 5, pathMidpoint.y);
  await page.mouse.move(pathMidpoint.x + 72, pathMidpoint.y + 64, { steps: 8 });
  await page.mouse.up();
  expect((await firstRouteSave).ok()).toBe(true);

  let waypoint = page.getByRole('button', { name: /Move waypoint 1 for link/ });
  await expect(waypoint).toHaveCount(1);
  await expect(waypoint).toBeVisible();
  const createdWaypointTransform = await waypoint.evaluate(
    (element) => (element as HTMLElement).style.transform,
  );
  const createdManualPath = await waitForPathToSettle(hitPath);

  const keyboardRouteSave = page.waitForResponse((response) =>
    linkRouteMutation(response, fixture.routeMapId, fixture.linkId, 'PUT'),
  );
  await waypoint.press('ArrowRight');
  expect((await keyboardRouteSave).ok()).toBe(true);
  await expect
    .poll(() => waypoint.evaluate((element) => (element as HTMLElement).style.transform))
    .not.toBe(createdWaypointTransform);
  const keyboardManualPath = await waitForPathToSettle(hitPath);
  expect(keyboardManualPath).not.toBe(createdManualPath);

  const waypointBeforeDrag = await waypoint.evaluate(
    (element) => (element as HTMLElement).style.transform,
  );
  const waypointBox = await waypoint.boundingBox();
  expect(waypointBox).not.toBeNull();
  if (!waypointBox) throw new Error('The waypoint did not expose a bounding box');
  const waypointRouteSave = page.waitForResponse((response) =>
    linkRouteMutation(response, fixture.routeMapId, fixture.linkId, 'PUT'),
  );
  await page.mouse.move(
    waypointBox.x + waypointBox.width / 2,
    waypointBox.y + waypointBox.height / 2,
  );
  await page.mouse.down();
  await page.mouse.move(
    waypointBox.x + waypointBox.width / 2 + 48,
    waypointBox.y + waypointBox.height / 2 - 36,
    { steps: 6 },
  );
  await page.mouse.up();
  expect((await waypointRouteSave).ok()).toBe(true);
  await expect
    .poll(() => waypoint.evaluate((element) => (element as HTMLElement).style.transform))
    .not.toBe(waypointBeforeDrag);

  const waypointAfterEditing = await waypoint.evaluate(
    (element) => (element as HTMLElement).style.transform,
  );
  const pathBeforeNodeMove = await waitForPathToSettle(hitPath);
  const sourceBox = await sourceNode.boundingBox();
  expect(sourceBox).not.toBeNull();
  if (!sourceBox) throw new Error('The source node did not expose a bounding box');
  const nodePositionSave = page.waitForResponse((response) =>
    savesDevicePosition(response, fixture.routeMapId, fixture.sourceDeviceId),
  );
  await page.mouse.move(sourceBox.x + 24, sourceBox.y + 24);
  await page.mouse.down();
  await page.mouse.move(sourceBox.x + 144, sourceBox.y + 114, { steps: 8 });
  await page.mouse.up();
  expect((await nodePositionSave).ok()).toBe(true);
  await expect.poll(() => hitPath.getAttribute('d')).not.toBe(pathBeforeNodeMove);
  await selectLinkAtMidpoint(page, hitPath);
  await expect
    .poll(() => waypoint.evaluate((element) => (element as HTMLElement).style.transform))
    .toBe(waypointAfterEditing);
  const movedManualPath = await waitForPathToSettle(hitPath);
  await expectPathAnchoredToNodeBorders(page, hitPath, sourceNode, targetNode);

  await page.reload();
  await openMap(page, TEST_MAP_NAME);
  hitPath = visibleLinkHitPath(page);
  await expect(hitPath).toBeVisible();
  await page.getByTitle('Edit Mode (E)').click();
  await selectLinkAtMidpoint(page, hitPath);
  waypoint = page.getByRole('button', { name: /Move waypoint 1 for link/ });
  await expect(waypoint).toBeVisible();
  await expect
    .poll(() => waypoint.evaluate((element) => (element as HTMLElement).style.transform))
    .toBe(waypointAfterEditing);
  await expect(hitPath).toHaveAttribute('d', movedManualPath);

  await openMap(page, DUPLICATE_TEST_MAP_NAME);
  await waitForPersistedPositions(page, fixture.isolationMapId, [
    fixture.sourceDeviceId,
    fixture.targetDeviceId,
  ]);
  hitPath = visibleLinkHitPath(page);
  await expect(hitPath).toBeVisible();
  await selectLinkAtMidpoint(page, hitPath);
  await expect(page.getByRole('button', { name: /Move waypoint/ })).toHaveCount(0);

  await openMap(page, TEST_MAP_NAME);
  hitPath = visibleLinkHitPath(page);
  await expect(hitPath).toBeVisible();
  await selectLinkAtMidpoint(page, hitPath);
  waypoint = page.getByRole('button', { name: /Move waypoint 1 for link/ });
  await expect(waypoint).toBeVisible();
  await expect
    .poll(() => waypoint.evaluate((element) => (element as HTMLElement).style.transform))
    .toBe(waypointAfterEditing);

  const contextPoint = await pathScreenPoint(hitPath, 0.5);
  await page.mouse.click(contextPoint.x, contextPoint.y, { button: 'right' });
  const resetRoute = page.getByRole('button', { name: 'Reset automatic route', exact: true });
  await expect(resetRoute).toBeVisible();
  const resetResponse = page.waitForResponse((response) =>
    linkRouteMutation(response, fixture.routeMapId, fixture.linkId, 'DELETE'),
  );
  await resetRoute.click();
  expect((await resetResponse).ok()).toBe(true);

  await page.reload();
  await openMap(page, TEST_MAP_NAME);
  hitPath = visibleLinkHitPath(page);
  await expect(hitPath).toBeVisible();
  await page.getByTitle('Edit Mode (E)').click();
  await selectLinkAtMidpoint(page, hitPath);
  await expect(page.getByRole('button', { name: /Move waypoint/ })).toHaveCount(0);
  await expect(hitPath).not.toHaveAttribute('d', movedManualPath);
});
