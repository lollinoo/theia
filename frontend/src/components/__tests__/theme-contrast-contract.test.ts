/**
 * Exercises theme contrast contract component behavior so refactors preserve the documented contract.
 */
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

  it('uses a neutral light surface scale with restrained canvas depth', () => {
    const lightSurfaceTokens = {
      background: token(lightBlock, '--nt-bg'),
      surface: token(lightBlock, '--nt-surface'),
      surfaceContainer: token(lightBlock, '--nt-surface-container'),
      surfaceContainerHigh: token(lightBlock, '--nt-surface-container-high'),
      elevated: token(lightBlock, '--nt-elevated'),
    };

    expect(lightSurfaceTokens).toEqual({
      background: '#f1f4f7',
      surface: '#ffffff',
      surfaceContainer: '#f4f6f8',
      surfaceContainerHigh: '#fafbfc',
      elevated: '#ffffff',
    });

    for (const [label, value] of Object.entries(lightSurfaceTokens).filter(
      ([key]) => key !== 'surface' && key !== 'elevated',
    )) {
      expect(value, `${label} should not be pure white`).not.toBe('#ffffff');
    }

    const backgroundLuminance = luminance(lightSurfaceTokens.background);
    const surfaceLuminance = luminance(lightSurfaceTokens.surface);
    const containerHighLuminance = luminance(lightSurfaceTokens.surfaceContainerHigh);
    const elevatedLuminance = luminance(lightSurfaceTokens.elevated);

    expect(
      backgroundLuminance,
      'light canvas should remain below near-white working surfaces',
    ).toBeLessThanOrEqual(0.91);
    expect(
      surfaceLuminance - backgroundLuminance,
      'nodes need restrained depth over canvas',
    ).toBeGreaterThanOrEqual(0.09);
    expect(
      containerHighLuminance - backgroundLuminance,
      'raised layers need visible separation from canvas',
    ).toBeGreaterThanOrEqual(0.05);
    expect(
      elevatedLuminance - containerHighLuminance,
      'raised surfaces should stay subtle',
    ).toBeLessThanOrEqual(0.04);
  });

  it('uses a neutral dark surface scale with clear layer separation', () => {
    const darkSurfaceTokens = {
      background: token(darkBlock, '--nt-bg'),
      surface: token(darkBlock, '--nt-surface'),
      surfaceContainer: token(darkBlock, '--nt-surface-container'),
      surfaceContainerHigh: token(darkBlock, '--nt-surface-container-high'),
      elevated: token(darkBlock, '--nt-elevated'),
    };

    expect(darkSurfaceTokens).toEqual({
      background: '#101315',
      surface: '#171b1e',
      surfaceContainer: '#1d2327',
      surfaceContainerHigh: '#252c31',
      elevated: '#2b3439',
    });
  });

  it('keeps light-mode chrome layered without washing out controls', () => {
    expect(token(lightBlock, '--nt-outline')).toBe('#d1d9e2');
    expect(token(lightBlock, '--nt-outline-strong')).toBe('#98a8b8');
    expect(token(lightBlock, '--nt-edge-default')).toBe('#657a8e');
    expect(token(lightBlock, '--nt-edge-muted')).toBe('#b5c0cb');
    expect(declaration(lightBlock, '--nt-glass-bg')).toBe('rgba(255, 255, 255, 0.94)');
    expect(declaration(lightBlock, '--nt-glass-border')).toBe('rgba(24, 34, 48, 0.1)');
    expect(declaration(lightBlock, '--nt-glass-backdrop')).toBe('none');
    expect(declaration(lightBlock, '--nt-minimap-mask')).toBe('rgba(225, 230, 235, 0.88)');
    expect(declaration(lightBlock, '--nt-canvas-backdrop')).toBe('#edf1f5');
    expect(declaration(lightBlock, '--nt-shadow-panel')).toBe('0 10px 24px rgba(16, 24, 40, 0.1)');
    expect(declaration(lightBlock, '--nt-shadow-floating')).toBe(
      '0 6px 16px rgba(16, 24, 40, 0.09)',
    );
    expect(declaration(lightBlock, '--nt-shadow-pill')).toBe('0 2px 8px rgba(16, 24, 40, 0.07)');
    expect(declaration(lightBlock, '--nt-shadow-canvas')).toBe(
      '0 10px 28px rgba(16, 24, 40, 0.08)',
    );
    expect(compact(declaration(lightBlock, '--nt-node-shadow'))).toBe(
      '0 1px 2px rgba(16, 24, 40, 0.06), 0 4px 12px rgba(16, 24, 40, 0.08)',
    );
  });

  it('keeps dark-mode chrome neutral and restrained over the topology canvas', () => {
    expect(token(darkBlock, '--nt-outline')).toBe('#343d40');
    expect(token(darkBlock, '--nt-outline-strong')).toBe('#59666a');
    expect(token(darkBlock, '--nt-edge-default')).toBe('#82908f');
    expect(token(darkBlock, '--nt-edge-muted')).toBe('#4b5758');
    expect(token(darkBlock, '--nt-edge-active')).toBe('#67d9c0');
    expect(token(darkBlock, '--nt-node-selected')).toBe('#67d9c0');
    expect(declaration(darkBlock, '--nt-glass-bg')).toBe('rgba(23, 27, 30, 0.94)');
    expect(declaration(darkBlock, '--nt-glass-border')).toBe('rgba(241, 245, 244, 0.12)');
    expect(declaration(darkBlock, '--nt-glass-backdrop')).toBe('blur(10px)');
    expect(declaration(darkBlock, '--nt-minimap-mask')).toBe('rgba(16, 19, 21, 0.74)');
    expect(declaration(darkBlock, '--nt-canvas-backdrop')).toBe('#0e1213');
    expect(declaration(darkBlock, '--nt-shadow-panel')).toBe('0 12px 28px rgba(0, 0, 0, 0.3)');
    expect(declaration(darkBlock, '--nt-shadow-floating')).toBe(
      '0 8px 20px rgba(0, 0, 0, 0.26)',
    );
    expect(declaration(darkBlock, '--nt-shadow-pill')).toBe('0 4px 12px rgba(0, 0, 0, 0.22)');
    expect(declaration(darkBlock, '--nt-shadow-canvas')).toBe(
      '0 14px 36px rgba(0, 0, 0, 0.26)',
    );
    expect(declaration(darkBlock, '--nt-node-shadow')).toBe('0 6px 16px rgba(0, 0, 0, 0.24)');
    expect(declaration(darkBlock, '--nt-glow-shadow-opacity')).toBe('0.26');
    expect(declaration(darkBlock, '--nt-glow-bloom-opacity')).toBe('0.07');
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

    expect(token(lightBlock, '--nt-text-primary')).toBe('#182230');
    expect(token(lightBlock, '--nt-text-secondary')).toBe('#43566b');
    expect(token(lightBlock, '--nt-text-muted')).toBe('#5b6f84');
    expect(token(lightBlock, '--nt-primary')).toBe('#08745a');
    expect(token(lightBlock, '--nt-text-primary')).not.toBe('#000000');
  });

  it('keeps dark-mode operational colors readable on primary surfaces', () => {
    const backgrounds = [
      token(darkBlock, '--nt-bg'),
      token(darkBlock, '--nt-surface'),
      token(darkBlock, '--nt-surface-container'),
      token(darkBlock, '--nt-surface-container-high'),
    ];
    const foregrounds = [
      ['primary text', token(darkBlock, '--nt-text-primary'), 7],
      ['secondary text', token(darkBlock, '--nt-text-secondary'), 4.5],
      ['muted text', token(darkBlock, '--nt-text-muted'), 4.5],
      ['primary teal', token(darkBlock, '--nt-primary'), 4.5],
      ['status up', token(darkBlock, '--nt-status-up'), 4.5],
      ['warning', token(darkBlock, '--nt-warning'), 4.5],
      ['critical', token(darkBlock, '--nt-critical'), 4.5],
      ['down', token(darkBlock, '--nt-status-down'), 4.5],
      ['unknown', token(darkBlock, '--nt-status-unknown'), 4.5],
    ] as const;

    for (const background of backgrounds) {
      for (const [label, foreground, minimum] of foregrounds) {
        expect(
          contrastRatio(foreground, background),
          `${label} ${foreground} on ${background}`,
        ).toBeGreaterThanOrEqual(minimum);
      }
    }

    expect(token(darkBlock, '--nt-text-primary')).toBe('#f1f5f4');
    expect(token(darkBlock, '--nt-text-secondary')).toBe('#b8c4c1');
    expect(token(darkBlock, '--nt-text-muted')).toBe('#94a5a1');
    expect(token(darkBlock, '--nt-primary')).toBe('#4cc9b0');
    expect(token(darkBlock, '--nt-on-primary')).toBe('#071411');
    expect(token(darkBlock, '--nt-status-ok')).toBe('#6edb8f');
    expect(token(darkBlock, '--nt-status-warning')).toBe('#efbd69');
    expect(token(darkBlock, '--nt-status-critical')).toBe('#ff8296');
    expect(token(darkBlock, '--nt-status-unknown')).toBe('#a1aaa8');
    expect(token(darkBlock, '--nt-status-down')).toBe('#ff5c6c');
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
