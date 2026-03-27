/** Props for the MaterialIcon component. */
interface MaterialIconProps {
  /** Material Symbols ligature name (e.g., "settings", "search"). */
  name: string;
  /** Additional CSS classes to apply alongside the base icon class. */
  className?: string;
  /** Icon size in pixels. Defaults to 18 (matches CSS base class). */
  size?: number;
}

/** Renders a Material Symbols Rounded icon glyph via the self-hosted subset font. */
export function MaterialIcon({ name, className = '', size = 18 }: MaterialIconProps) {
  return (
    <span
      className={`material-symbols-rounded ${className}`}
      aria-hidden="true"
      style={size !== 18 ? { fontSize: size } : undefined}
    >
      {name}
    </span>
  );
}
