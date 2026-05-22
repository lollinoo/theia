import { readFileSync } from 'fs';
import { join } from 'path';
import { describe, expect, it } from 'vitest';

const css = readFileSync(join(__dirname, '../../index.css'), 'utf-8').replace(/\r\n/g, '\n');

function extractThemeBlock(marker: string): string {
  const markerIndex = css.indexOf(marker);
  if (markerIndex < 0) {
    throw new Error(`Missing theme marker ${marker}`);
  }

  const openIndex = css.indexOf('{', markerIndex);
  const closeIndex = css.indexOf('\n}', openIndex);
  if (openIndex < 0 || closeIndex < 0) {
    throw new Error(`Missing block for ${marker}`);
  }

  return css.slice(openIndex + 1, closeIndex);
}

function token(block: string, name: string): string {
  const match = block.match(new RegExp(`${name}:\\s*(#[0-9a-fA-F]{6}|var\\(--[\\w-]+\\))`));
  if (!match) {
    throw new Error(`Missing token ${name}`);
  }

  const value = match[1];
  const alias = value.match(/^var\((--[\w-]+)\)$/);
  if (!alias) {
    return value;
  }

  return token(block, alias[1]);
}

function declaration(block: string, name: string): string {
  const match = block.match(new RegExp(`${name}:\\s*([^;]+);`));
  if (!match) {
    throw new Error(`Missing declaration ${name}`);
  }

  return match[1].trim();
}

function compact(value: string): string {
  return value.replace(/\s+/g, ' ');
}

function channelToLinear(value: number): number {
  const normalized = value / 255;
  if (normalized <= 0.03928) {
    return normalized / 12.92;
  }
  return ((normalized + 0.055) / 1.055) ** 2.4;
}

function luminance(hex: string): number {
  const red = Number.parseInt(hex.slice(1, 3), 16);
  const green = Number.parseInt(hex.slice(3, 5), 16);
  const blue = Number.parseInt(hex.slice(5, 7), 16);
  return (
    0.2126 * channelToLinear(red) + 0.7152 * channelToLinear(green) + 0.0722 * channelToLinear(blue)
  );
}

function contrastRatio(foreground: string, background: string): number {
  const fg = luminance(foreground);
  const bg = luminance(background);
  const lighter = Math.max(fg, bg);
  const darker = Math.min(fg, bg);
  return (lighter + 0.05) / (darker + 0.05);
}

const darkBlock = extractThemeBlock(':root,\n[data-theme="dark"]');
const lightBlock = extractThemeBlock('[data-theme="light"]');
const tailwindThemeBlock = extractThemeBlock('@theme inline');

describe('enterprise NOC theme contrast contract', () => {
  it('resolves recursive same-block token aliases', () => {
    const block = `
      --source: #123456;
      --alias: var(--source);
      --recursive-alias: var(--alias);
    `;

    expect(token(block, '--recursive-alias')).toBe('#123456');
  });

  it('uses a warm greige light surface scale with real canvas depth', () => {
    const lightSurfaceTokens = {
      background: token(lightBlock, '--nt-bg'),
      surface: token(lightBlock, '--nt-surface'),
      surfaceContainer: token(lightBlock, '--nt-surface-container'),
      surfaceContainerHigh: token(lightBlock, '--nt-surface-container-high'),
      elevated: token(lightBlock, '--nt-elevated'),
    };

    expect(lightSurfaceTokens).toEqual({
      background: '#dde2e8',
      surface: '#f0f2f5',
      surfaceContainer: '#e8eaee',
      surfaceContainerHigh: '#f7f8fa',
      elevated: '#fbfcfd',
    });

    for (const [label, value] of Object.entries(lightSurfaceTokens)) {
      expect(value, `${label} should not be pure white`).not.toBe('#ffffff');
    }

    const backgroundLuminance = luminance(lightSurfaceTokens.background);
    const surfaceLuminance = luminance(lightSurfaceTokens.surface);
    const containerHighLuminance = luminance(lightSurfaceTokens.surfaceContainerHigh);
    const elevatedLuminance = luminance(lightSurfaceTokens.elevated);

    expect(
      backgroundLuminance,
      'light canvas should move away from near-white glare',
    ).toBeLessThanOrEqual(0.77);
    expect(
      surfaceLuminance - backgroundLuminance,
      'nodes need clear depth over canvas',
    ).toBeGreaterThanOrEqual(0.12);
    expect(
      containerHighLuminance - backgroundLuminance,
      'raised layers need visible separation from canvas',
    ).toBeGreaterThanOrEqual(0.18);
    expect(
      elevatedLuminance - containerHighLuminance,
      'raised surfaces should stay subtle',
    ).toBeLessThanOrEqual(0.04);
  });

  it('keeps light-mode chrome layered without washing out controls', () => {
    expect(token(lightBlock, '--nt-outline')).toBe('#bcc8d6');
    expect(token(lightBlock, '--nt-outline-strong')).toBe('#8fa3b8');
    expect(token(lightBlock, '--nt-edge-default')).toBe('#6b8299');
    expect(token(lightBlock, '--nt-edge-muted')).toBe('#a0b4c5');
    expect(declaration(lightBlock, '--nt-canvas-backdrop')).toContain(
      'radial-gradient(ellipse at 50% 0%, rgba(100, 120, 150, 0.13) 0%, transparent 55%)',
    );
    expect(declaration(lightBlock, '--nt-canvas-backdrop')).toContain(
      'linear-gradient(180deg, #dde2e8 0%, #d4dae2 100%)',
    );
    expect(declaration(lightBlock, '--nt-shadow-panel')).toBe('0 22px 46px rgba(20, 35, 55, 0.18)');
    expect(declaration(lightBlock, '--nt-shadow-floating')).toBe(
      '0 14px 28px rgba(20, 35, 55, 0.14)',
    );
    expect(compact(declaration(lightBlock, '--nt-node-shadow'))).toBe(
      '0 2px 4px rgba(20, 35, 55, 0.08), 0 8px 20px rgba(20, 35, 55, 0.12), 0 0 0 1px rgba(20, 35, 55, 0.06)',
    );
  });

  it('keeps light-mode operational text readable on all primary surfaces', () => {
    const backgrounds = [
      token(lightBlock, '--nt-bg'),
      token(lightBlock, '--nt-surface'),
      token(lightBlock, '--nt-surface-container'),
      token(lightBlock, '--nt-surface-container-high'),
    ];
    const foregrounds = [
      ['primary', token(lightBlock, '--nt-text-primary'), 7],
      ['secondary', token(lightBlock, '--nt-text-secondary'), 4.5],
      ['muted', token(lightBlock, '--nt-text-muted'), 4.5],
      ['status up', token(lightBlock, '--nt-status-up'), 4.5],
      ['warning', token(lightBlock, '--nt-warning'), 4.5],
      ['critical', token(lightBlock, '--nt-critical'), 4.5],
      ['down', token(lightBlock, '--nt-status-down'), 4.5],
    ] as const;

    for (const background of backgrounds) {
      for (const [label, foreground, minimum] of foregrounds) {
        expect(
          contrastRatio(foreground, background),
          `${label} ${foreground} on ${background}`,
        ).toBeGreaterThanOrEqual(minimum);
      }
    }

    expect(token(lightBlock, '--nt-text-primary')).toBe('#1a2332');
    expect(token(lightBlock, '--nt-text-secondary')).toBe('#38506a');
    expect(token(lightBlock, '--nt-text-muted')).toBe('#4d6781');
    expect(token(lightBlock, '--nt-text-primary')).not.toBe('#000000');
  });

  it('keeps dark-mode secondary and muted text readable on operational surfaces', () => {
    const backgrounds = [
      token(darkBlock, '--nt-bg'),
      token(darkBlock, '--nt-surface'),
      token(darkBlock, '--nt-surface-container'),
      token(darkBlock, '--nt-surface-container-high'),
    ];
    const foregrounds = [
      ['primary', token(darkBlock, '--nt-text-primary'), 7],
      ['secondary', token(darkBlock, '--nt-text-secondary'), 4.5],
      ['muted', token(darkBlock, '--nt-text-muted'), 4.5],
    ] as const;

    for (const background of backgrounds) {
      for (const [label, foreground, minimum] of foregrounds) {
        expect(
          contrastRatio(foreground, background),
          `${label} ${foreground} on ${background}`,
        ).toBeGreaterThanOrEqual(minimum);
      }
    }
  });

  it('defines a readable on-primary token for primary controls', () => {
    expect(declaration(tailwindThemeBlock, '--color-on-primary')).toBe('var(--nt-on-primary)');

    const themedBlocks = [
      ['dark', darkBlock],
      ['light', lightBlock],
    ] as const;

    for (const [theme, block] of themedBlocks) {
      const foreground = token(block, '--nt-on-primary');
      const background = token(block, '--nt-primary');

      expect(
        contrastRatio(foreground, background),
        `${theme} on-primary ${foreground} on ${background}`,
      ).toBeGreaterThanOrEqual(4.5);
    }
  });

  it('does not use viewport-width font sizing for UI text', () => {
    expect(css).not.toMatch(/text-\[.*vw/);
    expect(css).not.toMatch(/font-size:\s*.*vw/);
  });
});
