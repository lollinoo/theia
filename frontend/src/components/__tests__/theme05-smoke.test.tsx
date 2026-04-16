/**
 * THEME-05 Smoke Tests
 * Key components render without errors in both dark and light theme contexts.
 * Uses token classes (no hardcoded hex) so they work in either theme.
 */
import { describe, it, expect, vi } from 'vitest';
import { render } from '@testing-library/react';
import { AlertsPanel } from '../AlertsPanel';
import { StatusDot } from '../StatusDot';
import { Toolbar } from '../Toolbar';

// Mock MaterialIcon for Toolbar and AlertsPanel
vi.mock('../MaterialIcon', () => ({
  MaterialIcon: ({ name }: { name: string }) => (
    <span className="material-symbols-rounded">{name}</span>
  ),
}));

describe('THEME-05 Component smoke tests', () => {
  describe('StatusDot', () => {
    it('renders without error for all statuses', () => {
      const statuses = ['up', 'down', 'critical', 'probing', 'unknown', 'degraded', 'unmonitored'] as const;
      for (const status of statuses) {
        const { container } = render(<StatusDot status={status} />);
        expect(container.firstChild).toBeTruthy();
      }
    });
  });

  describe('AlertsPanel', () => {
    it('renders without error with no alerts', () => {
      const { container } = render(
        <AlertsPanel alerts={[]} devices={[]} prometheusStatus={null} />,
      );
      expect(container.firstChild).toBeTruthy();
    });

    it('renders without error with firing alerts', () => {
      const { container } = render(
        <AlertsPanel
          alerts={[
            {
              device_id: 'dev-1',
              alert_name: 'HighCPU',
              severity: 'critical',
              state: 'firing',
              summary: 'CPU high',
            },
          ]}
          devices={[]}
          prometheusStatus={null}
        />,
      );
      expect(container.firstChild).toBeTruthy();
      // Should have transition-colors for theme switching
      expect(container.innerHTML).toContain('transition-colors');
    });
  });

  describe('Toolbar', () => {
    it('renders without error', () => {
      const { container } = render(
        <Toolbar
          onSearch={vi.fn()}
          onAddDevice={vi.fn()}
          onCreateLink={vi.fn()}
          onAlerts={vi.fn()}
          onSettings={vi.fn()}
          onToggleEditMode={vi.fn()}
          editMode={false}
          alertCount={0}
        />,
      );
      expect(container.firstChild).toBeTruthy();
    });
  });
});
