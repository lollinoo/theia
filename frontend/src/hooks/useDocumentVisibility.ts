/**
 * Coordinates document visibility state and side effects for consuming components.
 * Owns cleanup-sensitive lifecycle work so callers receive stable state and actions.
 */
import { useEffect, useState } from 'react';

const subscribers = new Set<(visible: boolean) => void>();
let isListening = false;

function currentVisibility(): boolean {
  return typeof document === 'undefined' ? true : !document.hidden;
}

function publishVisibility() {
  const visible = currentVisibility();
  subscribers.forEach((notify) => {
    notify(visible);
  });
}

function handleVisibilityChange() {
  publishVisibility();
}

function ensureListener() {
  if (isListening || typeof document === 'undefined') {
    return;
  }

  document.addEventListener('visibilitychange', handleVisibilityChange);
  isListening = true;
}

function stopListenerIfIdle() {
  if (!isListening || subscribers.size > 0 || typeof document === 'undefined') {
    return;
  }

  document.removeEventListener('visibilitychange', handleVisibilityChange);
  isListening = false;
}

/** Coordinates document visibility behavior for the React hook lifecycle. */
export function useDocumentVisibility(): boolean {
  const [visible, setVisible] = useState(currentVisibility);

  useEffect(() => {
    setVisible(currentVisibility());
    subscribers.add(setVisible);
    ensureListener();

    return () => {
      subscribers.delete(setVisible);
      stopListenerIfIdle();
    };
  }, []);

  return visible;
}
