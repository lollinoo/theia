import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import ZoomControls from './ZoomControls';

describe('ZoomControls', () => {
  it('anchors inside the canvas viewport instead of the browser window', () => {
    render(<ZoomControls onZoomIn={vi.fn()} onZoomOut={vi.fn()} onFitView={vi.fn()} />);

    const zoomInButton = screen.getByRole('button', { name: /zoom in/i });
    const wrapper = zoomInButton.parentElement?.parentElement as HTMLElement;

    expect(wrapper.className).toContain('absolute');
    expect(wrapper.className).toContain('bottom-[calc(6rem+env(safe-area-inset-bottom))]');
    expect(wrapper.className).toContain('sm:bottom-4');
    expect(wrapper.className).toContain('left-4');
    expect(wrapper.className).not.toContain('fixed');
  });
});
