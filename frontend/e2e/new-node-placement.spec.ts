/**
 * Exercises viewport-aware new-node placement in the real topology canvas.
 * Keeps browser pan, zoom, creation, containment, and cleanup behavior covered together.
 */
import { expect, type Locator, type Page, test } from '@playwright/test';

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

async function cleanupCreatedDevice(page: Page, deviceId: string): Promise<Error | undefined> {
  try {
    const csrfCookie = (await page.context().cookies()).find(
      (cookie) => cookie.name === 'theia_csrf',
    );
    if (!csrfCookie) {
      return new Error('The authenticated browser context did not contain theia_csrf');
    }

    const deleteResponse = await page.request.delete(
      `/api/v1/devices/${encodeURIComponent(deviceId)}`,
      {
        headers: { 'X-CSRF-Token': csrfCookie.value },
      },
    );
    if (!deleteResponse.ok()) {
      return new Error(
        `Device cleanup returned ${deleteResponse.status()}: ${await deleteResponse.text()}`,
      );
    }
  } catch (error) {
    return error instanceof Error ? error : new Error(String(error));
  }
}

test('keeps a new virtual node inside the panned and zoomed viewport', async ({
  page,
}, testInfo) => {
  const deviceName = `Viewport placement ${Date.now()}-${testInfo.workerIndex}`;
  let createdDeviceId: string | undefined;
  let primaryFailure: unknown;
  let cleanupFailure: Error | undefined;

  try {
    await page.goto('/');

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
  } catch (error) {
    primaryFailure = error;
  } finally {
    if (createdDeviceId) {
      cleanupFailure = await cleanupCreatedDevice(page, createdDeviceId);
    }
  }

  if (primaryFailure !== undefined) {
    if (cleanupFailure) {
      testInfo.annotations.push({
        type: 'cleanup-error',
        description: cleanupFailure.message,
      });
    }
    throw primaryFailure;
  }
  if (cleanupFailure) {
    throw cleanupFailure;
  }
});
