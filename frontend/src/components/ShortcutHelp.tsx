import { useEffect, useState } from 'react';
import { MaterialIcon } from './MaterialIcon';

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
                className="w-full max-w-md rounded-xl bg-surface p-6 shadow-panel transition-colors duration-200"
                onClick={(e) => e.stopPropagation()}
            >
                <div className="flex items-center justify-between mb-6">
                    <h2 className="text-xl font-bold text-on-bg">Keyboard Shortcuts</h2>
                    <button
                        onClick={onClose}
                        className="p-1 text-on-bg-secondary hover:text-on-bg hover:bg-elevated rounded-md transition-colors"
                    >
                        <MaterialIcon name="close" size={20} />
                    </button>
                </div>

                <div className="grid grid-cols-1 gap-1">
                    {shortcuts.map((s, i) => (
                        <div key={i} className="flex justify-between items-center py-2 hover:bg-elevated/50 px-2 rounded-lg transition-colors">
                            <span className="text-on-bg-secondary">{s.action}</span>
                            <kbd className="px-2 py-1 bg-surface-high rounded text-sm font-mono text-on-bg tracking-wider">
                                {s.key}
                            </kbd>
                        </div>
                    ))}
                </div>
            </div>
        </div>
    );
}
