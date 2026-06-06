/**
 * Renders context menu UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
import { useEffect, useRef, useState } from 'react';
import { MaterialIcon } from './MaterialIcon';

/** Describes the context menu item contract used by the UI component boundary. */
export interface ContextMenuItem {
  label: string;
  onClick: () => void;
  variant?: 'danger' | 'default';
  disabled?: boolean;
  icon?: string;
  separator?: boolean;
  title?: string;
}

interface ContextMenuProps {
  items: ContextMenuItem[];
  position: { x: number; y: number };
  onClose: () => void;
}

/** Renders the ContextMenu component within the UI component boundary. */
export function ContextMenu({ items, position, onClose }: ContextMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null);
  const [adjustedPosition, setAdjustedPosition] = useState<{ x: number; y: number } | null>(null);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    const handleClickOutside = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        onClose();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    window.addEventListener('mousedown', handleClickOutside);

    return () => {
      window.removeEventListener('keydown', handleKeyDown);
      window.removeEventListener('mousedown', handleClickOutside);
    };
  }, [onClose]);

  useEffect(() => {
    if (menuRef.current) {
      const rect = menuRef.current.getBoundingClientRect();
      let x = position.x;
      let y = position.y;

      const padding = 8;

      if (x + rect.width > window.innerWidth) {
        x = window.innerWidth - rect.width - padding;
      }
      if (y + rect.height > window.innerHeight) {
        y = window.innerHeight - rect.height - padding;
      }

      setAdjustedPosition({ x: Math.max(padding, x), y: Math.max(padding, y) });
    }
  }, [position]);

  const isMeasuring = adjustedPosition === null;
  const renderPos = adjustedPosition || position;

  return (
    <div
      ref={menuRef}
      className={`fixed z-[80] dark:rounded-[6px] rounded-[10px] border border-glass-border bg-glass-bg py-2 shadow-pill dark:backdrop-blur-[16px] transition-colors duration-200 ${
        isMeasuring ? 'opacity-0' : 'opacity-100'
      }`}
      style={{
        left: renderPos.x,
        top: renderPos.y,
        minWidth: 200,
      }}
    >
      {items.map((item) => (
        <div
          key={`${item.label}-${item.title ?? ''}-${item.icon ?? ''}-${item.separator ? 'sep' : 'item'}`}
        >
          {item.separator && <div className="mx-2 my-1 h-[1px] bg-outline" />}
          <button
            type="button"
            disabled={item.disabled}
            title={item.title}
            className={`group flex w-full items-center gap-3 px-3 text-left text-sm transition-colors dark:h-[36px] h-[40px] ${
              item.disabled
                ? 'cursor-not-allowed opacity-40'
                : item.variant === 'danger'
                  ? 'hover:bg-[rgba(255,23,68,0.08)]'
                  : 'hover:bg-outline-subtle'
            }`}
            onClick={(e) => {
              e.stopPropagation();
              if (!item.disabled) {
                item.onClick();
                onClose();
              }
            }}
          >
            {item.icon && (
              <MaterialIcon
                name={item.icon}
                className={
                  item.variant === 'danger'
                    ? 'text-critical'
                    : 'text-on-bg-muted group-hover:text-on-bg transition-colors'
                }
              />
            )}
            <span
              className={item.variant === 'danger' ? 'text-critical font-semibold' : 'text-on-bg'}
            >
              {item.label}
            </span>
          </button>
        </div>
      ))}
    </div>
  );
}
