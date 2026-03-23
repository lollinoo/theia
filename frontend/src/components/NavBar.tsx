import { useEffect, useState } from 'react';
import { fetchHealthVersion } from '../api/client';

type ActiveView = 'canvas' | 'dashboard';

interface NavBarProps {
  activeView: ActiveView;
  onViewChange: (view: ActiveView) => void;
}

export type { ActiveView };

export function NavBar({ activeView, onViewChange }: NavBarProps) {
  const [version, setVersion] = useState('');

  useEffect(() => {
    fetchHealthVersion().then((v) => setVersion(v.version));
  }, []);
  const tabs: { id: ActiveView; label: string }[] = [
    { id: 'canvas', label: 'Topology' },
    { id: 'dashboard', label: 'Devices' },
  ];

  return (
    <div className="fixed top-0 left-0 right-0 z-30 flex h-10 items-center border-b border-border-subtle bg-bg-surface/90 px-4 backdrop-blur-xl">
      <span className="mr-1 text-sm font-semibold text-text-primary tracking-wide">THEIA</span>
      {version && version !== 'unknown' && (
        <span className="mr-5 text-[10px] text-text-secondary/50 font-medium">v{version}</span>
      )}
      <div className="flex gap-1">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            onClick={() => onViewChange(tab.id)}
            className={`px-3 py-1.5 text-xs font-medium rounded-md transition-colors ${
              activeView === tab.id
                ? 'bg-accent/15 text-accent'
                : 'text-text-secondary hover:text-text-primary hover:bg-bg-elevated'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>
    </div>
  );
}
