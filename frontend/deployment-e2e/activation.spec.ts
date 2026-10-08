import { readFile } from 'node:fs/promises';
import { expect, test } from '@playwright/test';

const username = 'deployment-admin';
const password = 'DeploymentPassword42!';

test('activate with a saved recovery file and chosen credentials', async ({ page, request }) => {
  const activationURL = process.env.THEIA_ACTIVATION_URL;
  const recoveryPath = process.env.THEIA_OPERATOR_RECOVERY_FILE;
  test.skip(!activationURL || !recoveryPath, 'the install harness supplies activation material');
  if (!activationURL || !recoveryPath) return;

  await page.goto(activationURL);
  await expect(page.getByRole('heading', { name: 'Activate your Theia instance' })).toBeVisible();
  const downloadPromise = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Download recovery file' }).click();
  const download = await downloadPromise;
  await download.saveAs(recoveryPath);
  expect(await readFile(recoveryPath, 'utf8')).toContain('AGE-SECRET-KEY-');
  await page.getByLabel('Select the recovery file you saved').setInputFiles(recoveryPath);
  await page.getByLabel('Administrator username').fill(username);
  await page.getByLabel('Email', { exact: true }).fill('admin@example.test');
  await page.getByLabel('Password', { exact: true }).fill(password);
  await page.getByLabel('I saved the recovery file outside the instance host.').check();
  await page.getByRole('button', { name: 'Activate Theia', exact: true }).click();
  await expect(page.getByText('Your instance is ready.')).toBeVisible();

  const token = new URL(activationURL).hash.slice(7);
  const reuse = await request.post('/api/v1/setup/recovery', {
    headers: { Authorization: `Bearer ${token}` },
    data: {},
  });
  expect(reuse.status()).toBe(403);
  const login = await request.post('/api/v1/auth/login', {
    data: { identifier: username, password },
  });
  expect(login.status()).toBe(200);
  expect((await login.json()).authenticated).toBe(true);
  const legacy = await request.post('/api/v1/auth/login', {
    data: { identifier: 'administrator', password: 'theia' },
  });
  expect(legacy.status()).toBe(401);
});

test('the restored administrator can sign in after restart or replacement', async ({ request }) => {
  test.skip(Boolean(process.env.THEIA_ACTIVATION_URL), 'the activation test checks initial login');
  const response = await request.post('/api/v1/auth/login', {
    data: { identifier: username, password },
  });
  expect(response.status()).toBe(200);
  expect((await response.json()).authenticated).toBe(true);
  const setup = await request.post('/api/v1/setup/recovery', {
    headers: { Authorization: 'Bearer invalid' },
    data: {},
  });
  expect(setup.status()).toBe(403);
});
