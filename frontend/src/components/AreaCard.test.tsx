import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import AreaCard from './AreaCard';
import type { Area } from '../types/api';

vi.mock('../contexts/ThemeContext', () => ({
  useTheme: () => ({ theme: 'dark' as const, resolvedTheme: 'dark' as const, setTheme: vi.fn() }),
  adaptAreaColor: (hex: string) => hex,
}));

function mockArea(overrides: Partial<Area> = {}): Area {
  return {
    id: 'area-1',
    name: 'Backbone',
    description: 'Core backbone area',
    color: '#00E676',
    device_count: 12,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

describe('AreaCard', () => {
  it('renders area name and description', () => {
    render(
      <AreaCard
        area={mockArea()}
        healthPercentage={100}
        healthLabel="Optimal"
        healthColor="text-status-up"
        deviceCount={12}
        activeLinkCount={42}
        onClick={() => {}}
      />,
    );

    expect(screen.getByText('Backbone')).toBeInTheDocument();
    expect(screen.getByText('Core backbone area')).toBeInTheDocument();
  });

  it('renders health status with correct label', () => {
    const { rerender } = render(
      <AreaCard
        area={mockArea()}
        healthPercentage={100}
        healthLabel="Optimal"
        healthColor="text-status-up"
        deviceCount={10}
        activeLinkCount={5}
        onClick={() => {}}
      />,
    );

    expect(screen.getByText('Optimal')).toBeInTheDocument();

    rerender(
      <AreaCard
        area={mockArea()}
        healthPercentage={85}
        healthLabel="Degraded"
        healthColor="text-warning"
        deviceCount={10}
        activeLinkCount={5}
        onClick={() => {}}
      />,
    );

    expect(screen.getByText('Degraded')).toBeInTheDocument();

    rerender(
      <AreaCard
        area={mockArea()}
        healthPercentage={70}
        healthLabel="Critical"
        healthColor="text-status-down"
        deviceCount={10}
        activeLinkCount={5}
        onClick={() => {}}
      />,
    );

    expect(screen.getByText('Critical')).toBeInTheDocument();
  });

  it('renders device count and active link count', () => {
    render(
      <AreaCard
        area={mockArea()}
        healthPercentage={100}
        healthLabel="Optimal"
        healthColor="text-status-up"
        deviceCount={12}
        activeLinkCount={42}
        onClick={() => {}}
      />,
    );

    expect(screen.getByText('12')).toBeInTheDocument();
    expect(screen.getByText('42')).toBeInTheDocument();
    expect(screen.getByText('Devices:')).toBeInTheDocument();
    expect(screen.getByText('Active Links:')).toBeInTheDocument();
  });

  it('renders glow dot with area accent color', () => {
    const { container } = render(
      <AreaCard
        area={mockArea({ color: '#2979FF' })}
        healthPercentage={100}
        healthLabel="Optimal"
        healthColor="text-status-up"
        deviceCount={5}
        activeLinkCount={3}
        onClick={() => {}}
      />,
    );

    // The glow dot should have the area's accent color as backgroundColor
    const dot = container.querySelector('[data-testid="area-glow-dot"]');
    expect(dot).toBeInTheDocument();
    expect(dot).toHaveStyle({ backgroundColor: '#2979FF' });
  });

  it('calls onClick when clicked', () => {
    const handleClick = vi.fn();
    render(
      <AreaCard
        area={mockArea()}
        healthPercentage={100}
        healthLabel="Optimal"
        healthColor="text-status-up"
        deviceCount={5}
        activeLinkCount={3}
        onClick={handleClick}
      />,
    );

    const card = screen.getByRole('button');
    fireEvent.click(card);
    expect(handleClick).toHaveBeenCalledTimes(1);
  });
});
