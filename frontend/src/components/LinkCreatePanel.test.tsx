/**
 * Exercises link create panel component behavior so refactors preserve the documented contract.
 */
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fetchDeviceInterfaces } from '../api/client';
import { ServerError, ValidationError } from '../api/errors';
import type { Device, InterfaceInfo } from '../types/api';
import { LinkCreatePanel } from './LinkCreatePanel';

// Mock API calls
vi.mock('../api/client', () => ({
  createLink: vi.fn().mockResolvedValue({
    id: 'link-1',
    source_device_id: 'dev-1',
    source_if_name: 'ether1',
    target_device_id: 'dev-2',
    target_if_name: '',
    discovery_protocol: '',
  }),
  fetchDeviceInterfaces: vi.fn().mockImplementation(() => new Promise<never>(() => {})),
}));

function mockPhysicalInterfaces() {
  vi.mocked(fetchDeviceInterfaces).mockImplementation((deviceId: string) => {
    const iface = {
      if_name: 'ether1',
      if_descr: 'Eth1',
      speed: 1000000000,
      oper_status: 'up',
      admin_status: 'up',
      in_use: false,
    };
    if (deviceId === 'dev-p1' || deviceId === 'dev-p2' || deviceId === 'dev-physical') {
      return Promise.resolve([iface]);
    }
    return Promise.resolve([]);
  });
}

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-physical',
    hostname: '10.0.0.1',
    ip: '10.0.0.1',
    device_type: 'router',
    status: 'up',
    sys_name: 'Router1',
    sys_descr: '',
    hardware_model: '',
    vendor: 'default',
    managed: true,
    interfaces: [
      {
        id: 'if-1',
        if_index: 1,
        if_name: 'ether1',
        if_descr: 'Ethernet 1',
        speed: 1000000000,
        admin_status: 'up',
        oper_status: 'up',
      },
    ],
    area_ids: [],
    backup_supported: false,
    metrics_source: 'snmp',
    prometheus_label_name: 'instance',
    prometheus_label_value: '',
    ...overrides,
  };
}

function mockVirtualDevice(overrides: Partial<Device> = {}): Device {
  return mockDevice({
    id: 'dev-virtual',
    hostname: 'ISP Gateway',
    ip: '',
    device_type: 'virtual',
    status: 'unknown',
    sys_name: '',
    interfaces: [],
    tags: { display_name: 'ISP Gateway', virtual_subtype: 'internet' },
    ...overrides,
  });
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

function mockInterface(ifName: string): InterfaceInfo {
  return {
    if_name: ifName,
    if_descr: ifName,
    speed: 1_000_000_000,
    oper_status: 'up',
    admin_status: 'up',
    in_use: false,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(fetchDeviceInterfaces).mockImplementation(() => new Promise<never>(() => {}));
});

describe('LinkCreatePanel', () => {
  const baseProps = {
    onCreated: vi.fn(),
    onClose: vi.fn(),
  };

  describe('virtual device handling', () => {
    it('hides interface selector when source device is virtual', () => {
      const devices = [mockVirtualDevice(), mockDevice()];
      render(
        <LinkCreatePanel {...baseProps} devices={devices} initialSourceDeviceId="dev-virtual" />,
      );
      expect(screen.getByText(/virtual node/i)).toBeInTheDocument();
    });

    it('shows interface selector for physical device', () => {
      const devices = [mockDevice(), mockVirtualDevice()];
      render(
        <LinkCreatePanel {...baseProps} devices={devices} initialSourceDeviceId="dev-physical" />,
      );
      // Physical device should show port selector, not "(virtual node ...)" label
      expect(screen.queryByText(/virtual node/i)).not.toBeInTheDocument();
    });

    it('shows error when both devices are virtual', () => {
      const virtual1 = mockVirtualDevice({ id: 'virt-1' });
      const virtual2 = mockVirtualDevice({
        id: 'virt-2',
        hostname: 'Cloud Endpoint',
        tags: { display_name: 'Cloud Endpoint', virtual_subtype: 'cloud' },
      });
      const devices = [virtual1, virtual2];
      render(
        <LinkCreatePanel
          {...baseProps}
          devices={devices}
          initialSourceDeviceId="virt-1"
          initialTargetDeviceId="virt-2"
        />,
      );
      expect(screen.getByText('At least one device must be physical')).toBeInTheDocument();
    });

    it('disables Create button when both devices are virtual', () => {
      const virtual1 = mockVirtualDevice({ id: 'virt-1' });
      const virtual2 = mockVirtualDevice({
        id: 'virt-2',
        hostname: 'Cloud Endpoint',
        tags: { display_name: 'Cloud Endpoint', virtual_subtype: 'cloud' },
      });
      const devices = [virtual1, virtual2];
      render(
        <LinkCreatePanel
          {...baseProps}
          devices={devices}
          initialSourceDeviceId="virt-1"
          initialTargetDeviceId="virt-2"
        />,
      );
      const createBtn = screen.getByText('Create Link');
      expect(createBtn).toBeDisabled();
    });

    it('shows display name for virtual device without IP in dropdown', () => {
      const devices = [mockVirtualDevice(), mockDevice()];
      render(
        <LinkCreatePanel {...baseProps} devices={devices} initialSourceDeviceId="dev-virtual" />,
      );
      // The selected device display should show the display_name
      expect(screen.getByText('ISP Gateway')).toBeInTheDocument();
    });
  });

  describe('interface request ownership', () => {
    it('ignores a stale source response that resolves after the selected source response', async () => {
      const sourceARequest = deferred<InterfaceInfo[]>();
      const sourceBRequest = deferred<InterfaceInfo[]>();
      vi.mocked(fetchDeviceInterfaces).mockImplementation((deviceId: string) => {
        if (deviceId === 'source-a') return sourceARequest.promise;
        if (deviceId === 'source-b') return sourceBRequest.promise;
        return Promise.resolve([]);
      });
      const sourceA = mockDevice({
        id: 'source-a',
        ip: '10.0.1.1',
        hostname: '10.0.1.1',
        sys_name: 'Source A',
      });
      const sourceB = mockDevice({
        id: 'source-b',
        ip: '10.0.1.2',
        hostname: '10.0.1.2',
        sys_name: 'Source B',
      });

      render(
        <LinkCreatePanel
          {...baseProps}
          devices={[sourceA, sourceB, mockVirtualDevice()]}
          initialSourceDeviceId="source-a"
          initialTargetDeviceId="dev-virtual"
        />,
      );

      act(() => {
        fireEvent.click(screen.getByRole('button', { name: /10\.0\.1\.1/ }));
      });
      act(() => {
        fireEvent.click(screen.getByRole('button', { name: /10\.0\.1\.2/ }));
      });

      await act(async () => {
        sourceBRequest.resolve([mockInterface('source-b-port')]);
      });
      const sourcePort = screen.getByRole('combobox');
      act(() => {
        fireEvent.change(sourcePort, { target: { value: 'source-b-port' } });
      });

      await act(async () => {
        sourceARequest.resolve([mockInterface('source-a-port')]);
      });

      expect(screen.getByRole('option', { name: /source-b-port/ })).toBeInTheDocument();
      expect(screen.queryByRole('option', { name: /source-a-port/ })).not.toBeInTheDocument();
      expect(sourcePort).toHaveValue('source-b-port');
      expect(screen.getByRole('button', { name: 'Create Link' })).toBeEnabled();
    });

    it('ignores a stale target response that resolves after the selected target response', async () => {
      const targetARequest = deferred<InterfaceInfo[]>();
      const targetBRequest = deferred<InterfaceInfo[]>();
      vi.mocked(fetchDeviceInterfaces).mockImplementation((deviceId: string) => {
        if (deviceId === 'target-a') return targetARequest.promise;
        if (deviceId === 'target-b') return targetBRequest.promise;
        return Promise.resolve([]);
      });
      const targetA = mockDevice({
        id: 'target-a',
        ip: '10.0.2.1',
        hostname: '10.0.2.1',
        sys_name: 'Target A',
      });
      const targetB = mockDevice({
        id: 'target-b',
        ip: '10.0.2.2',
        hostname: '10.0.2.2',
        sys_name: 'Target B',
      });

      render(
        <LinkCreatePanel
          {...baseProps}
          devices={[mockVirtualDevice(), targetA, targetB]}
          initialSourceDeviceId="dev-virtual"
          initialTargetDeviceId="target-a"
        />,
      );

      act(() => {
        fireEvent.click(screen.getByRole('button', { name: /10\.0\.2\.1/ }));
      });
      act(() => {
        fireEvent.click(screen.getByRole('button', { name: /10\.0\.2\.2/ }));
      });

      await act(async () => {
        targetBRequest.resolve([mockInterface('target-b-port')]);
      });
      const targetPort = screen.getByRole('combobox');
      act(() => {
        fireEvent.change(targetPort, { target: { value: 'target-b-port' } });
      });

      await act(async () => {
        targetARequest.resolve([mockInterface('target-a-port')]);
      });

      expect(screen.getByRole('option', { name: /target-b-port/ })).toBeInTheDocument();
      expect(screen.queryByRole('option', { name: /target-a-port/ })).not.toBeInTheDocument();
      expect(targetPort).toHaveValue('target-b-port');
      expect(screen.getByRole('button', { name: 'Create Link' })).toBeEnabled();
    });

    it('clears incompatible source options and keeps the active source request loading', async () => {
      const sourceARequest = deferred<InterfaceInfo[]>();
      const sourceBRequest = deferred<InterfaceInfo[]>();
      vi.mocked(fetchDeviceInterfaces).mockImplementation((deviceId: string) => {
        if (deviceId === 'source-a') return sourceARequest.promise;
        if (deviceId === 'source-b') return sourceBRequest.promise;
        return Promise.resolve([]);
      });
      const sourceA = mockDevice({
        id: 'source-a',
        ip: '10.0.3.1',
        hostname: '10.0.3.1',
        sys_name: 'Source A',
      });
      const sourceB = mockDevice({
        id: 'source-b',
        ip: '10.0.3.2',
        hostname: '10.0.3.2',
        sys_name: 'Source B',
      });

      render(
        <LinkCreatePanel
          {...baseProps}
          devices={[sourceA, sourceB, mockVirtualDevice()]}
          initialSourceDeviceId="source-a"
          initialTargetDeviceId="dev-virtual"
        />,
      );

      act(() => {
        fireEvent.click(screen.getByRole('button', { name: /10\.0\.3\.1/ }));
      });
      act(() => {
        fireEvent.click(screen.getByRole('button', { name: /10\.0\.3\.2/ }));
      });

      const sourcePort = screen.getByRole('combobox');
      expect(sourcePort).toBeDisabled();
      expect(sourcePort).toHaveValue('');

      await act(async () => {
        sourceARequest.resolve([mockInterface('source-a-port')]);
      });

      expect(sourcePort).toBeDisabled();
      expect(sourcePort).toHaveValue('');
      expect(screen.queryByRole('option', { name: /source-a-port/ })).not.toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Create Link' })).toBeDisabled();

      await act(async () => {
        sourceBRequest.resolve([mockInterface('source-b-port')]);
      });

      expect(sourcePort).toBeEnabled();
      expect(screen.getByRole('option', { name: /source-b-port/ })).toBeInTheDocument();
    });
  });
});

// --- Gap 10: LinkCreatePanel typed errors ---

describe('LinkCreatePanel — handleSubmit catch handles typed errors', () => {
  const physical1 = {
    id: 'dev-p1',
    hostname: '10.0.0.1',
    ip: '10.0.0.1',
    device_type: 'router' as const,
    status: 'up' as const,
    sys_name: 'Router1',
    sys_descr: '',
    hardware_model: '',
    vendor: 'default',
    managed: true,
    interfaces: [
      {
        id: 'if-1',
        if_index: 1,
        if_name: 'ether1',
        if_descr: 'Eth1',
        speed: 1000000000,
        admin_status: 'up' as const,
        oper_status: 'up' as const,
      },
    ],
    area_ids: [],
    backup_supported: false,
    metrics_source: 'snmp' as const,
    prometheus_label_name: 'instance',
    prometheus_label_value: '',
  };

  const physical2 = {
    ...physical1,
    id: 'dev-p2',
    ip: '10.0.0.2',
    hostname: '10.0.0.2',
    sys_name: 'Router2',
    interfaces: [
      {
        id: 'if-2',
        if_index: 1,
        if_name: 'ether1',
        if_descr: 'Eth1',
        speed: 1000000000,
        admin_status: 'up' as const,
        oper_status: 'up' as const,
      },
    ],
  };

  it('shows ServerError ref message when createLink throws ServerError', async () => {
    mockPhysicalInterfaces();
    const { createLink } = await import('../api/client');
    (createLink as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ServerError('internal error, ref: lnk001', 'lnk001'),
    );

    render(
      <LinkCreatePanel
        devices={[physical1, physical2]}
        onCreated={vi.fn()}
        onClose={vi.fn()}
        initialSourceDeviceId="dev-p1"
        initialTargetDeviceId="dev-p2"
      />,
    );

    // Wait for async interface fetch to populate the dropdowns
    await screen.findAllByText(/ether1/);

    const selects = screen.getAllByRole('combobox');
    fireEvent.change(selects[0], { target: { value: 'ether1' } });
    fireEvent.change(selects[1], { target: { value: 'ether1' } });

    fireEvent.click(screen.getByText('Create Link'));

    await waitFor(() => {
      expect(screen.getByText('Something went wrong (ref: lnk001)')).toBeInTheDocument();
    });
  });

  it('shows ValidationError message when createLink throws ValidationError', async () => {
    mockPhysicalInterfaces();
    const { createLink } = await import('../api/client');
    (createLink as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ValidationError('link already exists between these ports'),
    );

    render(
      <LinkCreatePanel
        devices={[physical1, physical2]}
        onCreated={vi.fn()}
        onClose={vi.fn()}
        initialSourceDeviceId="dev-p1"
        initialTargetDeviceId="dev-p2"
      />,
    );

    // Wait for async interface fetch to populate the dropdowns
    await screen.findAllByText(/ether1/);

    const selects = screen.getAllByRole('combobox');
    fireEvent.change(selects[0], { target: { value: 'ether1' } });
    fireEvent.change(selects[1], { target: { value: 'ether1' } });

    fireEvent.click(screen.getByText('Create Link'));

    await waitFor(() => {
      expect(screen.getByText('link already exists between these ports')).toBeInTheDocument();
    });
  });
});
