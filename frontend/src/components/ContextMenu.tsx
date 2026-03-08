import { useEffect, useRef, useState } from 'react';

export interface ContextMenuItem {
    label: string;
    onClick: () => void;
    variant?: 'danger' | 'default';
    disabled?: boolean;
}

interface ContextMenuProps {
    items: ContextMenuItem[];
    position: { x: number; y: number };
    onClose: () => void;
}

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
            className={`fixed z-30 rounded-xl border border-border-subtle bg-bg-surface/95 p-1 shadow-lg backdrop-blur-xl ${isMeasuring ? 'opacity-0' : 'opacity-100'
                }`}
            style={{
                left: renderPos.x,
                top: renderPos.y,
                minWidth: 160,
            }}
        >
            {items.map((item, index) => (
                <button
                    key={index}
                    disabled={item.disabled}
                    className={`w-full text-left px-3 py-2 text-sm rounded-lg transition-colors ${item.disabled ? 'cursor-not-allowed opacity-40' : 'hover:bg-bg-elevated'} ${item.variant === 'danger' ? 'text-status-down' : 'text-text-primary'
                        }`}
                    onClick={(e) => {
                        e.stopPropagation();
                        if (!item.disabled) {
                            item.onClick();
                            onClose();
                        }
                    }}
                >
                    {item.label}
                </button>
            ))}
        </div>
    );
}
