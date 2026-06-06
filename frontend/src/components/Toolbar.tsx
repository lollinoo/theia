/**
 * Renders toolbar UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
import { useEffect, useState } from 'react';
import { MaterialIcon } from './MaterialIcon';

interface ToolbarProps {
  onSearch: () => void;
  onAddDevice: () => void;
  onCreateLink: () => void;
  onAlerts: () => void;
  onToggleEditMode: () => void;
  editMode: boolean;
  alertCount?: number;
}

/** Renders the Toolbar component within the UI component boundary. */
export function Toolbar({
  onSearch,
  onAddDevice,
  onCreateLink,
  onAlerts,
  onToggleEditMode,
  editMode,
  alertCount = 0,
}: ToolbarProps) {
  const [isMac, setIsMac] = useState(false);
  const [expanded, setExpanded] = useState(false);

  useEffect(() => {
    setIsMac(navigator.platform.toUpperCase().indexOf('MAC') >= 0);
  }, []);

  const modifier = isMac ? '⌘' : 'Ctrl';

  const buttons = [
    {
      label: 'Edit Mode (E)',
      onClick: onToggleEditMode,
      active: editMode,
      icon: <MaterialIcon name="edit" />,
    },
    {
      label: `Search (${modifier}+K)`,
      onClick: onSearch,
      icon: <MaterialIcon name="search" />,
    },
    {
      label: 'Add Device (A)',
      onClick: onAddDevice,
      icon: <MaterialIcon name="add" />,
    },
    {
      label: 'Create Link (L)',
      onClick: onCreateLink,
      icon: <MaterialIcon name="link" />,
    },
    {
      label: 'Alerts',
      onClick: onAlerts,
      icon: (
        <div className="relative">
          <MaterialIcon name="notifications" />
          {alertCount > 0 && (
            <span className="absolute -right-1.5 -top-1.5 flex h-3.5 min-w-[14px] items-center justify-center rounded-full bg-status-down px-0.5 text-[9px] font-bold text-surface-container-high">
              {alertCount > 99 ? '99+' : alertCount}
            </span>
          )}
        </div>
      ),
    },
  ];

  return (
    <div className="topology-glass topology-floating-shadow absolute right-4 top-32 z-10 flex flex-col gap-1 rounded-[16px] p-1.5 transition-colors duration-200 sm:top-20">
      <button
        type="button"
        aria-label={expanded ? 'Hide canvas tools' : 'Show canvas tools'}
        aria-expanded={expanded}
        onClick={() => setExpanded((current) => !current)}
        title={expanded ? 'Hide canvas tools' : 'Show canvas tools'}
        className="relative flex h-11 w-11 items-center justify-center rounded-xl border border-transparent text-on-bg-secondary transition-[background-color,color,border-color,transform] duration-150 hover:-translate-y-0.5 hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg sm:hidden"
      >
        <MaterialIcon name={expanded ? 'close' : 'build'} />
        {alertCount > 0 && !expanded && (
          <span className="absolute -right-1.5 -top-1.5 flex h-3.5 min-w-[14px] items-center justify-center rounded-full bg-status-down px-0.5 text-[9px] font-bold text-surface-container-high">
            {alertCount > 99 ? '99+' : alertCount}
          </span>
        )}
      </button>

      {buttons.map((btn) => (
        <button
          key={btn.label}
          type="button"
          className={`relative ${expanded ? 'flex' : 'hidden sm:flex'} h-11 w-11 items-center justify-center rounded-xl border border-transparent transition-[background-color,color,border-color,transform] duration-150 hover:-translate-y-0.5 hover:bg-surface-container focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg ${'active' in btn && btn.active ? 'border-primary/35 bg-primary/12 text-primary' : 'text-on-bg-secondary hover:text-on-bg'}`}
          onClick={btn.onClick}
          title={btn.label}
        >
          {btn.icon}
        </button>
      ))}
    </div>
  );
}
