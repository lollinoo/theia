/**
 * Exercises status dot component behavior so refactors preserve the documented contract.
 */
import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
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

  it('preserves the approved UP, Warning, Critical, and Down dot styles', () => {
    const up = render(<StatusDot status="up" />);
    expect(up.container.querySelector('span')?.className).toContain('bg-status-up');
    expect(up.container.querySelector('span')?.getAttribute('style')).toContain(
      'var(--nt-glow-status-ok)',
    );
    up.unmount();

    const warning = render(<StatusDot status="degraded" />);
    expect(warning.container.querySelector('span')?.className).toContain('bg-warning');
    expect(warning.container.querySelector('span')?.className).toContain('animate-pulse');
    expect(warning.container.querySelector('span')?.getAttribute('style')).toContain(
      'var(--nt-glow-status-warning)',
    );
    warning.unmount();

    const critical = render(<StatusDot status="critical" />);
    expect(critical.container.querySelector('span')?.className).toContain('bg-status-critical');
    expect(critical.container.querySelector('span')?.className).not.toContain('animate-pulse');
    expect(critical.container.querySelector('span')?.getAttribute('style')).toContain(
      'var(--nt-node-critical-badge-border)',
    );
    critical.unmount();

    const down = render(<StatusDot status="down" />);
    expect(down.container.querySelector('span')?.className).toContain('bg-status-down');
    expect(down.container.querySelector('span')?.className).toContain('animate-pulse');
    expect(down.container.querySelector('span')?.getAttribute('style')).toContain(
      'var(--nt-glow-status-down)',
    );
  });
});
