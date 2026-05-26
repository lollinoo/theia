import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { SettingsPanel } from './SettingsPanel';

// Mock API calls made in SettingsPanel on mount
vi.mock('../api/client', () => ({
  fetchSettings: vi.fn().mockResolvedValue({}),
  fetchSettingsWithMetadata: vi.fn().mockResolvedValue({ data: {}, secrets: {} }),
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

vi.mock('./GrafanaDashboardProfileManager', () => ({
  GrafanaDashboardProfileManager: () => <div data-testid="grafana-profile-manager" />,
}));

vi.mock('./InstanceBackupManager', () => ({
  InstanceBackupManager: () => <div data-testid="instance-backup-manager" />,
}));

describe('SettingsPanel (COMP-05)', () => {
  it('renders settings in UserSettingsPage-style section cards', () => {
    render(<SettingsPanel />);

    for (const heading of [
      'Polling',
      'Topology',
      'Integrations',
      'Bridge',
      'Profiles',
      'Backups',
      'About',
    ]) {
      const title = screen.getByRole('heading', { name: heading });
      const section = title.closest('section');
      expect(section?.className).toContain('shadow-panel');
    }
  });

  it('uses independent desktop columns so expanded sections do not push the opposite column', () => {
    render(<SettingsPanel />);

    const layout = screen.getByTestId('settings-panel-layout');
    expect(layout.className).toContain('lg:grid-cols-2');
    expect(Array.from(layout.children).filter((child) => child.tagName === 'SECTION')).toHaveLength(
      0,
    );
    expect(screen.getByTestId('settings-panel-left-column').className).toContain('content-start');
    expect(screen.getByTestId('settings-panel-right-column').className).toContain('content-start');
    expect(
      Array.from(screen.getByTestId('settings-panel-left-column').children).filter(
        (child) => child.tagName === 'SECTION',
      ),
    ).toHaveLength(3);
    expect(
      Array.from(screen.getByTestId('settings-panel-right-column').children).filter(
        (child) => child.tagName === 'SECTION',
      ),
    ).toHaveLength(4);
  });

  it('keeps section cards from stretching when a neighboring section expands', () => {
    render(<SettingsPanel />);

    const layout = screen.getByTestId('settings-panel-layout');
    expect(layout.className).toContain('items-start');

    for (const column of Array.from(layout.children)) {
      for (const section of Array.from(column.children)) {
        expect(section.className).toContain('self-start');
      }
    }
  });

  it('uses fixed-height cards with internal scrolling so columns remain aligned', () => {
    render(<SettingsPanel />);

    for (const heading of [
      'Polling',
      'Topology',
      'Integrations',
      'Bridge',
      'Profiles',
      'Backups',
      'About',
    ]) {
      const section = screen.getByRole('heading', { name: heading }).closest('section');
      expect(section?.className).toContain('h-[22rem]');
      expect(section?.querySelector('[data-testid="settings-section-body"]')?.className).toContain(
        'overflow-y-auto',
      );
    }
  });

  it('gives profile managers a surfaced well inside the Profiles section', () => {
    render(<SettingsPanel />);

    expect(screen.getByTestId('snmp-profile-well').className).toContain(
      'bg-surface-container-high',
    );
    expect(screen.getByTestId('credential-profile-well').className).toContain(
      'bg-surface-container-high',
    );
    expect(screen.getByTestId('grafana-profile-well').className).toContain(
      'bg-surface-container-high',
    );
  });

  it('does not render the legacy Grafana URL field in Integrations', () => {
    render(<SettingsPanel />);

    expect(screen.queryByLabelText('Grafana URL')).not.toBeInTheDocument();
    expect(screen.queryByPlaceholderText('http://localhost:3001')).not.toBeInTheDocument();
    expect(screen.getByTestId('grafana-profile-manager')).toBeInTheDocument();
  });

  it('uses stable label rows for device backup fields', async () => {
    render(<SettingsPanel />);

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /device backups/i }));
    });

    expect(screen.getByTestId('device-backup-schedule-label-row').className).toContain('min-h-10');
    expect(screen.getByTestId('device-backup-retention-label-row').className).toContain('min-h-10');
  });

  it('uses stable helper rows for device backup fields', async () => {
    render(<SettingsPanel />);

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /device backups/i }));
    });

    expect(screen.getByTestId('device-backup-schedule-helper-row').className).toContain('min-h-4');
    expect(screen.getByTestId('device-backup-retention-helper-row').className).toContain('min-h-4');
  });

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

  it('does not render area management controls in global settings', () => {
    render(<SettingsPanel />);

    expect(screen.queryByTestId('area-manager')).not.toBeInTheDocument();
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
    const { fetchSettingsWithMetadata } = await import('../api/client');
    (fetchSettingsWithMetadata as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      data: {
        device_backup_interval_hours: '24',
        device_backup_retention_count: '10',
      },
      secrets: {},
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
    const { fetchSettingsWithMetadata } = await import('../api/client');
    (fetchSettingsWithMetadata as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      data: {
        device_backup_interval_hours: '24',
        device_backup_retention_count: '10',
      },
      secrets: {},
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

describe('SettingsPanel — bridge secret migration', () => {
  it('does not render the legacy global bridge secret control', async () => {
    const { fetchSettingsWithMetadata } = await import('../api/client');
    (fetchSettingsWithMetadata as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      data: {
        bridge_port: '1337',
      },
      secrets: {},
    });

    render(<SettingsPanel />);

    await waitFor(() => {
      expect(screen.getByDisplayValue('1337')).toBeInTheDocument();
    });
    expect(screen.queryByLabelText('WinBox Bridge Secret')).not.toBeInTheDocument();
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

describe('SettingsPanel — Polling Workers settings', () => {
  beforeEach(async () => {
    const { fetchSettingsWithMetadata, updateSetting } = await import('../api/client');
    (fetchSettingsWithMetadata as ReturnType<typeof vi.fn>).mockClear();
    (updateSetting as ReturnType<typeof vi.fn>).mockClear();
    (fetchSettingsWithMetadata as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: {},
      secrets: {},
    });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('renders the Polling Workers section collapsed by default and expands on click', async () => {
    render(<SettingsPanel />);

    expect(screen.getByRole('button', { name: /polling workers/i })).toBeInTheDocument();
    expect(screen.queryByLabelText('Essential Workers')).not.toBeInTheDocument();

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /polling workers/i }));
      await Promise.resolve();
    });

    expect(screen.getByText('Worker Pools')).toBeInTheDocument();
    expect(screen.getByText('Isolation Limits')).toBeInTheDocument();
    expect(screen.getByLabelText('Essential Workers')).toBeInTheDocument();
    expect(screen.getByLabelText('Performance Pool')).toBeInTheDocument();
    expect(screen.getByLabelText('Max Inflight Per SNMP Profile')).toBeInTheDocument();
  });

  it('loads worker setting values from fetchSettings', async () => {
    const { fetchSettingsWithMetadata } = await import('../api/client');
    (fetchSettingsWithMetadata as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      data: {
        polling_essential_workers: '64',
        snmp_worker_pool_performance_size: '12',
        snmp_worker_pool_operational_size: '6',
        snmp_worker_pool_static_size: '3',
        polling_max_workers_per_device: '2',
        polling_max_workers_per_site: '64',
        polling_max_workers_per_subnet: '8',
        polling_max_inflight_per_snmp_profile: '64',
      },
      secrets: {},
    });

    render(<SettingsPanel />);

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /polling workers/i }));
      await Promise.resolve();
    });

    await waitFor(() => {
      expect(screen.getByLabelText('Essential Workers')).toHaveValue(64);
      expect(screen.getByLabelText('Performance Pool')).toHaveValue(12);
      expect(screen.getByLabelText('Operational Pool')).toHaveValue(6);
      expect(screen.getByLabelText('Static Pool')).toHaveValue(3);
      expect(screen.getByLabelText('Max Workers Per Device')).toHaveValue(2);
      expect(screen.getByLabelText('Max Workers Per Site')).toHaveValue(64);
      expect(screen.getByLabelText('Max Workers Per Subnet')).toHaveValue(8);
      expect(screen.getByLabelText('Max Inflight Per SNMP Profile')).toHaveValue(64);
    });
  });

  it('saves a valid worker setting after the debounce window', async () => {
    vi.useFakeTimers();
    const { updateSetting } = await import('../api/client');
    render(<SettingsPanel />);

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /polling workers/i }));
      await Promise.resolve();
    });

    fireEvent.change(screen.getByLabelText('Essential Workers'), { target: { value: '72' } });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
      await Promise.resolve();
    });

    expect(updateSetting).toHaveBeenCalledWith('polling_essential_workers', '72');
  });

  it('rejects non-positive worker settings and does not save them', async () => {
    vi.useFakeTimers();
    const { updateSetting } = await import('../api/client');
    render(<SettingsPanel />);

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /polling workers/i }));
      await Promise.resolve();
    });

    const field = screen.getByLabelText('Max Workers Per Device');
    const workerSection = screen.getByText('Polling Workers').closest('div');
    fireEvent.change(field, { target: { value: '0' } });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
      await Promise.resolve();
    });

    expect(updateSetting).not.toHaveBeenCalledWith('polling_max_workers_per_device', '0');
    expect(workerSection).not.toBeNull();
    expect(
      within(workerSection as HTMLElement).getByText('Must be greater than 0'),
    ).toBeInTheDocument();
  });

  it('wraps long worker setting keys within the panel', async () => {
    render(<SettingsPanel />);

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /polling workers/i }));
      await Promise.resolve();
    });

    expect(screen.getByText('polling_max_inflight_per_snmp_profile').className).toContain(
      'break-all',
    );
  });
});
