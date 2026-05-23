import { readFileSync } from 'fs';
import { join } from 'path';
import { describe, expect, it } from 'vitest';

const css = readFileSync(join(__dirname, '../../index.css'), 'utf-8').replace(/\r\n/g, '\n');

describe('font family contract', () => {
  it('uses Inter as the global UI font while keeping JetBrains Mono for technical values', () => {
    expect(css).toContain('@import "@fontsource-variable/inter";');
    expect(css).toContain('--font-display: "Inter Variable", "Inter", system-ui, sans-serif;');
    expect(css).toContain('--font-sans: "Inter Variable", "Inter", system-ui, sans-serif;');
    expect(css).toContain(
      '--font-mono: "JetBrains Mono Variable", "JetBrains Mono", ui-monospace, monospace;',
    );
  });
});
