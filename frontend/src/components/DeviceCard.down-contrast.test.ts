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

    expect(darkTheme).toContain('--nt-status-down: #ff5c6c;');
    expect(darkTheme).toContain('--nt-node-down-border: rgba(255, 92, 108, 0.98);');
    expect(darkTheme).toContain('--nt-node-down-card-bg: #35191e;');
    expect(darkTheme).not.toContain('--nt-node-down-card-pulse-bg');
    expect(darkTheme).not.toContain('--nt-node-probing-card-pulse-bg');
    expect(darkTheme).toContain('--nt-node-down-glow: rgba(255, 92, 108, 0.36);');
  });

  it('uses high-contrast down reds in light mode', () => {
    const lightTheme = themeBlock('[data-theme="light"]');

    expect(lightTheme).toContain('--nt-status-down: #b4232d;');
    expect(lightTheme).toContain('--nt-node-down-border: rgba(180, 35, 45, 0.78);');
    expect(lightTheme).toContain('--nt-node-down-card-bg: #fff1f2;');
    expect(lightTheme).not.toContain('--nt-node-down-card-pulse-bg');
    expect(lightTheme).not.toContain('--nt-node-probing-card-pulse-bg');
    expect(lightTheme).toContain('--nt-node-down-glow: rgba(180, 35, 45, 0.16);');
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
