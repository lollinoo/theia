import { describe, it, expect, vi } from 'vitest';
import { render } from '@testing-library/react';
import { SettingsPanel } from './SettingsPanel';

// Mock API calls made in SettingsPanel on mount
vi.mock('../api/client', () => ({
  fetchSettings: vi.fn().mockResolvedValue({}),
  updateSetting: vi.fn().mockResolvedValue(undefined),
  fetchHealthVersion: vi.fn().mockResolvedValue({ version: '1.3.0', git_commit: 'abc', build_date: '2026-01-01' }),
}));

// Mock sub-components that have their own complex dependencies
vi.mock('./AreaManager', () => ({
  AreaManager: () => <div data-testid="area-manager" />,
}));

vi.mock('./SNMPProfileManager', () => ({
  SNMPProfileManager: () => <div data-testid="snmp-profile-manager" />,
}));

vi.mock('./SSHProfileManager', () => ({
  SSHProfileManager: () => <div data-testid="ssh-profile-manager" />,
}));

describe('SettingsPanel (COMP-05)', () => {
  it('form inputs use border-outline-subtle (not border-outline)', () => {
    const { container } = render(<SettingsPanel />);
    const inputs = Array.from(container.querySelectorAll('input, select'));
    // Every input/select with a border class should use border-outline-subtle
    const withOldBorder = inputs.filter((el) =>
      el.className.includes('border-outline') && !el.className.includes('border-outline-subtle'),
    );
    expect(withOldBorder).toHaveLength(0);
  });

  it('does not have border-t section separators (no-line rule)', () => {
    const { container } = render(<SettingsPanel />);
    const html = container.innerHTML;
    expect(html).not.toContain('border-t border-outline');
  });

  it('inputs have focus:ring-primary/30 for standardized focus ring', () => {
    const { container } = render(<SettingsPanel />);
    const inputs = Array.from(container.querySelectorAll('input, select'));
    // At least one input should have the standardized focus ring
    const withFocusRing = inputs.filter((el) => el.className.includes('ring-primary'));
    expect(withFocusRing.length).toBeGreaterThan(0);
  });

  it('dev badge uses semantic warning token (not hardcoded yellow)', () => {
    const { container } = render(<SettingsPanel />);
    const html = container.innerHTML;
    expect(html).not.toContain('yellow-500');
    expect(html).not.toContain('yellow-400');
  });
});
