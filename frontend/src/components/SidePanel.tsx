/**
 * Renders side panel UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
import React, { useEffect } from 'react';
import { createPortal } from 'react-dom';
import { MaterialIcon } from './MaterialIcon';

interface SidePanelProps {
  open: boolean;
  onClose: () => void;
  title: string;
  children: React.ReactNode;
  testId?: string;
}

/** Renders the SidePanel component within the UI component boundary. */
export function SidePanel({ open, onClose, title, children, testId }: SidePanelProps) {
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (open && e.key === 'Escape') {
        onClose();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [open, onClose]);

  const panel = (
    <div
      data-testid={testId}
      className={`fixed right-0 top-0 z-40 flex h-full w-[min(420px,100vw)] transform flex-col border-l border-outline bg-surface-container-high transition-transform duration-200 ease-in-out shadow-panel ${
        open ? 'translate-x-0' : 'translate-x-full'
      }`}
    >
      <div className="topology-glass flex items-center justify-between px-5 py-3 transition-colors duration-200">
        <div>
          <p className="text-xs font-semibold uppercase text-on-bg-secondary">Topology detail</p>
          <h2 className="mt-1 text-sm font-semibold text-on-bg">{title}</h2>
        </div>
        <button
          type="button"
          onClick={onClose}
          className="rounded-xl p-2 text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-surface-container-high"
          title="Close"
        >
          <MaterialIcon name="close" size={18} />
        </button>
      </div>
      <div className="flex-1 overflow-y-auto p-5 transition-colors duration-200">{children}</div>
    </div>
  );

  return createPortal(panel, document.body);
}
