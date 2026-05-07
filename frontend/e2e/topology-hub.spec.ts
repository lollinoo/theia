import { expect, test } from '@playwright/test';

test('opens Topology Hub and returns to the global canvas', async ({ page }) => {
  await page.goto('/');

  await page.getByLabel('Topology Hub').click();

  await expect(page.getByRole('heading', { name: 'Topology Hub' })).toBeVisible();
  await expect(page.getByText('OSPF Area Hub')).toHaveCount(0);

  await page.getByRole('button', { name: /open global/i }).click();

  await expect(page.getByLabel(/Select topology map/)).toContainText('Default');
});
