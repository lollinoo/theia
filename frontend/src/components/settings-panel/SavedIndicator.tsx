interface SavedIndicatorProps {
  visible: boolean;
}

export function SavedIndicator({ visible }: SavedIndicatorProps) {
  return (
    <span
      className={`text-xs text-status-up transition-opacity duration-500 ${visible ? 'opacity-100' : 'opacity-0'}`}
    >
      Saved
    </span>
  );
}
