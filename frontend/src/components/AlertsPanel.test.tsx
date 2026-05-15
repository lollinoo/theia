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
          activeAlertCount: 0,
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
          activeAlertCount: 0,
          firingAlerts: [],
          resolvedAlerts: [mockAlert({ state: 'resolved', alertName: 'RecoveredCPU' })],
        })}
      />,
    );

    expect(getByText('Resolved alerts (1)')).toBeInTheDocument();
    expect(getByText('Problem: Recovered CPU')).toBeInTheDocument();
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

    expect(getByText('Active alerts (3)')).toBeInTheDocument();
    expect(
      getByText(
        /runtime reports 3 active alerts, but only 1 can be shown as individual rows right now/i,
      ),
    ).toBeInTheDocument();
  });

  it('shows runtime active alert count when no individual rows can be rendered', () => {
    const { getByText, queryByText } = render(
      <AlertsPanel
        model={mockModel({
          activeAlertCount: 2,
          firingAlerts: [],
        })}
      />,
    );

    expect(getByText('Active alerts (2)')).toBeInTheDocument();
    expect(
      getByText(/runtime reports 2 active alerts, but no individual rows can be shown right now/i),
    ).toBeInTheDocument();
    expect(queryByText('No active alerts')).not.toBeInTheDocument();
  });

  it('uses singular active alert copy when one runtime alert has no row', () => {
    const { getByText, queryByText } = render(
      <AlertsPanel
        model={mockModel({
          activeAlertCount: 1,
          firingAlerts: [],
        })}
      />,
    );

    expect(getByText('Active alerts (1)')).toBeInTheDocument();
    expect(
      getByText(/runtime reports 1 active alert, but no individual rows can be shown right now/i),
    ).toBeInTheDocument();
    expect(queryByText('No active alerts')).not.toBeInTheDocument();
  });

  it.each([
    ['HighCPU', 'High CPU'],
    ['SNMPDegraded', 'SNMP degraded'],
    ['ALERT_CPU_HIGH', 'Alert CPU high'],
    ['HTTP5xxErrors', 'HTTP 5xx errors'],
    ['BGPNeighborDown', 'BGP neighbor down'],
  ])('renders %s as the readable problem title %s', (alertName, expectedTitle) => {
    const { getByText } = render(
      <AlertsPanel
        model={mockModel({
          firingAlerts: [mockAlert({ alertName })],
        })}
      />,
    );

    expect(getByText(`Problem: ${expectedTitle}`)).toBeInTheDocument();
  });

  it('renders the device label as the primary location and normalizes the problem title', () => {
    const { getByText } = render(
      <AlertsPanel
        model={mockModel({
          firingAlerts: [
            mockAlert({
              deviceLabel: 'core-router-01',
              alertName: 'LinkTelemetryUnavailable',
            }),
          ],
        })}
      />,
    );

    expect(getByText('core-router-01')).toHaveClass('text-sm', 'font-medium', 'text-on-bg');
    expect(getByText('Problem: Link telemetry unavailable')).toBeInTheDocument();
    expect(getByText('Details: CPU usage is high')).toBeInTheDocument();
  });

  it('does not render Prometheus diagnostics without a diagnostics model', () => {
    const { queryByText } = render(
      <AlertsPanel model={mockModel({ activeAlertCount: 0, firingAlerts: [] })} />,
    );

    expect(queryByText('Prometheus diagnostics unavailable')).not.toBeInTheDocument();
  });
});
