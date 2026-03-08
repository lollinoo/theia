import React, { useEffect } from 'react';

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
            className={`fixed top-0 right-0 h-full w-[320px] z-20 transform transition-transform duration-200 ease-in-out bg-bg-surface border-l border-border-subtle flex flex-col shadow-2xl ${open ? 'translate-x-0' : 'translate-x-full'
                }`}
        >
            <div className="flex items-center justify-between px-4 py-4 border-b border-border-subtle">
                <h2 className="text-lg font-semibold text-text-primary">{title}</h2>
                <button
                    onClick={onClose}
                    className="p-1 text-text-secondary hover:text-text-primary hover:bg-bg-elevated rounded-md transition-colors"
                    title="Close"
                >
                    <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                    </svg>
                </button>
            </div>
            <div className="flex-1 overflow-y-auto p-4">
                {children}
            </div>
        </div>
    );
}
