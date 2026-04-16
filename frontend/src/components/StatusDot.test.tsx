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
    expect(dot?.getAttribute('style')).toContain('var(--nt-glow-status-down)');
  });

  it('status critical renders with a dedicated static token', () => {
    const { container } = render(<StatusDot status="critical" />);
    const dot = container.querySelector('span');
    expect(dot?.className).toContain('bg-status-critical');
    expect(dot?.className).not.toContain('animate-pulse');
    expect(dot?.getAttribute('style')).toContain('var(--nt-node-critical-badge-border)');
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

  it('status unmonitored renders with a neutral outlined token', () => {
    const { container } = render(<StatusDot status="unmonitored" />);
    const dot = container.querySelector('span');
    expect(dot?.className).toContain('border-outline-strong');
    expect(dot?.className).toContain('bg-surface-container-high');
  });
});
