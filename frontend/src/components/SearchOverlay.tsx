import { useDeferredValue, useEffect, useState } from 'react';
import type { Device } from '../types/api';
import { VendorIcon } from './icons/VendorIcon';
import { MaterialIcon } from './MaterialIcon';

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
          const sysName = (device.sys_name || '').toLowerCase();
          const displayName = (device.tags?.display_name || '').toLowerCase();
          return (
            hostname.includes(normalizedQuery) ||
            ip.includes(normalizedQuery) ||
            sysName.includes(normalizedQuery) ||
            displayName.includes(normalizedQuery)
          );
        })
        .slice(0, 8);

  const showDropdown = query.trim().length > 0;

  function handleSelect(deviceId: string) {
    onSelectDevice(deviceId);
    setQuery('');
    setDebouncedQuery('');
  }

  return (
    <div className="pointer-events-none fixed left-5 top-14 z-20 w-[min(420px,calc(100vw-2.5rem))]">
      <div className="pointer-events-auto rounded-2xl border border-glass-border bg-glass-bg p-3 shadow-canvas dark:backdrop-blur-[16px] transition-colors duration-200">
        <label className="flex items-center gap-3 rounded-xl bg-elevated px-4 py-3">
          <MaterialIcon name="search" size={16} className="text-on-bg-secondary" />
          <input
            autoFocus
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
            className="w-full border-0 bg-transparent text-sm text-on-bg outline-none placeholder:text-on-bg-secondary"
          />
        </label>
        {showDropdown ? (
          <div className="mt-3 overflow-hidden rounded-xl bg-surface-high">
            {results.length > 0 ? (
              results.map((device) => (
                <button
                  key={device.id}
                  type="button"
                  onClick={() => handleSelect(device.id)}
                  className="flex w-full items-center gap-3 px-4 py-3 text-left transition-colors duration-200 hover:bg-elevated"
                >
                  <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-surface">
                    <VendorIcon vendor={device.vendor || ''} size={20} />
                  </div>
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium text-on-bg">
                      {device.tags?.display_name || device.sys_name || device.hostname || device.ip}
                    </p>
                    <p className="truncate text-xs text-on-bg-secondary">{device.ip}</p>
                  </div>
                </button>
              ))
            ) : (
              <p className="px-4 py-3 text-sm text-on-bg-secondary">No devices found</p>
            )}
          </div>
        ) : null}
      </div>
    </div>
  );
}
