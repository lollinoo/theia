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

    expect(darkTheme).toContain('--nt-status-down: #ff3045;');
    expect(darkTheme).toContain('--nt-node-down-border: rgba(255, 48, 69, 0.98);');
    expect(darkTheme).toContain('--nt-node-down-card-bg: #3b101b;');
    expect(darkTheme).toContain('--nt-node-down-card-pulse-bg: #5a0f21;');
    expect(darkTheme).toContain('--nt-node-down-glow: rgba(255, 48, 69, 0.42);');
  });

  it('uses high-contrast down reds in light mode', () => {
    const lightTheme = themeBlock('[data-theme="light"]');

    expect(lightTheme).toContain('--nt-status-down: #c91f17;');
    expect(lightTheme).toContain('--nt-node-down-border: rgba(201, 31, 23, 0.94);');
    expect(lightTheme).toContain('--nt-node-down-card-bg: #ffd8d2;');
    expect(lightTheme).toContain('--nt-node-down-card-pulse-bg: #ffb8ad;');
    expect(lightTheme).toContain('--nt-node-down-glow: rgba(201, 31, 23, 0.28);');
  });

  it('accentuates the existing down pulse animation', () => {
    expect(css).toContain('animation: topology-node-down-pulse 0.95s ease-in-out infinite;');
    expect(css).toContain('box-shadow: 0 0 0 1px var(--nt-node-down-border)');
    expect(css).toContain('0 0 34px var(--nt-node-down-glow)');
  });
});
