import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import type { DeviceImportTopologyRunSnapshot } from '../../api/deviceImport';
import { DeviceImportTopologyOverlay } from './DeviceImportTopologyOverlay';

const snapshot: DeviceImportTopologyRunSnapshot = {
  run: {
    id: 'run-1',
    map_id: 'map-1',
    file_digest: 'sha256:file',
    layout_scope: 'preserve',
    state: 'discovering',
    auto_layout_allowed: true,
    backgrounded: false,
    reconcile_attempts: 0,
    created_at: '2026-07-31T12:00:00Z',
    updated_at: '2026-07-31T12:00:00Z',
  },
  items: [
    {
      device_id: 'device-a',
      state: 'succeeded',
      attempt: 1,
      neighbor_count: 2,
      links_created: 1,
      unresolved_neighbors: 0,
      updated_at: '2026-07-31T12:00:00Z',
    },
    {
      device_id: 'device-b',
      state: 'warning',
      attempt: 1,
      result_code: 'unresolved_neighbors',
      message: 'Some neighbors could not be linked automatically.',
      neighbor_count: 1,
      links_created: 0,
      unresolved_neighbors: 1,
      updated_at: '2026-07-31T12:00:00Z',
    },
    {
      device_id: 'device-c',
      state: 'running',
      attempt: 1,
      neighbor_count: 0,
      links_created: 0,
      unresolved_neighbors: 0,
      updated_at: '2026-07-31T12:00:00Z',
    },
  ],
};

const progress = {
  total: 3,
  completed: 2,
  running: 1,
  warnings: 1,
  failed: 0,
  neighbors: 3,
  linksCreated: 1,
  unresolved: 1,
};

describe('DeviceImportTopologyOverlay', () => {
  it('keeps an initial run-lookup failure visible and retryable', () => {
    const onRefresh = vi.fn();
    render(
      <DeviceImportTopologyOverlay
        snapshot={null}
        phase={null}
        progress={{
          total: 0,
          completed: 0,
          running: 0,
          warnings: 0,
          failed: 0,
          neighbors: 0,
          linksCreated: 0,
          unresolved: 0,
        }}
        applyingLayout={false}
        error="Could not check active topology imports"
        deviceNames={new Map()}
        onContinue={vi.fn()}
        onRetry={vi.fn()}
        onConfigureDevice={vi.fn()}
        onCreateManualLink={vi.fn()}
        onRefresh={onRefresh}
      />,
    );

    expect(screen.getByRole('alert')).toHaveTextContent('Could not check active topology imports');
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(onRefresh).toHaveBeenCalledOnce();
  });

  it('shows explicit phases, progress, and non-blocking partial-map action', () => {
    const onContinue = vi.fn();
    render(
      <DeviceImportTopologyOverlay
        snapshot={snapshot}
        phase="discovery"
        progress={progress}
        applyingLayout={false}
        error={null}
        deviceNames={new Map([['device-b', 'Distribution switch']])}
        onContinue={onContinue}
        onRetry={vi.fn()}
        onConfigureDevice={vi.fn()}
        onCreateManualLink={vi.fn()}
        onRefresh={vi.fn()}
      />,
    );

    expect(screen.getByText('Discovering LLDP/CDP neighbors')).toBeInTheDocument();
    expect(screen.getByText('2 of 3 devices checked')).toBeInTheDocument();
    expect(screen.getByText('Discovery')).toHaveAttribute('aria-current', 'step');
    fireEvent.click(screen.getByRole('button', { name: 'Continue with partial map' }));
    expect(onContinue).toHaveBeenCalledOnce();
  });

  it('keeps warning diagnostics actionable after discovery', () => {
    const onRetry = vi.fn();
    const onConfigureDevice = vi.fn();
    const onCreateManualLink = vi.fn();
    render(
      <DeviceImportTopologyOverlay
        snapshot={{
          ...snapshot,
          run: { ...snapshot.run, state: 'ready_for_layout' },
        }}
        phase="layout"
        progress={{ ...progress, running: 0, completed: 3 }}
        applyingLayout={true}
        error={null}
        deviceNames={new Map([['device-b', 'Distribution switch']])}
        onContinue={vi.fn()}
        onRetry={onRetry}
        onConfigureDevice={onConfigureDevice}
        onCreateManualLink={onCreateManualLink}
        onRefresh={vi.fn()}
      />,
    );

    expect(screen.getByText('Distribution switch')).toBeInTheDocument();
    expect(
      screen.getByText('Some neighbors could not be linked automatically.'),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Retry affected devices' }));
    fireEvent.click(screen.getByRole('button', { name: 'Configure device' }));
    fireEvent.click(screen.getByRole('button', { name: 'Create link manually' }));
    expect(onRetry).toHaveBeenCalledWith(['device-b']);
    expect(onConfigureDevice).toHaveBeenCalledWith('device-b');
    expect(onCreateManualLink).toHaveBeenCalledOnce();
  });

  it('shows a compact background status after continuing', () => {
    render(
      <DeviceImportTopologyOverlay
        snapshot={{ ...snapshot, run: { ...snapshot.run, backgrounded: true } }}
        phase="discovery"
        progress={progress}
        applyingLayout={false}
        error={null}
        deviceNames={new Map()}
        onContinue={vi.fn()}
        onRetry={vi.fn()}
        onConfigureDevice={vi.fn()}
        onCreateManualLink={vi.fn()}
        onRefresh={vi.fn()}
      />,
    );

    expect(screen.getByTestId('topology-bootstrap-background-status')).toHaveTextContent(
      'Topology discovery continues in the background',
    );
    expect(screen.queryByRole('button', { name: 'Continue with partial map' })).toBeNull();
  });

  it('keeps a durable reconciliation failure actionable for manual recovery', () => {
    const onContinue = vi.fn();
    render(
      <DeviceImportTopologyOverlay
        snapshot={{
          ...snapshot,
          run: {
            ...snapshot.run,
            state: 'failed',
            failure_code: 'persistence_failed',
            failure_message: 'Automatic link creation could not complete.',
            failure_reference: 'safe-reference',
            reconcile_attempts: 2,
          },
        }}
        phase="links"
        progress={{ ...progress, running: 0, completed: 3 }}
        applyingLayout={false}
        error={null}
        deviceNames={new Map()}
        onContinue={onContinue}
        onRetry={vi.fn()}
        onConfigureDevice={vi.fn()}
        onCreateManualLink={vi.fn()}
        onRefresh={vi.fn()}
      />,
    );

    expect(screen.getByText('Automatic link creation needs attention')).toBeInTheDocument();
    expect(screen.getByText('Reference: safe-reference')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Continue with manual map' }));
    expect(onContinue).toHaveBeenCalledOnce();
  });
});
