import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createOperatorSession, fetchOperatorSession } from '../api/client';
import { AuthGate } from './AuthGate';

vi.mock('../api/client', () => ({
  fetchOperatorSession: vi.fn(),
  createOperatorSession: vi.fn(),
}));

describe('AuthGate', () => {
  beforeEach(() => {
    vi.mocked(fetchOperatorSession).mockReset();
    vi.mocked(createOperatorSession).mockReset();
  });

  it('renders children when a session already exists', async () => {
    vi.mocked(fetchOperatorSession).mockResolvedValue({ authenticated: true, subject: 'alice' });

    render(
      <AuthGate>
        <div>secured app</div>
      </AuthGate>,
    );

    expect(await screen.findByText('secured app')).toBeInTheDocument();
  });

  it('creates an operator session before rendering children', async () => {
    vi.mocked(fetchOperatorSession).mockResolvedValue({ authenticated: false });
    vi.mocked(createOperatorSession).mockResolvedValue({ authenticated: true, subject: 'alice' });

    render(
      <AuthGate>
        <div>secured app</div>
      </AuthGate>,
    );

    fireEvent.change(await screen.findByLabelText('Token'), {
      target: { value: 'secret-token' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Entra' }));

    await waitFor(() => {
      expect(createOperatorSession).toHaveBeenCalledWith('secret-token', 'operator');
    });
    expect(await screen.findByText('secured app')).toBeInTheDocument();
  });

  it('keeps the app behind the login form when the session probe fails', async () => {
    vi.mocked(fetchOperatorSession).mockRejectedValue(new Error('network unavailable'));

    render(
      <AuthGate>
        <div>secured app</div>
      </AuthGate>,
    );

    expect(await screen.findByText('Accesso operatore')).toBeInTheDocument();
    expect(screen.queryByText('secured app')).not.toBeInTheDocument();
  });
});
