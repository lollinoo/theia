import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Watermark } from './Watermark';
import type { Area } from '../types/api';

function mockArea(overrides: Partial<Area> = {}): Area {
  return {
    id: 'area-1',
    name: 'Backbone',
    description: 'Core backbone area',
    color: '#00E676',
    device_count: 10,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

describe('Watermark', () => {
  it('renders GLOBAL TOPOLOGY when selectedAreaId is null on canvas view', () => {
    render(<Watermark activeView="canvas" selectedAreaId={null} areas={[mockArea()]} />);
    expect(screen.getByText('GLOBAL TOPOLOGY')).toBeDefined();
  });

  it('renders area name uppercase when selectedAreaId matches an area', () => {
    const areas = [mockArea(), mockArea({ id: 'area-2', name: 'Distribution' })];
    render(<Watermark activeView="canvas" selectedAreaId="area-2" areas={areas} />);
    expect(screen.getByText('DISTRIBUTION')).toBeDefined();
  });

  it('renders nothing on hub view', () => {
    const { container } = render(<Watermark activeView="hub" selectedAreaId={null} areas={[mockArea()]} />);
    expect(container.firstChild).toBeNull();
  });

  it('renders nothing on dashboard view', () => {
    const { container } = render(<Watermark activeView="dashboard" selectedAreaId={null} areas={[mockArea()]} />);
    expect(container.firstChild).toBeNull();
  });

  it('has pointer-events-none and aria-hidden attributes', () => {
    const { container } = render(<Watermark activeView="canvas" selectedAreaId={null} areas={[]} />);
    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper.className).toContain('pointer-events-none');
    expect(wrapper.getAttribute('aria-hidden')).toBe('true');
  });

  it('has fixed positioning classes near minimap', () => {
    const { container } = render(<Watermark activeView="canvas" selectedAreaId={null} areas={[]} />);
    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper.className).toContain('fixed');
    expect(wrapper.className).toContain('bottom-[170px]');
    expect(wrapper.className).toContain('right-3');
  });
});
