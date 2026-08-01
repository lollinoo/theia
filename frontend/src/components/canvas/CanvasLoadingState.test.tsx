import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { CanvasLoadingState } from './CanvasLoadingState';

describe('CanvasLoadingState', () => {
  it('announces a single imported node while its collision-free position is being saved', () => {
    render(<CanvasLoadingState importedNodeCount={1} overlay />);

    const status = screen.getByRole('status');
    expect(status).toHaveAttribute('aria-live', 'polite');
    expect(status).toHaveAttribute('aria-busy', 'true');
    expect(status).toHaveTextContent('Arranging 1 imported node…');
    expect(status).toHaveTextContent('Finding available space and saving the layout.');
  });

  it('uses plural copy for an imported batch', () => {
    render(<CanvasLoadingState importedNodeCount={4} overlay />);

    expect(screen.getByRole('status')).toHaveTextContent('Arranging 4 imported nodes…');
  });
});
