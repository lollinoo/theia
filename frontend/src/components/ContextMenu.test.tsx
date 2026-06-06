/**
 * Exercises context menu component behavior so refactors preserve the documented contract.
 */
import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { ContextMenu } from './ContextMenu';
import type { ContextMenuItem } from './ContextMenu';

const defaultPosition = { x: 100, y: 100 };
const defaultOnClose = vi.fn();

describe('ContextMenu', () => {
  it('renders icon span with material-symbols-rounded class when item has icon prop', () => {
    const items: ContextMenuItem[] = [
      { label: 'Open Terminal', onClick: vi.fn(), icon: 'terminal' },
    ];
    const { container } = render(
      <ContextMenu items={items} position={defaultPosition} onClose={defaultOnClose} />,
    );
    const iconSpan = container.querySelector('.material-symbols-rounded');
    expect(iconSpan).not.toBeNull();
    expect(iconSpan?.textContent).toBe('terminal');
  });

  it('renders separator div with bg-outline class when item has separator: true', () => {
    const items: ContextMenuItem[] = [
      { label: 'Copy', onClick: vi.fn() },
      { label: 'Delete', onClick: vi.fn(), separator: true },
    ];
    const { container } = render(
      <ContextMenu items={items} position={defaultPosition} onClose={defaultOnClose} />,
    );
    const separators = container.querySelectorAll('.bg-outline');
    expect(separators.length).toBeGreaterThanOrEqual(1);
  });

  it('renders danger items with text-critical class on both icon and label', () => {
    const items: ContextMenuItem[] = [
      { label: 'Delete Device', onClick: vi.fn(), variant: 'danger', icon: 'delete' },
    ];
    const { container } = render(
      <ContextMenu items={items} position={defaultPosition} onClose={defaultOnClose} />,
    );
    const iconSpan = container.querySelector('.material-symbols-rounded');
    expect(iconSpan?.className).toContain('text-critical');
    const label = screen.getByText('Delete Device');
    expect(label.className).toContain('text-critical');
  });

  it('has bg-glass-bg and border-glass-border classes on the menu container', () => {
    const items: ContextMenuItem[] = [{ label: 'Edit', onClick: vi.fn() }];
    const { container } = render(
      <ContextMenu items={items} position={defaultPosition} onClose={defaultOnClose} />,
    );
    const menuDiv = container.firstChild as HTMLElement;
    expect(menuDiv.className).toContain('bg-glass-bg');
    expect(menuDiv.className).toContain('border-glass-border');
  });

  it('renders disabled items with cursor-not-allowed and opacity-40 classes', () => {
    const items: ContextMenuItem[] = [
      { label: 'Disabled Action', onClick: vi.fn(), disabled: true },
    ];
    const { container } = render(
      <ContextMenu items={items} position={defaultPosition} onClose={defaultOnClose} />,
    );
    const button = container.querySelector('button');
    expect(button?.className).toContain('cursor-not-allowed');
    expect(button?.className).toContain('opacity-40');
  });
});
