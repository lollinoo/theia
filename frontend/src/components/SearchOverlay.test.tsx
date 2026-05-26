import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import SearchOverlay from './SearchOverlay';

describe('SearchOverlay', () => {
  it('sits below the navigation pill instead of overlapping it', () => {
    render(<SearchOverlay devices={[]} onSelectDevice={() => undefined} />);

    const overlay = screen.getByTestId('search-overlay');
    expect(overlay.className).toContain('top-20');
    expect(overlay.className).not.toContain('top-14');
  });
});
