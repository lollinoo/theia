/**
 * Exercises theme05 smoke component behavior so refactors preserve the documented contract.
 */
import { render } from '@testing-library/react';
/**
 * THEME-05 Smoke Tests
 * Key components render without errors in both dark and light theme contexts.
 * Uses token classes (no hardcoded hex) so they work in either theme.
 */
import { describe, expect, it, vi } from 'vitest';
import { AlertsPanel } from '../AlertsPanel';
import type { AlertsPanelModel } from '../panelModels';
import { StatusDot } from '../StatusDot';
import { Toolbar } from '../Toolbar';

// Mock MaterialIcon for Toolbar and AlertsPanel
vi.mock('../MaterialIcon', () => ({
  MaterialIcon: ({ name }: { name: string }) => (
    <span className="material-symbols-rounded">{name}</span>
  ),
}));

describe('THEME-05 Component smoke tests', () => {
  function alertsModel(overrides: Partial<AlertsPanelModel> = {}): AlertsPanelModel {
    return {
      activeAlertCount: 0,
      firingAlerts: [],
      resolvedAlerts: [],
      prometheusDiagnostics: null,
      ...overrides,
    };
  }

  describe('StatusDot', () => {
    it('renders without error for all statuses', () => {
      const statuses = [
        'up',
        'down',
        'critical',
        'probing',
        'unknown',
        'degraded',
        'unmonitored',
      ] as const;
      for (const status of statuses) {
        const { container } = render(<StatusDot status={status} />);
        expect(container.firstChild).toBeTruthy();
      }
    });
  });

  describe('AlertsPanel', () => {
    it('renders without error with no alerts', () => {
      const { container } = render(<AlertsPanel model={alertsModel()} />);
      expect(container.firstChild).toBeTruthy();
    });

    it('renders without error with firing alerts', () => {
      const { container } = render(
        <AlertsPanel
          model={alertsModel({
            firingAlerts: [
              {
                deviceId: 'dev-1',
                deviceLabel: 'router-01',
                alertName: 'HighCPU',
                severity: 'critical',
                state: 'firing',
                summary: 'CPU high',
              },
            ],
          })}
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
          onToggleEditMode={vi.fn()}
          editMode={false}
          alertCount={0}
        />,
      );
      expect(container.firstChild).toBeTruthy();
    });
  });
});
