import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { CanvasOverlays } from './CanvasOverlays';

vi.mock('../ReconnectBanner', () => ({
  ReconnectBanner: ({ visible }: { visible: boolean }) => (
    <div data-testid="reconnect-banner">{String(visible)}</div>
  ),
}));

describe('CanvasOverlays', () => {
  it('positions the status stack near the top on mobile and bottom-center on wider screens', () => {
    const { getByTestId } = render(
      <CanvasOverlays
        editMode={false}
        reconnecting={false}
        topologyRecoveryNotice={null}
        dismissTopologyRecoveryNotice={vi.fn()}
        retryTopologyRefresh={vi.fn()}
        selectedNodeCount={0}
        prometheusDiagnosticsVisible={false}
      />,
    );

    const stack = getByTestId('canvas-overlay-stack');
    expect(stack.className).toContain('top-20');
    expect(stack.className).toContain('bottom-auto');
    expect(stack.className).toContain('sm:bottom-16');
    expect(stack.className).toContain('sm:top-auto');
  });

  it('renders a non-alerting Prometheus diagnostics pill when degraded', () => {
    const { getByText } = render(
      <CanvasOverlays
        editMode={false}
        reconnecting={false}
        topologyRecoveryNotice={null}
        dismissTopologyRecoveryNotice={vi.fn()}
        retryTopologyRefresh={vi.fn()}
        selectedNodeCount={0}
        prometheusDiagnosticsVisible
      />,
    );

    expect(getByText('Prometheus degraded')).toBeInTheDocument();
    expect(getByText(/diagnostics only/i)).toBeInTheDocument();
  });

  it('does not render the Prometheus diagnostics pill when healthy', () => {
    const { queryByText } = render(
      <CanvasOverlays
        editMode={false}
        reconnecting={false}
        topologyRecoveryNotice={null}
        dismissTopologyRecoveryNotice={vi.fn()}
        retryTopologyRefresh={vi.fn()}
        selectedNodeCount={0}
        prometheusDiagnosticsVisible={false}
      />,
    );

    expect(queryByText('Prometheus degraded')).not.toBeInTheDocument();
  });

  it('still dismisses topology recovery notices independently of diagnostics pills', () => {
    const dismissTopologyRecoveryNotice = vi.fn();
    const { getByTitle } = render(
      <CanvasOverlays
        editMode={false}
        reconnecting={false}
        topologyRecoveryNotice={{ tone: 'warning', message: 'Delayed', actionLabel: 'Retry' }}
        dismissTopologyRecoveryNotice={dismissTopologyRecoveryNotice}
        retryTopologyRefresh={vi.fn()}
        selectedNodeCount={0}
        prometheusDiagnosticsVisible
      />,
    );

    fireEvent.click(getByTitle('Dismiss'));
    expect(dismissTopologyRecoveryNotice).toHaveBeenCalledTimes(1);
  });

  it('does not expose bulk edit when nodes are selected outside edit mode', () => {
    const onBulkEditClick = vi.fn();
    render(
      <CanvasOverlays
        editMode={false}
        reconnecting={false}
        topologyRecoveryNotice={null}
        dismissTopologyRecoveryNotice={vi.fn()}
        retryTopologyRefresh={vi.fn()}
        selectedNodeCount={2}
        prometheusDiagnosticsVisible={false}
        onBulkEditClick={onBulkEditClick}
      />,
    );

    expect(screen.getByText('2')).toBeInTheDocument();
    expect(screen.getByText('nodes selected')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /nodes selected/i })).not.toBeInTheDocument();
    expect(onBulkEditClick).not.toHaveBeenCalled();
  });
});
