import { useEffect, useState } from 'react';

interface ShortcutHelpProps {
    open: boolean;
    onClose: () => void;
}

export function ShortcutHelp({ open, onClose }: ShortcutHelpProps) {
    const [isMac, setIsMac] = useState(false);

    useEffect(() => {
        setIsMac(navigator.platform.toUpperCase().indexOf('MAC') >= 0);
    }, []);

    useEffect(() => {
        if (!open) return;

        const handleKeyDown = (e: KeyboardEvent) => {
            if (e.key === 'Escape') onClose();
        };
        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, [open, onClose]);

    if (!open) return null;

    const modifier = isMac ? '⌘' : 'Ctrl';

    const shortcuts = [
        { key: `${modifier}+K`, action: 'Search' },
        { key: 'A', action: 'Add device' },
        { key: '+ / - / 0', action: 'Zoom In/Out/Fit' },
        { key: `${modifier}+,`, action: 'Settings' },
        { key: '?', action: 'Shortcuts help' },
        { key: 'Esc', action: 'Close panel/menu' },
    ];

    return (
        <div
            className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
            onClick={onClose}
        >
            <div
                className="w-full max-w-md rounded-xl border border-border-subtle bg-bg-surface p-6 shadow-2xl"
                onClick={(e) => e.stopPropagation()}
            >
                <div className="flex items-center justify-between mb-6">
                    <h2 className="text-xl font-bold text-text-primary">Keyboard Shortcuts</h2>
                    <button
                        onClick={onClose}
                        className="p-1 text-text-secondary hover:text-text-primary hover:bg-bg-elevated rounded-md transition-colors"
                    >
                        <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                        </svg>
                    </button>
                </div>

                <div className="grid grid-cols-1 gap-3">
                    {shortcuts.map((s, i) => (
                        <div key={i} className="flex justify-between items-center py-2 border-b border-border-subtle last:border-0 hover:bg-bg-elevated/50 px-2 rounded-lg transition-colors">
                            <span className="text-text-secondary">{s.action}</span>
                            <kbd className="px-2 py-1 bg-bg-elevated border border-border-subtle rounded text-sm font-mono text-text-primary tracking-wider shadow-sm">
                                {s.key}
                            </kbd>
                        </div>
                    ))}
                </div>
            </div>
        </div>
    );
}
