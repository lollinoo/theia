import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { LinkCreatePanel } from './LinkCreatePanel';
import type { Device, Link } from '../types/api';
import { ValidationError, ServerError } from '../api/errors';

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
}));

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
      { id: 'if-1', if_index: 1, if_name: 'ether1', if_descr: 'Ethernet 1', speed: 1000000000, admin_status: 'up', oper_status: 'up' },
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

beforeEach(() => {
  vi.clearAllMocks();
});

describe('LinkCreatePanel', () => {
  const baseProps = {
    links: [] as Link[],
    onCreated: vi.fn(),
    onClose: vi.fn(),
  };

  describe('virtual device handling', () => {
    it('hides interface selector when source device is virtual', () => {
      const devices = [mockVirtualDevice(), mockDevice()];
      render(
        <LinkCreatePanel
          {...baseProps}
          devices={devices}
          initialSourceDeviceId="dev-virtual"
        />,
      );
      expect(screen.getByText(/virtual node/i)).toBeInTheDocument();
    });

    it('shows interface selector for physical device', () => {
      const devices = [mockDevice(), mockVirtualDevice()];
      render(
        <LinkCreatePanel
          {...baseProps}
          devices={devices}
          initialSourceDeviceId="dev-physical"
        />,
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
        <LinkCreatePanel
          {...baseProps}
          devices={devices}
          initialSourceDeviceId="dev-virtual"
        />,
      );
      // The selected device display should show the display_name
      expect(screen.getByText('ISP Gateway')).toBeInTheDocument();
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
      { id: 'if-1', if_index: 1, if_name: 'ether1', if_descr: 'Eth1', speed: 1000000000, admin_status: 'up' as const, oper_status: 'up' as const },
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
      { id: 'if-2', if_index: 1, if_name: 'ether1', if_descr: 'Eth1', speed: 1000000000, admin_status: 'up' as const, oper_status: 'up' as const },
    ],
  };

  it('shows ServerError ref message when createLink throws ServerError', async () => {
    const { createLink } = await import('../api/client');
    (createLink as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ServerError('internal error, ref: lnk001', 'lnk001'),
    );

    render(
      <LinkCreatePanel
        devices={[physical1, physical2]}
        links={[]}
        onCreated={vi.fn()}
        onClose={vi.fn()}
        initialSourceDeviceId="dev-p1"
        initialTargetDeviceId="dev-p2"
      />,
    );

    // Select source interface
    const selects = screen.getAllByRole('combobox');
    // First combobox is source interface selector (physical device with ether1)
    fireEvent.change(selects[0], { target: { value: 'ether1' } });
    // Second is target interface selector
    fireEvent.change(selects[1], { target: { value: 'ether1' } });

    fireEvent.click(screen.getByText('Create Link'));

    await waitFor(() => {
      expect(screen.getByText('Something went wrong (ref: lnk001)')).toBeInTheDocument();
    });
  });

  it('shows ValidationError message when createLink throws ValidationError', async () => {
    const { createLink } = await import('../api/client');
    (createLink as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ValidationError('link already exists between these ports'),
    );

    render(
      <LinkCreatePanel
        devices={[physical1, physical2]}
        links={[]}
        onCreated={vi.fn()}
        onClose={vi.fn()}
        initialSourceDeviceId="dev-p1"
        initialTargetDeviceId="dev-p2"
      />,
    );

    const selects = screen.getAllByRole('combobox');
    fireEvent.change(selects[0], { target: { value: 'ether1' } });
    fireEvent.change(selects[1], { target: { value: 'ether1' } });

    fireEvent.click(screen.getByText('Create Link'));

    await waitFor(() => {
      expect(screen.getByText('link already exists between these ports')).toBeInTheDocument();
    });
  });
});
