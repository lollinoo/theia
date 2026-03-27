import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MaterialIcon } from './MaterialIcon';

describe('MaterialIcon', () => {
  it('renders a span with the icon name as text content', () => {
    render(<MaterialIcon name="settings" />);
    const icon = screen.getByText('settings');
    expect(icon).toBeDefined();
    expect(icon.tagName).toBe('SPAN');
  });

  it('applies the material-symbols-rounded CSS class', () => {
    render(<MaterialIcon name="search" />);
    const icon = screen.getByText('search');
    expect(icon.classList.contains('material-symbols-rounded')).toBe(true);
  });

  it('applies aria-hidden="true" attribute', () => {
    render(<MaterialIcon name="close" />);
    const icon = screen.getByText('close');
    expect(icon.getAttribute('aria-hidden')).toBe('true');
  });

  it('applies additional className when provided', () => {
    render(<MaterialIcon name="edit" className="text-primary" />);
    const icon = screen.getByText('edit');
    expect(icon.classList.contains('material-symbols-rounded')).toBe(true);
    expect(icon.classList.contains('text-primary')).toBe(true);
  });

  it('applies custom fontSize via inline style when size is not 18', () => {
    render(<MaterialIcon name="add" size={24} />);
    const icon = screen.getByText('add');
    expect(icon.style.fontSize).toBe('24px');
  });

  it('does NOT apply inline style when size is 18 (default)', () => {
    render(<MaterialIcon name="delete" />);
    const icon = screen.getByText('delete');
    expect(icon.style.fontSize).toBe('');
  });
});
