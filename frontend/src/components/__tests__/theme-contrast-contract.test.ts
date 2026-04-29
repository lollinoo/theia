import { readFileSync } from 'fs';
import { join } from 'path';
import { describe, expect, it } from 'vitest';

const css = readFileSync(join(__dirname, '../../index.css'), 'utf-8');

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

describe('enterprise NOC theme contrast contract', () => {
  it('resolves recursive same-block token aliases', () => {
    const block = `
      --source: #123456;
      --alias: var(--source);
      --recursive-alias: var(--alias);
    `;

    expect(token(block, '--recursive-alias')).toBe('#123456');
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

  it('does not use viewport-width font sizing for UI text', () => {
    expect(css).not.toMatch(/text-\[.*vw/);
    expect(css).not.toMatch(/font-size:\s*.*vw/);
  });
});
