/**
 * Exercises side panel component behavior so refactors preserve the documented contract.
 */
import { fireEvent, render, screen } from '@testing-library/react';
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

  it('renders visible and handles Escape keydown when open', () => {
    const onClose = vi.fn();
    render(
      <SidePanel open={true} onClose={onClose} title="Open Panel" testId="side-panel">
        Panel content
      </SidePanel>,
    );

    const panel = screen.getByTestId('side-panel');
    expect(panel.getAttribute('aria-hidden')).toBe('false');
    expect(panel.hasAttribute('inert')).toBe(false);
    expect(panel.className).toContain('translate-x-0');
    expect(panel.className).not.toContain('invisible');
    expect(panel.className).not.toContain('pointer-events-none');

    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('hides from accessibility and tab order without attaching keydown listener when closed', () => {
    const addEventListenerSpy = vi.spyOn(window, 'addEventListener');
    const onClose = vi.fn();
    render(
      <SidePanel open={false} onClose={onClose} title="Closed Panel" testId="side-panel">
        Panel content
      </SidePanel>,
    );

    const panel = screen.getByTestId('side-panel');
    expect(panel.getAttribute('aria-hidden')).toBe('true');
    expect(panel.hasAttribute('inert')).toBe(true);
    expect(panel.className).toContain('translate-x-full');
    expect(panel.className).toContain('invisible');
    expect(panel.className).toContain('pointer-events-none');

    expect(addEventListenerSpy).not.toHaveBeenCalledWith('keydown', expect.any(Function));

    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).not.toHaveBeenCalled();

    addEventListenerSpy.mockRestore();
  });
});
