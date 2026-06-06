/**
 * Defines settings panel styles behavior for settings screens.
 * Keeps validation, saved-state display, and defaults close to the controls that use them.
 */
export const fieldLabelClass = 'text-sm font-medium text-on-bg-secondary';
const inputClass =
  'theia-input focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none';
const compactInputClass =
  'w-full rounded-lg border border-outline-subtle bg-surface-container-high px-3 py-2 text-sm text-on-bg outline-none transition-colors focus:border-primary focus:ring-1 focus:ring-primary/30';
const invalidInputClass = 'border-status-down focus:border-status-down focus:ring-status-down/25';

/** Control class for the settings workflow. */
export function controlClass(hasError?: boolean, extra = ''): string {
  return `${inputClass}${hasError ? ` ${invalidInputClass}` : ''}${extra ? ` ${extra}` : ''}`;
}

/** Compacts control class for the settings workflow. */
export function compactControlClass(hasError?: boolean, extra = ''): string {
  return `${compactInputClass}${hasError ? ` ${invalidInputClass}` : ''}${extra ? ` ${extra}` : ''}`;
}
