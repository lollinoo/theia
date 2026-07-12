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
    expect(darkTheme).toContain('--nt-node-down-border: #ff5964;');
    expect(darkTheme).toContain('--nt-node-down-badge-bg: rgba(255, 89, 100, 0.42);');
    expect(darkTheme).toContain('--nt-node-down-card-bg: #541720;');
    expect(darkTheme).toContain('--nt-node-down-panel-bg: rgba(255, 89, 100, 0.28);');
    expect(darkTheme).toContain('--nt-node-down-ring: rgba(255, 89, 100, 0.32);');
    expect(darkTheme).not.toContain('--nt-node-down-card-pulse-bg');
    expect(darkTheme).not.toContain('--nt-node-probing-card-pulse-bg');
    expect(darkTheme).toContain('--nt-node-down-glow: rgba(255, 89, 100, 0.36);');
  });

  it('uses high-contrast down reds in light mode', () => {
    const lightTheme = themeBlock('[data-theme="light"]');

    expect(lightTheme).toContain('--nt-status-down: #8f0d18;');
    expect(lightTheme).toContain('--nt-node-down-border: #d7192d;');
    expect(lightTheme).toContain('--nt-node-down-badge-bg: rgba(215, 25, 45, 0.24);');
    expect(lightTheme).toContain('--nt-node-down-card-bg: #ff9aa4;');
    expect(lightTheme).toContain('--nt-node-down-panel-bg: rgba(215, 25, 45, 0.18);');
    expect(lightTheme).toContain('--nt-node-down-ring: rgba(215, 25, 45, 0.2);');
    expect(lightTheme).not.toContain('--nt-node-down-card-pulse-bg');
    expect(lightTheme).not.toContain('--nt-node-probing-card-pulse-bg');
    expect(lightTheme).toContain('--nt-node-down-glow: rgba(215, 25, 45, 0.18);');
  });

  it('coordinates probing accents with the stronger theme yellows', () => {
    const darkTheme = themeBlock(':root,\n[data-theme="dark"]');
    const lightTheme = themeBlock('[data-theme="light"]');

    expect(darkTheme).toContain('--nt-node-probing-border: #ffd000;');
    expect(darkTheme).toContain('--nt-node-probing-badge-bg: rgba(255, 208, 0, 0.42);');
    expect(darkTheme).toContain('--nt-node-probing-card-bg: rgba(255, 208, 0, 0.36);');
    expect(darkTheme).toContain('--nt-node-probing-ring: rgba(255, 208, 0, 0.28);');
    expect(lightTheme).toContain('--nt-node-probing-border: #a97d00;');
    expect(lightTheme).toContain('--nt-node-probing-badge-bg: rgba(169, 125, 0, 0.24);');
    expect(lightTheme).toContain('--nt-node-probing-card-bg: #ffd54f;');
    expect(lightTheme).toContain('--nt-node-probing-ring: rgba(169, 125, 0, 0.18);');
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
