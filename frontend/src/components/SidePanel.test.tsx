import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { SidePanel } from './SidePanel';

vi.mock('./MaterialIcon', () => ({
  MaterialIcon: ({ name }: { name: string }) => (
    <span data-testid={`material-icon-${name}`}>{name}</span>
  ),
}));

describe('SidePanel', () => {
  it('portals above view-layer stacking contexts', () => {
    const { container } = render(
      <div data-testid="view-layer" className="absolute inset-0 z-10">
        <SidePanel open={true} onClose={vi.fn()} title="Device Details" testId="side-panel">
          Panel content
        </SidePanel>
      </div>,
    );

    const viewLayer = screen.getByTestId('view-layer');
    const panel = screen.getByTestId('side-panel');

    expect(viewLayer).not.toContainElement(panel);
    expect(document.body).toContainElement(panel);
    expect(panel.className).toContain('z-40');
    expect(container.firstElementChild).toBe(viewLayer);
  });
});
