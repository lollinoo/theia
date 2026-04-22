import { describe, expect, it } from 'vitest';

import config from '../playwright.config';

describe('playwright backend webServer', () => {
  it('uses explicit sqlite small-install settings for the e2e backend', () => {
    const backendServer = config.webServer?.[0];

    expect(backendServer?.command).toContain('THEIA_DB_DRIVER=sqlite');
    expect(backendServer?.command).toContain('THEIA_ALLOW_SQLITE_SMALL_INSTALL=true');
  });
});
