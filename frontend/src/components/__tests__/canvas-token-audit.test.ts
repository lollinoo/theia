import { readFileSync } from 'fs';
import { join } from 'path';
/**
 * FOUND-06 / THEME-05 Canvas Token Audit
 * Scans 6 canvas-scope files for stale Tailwind v4 token names and hardcoded hex values.
 * All classes must use valid @theme inline tokens defined in index.css.
 */
import { describe, expect, it } from 'vitest';

const SRC_DIR = join(__dirname, '../../');

const TARGET_FILES = [
  'App.tsx',
  'components/Canvas.tsx',
  'components/canvas/CanvasOverlays.tsx',
  'components/canvas/CanvasPanels.tsx',
  'components/canvas/canvasHelpers.ts',
  'components/ReconnectBanner.tsx',
];

// Stale token patterns: old names that should be replaced with valid @theme inline tokens
const STALE_TOKEN_PATTERNS: { pattern: RegExp; replacement: string }[] = [
  { pattern: /bg-bg-canvas/, replacement: 'bg-bg' },
  { pattern: /bg-bg-surface/, replacement: 'bg-surface' },
  { pattern: /text-text-primary/, replacement: 'text-on-bg' },
  { pattern: /text-text-secondary/, replacement: 'text-on-bg-secondary' },
  { pattern: /border-border-subtle/, replacement: 'border-outline-subtle' },
  { pattern: /(?<![-\w])text-accent(?![-\w])/, replacement: 'text-primary' },
  { pattern: /border-accent/, replacement: 'border-primary' },
  { pattern: /bg-accent/, replacement: 'bg-primary' },
  { pattern: /border-t-accent/, replacement: 'border-t-primary' },
];

// Hardcoded hex values that should use CSS variable references
const HARDCODED_HEX_PATTERNS: { pattern: RegExp; description: string }[] = [
  { pattern: /'#00c853'/, description: 'status up hex' },
  { pattern: /'#ff1744'/i, description: 'status down hex' },
  { pattern: /'#ffc107'/, description: 'status probing hex' },
  { pattern: /'#657786'/, description: 'status unknown hex' },
  { pattern: /'#4a4a5e'/, description: 'connection line hex' },
  { pattern: /'#3f3f53'/, description: 'background dots hex' },
];

// Fixed palette colors that should use semantic tokens
const FIXED_PALETTE_PATTERNS: { pattern: RegExp; replacement: string }[] = [
  { pattern: /bg-yellow-900/, replacement: 'bg-warning/*' },
  { pattern: /text-yellow-200/, replacement: 'text-warning' },
  { pattern: /border-yellow-200/, replacement: 'border-warning/*' },
  { pattern: /bg-green-400/, replacement: 'bg-primary/*' },
  { pattern: /text-green-300/, replacement: 'text-primary' },
  { pattern: /border-green-500/, replacement: 'border-primary' },
  { pattern: /bg-yellow-400/, replacement: 'bg-warning/*' },
  { pattern: /text-yellow-300/, replacement: 'text-warning' },
  { pattern: /border-yellow-500/, replacement: 'border-warning' },
  { pattern: /text-yellow-400/, replacement: 'text-warning' },
  { pattern: /hover:text-yellow-300/, replacement: 'hover:text-warning' },
];

function scanFile(filePath: string, patterns: { pattern: RegExp }[]): string[] {
  const content = readFileSync(filePath, 'utf-8');
  const lines = content.split('\n');
  const violations: string[] = [];

  lines.forEach((line, lineIndex) => {
    for (const { pattern } of patterns) {
      if (pattern.test(line)) {
        const relPath = filePath.replace(SRC_DIR, '');
        violations.push(`${relPath}:${lineIndex + 1}: ${line.trim()}`);
      }
    }
  });

  return violations;
}

describe('FOUND-06 / THEME-05: No stale token names in canvas files', () => {
  it('no stale Tailwind v4 token names remain', () => {
    const violations: string[] = [];

    for (const file of TARGET_FILES) {
      const fullPath = join(SRC_DIR, file);
      violations.push(...scanFile(fullPath, STALE_TOKEN_PATTERNS));
    }

    if (violations.length > 0) {
      console.error('Stale token name violations found:\n' + violations.join('\n'));
    }
    expect(violations).toHaveLength(0);
  });
});

describe('FOUND-06: No hardcoded hex or fixed palette colors in canvas files', () => {
  it('no hardcoded hex color values remain', () => {
    const violations: string[] = [];

    for (const file of TARGET_FILES) {
      const fullPath = join(SRC_DIR, file);
      violations.push(...scanFile(fullPath, HARDCODED_HEX_PATTERNS));
    }

    if (violations.length > 0) {
      console.error('Hardcoded hex color violations found:\n' + violations.join('\n'));
    }
    expect(violations).toHaveLength(0);
  });

  it('no fixed palette colors remain', () => {
    const violations: string[] = [];

    for (const file of TARGET_FILES) {
      const fullPath = join(SRC_DIR, file);
      violations.push(...scanFile(fullPath, FIXED_PALETTE_PATTERNS));
    }

    if (violations.length > 0) {
      console.error('Fixed palette color violations found:\n' + violations.join('\n'));
    }
    expect(violations).toHaveLength(0);
  });
});
