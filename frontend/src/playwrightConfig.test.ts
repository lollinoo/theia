import { describe, expect, it } from 'vitest';

import config from '../playwright.config';

describe('playwright backend webServer', () => {
  it('uses PostgreSQL-only settings for the e2e backend', () => {
    const backendServer = config.webServer?.[0];

    expect(backendServer?.command).toContain('THEIA_DB_DSN=');
    expect(backendServer?.command).toContain('THEIA_DATA_DIR=');
  });
});
