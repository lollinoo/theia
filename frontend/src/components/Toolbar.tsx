import { useEffect, useState } from 'react';

interface ToolbarProps {
    onSearch: () => void;
    onAddDevice: () => void;
    onCreateLink: () => void;
    onSettings: () => void;
}

export function Toolbar({ onSearch, onAddDevice, onCreateLink, onSettings }: ToolbarProps) {
    const [isMac, setIsMac] = useState(false);

    useEffect(() => {
        setIsMac(navigator.platform.toUpperCase().indexOf('MAC') >= 0);
    }, []);

    const modifier = isMac ? '⌘' : 'Ctrl';

    const buttons = [
        {
            label: `Search (${modifier}+K)`,
            onClick: onSearch,
            icon: (
                <svg fill="none" viewBox="0 0 24 24" stroke="currentColor" className="w-[18px] h-[18px]">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
                </svg>
            )
        },
        {
            label: 'Add Device (A)',
            onClick: onAddDevice,
            icon: (
                <svg fill="none" viewBox="0 0 24 24" stroke="currentColor" className="w-[18px] h-[18px]">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                </svg>
            )
        },
        {
            label: 'Create Link (L)',
            onClick: onCreateLink,
            icon: (
                <svg fill="none" viewBox="0 0 24 24" stroke="currentColor" className="w-[18px] h-[18px]">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
                </svg>
            )
        },
        {
            label: `Settings (${modifier}+,)`,
            onClick: onSettings,
            icon: (
                <svg fill="none" viewBox="0 0 24 24" stroke="currentColor" className="w-[18px] h-[18px]">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                </svg>
            )
        }
    ];

    return (
        <div className="absolute top-4 right-4 z-10 flex flex-col overflow-hidden rounded-xl border border-border-subtle bg-bg-surface/90 shadow-canvas backdrop-blur-xl">
            {buttons.map((btn, i) => (
                <button
                    key={i}
                    className={`flex h-[40px] w-[40px] items-center justify-center text-text-primary transition-colors hover:bg-bg-elevated ${i !== buttons.length - 1 ? 'border-b border-border-subtle' : ''
                        }`}
                    onClick={btn.onClick}
                    title={btn.label}
                >
                    {btn.icon}
                </button>
            ))}
        </div>
    );
}
