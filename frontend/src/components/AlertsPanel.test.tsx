import { describe, it, expect, vi } from 'vitest';
import { render } from '@testing-library/react';
import { AlertsPanel } from './AlertsPanel';
import type { AlertsPanelModel } from './panelModels';

vi.mock('./MaterialIcon', () => ({
  MaterialIcon: ({ name }: { name: string }) => (
    <span data-testid={`material-icon-${name}`} className="material-symbols-rounded">{name}</span>
  ),
}));

function mockAlert(overrides: Partial<AlertsPanelModel['firingAlerts'][number]> = {}): AlertsPanelModel['firingAlerts'][number] {
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
    firingAlerts: [mockAlert()],
    resolvedAlerts: [],
    prometheusOutage: null,
    ...overrides,
  };
}

describe('AlertsPanel (COMP-06)', () => {
  it('firing status dot has animate-pulse class', () => {
    const alerts = [mockAlert({ state: 'firing' })];
    const { container } = render(
      <AlertsPanel model={mockModel({ firingAlerts: alerts })} />,
    );
    const spans = Array.from(container.querySelectorAll('span'));
    const firingDot = spans.find((s) =>
      s.className.includes('bg-status-down') && s.className.includes('animate-pulse'),
    );
    expect(firingDot).not.toBeUndefined();
  });

  it('firing status dot has a glow shadow', () => {
    const alerts = [mockAlert({ state: 'firing' })];
    const { container } = render(
      <AlertsPanel model={mockModel({ firingAlerts: alerts })} />,
    );
    expect(container.innerHTML).toContain('rgba(255,23,68');
  });

  it('alert cards do not have border border-outline class', () => {
    const alerts = [mockAlert({ state: 'firing' })];
    const { container } = render(
      <AlertsPanel model={mockModel({ firingAlerts: alerts })} />,
    );
    const alertCard = container.querySelector('.bg-elevated');
    expect(alertCard).not.toBeNull();
    expect(alertCard?.className).not.toContain('border border-outline');
  });

  it('has motion-reduce:animate-none on pulse elements', () => {
    const alerts = [mockAlert({ state: 'firing' })];
    const { container } = render(
      <AlertsPanel model={mockModel({ firingAlerts: alerts })} />,
    );
    expect(container.innerHTML).toContain('motion-reduce:animate-none');
  });

  it('renders Prometheus outage groupings from the adapted model', () => {
    const { getByText } = render(
      <AlertsPanel
        model={mockModel({
          firingAlerts: [],
          prometheusOutage: {
            offlineDevices: [{ id: 'dev-1', label: 'Core Router' }],
            fallbackDevices: [{ id: 'dev-2', label: 'Edge Switch' }],
          },
        })}
      />,
    );

    expect(getByText('Prometheus unreachable')).toBeInTheDocument();
    expect(getByText('Core Router')).toBeInTheDocument();
    expect(getByText('Edge Switch')).toBeInTheDocument();
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

  it('does not render Prometheus outage UI without an outage model', () => {
    const { queryByText } = render(
      <AlertsPanel model={mockModel({ firingAlerts: [] })} />,
    );

    expect(queryByText('Prometheus unreachable')).not.toBeInTheDocument();
  });
});
