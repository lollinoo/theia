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
const SELF_LINK_TEST_DEVICE_NAME = 'Editable self-link e2e target';
const ROUTE_TEST_DEVICE_NAMES = new Set([ROUTE_TEST_DEVICE_NAME, SELF_LINK_TEST_DEVICE_NAME]);

interface EditableRouteFixture {
  sourceDeviceId: string;
  targetDeviceId: string;
  linkId: string;
  routeMapId: string;
  isolationMapId: string;
}

interface EditableSelfLinkFixture {
  deviceId: string;
  linkId: string;
  mapId: string;
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
      typeof device.attributes?.tags?.display_name === 'string' &&
      ROUTE_TEST_DEVICE_NAMES.has(device.attributes.tags.display_name),
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

async function createEditableSelfLinkFixture(page: Page): Promise<EditableSelfLinkFixture> {
  const headers = await csrfHeaders(page);
  const deviceResponse = await page.request.post('/api/v1/devices', {
    headers,
    data: {
      hostname: SELF_LINK_TEST_DEVICE_NAME,
      ip: '127.0.10.23',
      snmp: { version: '2c', community: 'public' },
      tags: { display_name: SELF_LINK_TEST_DEVICE_NAME },
      skip_primary_map_membership: true,
    },
  });
  expect(deviceResponse.ok(), `device creation returned ${deviceResponse.status()}`).toBe(true);
  const devicePayload = (await deviceResponse.json()) as { data?: { id?: unknown } };
  expect(devicePayload.data?.id).toEqual(expect.any(String));
  const deviceId = devicePayload.data?.id as string;

  const linkResponse = await page.request.post('/api/v1/links', {
    headers,
    data: {
      source_device_id: deviceId,
      source_if_name: 'loop-e2e0',
      target_device_id: deviceId,
      target_if_name: 'loop-e2e1',
    },
  });
  expect(linkResponse.ok(), `self-link creation returned ${linkResponse.status()}`).toBe(true);
  const linkPayload = (await linkResponse.json()) as { data?: { id?: unknown } };
  expect(linkPayload.data?.id).toEqual(expect.any(String));
  const linkId = linkPayload.data?.id as string;

  const mapId = await createFixtureMap(page, TEST_MAP_NAME, [deviceId]);
  const positionsResponse = await page.request.put(
    `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/positions`,
    {
      headers,
      data: { positions: [{ device_id: deviceId, x: 420, y: 300, pinned: true }] },
    },
  );
  expect(positionsResponse.ok(), `position save returned ${positionsResponse.status()}`).toBe(true);
  const topologyResponse = await page.request.get(
    `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/topology`,
  );
  expect(topologyResponse.ok()).toBeTruthy();
  const topology = (await topologyResponse.json()) as {
    devices?: Array<{ id?: unknown }>;
    links?: Array<{ id?: unknown }>;
  };
  expect(topology.devices?.map((device) => device.id)).toContain(deviceId);
  expect(topology.links?.map((link) => link.id)).toContain(linkId);

  return { deviceId, linkId, mapId };
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

function visibleLinkHitPathById(page: Page, linkId: string): Locator {
  return page.locator(`.react-flow__edge[data-id="${linkId}"] path.cursor-pointer:visible`).first();
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

async function exposedPathScreenPoint(
  path: Locator,
  ratios = [0.05, 0.95, 0.1, 0.9, 0.15, 0.85, 0.2, 0.8, 0.25, 0.75, 0.3, 0.7],
): Promise<ScreenPoint> {
  return path.evaluate((element, candidateRatios) => {
    const svgPath = element as SVGPathElement;
    const edge = svgPath.closest('.react-flow__edge');
    const matrix = svgPath.getScreenCTM();
    if (!edge || !matrix) {
      throw new Error('The link path did not expose an edge or screen transform');
    }
    for (const ratio of candidateRatios) {
      const point = svgPath.getPointAtLength(svgPath.getTotalLength() * ratio);
      const screenPoint = new DOMPoint(point.x, point.y).matrixTransform(matrix);
      const hitTarget = document.elementFromPoint(screenPoint.x, screenPoint.y);
      if (hitTarget && edge.contains(hitTarget)) {
        return { x: screenPoint.x, y: screenPoint.y };
      }
    }
    throw new Error('The link path had no exposed screen point');
  }, ratios);
}

async function selectLinkAtPathRatio(page: Page, path: Locator, ratio = 0.5) {
  const point = await pathScreenPoint(path, ratio);
  await page.mouse.click(point.x, point.y);
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

async function expectAutomaticPathAnchoredToNodeHandles(
  path: Locator,
  sourceNode: Locator,
  targetNode: Locator,
) {
  const [source, target] = await path.evaluate((element) => {
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
    ] as const;
  });
  const handleBoxes = (node: Locator) =>
    node.locator('.react-flow__handle').evaluateAll((handles) =>
      handles
        .map((handle) => {
          const { x, y, width, height } = handle.getBoundingClientRect();
          return { x, y, width, height };
        })
        .filter((box) => box.width > 0 && box.height > 0),
    );
  const [sourceHandleBoxes, targetHandleBoxes] = await Promise.all([
    handleBoxes(sourceNode),
    handleBoxes(targetNode),
  ]);
  const endpointIsOnHandle = (
    point: ScreenPoint,
    boxes: Array<{ x: number; y: number; width: number; height: number }>,
  ) =>
    boxes.some(
      (box) =>
        point.x >= box.x - 2 &&
        point.x <= box.x + box.width + 2 &&
        point.y >= box.y - 2 &&
        point.y <= box.y + box.height + 2,
    );

  expect(sourceHandleBoxes).not.toHaveLength(0);
  expect(targetHandleBoxes).not.toHaveLength(0);
  expect(endpointIsOnHandle(source, sourceHandleBoxes)).toBe(true);
  expect(endpointIsOnHandle(target, targetHandleBoxes)).toBe(true);
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
  await expectAutomaticPathAnchoredToNodeHandles(hitPath, sourceNode, targetNode);

  const snapToggle = page.getByRole('button', { name: /Snap to grid: (On|Off)/ });
  await expect(snapToggle).toHaveCount(0);

  const editMode = page.getByTitle('Edit Mode (E)');
  await editMode.click();
  await expect(editMode).toHaveClass(/bg-primary\/12/);
  await expect(snapToggle).toHaveCount(1);

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
  const glyphShaping = await snapIcon.evaluate(async (element) => {
    await document.fonts.ready;
    const range = document.createRange();
    range.selectNodeContents(element);
    const glyphWidth = range.getBoundingClientRect().width;
    const styles = getComputedStyle(element);
    const literalProbe = document.createElement('span');
    literalProbe.textContent = element.textContent;
    literalProbe.style.cssText = [
      'position:fixed',
      'left:-10000px',
      'top:0',
      'white-space:nowrap',
      `font-size:${styles.fontSize}`,
      'font-family:monospace',
      'font-feature-settings:"liga" 0',
    ].join(';');
    document.body.append(literalProbe);
    range.selectNodeContents(literalProbe);
    const literalWidth = range.getBoundingClientRect().width;
    literalProbe.remove();
    range.detach();
    return { glyphWidth, literalWidth };
  });
  expect(glyphShaping.glyphWidth).toBeGreaterThan(0);
  expect(glyphShaping.literalWidth).toBeGreaterThan(glyphShaping.glyphWidth * 2);
  const initialSnapState = await snapToggle.getAttribute('aria-pressed');
  expect(initialSnapState === 'true' || initialSnapState === 'false').toBe(true);
  await snapToggle.click();
  await expect(snapToggle).toHaveAttribute(
    'aria-pressed',
    initialSnapState === 'true' ? 'false' : 'true',
  );
  await snapToggle.click();
  await expect(snapToggle).toHaveAttribute('aria-pressed', initialSnapState as string);

  await selectLinkAtPathRatio(page, hitPath);

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
  await selectLinkAtPathRatio(page, hitPath);
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
  await selectLinkAtPathRatio(page, hitPath);
  waypoint = page.getByRole('button', { name: /Move waypoint 1 for link/ });
  await expect(waypoint).toBeVisible();
  await expect
    .poll(() => waypoint.evaluate((element) => (element as HTMLElement).style.transform))
    .toBe(waypointAfterEditing);
  await waitForPathToSettle(hitPath);
  await expectPathAnchoredToNodeBorders(page, hitPath, sourceNode, targetNode);

  await openMap(page, DUPLICATE_TEST_MAP_NAME);
  await waitForPersistedPositions(page, fixture.isolationMapId, [
    fixture.sourceDeviceId,
    fixture.targetDeviceId,
  ]);
  hitPath = visibleLinkHitPath(page);
  await expect(hitPath).toBeVisible();
  await selectLinkAtPathRatio(page, hitPath);
  await expect(page.getByRole('button', { name: /Move waypoint/ })).toHaveCount(0);

  await openMap(page, TEST_MAP_NAME);
  hitPath = visibleLinkHitPath(page);
  await expect(hitPath).toBeVisible();
  await selectLinkAtPathRatio(page, hitPath);
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
  await selectLinkAtPathRatio(page, hitPath);
  await expect(page.getByRole('button', { name: /Move waypoint/ })).toHaveCount(0);
  await expect(hitPath).not.toHaveAttribute('d', movedManualPath);
});

test('edits, reloads, and resets a saved self-link route', async ({ page }) => {
  const fixture = await createEditableSelfLinkFixture(page);
  await page.goto('/');
  await openMap(page, TEST_MAP_NAME);
  await waitForPersistedPositions(page, fixture.mapId, [fixture.deviceId]);

  const editMode = page.getByTitle('Edit Mode (E)');
  await editMode.click();
  let hitPath = visibleLinkHitPathById(page, fixture.linkId);
  await expect(hitPath).toBeVisible();
  const automaticPath = await waitForPathToSettle(hitPath);
  const exposedLoopPoint = await exposedPathScreenPoint(hitPath);

  const firstRouteSave = page.waitForResponse((response) =>
    linkRouteMutation(response, fixture.mapId, fixture.linkId, 'PUT'),
  );
  await page.mouse.dblclick(exposedLoopPoint.x, exposedLoopPoint.y);
  expect((await firstRouteSave).ok()).toBe(true);
  const edge = hitPath.locator(
    'xpath=ancestor::*[contains(concat(" ", normalize-space(@class), " "), " react-flow__edge ")][1]',
  );
  await expect(edge).toHaveClass(/selected/);

  let waypoint = page.getByRole('button', {
    name: `Move waypoint 1 for link ${fixture.linkId}`,
    exact: true,
  });
  await expect(waypoint).toBeVisible();
  const waypointTransform = await waypoint.evaluate(
    (element) => (element as HTMLElement).style.transform,
  );
  const manualPath = await waitForPathToSettle(hitPath);
  expect(manualPath).not.toBe(automaticPath);

  await page.reload();
  await openMap(page, TEST_MAP_NAME);
  await page.getByTitle('Edit Mode (E)').click();
  hitPath = visibleLinkHitPathById(page, fixture.linkId);
  await expect(hitPath).toBeVisible();
  await waitForPathToSettle(hitPath);
  const reloadedLoopPoint = await exposedPathScreenPoint(hitPath);
  await page.mouse.click(reloadedLoopPoint.x, reloadedLoopPoint.y);
  waypoint = page.getByRole('button', {
    name: `Move waypoint 1 for link ${fixture.linkId}`,
    exact: true,
  });
  await expect(waypoint).toBeVisible();
  await expect
    .poll(() => waypoint.evaluate((element) => (element as HTMLElement).style.transform))
    .toBe(waypointTransform);
  expect(await waitForPathToSettle(hitPath)).toContain(' L ');

  const closeDetails = page
    .getByRole('heading', { name: 'Link Details', exact: true })
    .locator('xpath=../..')
    .getByTitle('Close');
  await closeDetails.click();
  await expect(closeDetails).not.toBeInViewport();
  await page.locator('.react-flow__pane').click({ position: { x: 40, y: 260 } });
  await expect(edge).not.toHaveClass(/selected/);
  await expect(page.getByRole('button', { name: /Move waypoint/ })).toHaveCount(0);
  const contextPoint = await exposedPathScreenPoint(hitPath);
  await page.mouse.click(contextPoint.x, contextPoint.y, { button: 'right' });
  const resetRoute = page.getByRole('button', { name: 'Reset automatic route', exact: true });
  await expect(resetRoute).toBeVisible();
  const resetResponse = page.waitForResponse((response) =>
    linkRouteMutation(response, fixture.mapId, fixture.linkId, 'DELETE'),
  );
  await resetRoute.click();
  expect((await resetResponse).ok()).toBe(true);
  await expect(page.getByRole('button', { name: /Move waypoint/ })).toHaveCount(0);

  await page.reload();
  await openMap(page, TEST_MAP_NAME);
  await page.getByTitle('Edit Mode (E)').click();
  hitPath = visibleLinkHitPathById(page, fixture.linkId);
  await expect(hitPath).toBeVisible();
  await waitForPathToSettle(hitPath);
  const resetLoopPoint = await exposedPathScreenPoint(hitPath);
  await page.mouse.click(resetLoopPoint.x, resetLoopPoint.y);
  await expect(page.getByRole('button', { name: /Move waypoint/ })).toHaveCount(0);
  expect(await waitForPathToSettle(hitPath)).not.toContain(' L ');
});
