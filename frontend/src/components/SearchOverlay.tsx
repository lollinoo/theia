import { useDeferredValue, useEffect, useState } from 'react';
import type { Device } from '../types/api';
import { DeviceIcon } from './icons/DeviceIcon';

interface SearchOverlayProps {
  devices: Device[];
  onSelectDevice: (deviceId: string) => void;
}

export default function SearchOverlay({
  devices,
  onSelectDevice,
}: SearchOverlayProps) {
  const [query, setQuery] = useState('');
  const [debouncedQuery, setDebouncedQuery] = useState('');
  const deferredQuery = useDeferredValue(debouncedQuery);

  useEffect(() => {
    const timeout = window.setTimeout(() => {
      setDebouncedQuery(query.trim());
    }, 150);

    return () => {
      window.clearTimeout(timeout);
    };
  }, [query]);

  const normalizedQuery = deferredQuery.toLowerCase();
  const results =
    normalizedQuery.length === 0
      ? []
      : devices
          .filter((device) => {
            const hostname = device.hostname.toLowerCase();
            const ip = device.ip.toLowerCase();
            return hostname.includes(normalizedQuery) || ip.includes(normalizedQuery);
          })
          .slice(0, 8);

  const showDropdown = query.trim().length > 0;

  function handleSelect(deviceId: string) {
    onSelectDevice(deviceId);
    setQuery('');
    setDebouncedQuery('');
  }

  return (
    <div className="pointer-events-none fixed left-5 top-5 z-20 w-[min(420px,calc(100vw-2.5rem))]">
      <div className="pointer-events-auto rounded-2xl border border-border-subtle bg-bg-surface/80 p-3 shadow-canvas backdrop-blur-xl">
        <label className="flex items-center gap-3 rounded-xl border border-white/5 bg-bg-canvas/85 px-4 py-3">
          <svg viewBox="0 0 24 24" className="h-4 w-4 text-text-secondary" fill="none">
            <path
              d="M10.5 4.5C7.19 4.5 4.5 7.19 4.5 10.5C4.5 13.81 7.19 16.5 10.5 16.5C12.11 16.5 13.57 15.87 14.65 14.84L18.75 18.94"
              stroke="currentColor"
              strokeWidth="1.8"
              strokeLinecap="round"
            />
          </svg>
          <input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Escape') {
                setQuery('');
                setDebouncedQuery('');
                return;
              }

              if (event.key === 'Enter' && results[0]) {
                handleSelect(results[0].id);
              }
            }}
            placeholder="Search devices..."
            className="w-full border-0 bg-transparent text-sm text-text-primary outline-none placeholder:text-text-secondary"
          />
        </label>
        {showDropdown ? (
          <div className="mt-3 overflow-hidden rounded-xl border border-border-subtle bg-bg-canvas/90">
            {results.length > 0 ? (
              results.map((device) => (
                <button
                  key={device.id}
                  type="button"
                  onClick={() => handleSelect(device.id)}
                  className="flex w-full items-center gap-3 border-b border-white/5 px-4 py-3 text-left transition-colors duration-150 last:border-b-0 hover:bg-bg-elevated"
                >
                  <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-bg-surface">
                    <DeviceIcon type={device.device_type} size={20} />
                  </div>
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium text-text-primary">{device.hostname}</p>
                    <p className="truncate text-xs text-text-secondary">{device.ip}</p>
                  </div>
                </button>
              ))
            ) : (
              <p className="px-4 py-3 text-sm text-text-secondary">No devices found</p>
            )}
          </div>
        ) : null}
      </div>
    </div>
  );
}
