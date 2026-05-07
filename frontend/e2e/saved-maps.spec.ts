import { expect, test } from '@playwright/test';

test('creates, opens, duplicates, and deletes a saved map', async ({ page }) => {
  await page.goto('/');

  await page.getByLabel('Topology Hub').click();
  await page.getByRole('button', { name: 'Create map from area Backbone', exact: true }).click();
  const createMapDialog = page.getByRole('dialog', { name: 'Create map' });
  await createMapDialog.getByLabel('Map name').fill('Backbone e2e');
  await createMapDialog.getByRole('button', { name: 'Create map', exact: true }).click();
  await expect(page.getByLabel(/Select topology map/)).toContainText('Backbone e2e');

  await page.getByLabel(/Select topology map/).click();
  await page.getByRole('button', { name: 'Manage maps' }).click();
  await page.getByRole('button', { name: 'Duplicate Backbone e2e', exact: true }).click();
  const duplicateMapDialog = page.getByRole('dialog', { name: 'Duplicate map' });
  await duplicateMapDialog.getByLabel('Map name').fill('Copy of Backbone e2e');
  await duplicateMapDialog.getByRole('button', { name: 'Duplicate map', exact: true }).click();
  await expect(page.getByLabel(/Select topology map/)).toContainText('Copy of Backbone e2e');

  await page.getByLabel(/Select topology map/).click();
  await page.getByRole('button', { name: 'Manage maps' }).click();
  page.once('dialog', (dialog) => dialog.accept());
  await page.getByRole('button', { name: 'Delete Copy of Backbone e2e' }).click();
  await expect(page.getByText('Copy of Backbone e2e')).toHaveCount(0);
});
