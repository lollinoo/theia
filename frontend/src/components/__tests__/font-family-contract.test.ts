/**
 * Exercises font family contract component behavior so refactors preserve the documented contract.
 */
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

  it('enables variable-font and legibility features for operational text', () => {
    expect(css).toContain('font-optical-sizing: auto;');
    expect(css).toContain('font-synthesis: none;');
    expect(css).toContain('text-rendering: optimizeLegibility;');
    expect(css).toContain('font-feature-settings: "cv02" 1, "cv03" 1, "cv04" 1, "cv11" 1;');
  });
});
