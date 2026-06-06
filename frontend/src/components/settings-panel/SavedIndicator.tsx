/**
 * Defines saved indicator behavior for settings screens.
 * Keeps validation, saved-state display, and defaults close to the controls that use them.
 */
interface SavedIndicatorProps {
  visible: boolean;
}

/** Renders the SavedIndicator component within the settings workflow. */
export function SavedIndicator({ visible }: SavedIndicatorProps) {
  return (
    <span
      className={`text-xs text-status-up transition-opacity duration-500 ${visible ? 'opacity-100' : 'opacity-0'}`}
    >
      Saved
    </span>
  );
}
