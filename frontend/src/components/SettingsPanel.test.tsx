/**
 * Exercises settings panel component behavior so refactors preserve the documented contract.
 */
import {
  act,
  fireEvent,
  type RenderResult,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { SettingsPanel } from './SettingsPanel';

// Mock API calls made in SettingsPanel on mount
vi.mock('../api/client', () => {
  const pendingApiCall = () => new Promise<never>(() => {});

  return {
    fetchSettings: vi.fn().mockImplementation(pendingApiCall),
    fetchSettingsWithMetadata: vi.fn().mockImplementation(pendingApiCall),
    updateSetting: vi.fn().mockResolvedValue(undefined),
    fetchHealthRuntime: vi.fn().mockResolvedValue({ environment: 'staging' }),
  };
});

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

async function renderSettingsPanel(ui = <SettingsPanel />): Promise<RenderResult> {
  let result: RenderResult | undefined;

  await act(async () => {
    result = render(ui);
    await Promise.resolve();
  });

  if (!result) {
    throw new Error('SettingsPanel render did not complete');
  }

  return result;
}

describe('SettingsPanel (COMP-05)', () => {
  it('renders settings in UserSettingsPage-style section cards', async () => {
    await renderSettingsPanel();

    for (const heading of [
      'Polling',
      'SNMP Debug',
      'Topology',
      'Integrations',
      'Bridge & Time',
      'Profiles',
      'Backups',
      'Runtime',
    ]) {
      const title = screen.getByRole('heading', { name: heading });
      const section = title.closest('section');
      expect(section?.className).toContain('shadow-panel');
    }
  });

  it('uses independent desktop columns so expanded sections do not push the opposite column', async () => {
    await renderSettingsPanel();

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
    ).toHaveLength(4);
    expect(
      Array.from(screen.getByTestId('settings-panel-right-column').children).filter(
        (child) => child.tagName === 'SECTION',
      ),
    ).toHaveLength(4);
  });

  it('keeps section cards from stretching when a neighboring section expands', async () => {
    await renderSettingsPanel();

    const layout = screen.getByTestId('settings-panel-layout');
    expect(layout.className).toContain('items-start');

    for (const column of Array.from(layout.children)) {
      for (const section of Array.from(column.children)) {
        expect(section.className).toContain('self-start');
      }
    }
  });

  it('uses fixed-height cards with internal scrolling so columns remain aligned', async () => {
    await renderSettingsPanel();

    for (const heading of [
      'Polling',
      'SNMP Debug',
      'Topology',
      'Integrations',
      'Bridge & Time',
      'Profiles',
      'Backups',
      'Runtime',
    ]) {
      const section = screen.getByRole('heading', { name: heading }).closest('section');
      expect(section?.className).toContain('h-[22rem]');
      expect(section?.querySelector('[data-testid="settings-section-body"]')?.className).toContain(
        'overflow-y-auto',
      );
    }
  });

  it('gives profile managers a surfaced well inside the Profiles section', async () => {
    await renderSettingsPanel();

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

  it('does not render the legacy Grafana URL field in Integrations', async () => {
    await renderSettingsPanel();

    expect(screen.queryByLabelText('Grafana URL')).not.toBeInTheDocument();
    expect(screen.queryByPlaceholderText('http://localhost:3001')).not.toBeInTheDocument();
    expect(screen.getByTestId('grafana-profile-manager')).toBeInTheDocument();
  });

  it('uses stable label rows for device backup fields', async () => {
    await renderSettingsPanel();

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /device backups/i }));
    });

    expect(screen.getByTestId('device-backup-schedule-label-row').className).toContain('min-h-10');
    expect(screen.getByTestId('device-backup-retention-label-row').className).toContain('min-h-10');
  });

  it('uses stable helper rows for device backup fields', async () => {
    await renderSettingsPanel();

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /device backups/i }));
    });

    expect(screen.getByTestId('device-backup-schedule-helper-row').className).toContain('min-h-4');
    expect(screen.getByTestId('device-backup-retention-helper-row').className).toContain('min-h-4');
  });

  it('form inputs use border-outline-subtle (not border-outline)', async () => {
    const { container } = await renderSettingsPanel();
    const inputs = Array.from(container.querySelectorAll('input, select'));
    // Every input/select with a border class should use border-outline-subtle
    const withOldBorder = inputs.filter(
      (el) =>
        el.className.includes('border-outline') && !el.className.includes('border-outline-subtle'),
    );
    expect(withOldBorder).toHaveLength(0);
  });

  it('does not have border-t section separators (no-line rule)', async () => {
    const { container } = await renderSettingsPanel();
    const html = container.innerHTML;
    expect(html).not.toContain('border-t border-outline');
  });

  it('inputs have focus:ring-primary/30 for standardized focus ring', async () => {
    const { container } = await renderSettingsPanel();
    const inputs = Array.from(container.querySelectorAll('input, select'));
    // At least one input should have the standardized focus ring
    const withFocusRing = inputs.filter((el) => el.className.includes('ring-primary'));
    expect(withFocusRing.length).toBeGreaterThan(0);
  });

  it('dev badge uses semantic warning token (not hardcoded yellow)', async () => {
    const { container } = await renderSettingsPanel();
    const html = container.innerHTML;
    expect(html).not.toContain('yellow-500');
    expect(html).not.toContain('yellow-400');
  });

  it('renders deployment environment without build metadata', async () => {
    await renderSettingsPanel();

    expect(await screen.findByText('staging')).toBeInTheDocument();
    expect(screen.queryByText(/Theia\s+v/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/Commit:/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/Built:/i)).not.toBeInTheDocument();
  });

  it('does not render area management controls in global settings', async () => {
    await renderSettingsPanel();

    expect(screen.queryByTestId('area-manager')).not.toBeInTheDocument();
  });

  it('renders topology discovery default selector and saves changes', async () => {
    const { updateSetting } = await import('../api/client');

    await renderSettingsPanel();

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

  it('describes timezone as the display timezone and notifies parents when it changes', async () => {
    const { updateSetting } = await import('../api/client');
    const onSettingsChange = vi.fn();

    await renderSettingsPanel(<SettingsPanel onSettingsChange={onSettingsChange} />);

    expect(screen.getByText('Display timezone')).toBeInTheDocument();
    expect(
      screen.getByText(/Admin audit log times, backup filenames, and zip timestamps/i),
    ).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText('Display timezone'), {
      target: { value: 'Europe/Rome' },
    });

    await waitFor(() => {
      expect(updateSetting).toHaveBeenCalledWith('timezone', 'Europe/Rome');
      expect(onSettingsChange).toHaveBeenCalledWith({ timezone: 'Europe/Rome' });
    });
  });
});

describe('SettingsPanel — default network probe ports', () => {
  beforeEach(async () => {
    const { fetchSettingsWithMetadata, updateSetting } = await import('../api/client');
    (fetchSettingsWithMetadata as ReturnType<typeof vi.fn>).mockClear();
    (updateSetting as ReturnType<typeof vi.fn>).mockClear();
  });

  it('loads network_probe_ports from settings into the default probe ports input', async () => {
    const { fetchSettingsWithMetadata } = await import('../api/client');
    (fetchSettingsWithMetadata as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      data: {
        network_probe_ports: '22,8291,80,443',
      },
      secrets: {},
    });

    await renderSettingsPanel();

    await waitFor(() => {
      expect(screen.getByLabelText('Default network probe ports')).toHaveValue('22,8291,80,443');
    });
  });

  it('saves normalized default network probe ports on blur', async () => {
    const { fetchSettingsWithMetadata, updateSetting } = await import('../api/client');
    (fetchSettingsWithMetadata as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      data: {
        network_probe_ports: '22,8291,80,443',
      },
      secrets: {},
    });

    await renderSettingsPanel();
    const input = await screen.findByLabelText('Default network probe ports');

    await act(async () => {
      fireEvent.change(input, { target: { value: '22,443' } });
      fireEvent.blur(input);
      await Promise.resolve();
    });

    await waitFor(() => {
      expect(updateSetting).toHaveBeenCalledWith('network_probe_ports', '22,443');
    });
  });

  it('rejects invalid default network probe ports without saving', async () => {
    const { fetchSettingsWithMetadata, updateSetting } = await import('../api/client');
    (fetchSettingsWithMetadata as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      data: {
        network_probe_ports: '22,8291,80,443',
      },
      secrets: {},
    });

    await renderSettingsPanel();
    const input = await screen.findByLabelText('Default network probe ports');

    await act(async () => {
      fireEvent.change(input, { target: { value: '0,443' } });
      fireEvent.blur(input);
      await Promise.resolve();
    });

    expect(screen.getByText('Ports must be between 1 and 65535')).toBeInTheDocument();
    expect(updateSetting).not.toHaveBeenCalledWith('network_probe_ports', '0,443');
  });
});

describe('SettingsPanel — Prometheus URL validation on blur', () => {
  it('shows error text when Prometheus URL is blurred with an invalid value', async () => {
    await renderSettingsPanel();

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
    await renderSettingsPanel();

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
    await renderSettingsPanel();

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
    await renderSettingsPanel();

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

// --- Gap 4: Retention input has min=1 max=365 attributes ---

describe('SettingsPanel — Device Backups retention input attributes (Gap 4)', () => {
  it('retention input has min=1 and max=365 after expanding the section', async () => {
    await renderSettingsPanel();

    const toggleBtn = screen.getByRole('button', { name: /device backups/i });
    await act(async () => {
      fireEvent.click(toggleBtn);
    });

    // The retention input is a number input; find it by its label text
    expect(screen.getByText('Keep last N backups per device')).toBeInTheDocument();

    // Find all number inputs and identify the retention one (has min=1, max=365)
    const numberInputs = screen
      .getAllByRole('spinbutton')
      .filter(
        (el) => (el as HTMLInputElement).min === '1' && (el as HTMLInputElement).max === '365',
      );
    expect(numberInputs).toHaveLength(1);
    expect((numberInputs[0] as HTMLInputElement).min).toBe('1');
    expect((numberInputs[0] as HTMLInputElement).max).toBe('365');
  });
});

// --- Gap 5: Helper text shows "Scheduling disabled" when interval is 0 ---

describe('SettingsPanel — Device Backups helper text when disabled (Gap 5)', () => {
  it('shows "Scheduling disabled" text when interval defaults to 0', async () => {
    await renderSettingsPanel();

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

    await renderSettingsPanel();

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

    await renderSettingsPanel();

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

    await renderSettingsPanel();

    await waitFor(() => {
      expect(screen.getByDisplayValue('1337')).toBeInTheDocument();
    });
    expect(screen.queryByLabelText('WinBox Bridge Secret')).not.toBeInTheDocument();
  });
});

// --- Gap 14: Instance Backups collapsible section collapsed by default ---

describe('SettingsPanel — Instance Backups collapsible section (Gap 14)', () => {
  it('Instance Backups toggle button is present in the initial render', async () => {
    await renderSettingsPanel();

    // The collapsible header button must be visible without any interaction
    expect(screen.getByRole('button', { name: /instance backups/i })).toBeInTheDocument();
  });

  it('InstanceBackupManager is NOT rendered before the toggle is clicked (collapsed by default)', async () => {
    await renderSettingsPanel();

    // The mock InstanceBackupManager renders a div[data-testid="instance-backup-manager"]
    // It must not be in the DOM until the toggle is clicked
    expect(screen.queryByTestId('instance-backup-manager')).not.toBeInTheDocument();
  });

  it('clicking the Instance Backups toggle renders InstanceBackupManager', async () => {
    await renderSettingsPanel();

    const toggleBtn = screen.getByRole('button', { name: /instance backups/i });
    await act(async () => {
      fireEvent.click(toggleBtn);
    });

    expect(screen.getByTestId('instance-backup-manager')).toBeInTheDocument();
  });

  it('clicking the toggle a second time collapses the section again', async () => {
    await renderSettingsPanel();

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
    await renderSettingsPanel();

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

    await renderSettingsPanel();

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
    await renderSettingsPanel();

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

  it('sets worker input min and max attributes from tuning metadata', async () => {
    await renderSettingsPanel();

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /polling workers/i }));
      await Promise.resolve();
    });

    const essentialWorkers = screen.getByLabelText('Essential Workers') as HTMLInputElement;
    const performancePool = screen.getByLabelText('Performance Pool') as HTMLInputElement;
    const maxWorkersPerDevice = screen.getByLabelText('Max Workers Per Device') as HTMLInputElement;
    const maxInflightPerSNMPProfile = screen.getByLabelText(
      'Max Inflight Per SNMP Profile',
    ) as HTMLInputElement;

    expect(essentialWorkers.value).toBe('64');
    expect(essentialWorkers.min).toBe('1');
    expect(essentialWorkers.max).toBe('256');
    expect(performancePool.min).toBe('1');
    expect(performancePool.max).toBe('128');
    expect(maxWorkersPerDevice.min).toBe('1');
    expect(maxWorkersPerDevice.max).toBe('32');
    expect(maxInflightPerSNMPProfile.value).toBe('16');
    expect(maxInflightPerSNMPProfile.min).toBe('1');
    expect(maxInflightPerSNMPProfile.max).toBe('256');
  });

  it('rejects out-of-range worker settings and does not save them', async () => {
    vi.useFakeTimers();
    const { updateSetting } = await import('../api/client');
    await renderSettingsPanel();

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /polling workers/i }));
      await Promise.resolve();
    });

    const field = screen.getByLabelText('Max Workers Per Device');
    const workerSection = screen.getByText('Polling Workers').closest('div');
    fireEvent.change(field, { target: { value: '33' } });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
      await Promise.resolve();
    });

    expect(updateSetting).not.toHaveBeenCalledWith('polling_max_workers_per_device', '33');
    expect(workerSection).not.toBeNull();
    expect(
      within(workerSection as HTMLElement).getByText('Must be between 1 and 32'),
    ).toBeInTheDocument();
  });

  it('wraps long worker setting keys within the panel', async () => {
    await renderSettingsPanel();

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /polling workers/i }));
      await Promise.resolve();
    });

    expect(screen.getByText('polling_max_inflight_per_snmp_profile').className).toContain(
      'break-all',
    );
  });
});

describe('SettingsPanel — SNMP Debug settings', () => {
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

  it('renders the SNMP Debug section collapsed by default and expands on click', async () => {
    await renderSettingsPanel();

    expect(screen.getByRole('heading', { name: 'SNMP Debug' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /snmp debug parameters/i })).toBeInTheDocument();
    expect(screen.queryByLabelText('Background Timeout')).not.toBeInTheDocument();

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /snmp debug parameters/i }));
      await Promise.resolve();
    });

    expect(screen.getByText('Request Profiles')).toBeInTheDocument();
    expect(screen.getByText('Worker Pools')).toBeInTheDocument();
    expect(screen.getByText('Isolation Limits')).toBeInTheDocument();
    expect(screen.getByLabelText('Background Timeout')).toBeInTheDocument();
    expect(screen.getByLabelText('Performance Counter Timeout')).toBeInTheDocument();
    expect(screen.getByLabelText('Performance Counter Retries')).toBeInTheDocument();
    expect(screen.getByLabelText('Essential Timeout')).toBeInTheDocument();
    expect(screen.getByLabelText('Max Inflight Per SNMP Profile')).toBeInTheDocument();
    expect(screen.getByText('GETBULK Max Repetitions')).toBeInTheDocument();
    expect(screen.getByText('25')).toBeInTheDocument();
  });

  it('loads SNMP debug values from fetchSettings', async () => {
    const { fetchSettingsWithMetadata } = await import('../api/client');
    (fetchSettingsWithMetadata as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      data: {
        snmp_timeout_seconds: '5',
        snmp_retries: '2',
        snmp_performance_counter_timeout_ms: '3500',
        snmp_performance_counter_retries: '1',
        polling_essential_timeout_ms: '900',
        polling_essential_retries: '0',
        snmp_worker_pool_size: '9',
        polling_essential_workers: '32',
        snmp_worker_pool_performance_size: '14',
        snmp_worker_pool_operational_size: '10',
        snmp_worker_pool_static_size: '2',
        polling_max_workers_per_device: '1',
        polling_max_workers_per_site: '16',
        polling_max_workers_per_subnet: '16',
        polling_max_inflight_per_snmp_profile: '16',
      },
      secrets: {},
    });

    await renderSettingsPanel();

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /snmp debug parameters/i }));
      await Promise.resolve();
    });

    await waitFor(() => {
      expect(screen.getByLabelText('Background Timeout')).toHaveValue(5);
      expect(screen.getByLabelText('Background Retries')).toHaveValue(2);
      expect(screen.getByLabelText('Performance Counter Timeout')).toHaveValue(3500);
      expect(screen.getByLabelText('Performance Counter Retries')).toHaveValue(1);
      expect(screen.getByLabelText('Essential Timeout')).toHaveValue(900);
      expect(screen.getByLabelText('Essential Retries')).toHaveValue(0);
      expect(screen.getByLabelText('Legacy Total Pool')).toHaveValue(9);
      expect(screen.getByLabelText('Essential Workers')).toHaveValue(32);
      expect(screen.getByLabelText('Performance Pool')).toHaveValue(14);
      expect(screen.getByLabelText('Operational Pool')).toHaveValue(10);
      expect(screen.getByLabelText('Static Pool')).toHaveValue(2);
      expect(screen.getByLabelText('Max Workers Per Device')).toHaveValue(1);
      expect(screen.getByLabelText('Max Workers Per Site')).toHaveValue(16);
      expect(screen.getByLabelText('Max Workers Per Subnet')).toHaveValue(16);
      expect(screen.getByLabelText('Max Inflight Per SNMP Profile')).toHaveValue(16);
    });
  });

  it('saves a valid SNMP debug setting after the debounce window', async () => {
    vi.useFakeTimers();
    const { updateSetting } = await import('../api/client');
    await renderSettingsPanel();

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /snmp debug parameters/i }));
      await Promise.resolve();
    });

    fireEvent.change(screen.getByLabelText('Background Retries'), { target: { value: '3' } });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
      await Promise.resolve();
    });

    expect(updateSetting).toHaveBeenCalledWith('snmp_retries', '3');
  });

  it('saves a valid performance counter SNMP setting after the debounce window', async () => {
    vi.useFakeTimers();
    const { updateSetting } = await import('../api/client');
    await renderSettingsPanel();

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /snmp debug parameters/i }));
      await Promise.resolve();
    });

    fireEvent.change(screen.getByLabelText('Performance Counter Timeout'), {
      target: { value: '3500' },
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
      await Promise.resolve();
    });

    expect(updateSetting).toHaveBeenCalledWith('snmp_performance_counter_timeout_ms', '3500');
  });

  it('rejects out-of-range SNMP debug settings and does not save them', async () => {
    vi.useFakeTimers();
    const { updateSetting } = await import('../api/client');
    await renderSettingsPanel();

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /snmp debug parameters/i }));
      await Promise.resolve();
    });

    const debugSection = screen.getByText('SNMP Debug Parameters').closest('div');
    fireEvent.change(screen.getByLabelText('Background Timeout'), { target: { value: '121' } });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
      await Promise.resolve();
    });

    expect(updateSetting).not.toHaveBeenCalledWith('snmp_timeout_seconds', '121');
    expect(debugSection).not.toBeNull();
    expect(
      within(debugSection as HTMLElement).getByText('Must be between 1 and 120'),
    ).toBeInTheDocument();
  });

  it('rejects out-of-range performance counter SNMP settings and does not save them', async () => {
    vi.useFakeTimers();
    const { updateSetting } = await import('../api/client');
    await renderSettingsPanel();

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /snmp debug parameters/i }));
      await Promise.resolve();
    });

    const debugSection = screen.getByText('SNMP Debug Parameters').closest('div');
    fireEvent.change(screen.getByLabelText('Performance Counter Timeout'), {
      target: { value: '99' },
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
      await Promise.resolve();
    });

    expect(updateSetting).not.toHaveBeenCalledWith('snmp_performance_counter_timeout_ms', '99');
    expect(debugSection).not.toBeNull();
    expect(
      within(debugSection as HTMLElement).getByText('Must be between 100 and 30000'),
    ).toBeInTheDocument();
  });

  it('syncs shared worker values between SNMP Debug and Polling Workers', async () => {
    await renderSettingsPanel();

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /snmp debug parameters/i }));
      await Promise.resolve();
    });
    fireEvent.change(screen.getByLabelText('Performance Pool'), { target: { value: '15' } });

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /polling workers/i }));
      await Promise.resolve();
    });

    const performancePoolFields = screen.getAllByLabelText('Performance Pool');
    expect(performancePoolFields).toHaveLength(2);
    for (const field of performancePoolFields) {
      expect(field).toHaveValue(15);
    }
  });
});
