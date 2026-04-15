import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/react';
import { StatusDot } from './StatusDot';

describe('StatusDot (COMP-11)', () => {
  it('renders a span with rounded-full class', () => {
    const { container } = render(<StatusDot status="up" />);
    const dot = container.querySelector('span');
    expect(dot).not.toBeNull();
    expect(dot?.className).toContain('rounded-full');
  });

  it('status down has animate-pulse class', () => {
    const { container } = render(<StatusDot status="down" />);
    const dot = container.querySelector('span');
    expect(dot?.className).toContain('animate-pulse');
  });

  it('status critical renders with the dedicated critical token', () => {
    const { container } = render(<StatusDot status="critical" />);
    const dot = container.querySelector('span');
    expect(dot?.className).toContain('bg-status-critical');
  });

  it('status up does NOT have animate-pulse class', () => {
    const { container } = render(<StatusDot status="up" />);
    const dot = container.querySelector('span');
    expect(dot?.className).not.toContain('animate-pulse');
  });

  it('has motion-reduce:animate-none class for accessibility', () => {
    const { container } = render(<StatusDot status="down" />);
    const dot = container.querySelector('span');
    expect(dot?.className).toContain('motion-reduce:animate-none');
  });

  it('has transition-[box-shadow] class for smooth theme switching', () => {
    const { container } = render(<StatusDot status="up" />);
    const dot = container.querySelector('span');
    expect(dot?.className).toContain('transition-[box-shadow]');
  });
});
