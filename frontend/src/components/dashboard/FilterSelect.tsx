import { useState, useRef, useEffect } from 'react';
import { MaterialIcon } from '../MaterialIcon';

export interface FilterOption {
  value: string;
  label: string;
  color?: string; // Optional accent color dot (for area options)
}

interface FilterSelectProps {
  value: string;
  onChange: (value: string) => void;
  options: FilterOption[];
  label: string; // e.g., "Status", "Type", "Area"
  defaultValue?: string; // value considered "not filtering" (default: first option's value)
}

/** Reusable custom select dropdown component matching Neon Topography design tokens. */
export function FilterSelect({ value, onChange, options, label, defaultValue }: FilterSelectProps) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const resolvedDefault = defaultValue ?? options[0]?.value;
  const isActive = value !== resolvedDefault;
  const selectedLabel = options.find(o => o.value === value)?.label ?? value;

  // Close on outside click
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [open]);

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className={`flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs transition-colors
          ${isActive
            ? 'bg-primary/15 text-primary'
            : 'bg-surface-high text-on-bg-secondary hover:text-on-bg hover:bg-elevated'
          }`}
      >
        <span className="font-medium">{label}:</span>
        <span>{selectedLabel}</span>
        <MaterialIcon name="expand_more" size={16} className={`transition-transform ${open ? 'rotate-180' : ''}`} />
      </button>
      {open && (
        <div className="absolute top-full left-0 mt-1 bg-elevated rounded-lg shadow-panel z-20 min-w-[160px] py-1">
          {options.map(opt => (
            <button
              key={opt.value}
              type="button"
              onClick={() => { onChange(opt.value); setOpen(false); }}
              className={`w-full text-left px-3 py-2 text-xs flex items-center gap-2 transition-colors
                ${opt.value === value
                  ? 'text-primary bg-primary/10'
                  : 'text-on-bg-secondary hover:text-on-bg hover:bg-surface-high'
                }`}
            >
              {opt.color && (
                <span
                  className="w-2 h-2 rounded-full flex-shrink-0"
                  style={{ backgroundColor: opt.color }}
                />
              )}
              {opt.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
