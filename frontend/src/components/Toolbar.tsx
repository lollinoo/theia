import { useEffect, useState } from 'react';
import { MaterialIcon } from './MaterialIcon';

interface ToolbarProps {
    onSearch: () => void;
    onAddDevice: () => void;
    onCreateLink: () => void;
    onAlerts: () => void;
    onSettings: () => void;
    onToggleEditMode: () => void;
    editMode: boolean;
    alertCount?: number;
}

export function Toolbar({ onSearch, onAddDevice, onCreateLink, onAlerts, onSettings, onToggleEditMode, editMode, alertCount = 0 }: ToolbarProps) {
    const [isMac, setIsMac] = useState(false);

    useEffect(() => {
        setIsMac(navigator.platform.toUpperCase().indexOf('MAC') >= 0);
    }, []);

    const modifier = isMac ? '⌘' : 'Ctrl';

    const buttons = [
        {
            label: 'Edit Mode (E)',
            onClick: onToggleEditMode,
            active: editMode,
            icon: <MaterialIcon name="edit" />
        },
        {
            label: `Search (${modifier}+K)`,
            onClick: onSearch,
            icon: <MaterialIcon name="search" />
        },
        {
            label: 'Add Device (A)',
            onClick: onAddDevice,
            icon: <MaterialIcon name="add" />
        },
        {
            label: 'Create Link (L)',
            onClick: onCreateLink,
            icon: <MaterialIcon name="link" />
        },
        {
            label: 'Alerts',
            onClick: onAlerts,
            icon: (
                <div className="relative">
                    <MaterialIcon name="notifications" />
                    {alertCount > 0 && (
                        <span className="absolute -top-1.5 -right-1.5 flex h-3.5 min-w-[14px] items-center justify-center rounded-full bg-status-down px-0.5 text-[9px] font-bold text-white">
                            {alertCount > 99 ? '99+' : alertCount}
                        </span>
                    )}
                </div>
            )
        },
        {
            label: `Settings (${modifier}+,)`,
            onClick: onSettings,
            icon: <MaterialIcon name="settings" />
        }
    ];

    return (
        <div className="absolute top-14 right-4 z-10 flex flex-col overflow-hidden rounded-xl bg-surface/90 shadow-canvas dark:backdrop-blur-xl transition-colors duration-200">
            {buttons.map((btn, i) => (
                <button
                    key={i}
                    className={`flex h-[40px] w-[40px] items-center justify-center transition-colors hover:bg-elevated ${'active' in btn && btn.active ? 'bg-primary/15 text-primary' : 'text-on-bg'}`}
                    onClick={btn.onClick}
                    title={btn.label}
                >
                    {btn.icon}
                </button>
            ))}
        </div>
    );
}
