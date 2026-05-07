import { type APIRequestContext, expect, test } from '@playwright/test';

const TEST_MAP_NAME = 'Backbone e2e';
const DUPLICATE_TEST_MAP_NAME = `Copy of ${TEST_MAP_NAME}`;
const TEST_MAP_NAMES = new Set([TEST_MAP_NAME, DUPLICATE_TEST_MAP_NAME]);

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

async function cleanupTestMaps(request: APIRequestContext) {
  const maps = await getTestMaps(request);

  for (const map of maps) {
    const response = await request.delete(`/api/v1/canvas/maps/${encodeURIComponent(map.id)}`);
    expect(response.ok()).toBeTruthy();
  }

  await expect.poll(async () => getTestMaps(request)).toEqual([]);
}

test.beforeEach(async ({ page }) => {
  await cleanupTestMaps(page.request);
});

test.afterEach(async ({ page }) => {
  await cleanupTestMaps(page.request);
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
  page.once('dialog', (dialog) => dialog.accept());
  await page.getByRole('button', { name: `Delete ${DUPLICATE_TEST_MAP_NAME}` }).click();
  await expect(page.getByText(DUPLICATE_TEST_MAP_NAME)).toHaveCount(0);
});
