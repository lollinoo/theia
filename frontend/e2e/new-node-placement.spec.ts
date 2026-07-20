/**
 * Exercises viewport-aware new-node placement in the real topology canvas.
 * Keeps browser pan, zoom, creation, containment, and cleanup behavior covered together.
 */
import { expect, type Locator, type Page, type Response, test } from '@playwright/test';

interface BoundingBox {
  x: number;
  y: number;
  width: number;
  height: number;
}

function expectInsideViewport(node: BoundingBox, viewport: BoundingBox): void {
  expect(node.x).toBeGreaterThanOrEqual(viewport.x + 15);
  expect(node.y).toBeGreaterThanOrEqual(viewport.y + 15);
  expect(node.x + node.width).toBeLessThanOrEqual(viewport.x + viewport.width - 15);
  expect(node.y + node.height).toBeLessThanOrEqual(viewport.y + viewport.height - 15);
}

async function viewportTransform(viewport: Locator): Promise<string> {
  return viewport.evaluate((element) => getComputedStyle(element).transform);
}

async function waitForViewportTransformToSettle(viewport: Locator): Promise<string> {
  await expect(viewport).toBeVisible();

  let previousTransform: string | undefined;
  let latestTransform = '';
  let stableReads = 0;
  await expect
    .poll(
      async () => {
        latestTransform = await viewportTransform(viewport);
        if (latestTransform === previousTransform) {
          stableReads += 1;
        } else {
          previousTransform = latestTransform;
          stableReads = 0;
        }
        return stableReads;
      },
      { intervals: [100], timeout: 5_000 },
    )
    .toBeGreaterThanOrEqual(2);

  return latestTransform;
}

async function visiblePaneDragStart(
  page: Page,
  paneBox: BoundingBox,
): Promise<{ x: number; y: number }> {
  const point = await page.evaluate((box) => {
    const candidateFractions = [
      { x: 0.3, y: 0.72 },
      { x: 0.7, y: 0.68 },
      { x: 0.25, y: 0.35 },
      { x: 0.75, y: 0.35 },
    ];

    for (const candidate of candidateFractions) {
      const x = box.x + box.width * candidate.x;
      const y = box.y + box.height * candidate.y;
      if (document.elementFromPoint(x, y)?.classList.contains('react-flow__pane')) {
        return { x, y };
      }
    }
    return null;
  }, paneBox);

  expect(point, 'expected a visible pane point for the drag').not.toBeNull();
  if (!point) {
    throw new Error('No visible React Flow pane point was available for dragging');
  }
  return point;
}

async function csrfHeaders(page: Page): Promise<{ 'X-CSRF-Token': string }> {
  const csrfCookie = (await page.context().cookies()).find(
    (cookie) => cookie.name === 'theia_csrf',
  );
  if (!csrfCookie) {
    throw new Error('The authenticated browser context did not contain theia_csrf');
  }
  return { 'X-CSRF-Token': csrfCookie.value };
}

async function findSeedDeviceId(page: Page): Promise<string> {
  const response = await page.request.get('/api/v1/devices');
  if (!response.ok()) {
    throw new Error(`Device list returned ${response.status()}: ${await response.text()}`);
  }

  const payload = (await response.json()) as {
    data?: Array<{ id?: unknown; attributes?: { hostname?: unknown } }>;
  };
  const seedDevice = payload.data?.find(
    (device) => device.attributes?.hostname === 'router-a' && typeof device.id === 'string',
  );
  if (typeof seedDevice?.id !== 'string' || seedDevice.id === '') {
    throw new Error('The seeded router-a device was not available');
  }
  return seedDevice.id;
}

async function createDedicatedMap(
  page: Page,
  mapName: string,
  seedDeviceId: string,
): Promise<string> {
  const response = await page.request.post('/api/v1/canvas/maps', {
    headers: await csrfHeaders(page),
    data: {
      name: mapName,
      source_area_id: null,
      filter: { device_ids: [seedDeviceId] },
    },
  });
  if (!response.ok()) {
    throw new Error(`Map creation returned ${response.status()}: ${await response.text()}`);
  }

  const payload = (await response.json()) as { data?: { id?: unknown } };
  if (typeof payload.data?.id !== 'string' || payload.data.id === '') {
    throw new Error('Map creation response did not include an id');
  }
  return payload.data.id;
}

function savesDevicePosition(response: Response, mapId: string, deviceId?: string): boolean {
  if (!deviceId) {
    return false;
  }

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
    return (
      Array.isArray(payload.positions) &&
      payload.positions.some((position) => position.device_id === deviceId)
    );
  } catch {
    return false;
  }
}

async function cleanupResource(
  page: Page,
  path: string,
  resourceName: string,
): Promise<Error | undefined> {
  try {
    const deleteResponse = await page.request.delete(path, {
      headers: await csrfHeaders(page),
    });
    if (!deleteResponse.ok()) {
      return new Error(
        `${resourceName} cleanup returned ${deleteResponse.status()}: ${await deleteResponse.text()}`,
      );
    }
  } catch (error) {
    return error instanceof Error ? error : new Error(String(error));
  }
}

test('keeps a new virtual node inside the panned and zoomed viewport', async ({
  page,
}, testInfo) => {
  const uniqueSuffix = `${Date.now()}-${testInfo.workerIndex}`;
  const mapName = `Viewport placement map ${uniqueSuffix}`;
  const deviceName = `Viewport placement ${uniqueSuffix}`;
  let dedicatedMapId: string | undefined;
  let createdDeviceId: string | undefined;
  let positionSavePromise: Promise<Response> | undefined;
  let positionSaveConfirmed = false;
  let primaryFailure: unknown;
  const cleanupFailures: Error[] = [];

  try {
    const seedDeviceId = await findSeedDeviceId(page);
    const mapId = await createDedicatedMap(page, mapName, seedDeviceId);
    dedicatedMapId = mapId;
    await page.goto('/');

    const mapSelector = page.getByLabel(/Select topology map/);
    await expect(mapSelector).toBeVisible();
    await mapSelector.click();
    await page.getByRole('option', { name: mapName, exact: true }).click();
    await expect(mapSelector).toContainText(mapName);

    const canvasRoot = page.getByTestId('topology-canvas-root');
    const viewport = page.locator('.react-flow__viewport');
    await expect(canvasRoot).toBeVisible();
    await waitForViewportTransformToSettle(viewport);

    const zoomIn = page.getByRole('button', { name: 'Zoom in' });
    await zoomIn.click();
    await waitForViewportTransformToSettle(viewport);
    await zoomIn.click();
    const zoomedTransform = await waitForViewportTransformToSettle(viewport);
    const zoomScale = await viewport.evaluate(
      (element) => new DOMMatrixReadOnly(getComputedStyle(element).transform).a,
    );
    expect(zoomScale).toBeGreaterThan(1);

    const pane = page.locator('.react-flow__pane');
    await expect(pane).toBeVisible();
    const paneBox = await pane.boundingBox();
    expect(paneBox).not.toBeNull();
    if (!paneBox) {
      throw new Error('The React Flow pane did not expose a bounding box');
    }

    const dragStart = await visiblePaneDragStart(page, paneBox);
    await page.mouse.move(dragStart.x, dragStart.y);
    await page.mouse.down();
    await page.mouse.move(dragStart.x + 120, dragStart.y - 80, { steps: 8 });
    await page.mouse.up();

    const placementTransform = await waitForViewportTransformToSettle(viewport);
    expect(placementTransform).not.toBe(zoomedTransform);

    const addDeviceButton = page.getByTitle(/Add Device/);
    await expect(addDeviceButton).toBeVisible();
    await addDeviceButton.click();
    await page.getByRole('button', { name: 'Virtual Node', exact: true }).click();
    await page.getByPlaceholder('e.g. ISP Gateway').fill(deviceName);

    positionSavePromise = page.waitForResponse(
      (response) => savesDevicePosition(response, mapId, createdDeviceId),
      { timeout: 10_000 },
    );
    // Keep creation failures from leaving the independently registered response waiter unhandled.
    void positionSavePromise.catch(() => {});
    const [createResponse] = await Promise.all([
      page.waitForResponse((response) => {
        const url = new URL(response.url());
        return response.request().method() === 'POST' && url.pathname === '/api/v1/devices';
      }),
      page.getByRole('button', { name: 'Add Virtual Node', exact: true }).click(),
    ]);
    expect(createResponse.ok(), `device creation returned ${createResponse.status()}`).toBe(true);

    const createPayload = (await createResponse.json()) as { data: { id: string } };
    createdDeviceId = createPayload.data.id;
    expect(createdDeviceId).toBeTruthy();

    const createdNode = page.locator('.react-flow__node').filter({ hasText: deviceName });
    await expect(createdNode).toHaveCount(1);
    await expect(createdNode).toBeVisible();

    const [nodeBox, canvasBox] = await Promise.all([
      createdNode.boundingBox(),
      canvasRoot.boundingBox(),
    ]);
    expect(nodeBox).not.toBeNull();
    expect(canvasBox).not.toBeNull();
    if (!nodeBox || !canvasBox) {
      throw new Error('The created node or topology canvas did not expose a bounding box');
    }

    expectInsideViewport(nodeBox, canvasBox);
    expect(await waitForViewportTransformToSettle(viewport)).toBe(placementTransform);

    const positionSaveResponse = await positionSavePromise;
    expect(
      positionSaveResponse.ok(),
      `position save returned ${positionSaveResponse.status()}`,
    ).toBe(true);
    positionSaveConfirmed = true;
  } catch (error) {
    primaryFailure = error;
  } finally {
    if (createdDeviceId && positionSavePromise && !positionSaveConfirmed) {
      try {
        const positionSaveResponse = await positionSavePromise;
        if (!positionSaveResponse.ok()) {
          cleanupFailures.push(
            new Error(`Position save returned ${positionSaveResponse.status()} before cleanup`),
          );
        }
      } catch (error) {
        cleanupFailures.push(error instanceof Error ? error : new Error(String(error)));
      }
    }
    if (createdDeviceId) {
      const cleanupFailure = await cleanupResource(
        page,
        `/api/v1/devices/${encodeURIComponent(createdDeviceId)}`,
        'Device',
      );
      if (cleanupFailure) {
        cleanupFailures.push(cleanupFailure);
      }
    }
    if (dedicatedMapId) {
      const cleanupFailure = await cleanupResource(
        page,
        `/api/v1/canvas/maps/${encodeURIComponent(dedicatedMapId)}`,
        'Map',
      );
      if (cleanupFailure) {
        cleanupFailures.push(cleanupFailure);
      }
    }
  }

  if (primaryFailure !== undefined) {
    for (const cleanupFailure of cleanupFailures) {
      testInfo.annotations.push({
        type: 'cleanup-error',
        description: cleanupFailure.message,
      });
    }
    throw primaryFailure;
  }
  if (cleanupFailures.length > 0) {
    throw new Error(cleanupFailures.map((failure) => failure.message).join('; '));
  }
});
