import { useEffect } from 'react';

export interface ShortcutHandler {
    key: string;
    ctrl?: boolean;
    handler: () => void;
    description: string;
}

export function useKeyboardShortcuts(shortcuts: Record<string, ShortcutHandler>) {
    useEffect(() => {
        const handleKeyDown = (e: KeyboardEvent) => {
            // Ignore when focused in input, textarea, or select
            const target = e.target as HTMLElement;
            if (
                target.tagName === 'INPUT' ||
                target.tagName === 'TEXTAREA' ||
                target.tagName === 'SELECT' ||
                target.isContentEditable
            ) {
                return;
            }

            const isMac = navigator.platform.toUpperCase().indexOf('MAC') >= 0;
            const ctrlKey = isMac ? e.metaKey : e.ctrlKey;
            const key = e.key.toLowerCase();

            // For +/- we also want to catch NumpadAdd/NumpadSubtract if they correspond to + or -
            // But e.key generally normalizes those.
            // Also + usually requires Shift on some layouts (shift+= is +).
            // We look at e.key, which handles shift properly.

            for (const def of Object.values(shortcuts)) {
                if (def.key.toLowerCase() === key && !!def.ctrl === ctrlKey) {
                    e.preventDefault();
                    def.handler();
                    return;
                }
            }
        };

        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, [shortcuts]);
}
