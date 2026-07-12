/**
 * Exercises device card down contrast component behavior so refactors preserve the documented contract.
 */
import { readFileSync } from 'node:fs';

const css = readFileSync('src/index.css', 'utf8');

function themeBlock(selector: string): string {
  const start = css.indexOf(selector);
  expect(start).toBeGreaterThanOrEqual(0);

  const nextThemeStart = css.indexOf('\n[data-theme="light"]', start + selector.length);
  const end =
    selector === '[data-theme="light"]' ? css.indexOf('\n@theme inline', start) : nextThemeStart;
  expect(end).toBeGreaterThan(start);

  return css.slice(start, end);
}

describe('DeviceCard down node contrast tokens', () => {
  it('uses high-contrast down reds in dark mode', () => {
    const darkTheme = themeBlock(':root,\n[data-theme="dark"]');

    expect(darkTheme).toContain('--nt-status-down: #ff5964;');
    expect(darkTheme).toContain('--nt-node-down-border: rgba(255, 89, 100, 0.98);');
    expect(darkTheme).toContain('--nt-node-down-card-bg: #3a171c;');
    expect(darkTheme).not.toContain('--nt-node-down-card-pulse-bg');
    expect(darkTheme).not.toContain('--nt-node-probing-card-pulse-bg');
    expect(darkTheme).toContain('--nt-node-down-glow: rgba(255, 89, 100, 0.36);');
  });

  it('uses high-contrast down reds in light mode', () => {
    const lightTheme = themeBlock('[data-theme="light"]');

    expect(lightTheme).toContain('--nt-status-down: #c51624;');
    expect(lightTheme).toContain('--nt-node-down-border: rgba(197, 22, 36, 0.82);');
    expect(lightTheme).toContain('--nt-node-down-card-bg: #fff0f1;');
    expect(lightTheme).not.toContain('--nt-node-down-card-pulse-bg');
    expect(lightTheme).not.toContain('--nt-node-probing-card-pulse-bg');
    expect(lightTheme).toContain('--nt-node-down-glow: rgba(197, 22, 36, 0.16);');
  });

  it('coordinates probing accents with the stronger theme yellows', () => {
    const darkTheme = themeBlock(':root,\n[data-theme="dark"]');
    const lightTheme = themeBlock('[data-theme="light"]');

    expect(darkTheme).toContain('--nt-node-probing-border: rgba(255, 208, 0, 0.92);');
    expect(darkTheme).toContain('--nt-node-probing-card-bg: rgba(255, 208, 0, 0.15);');
    expect(lightTheme).toContain('--nt-node-probing-border: rgba(118, 90, 0, 0.72);');
    expect(lightTheme).toContain('--nt-node-probing-card-bg: #fff9dc;');
  });

  it('keeps whole-node status surfaces static', () => {
    expect(css).not.toContain('@keyframes topology-node-down-pulse');
    expect(css).not.toContain('@keyframes topology-node-status-pulse');
    expect(css).not.toContain('@keyframes topology-virtual-node-status-pulse');
    expect(css).not.toContain('.topology-node-down-pulse');
    expect(css).not.toContain('.topology-node-status-pulse');
    expect(css).not.toContain('.topology-virtual-node-status-pulse');
    expect(css).not.toContain('will-change: background-color');
    expect(css).not.toContain('0 0 34px var(--nt-node-down-glow)');
  });
});
