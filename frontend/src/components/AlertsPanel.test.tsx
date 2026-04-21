import { render } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { AlertsPanel } from './AlertsPanel';
import type { AlertsPanelModel } from './panelModels';

vi.mock('./MaterialIcon', () => ({
  MaterialIcon: ({ name }: { name: string }) => (
    <span data-testid={`material-icon-${name}`} className="material-symbols-rounded">
      {name}
    </span>
  ),
}));

function mockAlert(
  overrides: Partial<AlertsPanelModel['firingAlerts'][number]> = {},
): AlertsPanelModel['firingAlerts'][number] {
  return {
    deviceId: 'dev-1',
    deviceLabel: 'router-01',
    alertName: 'HighCPU',
    severity: 'critical',
    state: 'firing',
    summary: 'CPU usage is high',
    ...overrides,
  };
}

function mockModel(overrides: Partial<AlertsPanelModel> = {}): AlertsPanelModel {
  return {
    activeAlertCount: 1,
    firingAlerts: [mockAlert()],
    resolvedAlerts: [],
    prometheusDiagnostics: null,
    ...overrides,
  };
}

describe('AlertsPanel (COMP-06)', () => {
  it('firing status dot has animate-pulse class', () => {
    const alerts = [mockAlert({ state: 'firing' })];
    const { container } = render(<AlertsPanel model={mockModel({ firingAlerts: alerts })} />);
    const spans = Array.from(container.querySelectorAll('span'));
    const firingDot = spans.find(
      (s) => s.className.includes('bg-status-down') && s.className.includes('animate-pulse'),
    );
    expect(firingDot).not.toBeUndefined();
  });

  it('firing status dot has a glow shadow', () => {
    const alerts = [mockAlert({ state: 'firing' })];
    const { container } = render(<AlertsPanel model={mockModel({ firingAlerts: alerts })} />);
    expect(container.innerHTML).toContain('rgba(255,23,68');
  });

  it('alert cards do not have border border-outline class', () => {
    const alerts = [mockAlert({ state: 'firing' })];
    const { container } = render(<AlertsPanel model={mockModel({ firingAlerts: alerts })} />);
    const alertCard = container.querySelector('.bg-elevated');
    expect(alertCard).not.toBeNull();
    expect(alertCard?.className).not.toContain('border border-outline');
  });

  it('has motion-reduce:animate-none on pulse elements', () => {
    const alerts = [mockAlert({ state: 'firing' })];
    const { container } = render(<AlertsPanel model={mockModel({ firingAlerts: alerts })} />);
    expect(container.innerHTML).toContain('motion-reduce:animate-none');
  });

  it('renders Prometheus health as diagnostics only', () => {
    const { getByText } = render(
      <AlertsPanel
        model={mockModel({
          firingAlerts: [],
          prometheusDiagnostics: {
            title: 'Prometheus diagnostics unavailable',
            detail:
              'Runtime status and alerts use normalized telemetry. Prometheus health is shown here for operator diagnostics only.',
          },
        })}
      />,
    );

    expect(getByText('Prometheus diagnostics unavailable')).toBeInTheDocument();
    expect(getByText(/operator diagnostics only/i)).toBeInTheDocument();
  });

  it('renders resolved alerts from the adapted model', () => {
    const { getByText } = render(
      <AlertsPanel
        model={mockModel({
          firingAlerts: [],
          resolvedAlerts: [mockAlert({ state: 'resolved', alertName: 'RecoveredCPU' })],
        })}
      />,
    );

    expect(getByText('Resolved (1)')).toBeInTheDocument();
    expect(getByText('RecoveredCPU')).toBeInTheDocument();
  });

  it('shows the authoritative active alert count when normalized totals exceed rendered rows', () => {
    const { getByText } = render(
      <AlertsPanel
        model={mockModel({
          firingAlerts: [mockAlert()],
          activeAlertCount: 3,
        })}
      />,
    );

    expect(getByText('Active (3)')).toBeInTheDocument();
    expect(
      getByText(/showing 1 alert row while normalized runtime reports 3 active alerts/i),
    ).toBeInTheDocument();
  });

  it('does not render Prometheus diagnostics without a diagnostics model', () => {
    const { queryByText } = render(<AlertsPanel model={mockModel({ firingAlerts: [] })} />);

    expect(queryByText('Prometheus diagnostics unavailable')).not.toBeInTheDocument();
  });
});
