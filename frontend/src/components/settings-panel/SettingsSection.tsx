/**
 * Defines settings section behavior for settings screens.
 * Keeps validation, saved-state display, and defaults close to the controls that use them.
 */
import type { ReactNode } from 'react';

import { MaterialIcon } from '../MaterialIcon';

interface SettingsSectionProps {
  id: string;
  title: string;
  description: string;
  icon: string;
  accent?: 'primary' | 'secondary' | 'warning' | 'status-up';
  aside?: ReactNode;
  className?: string;
  children: ReactNode;
}

/** Renders the SettingsSection component within the settings workflow. */
export function SettingsSection({
  id,
  title,
  description,
  icon,
  accent = 'primary',
  aside,
  className = '',
  children,
}: SettingsSectionProps) {
  const accentClass = {
    primary: 'bg-primary/15 text-primary',
    secondary: 'bg-area-secondary/15 text-area-secondary',
    warning: 'bg-warning/15 text-warning',
    'status-up': 'bg-status-up/15 text-status-up',
  }[accent];

  return (
    <section
      aria-labelledby={id}
      className={`flex h-[22rem] min-w-0 self-start flex-col rounded-lg border border-outline-subtle bg-surface-container/80 p-5 shadow-panel ${className}`}
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <span
            aria-hidden="true"
            className={`flex h-10 w-10 flex-none items-center justify-center rounded-lg ${accentClass}`}
          >
            <MaterialIcon name={icon} className="text-[20px]" />
          </span>
          <div className="min-w-0">
            <h2 id={id} className="text-lg font-semibold text-on-bg">
              {title}
            </h2>
            <p className="text-sm text-on-bg-secondary">{description}</p>
          </div>
        </div>
        {aside}
      </div>
      <div
        data-testid="settings-section-body"
        className="mt-5 min-h-0 min-w-0 flex-1 overflow-y-auto pr-1"
      >
        {children}
      </div>
    </section>
  );
}
