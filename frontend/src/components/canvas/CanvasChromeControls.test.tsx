/**
 * Exercises canvas chrome controls positioning so mobile navigation does not hide them.
 */
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { CanvasChromeControls } from './CanvasChromeControls';

describe('CanvasChromeControls', () => {
  it('keeps the visible-chrome fullscreen toggle beside the tool menu', () => {
    render(
      <CanvasChromeControls
        chromeHidden={false}
        onToggleChrome={vi.fn()}
        onSearch={vi.fn()}
        onFitView={vi.fn()}
      />,
    );

    const controls = screen.getByTestId('canvas-chrome-controls');

    expect(controls.className).toContain('right-20');
    expect(controls.className).toContain('top-32');
    expect(controls.className).toContain('sm:top-20');
    expect(controls.className).toContain('xl:top-4');
    expect(controls.className).not.toContain('lg:top-4');
    expect(controls.className).not.toContain('right-4');
    expect(screen.queryByRole('button', { name: 'Search devices' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Fit view' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Hide canvas controls' })).toBeInTheDocument();
  });

  it('keeps hidden-chrome action buttons aligned to the same top-right offset on all viewports', () => {
    render(
      <CanvasChromeControls
        chromeHidden
        onToggleChrome={vi.fn()}
        onSearch={vi.fn()}
        onFitView={vi.fn()}
      />,
    );

    const controls = screen.getByTestId('canvas-chrome-controls');

    expect(controls.className).toContain('right-4');
    expect(controls.className).toContain('top-4');
    expect(controls.className).not.toContain('left-4');
    expect(controls.className).not.toContain('top-20');
    expect(controls.className).not.toContain('top-32');
    expect(controls.className).not.toContain('sm:top-6');
    expect(controls.className).not.toContain('lg:top-4');
    expect(screen.getByRole('button', { name: 'Search devices' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Fit view' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Show canvas controls' })).toBeInTheDocument();
  });

  it('delegates visible actions to the supplied handlers', () => {
    const onSearch = vi.fn();
    const onFitView = vi.fn();
    const onToggleChrome = vi.fn();

    render(
      <CanvasChromeControls
        chromeHidden
        onToggleChrome={onToggleChrome}
        onSearch={onSearch}
        onFitView={onFitView}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Search devices' }));
    fireEvent.click(screen.getByRole('button', { name: 'Fit view' }));
    fireEvent.click(screen.getByRole('button', { name: 'Show canvas controls' }));

    expect(onSearch).toHaveBeenCalledTimes(1);
    expect(onFitView).toHaveBeenCalledTimes(1);
    expect(onToggleChrome).toHaveBeenCalledTimes(1);
  });
});
