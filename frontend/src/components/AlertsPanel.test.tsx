import { describe, it, expect, vi } from 'vitest';
import { render } from '@testing-library/react';
import { AlertsPanel } from './AlertsPanel';
import type { AlertDTO } from '../types/metrics';
import type { Device } from '../types/api';

// Mock MaterialIcon
vi.mock('./MaterialIcon', () => ({
  MaterialIcon: ({ name }: { name: string }) => (
    <span data-testid={`material-icon-${name}`} className="material-symbols-rounded">{name}</span>
  ),
}));

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-1',
    hostname: 'router-01',
    ip: '10.0.0.1',
    device_type: 'router',
    status: 'up',
    sys_name: 'router-01',
    sys_descr: 'RouterOS',
    hardware_model: 'RB4011',
    vendor: 'mikrotik',
    managed: true,
    interfaces: [],
    backup_supported: true,
    metrics_source: 'prometheus',
    prometheus_label_name: 'instance',
    prometheus_label_value: '10.0.0.1:9100',
    ...overrides,
  };
}

function mockAlert(overrides: Partial<AlertDTO> = {}): AlertDTO {
  return {
    id: 'alert-1',
    device_id: 'dev-1',
    alert_name: 'HighCPU',
    severity: 'critical',
    state: 'firing',
    summary: 'CPU usage is high',
    description: '',
    starts_at: '2026-01-01T00:00:00Z',
    ends_at: null,
    ...overrides,
  };
}

describe('AlertsPanel (COMP-06)', () => {
  it('firing status dot has animate-pulse class', () => {
    const alerts = [mockAlert({ state: 'firing' })];
    const { container } = render(
      <AlertsPanel alerts={alerts} devices={[mockDevice()]} prometheusStatus={null} />,
    );
    // The firing stateBadge span should have animate-pulse
    const spans = Array.from(container.querySelectorAll('span'));
    const firingDot = spans.find((s) =>
      s.className.includes('bg-status-down') && s.className.includes('animate-pulse'),
    );
    expect(firingDot).not.toBeUndefined();
  });

  it('firing status dot has a glow shadow', () => {
    const alerts = [mockAlert({ state: 'firing' })];
    const { container } = render(
      <AlertsPanel alerts={alerts} devices={[mockDevice()]} prometheusStatus={null} />,
    );
    const html = container.innerHTML;
    // Should contain shadow with red glow rgba value
    expect(html).toContain('rgba(255,23,68');
  });

  it('alert cards do not have border border-outline class', () => {
    const alerts = [mockAlert({ state: 'firing' })];
    const { container } = render(
      <AlertsPanel alerts={alerts} devices={[mockDevice()]} prometheusStatus={null} />,
    );
    const alertCard = container.querySelector('.bg-elevated');
    expect(alertCard).not.toBeNull();
    expect(alertCard?.className).not.toContain('border border-outline');
  });

  it('has motion-reduce:animate-none on pulse elements', () => {
    const alerts = [mockAlert({ state: 'firing' })];
    const { container } = render(
      <AlertsPanel alerts={alerts} devices={[mockDevice()]} prometheusStatus={null} />,
    );
    const html = container.innerHTML;
    expect(html).toContain('motion-reduce:animate-none');
  });
});
