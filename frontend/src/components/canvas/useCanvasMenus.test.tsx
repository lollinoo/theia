import { renderHook } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { useCanvasMenus } from './useCanvasMenus';

describe('useCanvasMenus', () => {
  it('does not expose global settings as a canvas shortcut', () => {
    const reactFlow = {
      zoomIn: vi.fn(),
      zoomOut: vi.fn(),
      fitView: vi.fn(),
    };

    const { result } = renderHook(() => useCanvasMenus({ reactFlow: reactFlow as never }));

    expect(result.current.shortcuts).not.toHaveProperty('settings');
  });
});
