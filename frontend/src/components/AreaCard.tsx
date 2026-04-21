import { useMemo, useState } from 'react';
import { adaptAreaColor, useTheme } from '../contexts/ThemeContext';
import type { Area } from '../types/api';
import { MaterialIcon } from './MaterialIcon';

/** Props for the AreaCard component. */
interface AreaCardProps {
  area: Area;
  healthPercentage: number;
  healthLabel: string;
  healthColor: string;
  deviceCount: number;
  activeLinkCount: number;
  onClick: () => void;
}

/** Individual area card with bloom effect, accent dot, health/device/link stats. */
export default function AreaCard({
  area,
  healthLabel,
  healthColor,
  deviceCount,
  activeLinkCount,
  onClick,
}: AreaCardProps) {
  const [hovered, setHovered] = useState(false);
  const { resolvedTheme } = useTheme();
  const color = useMemo(
    () => adaptAreaColor(area.color, resolvedTheme),
    [area.color, resolvedTheme],
  );

  return (
    <button
      type="button"
      role="button"
      tabIndex={0}
      onClick={onClick}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onClick();
        }
      }}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      className="bg-surface border border-outline rounded-xl p-6 relative overflow-hidden group cursor-pointer text-left w-full transition-colors duration-200 motion-reduce:transition-none"
      style={{ borderColor: hovered ? color : undefined }}
    >
      {/* BLOOM CIRCLE */}
      <div
        className="absolute top-0 right-0 w-32 h-32 rounded-full filter blur-[80px] transition-opacity duration-200 pointer-events-none motion-reduce:transition-none"
        style={{
          backgroundColor: color,
          opacity: hovered ? 0.2 : 0.1,
        }}
      />

      {/* CONTENT (above bloom) */}
      <div className="relative z-10">
        {/* HEADER ROW */}
        <div className="flex items-center justify-between mb-2">
          <div className="flex items-center gap-3">
            {/* GLOW DOT */}
            <div
              data-testid="area-glow-dot"
              className="w-3 h-3 rounded-full shrink-0"
              style={{
                backgroundColor: color,
                boxShadow: `0 0 10px ${color}`,
              }}
            />
            <h2 className="font-sans font-semibold text-xl text-on-bg">{area.name}</h2>
          </div>
          <MaterialIcon name="hub" size={20} className="text-on-bg-secondary" />
        </div>

        {/* DESCRIPTION */}
        <p className="text-on-bg-secondary text-base">{area.description}</p>

        {/* METRICS */}
        <div className="mt-4 flex flex-col gap-2">
          <div className="flex justify-between font-mono text-xs">
            <span className="text-on-bg-secondary">Health:</span>
            <span className={healthColor}>{healthLabel}</span>
          </div>
          <div className="flex justify-between font-mono text-xs">
            <span className="text-on-bg-secondary">Devices:</span>
            <span className="text-on-bg">{deviceCount}</span>
          </div>
          <div className="flex justify-between font-mono text-xs">
            <span className="text-on-bg-secondary">Active Links:</span>
            <span className="text-on-bg">{activeLinkCount}</span>
          </div>
        </div>
      </div>
    </button>
  );
}
