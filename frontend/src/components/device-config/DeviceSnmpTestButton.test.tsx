/**
 * Exercises device SNMP test button device configuration behavior so refactors preserve the documented contract.
 */
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useLayoutEffect } from 'react';
import { flushSync } from 'react-dom';
import { createRoot } from 'react-dom/client';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { testSNMPConnection } from '../../api/client';
import { DeviceSnmpTestButton } from './DeviceSnmpTestButton';

vi.mock('../../api/client', () => ({
  testSNMPConnection: vi.fn(),
}));

const mockTestSNMPConnection = vi.mocked(testSNMPConnection);

beforeEach(() => {
  vi.clearAllMocks();
});

describe('DeviceSnmpTestButton', () => {
  it('calls the SNMP test endpoint and renders successful sysName and sysDescr details', async () => {
    mockTestSNMPConnection.mockResolvedValueOnce({
      success: true,
      sys_name: 'core-router',
      sys_descr: 'RouterOS 7.15',
    });

    render(<DeviceSnmpTestButton deviceId="dev-1" />);

    fireEvent.click(screen.getByRole('button', { name: 'Test SNMP Connectivity' }));

    await waitFor(() => {
      expect(mockTestSNMPConnection).toHaveBeenCalledWith('dev-1');
    });
    expect(await screen.findByText('SNMP OK')).toBeInTheDocument();
    expect(screen.getByText('sysName: core-router')).toBeInTheDocument();
    expect(screen.getByText('sysDescr: RouterOS 7.15')).toBeInTheDocument();
  });

  it('renders failed SNMP test details with target, version, and guidance', async () => {
    mockTestSNMPConnection.mockResolvedValueOnce({
      success: false,
      error: 'timeout waiting for response',
      target_ip: '10.0.0.1',
      snmp_version: '2c',
    });

    render(<DeviceSnmpTestButton deviceId="dev-1" />);

    fireEvent.click(screen.getByRole('button', { name: 'Test SNMP Connectivity' }));

    expect(await screen.findByText('SNMP Failed')).toBeInTheDocument();
    expect(screen.getByText('timeout waiting for response')).toBeInTheDocument();
    expect(screen.getByText('Target: 10.0.0.1:161 (UDP)')).toBeInTheDocument();
    expect(screen.getByText('Version: 2c')).toBeInTheDocument();
    expect(
      screen.getByText(
        'Check: SNMP enabled on device, community/credentials correct, UDP/161 reachable from container',
      ),
    ).toBeInTheDocument();
  });

  it('disables the button, shows loading text, and clears the previous result while testing', async () => {
    let resolveSecondTest: (value: Awaited<ReturnType<typeof testSNMPConnection>>) => void =
      () => {};
    mockTestSNMPConnection
      .mockResolvedValueOnce({ success: true, sys_name: 'before-test' })
      .mockReturnValueOnce(
        new Promise((resolve) => {
          resolveSecondTest = resolve;
        }),
      );

    render(<DeviceSnmpTestButton deviceId="dev-1" />);

    fireEvent.click(screen.getByRole('button', { name: 'Test SNMP Connectivity' }));
    expect(await screen.findByText('SNMP OK')).toBeInTheDocument();
    expect(screen.getByText('sysName: before-test')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Test SNMP Connectivity' }));

    expect(screen.getByRole('button', { name: 'Testing SNMP...' })).toBeDisabled();
    expect(screen.queryByText('SNMP OK')).not.toBeInTheDocument();
    expect(screen.queryByText('sysName: before-test')).not.toBeInTheDocument();

    resolveSecondTest({ success: false, error: 'Test failed' });

    expect(await screen.findByText('SNMP Failed')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Test SNMP Connectivity' })).not.toBeDisabled();
  });

  it('uses the fallback error message when a thrown value is not an Error', async () => {
    mockTestSNMPConnection.mockRejectedValueOnce('connection failed');

    render(<DeviceSnmpTestButton deviceId="dev-1" />);

    fireEvent.click(screen.getByRole('button', { name: 'Test SNMP Connectivity' }));

    expect(await screen.findByText('SNMP Failed')).toBeInTheDocument();
    expect(screen.getByText('Test failed')).toBeInTheDocument();
  });

  it('ignores delayed SNMP test results after switching devices', async () => {
    let resolveTest: (value: Awaited<ReturnType<typeof testSNMPConnection>>) => void = () => {};
    mockTestSNMPConnection.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveTest = resolve;
      }),
    );
    const view = render(<DeviceSnmpTestButton deviceId="dev-1" />);

    fireEvent.click(screen.getByRole('button', { name: 'Test SNMP Connectivity' }));

    view.rerender(<DeviceSnmpTestButton deviceId="dev-2" />);

    await act(async () => {
      resolveTest({ success: true, sys_name: 'old-device' });
    });

    expect(screen.queryByText('SNMP OK')).not.toBeInTheDocument();
    expect(screen.queryByText('sysName: old-device')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Test SNMP Connectivity' })).toBeEnabled();
  });

  it('ignores a prior-device SNMP result that resolves during the next device commit', async () => {
    let resolveTest: (value: Awaited<ReturnType<typeof testSNMPConnection>>) => void = () => {};
    mockTestSNMPConnection.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveTest = resolve;
      }),
    );

    function ResolveDuringCommit({ active }: { active: boolean }) {
      useLayoutEffect(() => {
        if (!active) return;
        resolveTest({ success: true, sys_name: 'old-device' });
      }, [active]);
      return null;
    }

    function Harness({ deviceId, resolve }: { deviceId: string; resolve: boolean }) {
      return (
        <>
          <DeviceSnmpTestButton deviceId={deviceId} />
          <ResolveDuringCommit active={resolve} />
        </>
      );
    }

    const container = document.createElement('div');
    document.body.appendChild(container);
    const root = createRoot(container);
    try {
      await act(async () => {
        flushSync(() => {
          root.render(<Harness deviceId="dev-1" resolve={false} />);
        });
      });

      fireEvent.click(screen.getByRole('button', { name: 'Test SNMP Connectivity' }));

      await act(async () => {
        flushSync(() => {
          root.render(<Harness deviceId="dev-2" resolve={true} />);
        });
      });

      expect(screen.queryByText('SNMP OK')).not.toBeInTheDocument();
      expect(screen.queryByText('sysName: old-device')).not.toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Test SNMP Connectivity' })).toBeEnabled();
    } finally {
      await act(async () => {
        root.unmount();
      });
      container.remove();
    }
  });
});
