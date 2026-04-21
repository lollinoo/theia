import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { SettingsPanel } from './SettingsPanel';

// Mock API calls made in SettingsPanel on mount
vi.mock('../api/client', () => ({
  fetchSettings: vi.fn().mockResolvedValue({}),
  updateSetting: vi.fn().mockResolvedValue(undefined),
  fetchHealthVersion: vi
    .fn()
    .mockResolvedValue({ version: '1.3.0', git_commit: 'abc', build_date: '2026-01-01' }),
}));

// Mock sub-components that have their own complex dependencies
vi.mock('./AreaManager', () => ({
  AreaManager: () => <div data-testid="area-manager" />,
}));

vi.mock('./SNMPProfileManager', () => ({
  SNMPProfileManager: () => <div data-testid="snmp-profile-manager" />,
}));

vi.mock('./CredentialProfileManager', () => ({
  CredentialProfileManager: () => <div data-testid="credential-profile-manager" />,
}));

vi.mock('./InstanceBackupManager', () => ({
  InstanceBackupManager: () => <div data-testid="instance-backup-manager" />,
}));

describe('SettingsPanel (COMP-05)', () => {
  it('form inputs use border-outline-subtle (not border-outline)', () => {
    const { container } = render(<SettingsPanel />);
    const inputs = Array.from(container.querySelectorAll('input, select'));
    // Every input/select with a border class should use border-outline-subtle
    const withOldBorder = inputs.filter(
      (el) =>
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

  it('renders topology discovery default selector and saves changes', async () => {
    const { updateSetting } = await import('../api/client');

    render(<SettingsPanel />);

    fireEvent.change(screen.getByLabelText('Topology Discovery Default'), {
      target: { value: 'bootstrap_once' },
    });

    await waitFor(() => {
      expect(updateSetting).toHaveBeenCalledWith(
        'topology_discovery_default_mode',
        'bootstrap_once',
      );
    });
  });
});

// --- Gap 7: SettingsPanel URL validation on blur ---

describe('SettingsPanel — Grafana URL validation on blur', () => {
  it('shows error text when Grafana URL is blurred with an invalid value', async () => {
    render(<SettingsPanel />);

    const grafanaInput = screen.getByPlaceholderText('http://localhost:3001');
    fireEvent.change(grafanaInput, { target: { value: 'not-a-url' } });
    fireEvent.blur(grafanaInput);

    await waitFor(() => {
      expect(screen.getByText('URL must start with http:// or https://')).toBeInTheDocument();
    });
  });

  it('applies border-status-down class to Grafana URL input on invalid blur', async () => {
    render(<SettingsPanel />);

    const grafanaInput = screen.getByPlaceholderText('http://localhost:3001');
    fireEvent.change(grafanaInput, { target: { value: 'ftp://invalid' } });
    fireEvent.blur(grafanaInput);

    await waitFor(() => {
      expect(grafanaInput.className).toContain('border-status-down');
    });
  });

  it('clears Grafana URL error when user edits the field', async () => {
    render(<SettingsPanel />);

    const grafanaInput = screen.getByPlaceholderText('http://localhost:3001');
    fireEvent.change(grafanaInput, { target: { value: 'not-a-url' } });
    fireEvent.blur(grafanaInput);

    await waitFor(() => {
      expect(screen.getByText('URL must start with http:// or https://')).toBeInTheDocument();
    });

    fireEvent.change(grafanaInput, { target: { value: 'http://grafana.local' } });

    await waitFor(() => {
      expect(screen.queryByText('URL must start with http:// or https://')).not.toBeInTheDocument();
    });
  });
});

describe('SettingsPanel — Prometheus URL validation on blur', () => {
  it('shows error text when Prometheus URL is blurred with an invalid value', async () => {
    render(<SettingsPanel />);

    const prometheusInput = screen.getByPlaceholderText('http://localhost:9090');
    fireEvent.change(prometheusInput, { target: { value: 'not-a-url' } });
    fireEvent.blur(prometheusInput);

    await waitFor(() => {
      expect(screen.getByText('URL must start with http:// or https://')).toBeInTheDocument();
    });
  });
});

// --- Gap 8: SettingsPanel auto-save gated on validation ---

describe('SettingsPanel — invalid URL prevents updateSetting call', () => {
  it('does not call updateSetting for grafana_url when URL is invalid', async () => {
    const { updateSetting } = await import('../api/client');
    render(<SettingsPanel />);

    const grafanaInput = screen.getByPlaceholderText('http://localhost:3001');
    fireEvent.change(grafanaInput, { target: { value: 'not-a-url' } });

    // scheduleGrafanaUpdate is called on change — but it gates on validateURL
    // Give it a moment to check if updateSetting was called
    await waitFor(() => {
      const calls = (updateSetting as ReturnType<typeof vi.fn>).mock.calls;
      const grafanaCalls = calls.filter(([key]: [string]) => key === 'grafana_url');
      expect(grafanaCalls).toHaveLength(0);
    });
  });

  it('does not call updateSetting for prometheus_url when URL is invalid', async () => {
    const { updateSetting } = await import('../api/client');
    render(<SettingsPanel />);

    const prometheusInput = screen.getByPlaceholderText('http://localhost:9090');
    fireEvent.change(prometheusInput, { target: { value: 'ftp://bad' } });

    await waitFor(() => {
      const calls = (updateSetting as ReturnType<typeof vi.fn>).mock.calls;
      const prometheusCalls = calls.filter(([key]: [string]) => key === 'prometheus_url');
      expect(prometheusCalls).toHaveLength(0);
    });
  });
});

// --- Gap 2: Device Backups collapsible section renders after clicking toggle ---

describe('SettingsPanel — Device Backups collapsible section (Gap 2)', () => {
  it('clicking the Device Backups button reveals the section content', async () => {
    render(<SettingsPanel />);

    // The section content should not be visible before toggling
    expect(screen.queryByText('Automatic Backup Schedule')).not.toBeInTheDocument();

    // Click the "Device Backups" toggle button
    const toggleBtn = screen.getByRole('button', { name: /device backups/i });
    await act(async () => {
      fireEvent.click(toggleBtn);
    });

    // Section content should now be visible
    expect(screen.getByText('Automatic Backup Schedule')).toBeInTheDocument();
  });
});

// --- Gap 3: Schedule dropdown has all 6 preset options ---

describe('SettingsPanel — Device Backups schedule dropdown options (Gap 3)', () => {
  it('schedule dropdown has all 6 preset options after expanding the section', async () => {
    render(<SettingsPanel />);

    const toggleBtn = screen.getByRole('button', { name: /device backups/i });
    await act(async () => {
      fireEvent.click(toggleBtn);
    });

    // Find the schedule dropdown (only select inside the device backup section)
    const scheduleLabel = screen.getByText('Automatic Backup Schedule');
    // The select is sibling to the label wrapper — find it by querying within the section
    const selects = screen.getAllByRole('combobox');
    // The schedule dropdown is the last select added (others are polling + timezone)
    // Find the one that contains the "Disabled" option with value "0"
    const scheduleSelect = selects.find((s) =>
      Array.from(s.querySelectorAll('option')).some(
        (o) => (o as HTMLOptionElement).value === '0' && o.textContent === 'Disabled',
      ),
    );
    expect(scheduleLabel).toBeInTheDocument();
    expect(scheduleSelect).toBeDefined();

    const options = Array.from(scheduleSelect!.querySelectorAll('option')).map(
      (o) => (o as HTMLOptionElement).value,
    );
    expect(options).toEqual(['0', '6', '12', '24', '48', '168']);
  });
});

// --- Gap 4: Retention input has min=1 max=50 attributes ---

describe('SettingsPanel — Device Backups retention input attributes (Gap 4)', () => {
  it('retention input has min=1 and max=50 after expanding the section', async () => {
    render(<SettingsPanel />);

    const toggleBtn = screen.getByRole('button', { name: /device backups/i });
    await act(async () => {
      fireEvent.click(toggleBtn);
    });

    // The retention input is a number input; find it by its label text
    expect(screen.getByText('Keep last N backups per device')).toBeInTheDocument();

    // Find all number inputs and identify the retention one (has min=1, max=50)
    const numberInputs = screen
      .getAllByRole('spinbutton')
      .filter(
        (el) => (el as HTMLInputElement).min === '1' && (el as HTMLInputElement).max === '50',
      );
    expect(numberInputs).toHaveLength(1);
    expect((numberInputs[0] as HTMLInputElement).min).toBe('1');
    expect((numberInputs[0] as HTMLInputElement).max).toBe('50');
  });
});

// --- Gap 5: Helper text shows "Scheduling disabled" when interval is 0 ---

describe('SettingsPanel — Device Backups helper text when disabled (Gap 5)', () => {
  it('shows "Scheduling disabled" text when interval defaults to 0', async () => {
    render(<SettingsPanel />);

    const toggleBtn = screen.getByRole('button', { name: /device backups/i });
    await act(async () => {
      fireEvent.click(toggleBtn);
    });

    // Default state: interval is "0" (fetchSettings returns {} → defaults to "0")
    expect(screen.getByText('Scheduling disabled')).toBeInTheDocument();
  });
});

// --- Gap 6: Settings load device backup values on mount from fetchSettings ---

describe('SettingsPanel — Device Backups values loaded from fetchSettings on mount (Gap 6)', () => {
  it('schedule dropdown reflects device_backup_interval_hours from fetchSettings', async () => {
    const { fetchSettings } = await import('../api/client');
    (fetchSettings as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      device_backup_interval_hours: '24',
      device_backup_retention_count: '10',
    });

    render(<SettingsPanel />);

    // Expand the section
    const toggleBtn = screen.getByRole('button', { name: /device backups/i });
    await act(async () => {
      fireEvent.click(toggleBtn);
    });

    // Wait for fetchSettings to resolve and state to update
    await waitFor(() => {
      const selects = screen.getAllByRole('combobox');
      const scheduleSelect = selects.find((s) =>
        Array.from(s.querySelectorAll('option')).some(
          (o) => (o as HTMLOptionElement).value === '0' && o.textContent === 'Disabled',
        ),
      );
      expect(scheduleSelect).toBeDefined();
      expect((scheduleSelect as HTMLSelectElement).value).toBe('24');
    });
  });

  it('helper text changes from "Scheduling disabled" to interval description after loading 24h setting', async () => {
    const { fetchSettings } = await import('../api/client');
    (fetchSettings as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      device_backup_interval_hours: '24',
      device_backup_retention_count: '10',
    });

    render(<SettingsPanel />);

    const toggleBtn = screen.getByRole('button', { name: /device backups/i });
    await act(async () => {
      fireEvent.click(toggleBtn);
    });

    await waitFor(() => {
      expect(screen.getByText('Backups run every 24 hours')).toBeInTheDocument();
    });
  });
});

// --- Gap 14: Instance Backups collapsible section collapsed by default ---

describe('SettingsPanel — Instance Backups collapsible section (Gap 14)', () => {
  it('Instance Backups toggle button is present in the initial render', () => {
    render(<SettingsPanel />);

    // The collapsible header button must be visible without any interaction
    expect(screen.getByRole('button', { name: /instance backups/i })).toBeInTheDocument();
  });

  it('InstanceBackupManager is NOT rendered before the toggle is clicked (collapsed by default)', () => {
    render(<SettingsPanel />);

    // The mock InstanceBackupManager renders a div[data-testid="instance-backup-manager"]
    // It must not be in the DOM until the toggle is clicked
    expect(screen.queryByTestId('instance-backup-manager')).not.toBeInTheDocument();
  });

  it('clicking the Instance Backups toggle renders InstanceBackupManager', async () => {
    render(<SettingsPanel />);

    const toggleBtn = screen.getByRole('button', { name: /instance backups/i });
    await act(async () => {
      fireEvent.click(toggleBtn);
    });

    expect(screen.getByTestId('instance-backup-manager')).toBeInTheDocument();
  });

  it('clicking the toggle a second time collapses the section again', async () => {
    render(<SettingsPanel />);

    const toggleBtn = screen.getByRole('button', { name: /instance backups/i });

    // Expand
    await act(async () => {
      fireEvent.click(toggleBtn);
    });
    expect(screen.getByTestId('instance-backup-manager')).toBeInTheDocument();

    // Collapse
    await act(async () => {
      fireEvent.click(toggleBtn);
    });
    expect(screen.queryByTestId('instance-backup-manager')).not.toBeInTheDocument();
  });
});
