/**
 * Exercises playwright config frontend behavior so refactors preserve the documented contract.
 */

import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import config from '../playwright.config';

describe('playwright backend webServer', () => {
  it('uses PostgreSQL-only settings for the e2e backend', () => {
    const backendServer = config.webServer?.[0];

    expect(backendServer?.command).toContain('THEIA_DB_DSN=');
    expect(backendServer?.command).toContain(
      'postgres://theia:theia@127.0.0.1:5432/theia?sslmode=disable',
    );
    expect(backendServer?.command).toContain('THEIA_DATA_DIR=');
    expect(backendServer?.command).toContain('THEIA_SESSION_SECRET=');
    expect(backendServer?.command).toContain('THEIA_ALLOWED_ORIGINS=http://127.0.0.1:3300');
    expect(backendServer?.url).toContain('/api/v1/auth/me');
    expect(config.use?.storageState).toBe('/tmp/theia-playwright-auth.json');
  });
});

describe('playwright global setup password', () => {
  it('uses an e2e administrator password that satisfies the active password policy', () => {
    const setupSource = readFileSync(resolve(__dirname, '../e2e/global.setup.ts'), 'utf8');
    const passwordMatch = setupSource.match(/const e2ePassword = '([^']+)'/);

    expect(passwordMatch).not.toBeNull();

    const password = passwordMatch?.[1] ?? '';

    expect([...password]).toHaveLength(password.length);
    expect([...password].length).toBeGreaterThanOrEqual(10);
    expect([...password].length).toBeLessThanOrEqual(24);
    expect(password).toMatch(/[A-Z]/);
    expect(password).toMatch(/[a-z]/);
    expect(password).toMatch(/\d/);
    expect(password).toMatch(/[^A-Za-z0-9]/);
  });
});
