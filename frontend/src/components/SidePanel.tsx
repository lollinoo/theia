import React, { useEffect } from 'react';
import { MaterialIcon } from './MaterialIcon';

interface SidePanelProps {
    open: boolean;
    onClose: () => void;
    title: string;
    children: React.ReactNode;
}

export function SidePanel({ open, onClose, title, children }: SidePanelProps) {
    useEffect(() => {
        const handleKeyDown = (e: KeyboardEvent) => {
            if (open && e.key === 'Escape') {
                onClose();
            }
        };

        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, [open, onClose]);

    return (
        <div
            className={`fixed top-0 right-0 h-full w-[320px] z-40 transform transition-transform duration-200 ease-in-out bg-surface flex flex-col shadow-panel ${open ? 'translate-x-0' : 'translate-x-full'
                }`}
        >
            <div className="flex items-center justify-between px-4 py-3 bg-surface-high/80 transition-colors duration-200">
                <h2 className="text-sm font-semibold text-on-bg tracking-wide">{title}</h2>
                <button
                    onClick={onClose}
                    className="p-1 text-on-bg-secondary hover:text-on-bg hover:bg-elevated rounded-md transition-colors"
                    title="Close"
                >
                    <MaterialIcon name="close" size={18} />
                </button>
            </div>
            <div className="flex-1 overflow-y-auto p-4 transition-colors duration-200">
                {children}
            </div>
        </div>
    );
}
