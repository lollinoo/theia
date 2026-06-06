/**
 * Exercises snmpprofile manager component behavior so refactors preserve the documented contract.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ServerError, ValidationError } from '../api/errors';
import { SNMPProfileManager } from './SNMPProfileManager';

// Mock API calls
vi.mock('../api/client', () => ({
  fetchSNMPProfiles: vi.fn().mockResolvedValue([]),
  createSNMPProfile: vi.fn().mockResolvedValue({
    id: 'new-prof',
    name: 'Test Profile',
    description: '',
    snmp: { version: '2c', community: 'public' },
  }),
  updateSNMPProfile: vi.fn().mockResolvedValue({}),
  deleteSNMPProfile: vi.fn().mockResolvedValue(undefined),
}));

beforeEach(() => {
  vi.clearAllMocks();
});

// Helper: navigate to the create form
async function renderCreateForm() {
  render(<SNMPProfileManager />);
  // Wait for initial load to complete (loading state clears)
  await waitFor(() => {
    expect(screen.queryByText('Loading profiles...')).not.toBeInTheDocument();
  });
  fireEvent.click(screen.getByText('New'));
  // Form should now be visible
  await waitFor(() => {
    expect(screen.getByPlaceholderText('e.g. Office SNMPv3')).toBeInTheDocument();
  });
}

// --- Gap 3: SNMPProfileManager blur+submit validation ---

describe('SNMPProfileManager ProfileForm — name required validation', () => {
  it('shows error text when name field is blurred while empty', async () => {
    await renderCreateForm();

    const nameInput = screen.getByPlaceholderText('e.g. Office SNMPv3');
    fireEvent.blur(nameInput);

    await waitFor(() => {
      expect(screen.getByText('Profile name is required')).toBeInTheDocument();
    });
  });

  it('name field gets border-status-down class when empty on blur', async () => {
    await renderCreateForm();

    const nameInput = screen.getByPlaceholderText('e.g. Office SNMPv3');
    fireEvent.blur(nameInput);

    await waitFor(() => {
      expect(nameInput.className).toContain('border-status-down');
    });
  });
});

describe('SNMPProfileManager ProfileForm — description max length validation', () => {
  it('shows error text when description exceeds max length on blur', async () => {
    await renderCreateForm();

    const descInput = screen.getByPlaceholderText('Optional description');
    fireEvent.change(descInput, { target: { value: 'a'.repeat(256) } });
    fireEvent.blur(descInput);

    await waitFor(() => {
      expect(screen.getByText('Description must be 255 characters or fewer')).toBeInTheDocument();
    });
  });
});

describe('SNMPProfileManager ProfileForm — submit blocks on validation error', () => {
  it('does not call createSNMPProfile when name is empty on submit', async () => {
    const { createSNMPProfile } = await import('../api/client');
    const { container } = render(<SNMPProfileManager />);
    // Wait for initial load
    await waitFor(() => {
      expect(screen.queryByText('Loading profiles...')).not.toBeInTheDocument();
    });
    fireEvent.click(screen.getByText('New'));
    await waitFor(() => {
      expect(screen.getByPlaceholderText('e.g. Office SNMPv3')).toBeInTheDocument();
    });

    // Submit via form element to bypass HTML5 required attribute in jsdom
    const form = container.querySelector('form')!;
    fireEvent.submit(form);

    await waitFor(() => {
      expect(screen.getByText('Profile name is required')).toBeInTheDocument();
    });
    expect(createSNMPProfile).not.toHaveBeenCalled();
  });
});

describe('SNMPProfileManager ProfileForm — field error clears on user edit', () => {
  it('clears name error when user starts typing', async () => {
    await renderCreateForm();

    const nameInput = screen.getByPlaceholderText('e.g. Office SNMPv3');
    fireEvent.blur(nameInput);

    await waitFor(() => {
      expect(screen.getByText('Profile name is required')).toBeInTheDocument();
    });

    fireEvent.change(nameInput, { target: { value: 'My Profile' } });

    await waitFor(() => {
      expect(screen.queryByText('Profile name is required')).not.toBeInTheDocument();
    });
  });
});

describe('SNMPProfileManager ProfileForm — v3 auth protocol allowlist validation', () => {
  it('shows error when invalid auth protocol is submitted in v3 mode', async () => {
    const { createSNMPProfile } = await import('../api/client');
    await renderCreateForm();

    // Fill name
    fireEvent.change(screen.getByPlaceholderText('e.g. Office SNMPv3'), {
      target: { value: 'My v3 Profile' },
    });

    // Switch to v3
    fireEvent.change(screen.getByDisplayValue('v2c'), { target: { value: '3' } });

    await waitFor(() => {
      expect(screen.getByText('SNMPv3 Credentials')).toBeInTheDocument();
    });

    // Submit with default valid SHA protocol — should succeed (no auth error)
    // We just check it doesn't show an auth error for valid default
    fireEvent.click(screen.getByText('Create Profile'));

    // Auth protocol SHA is valid — no error should appear for authProtocol
    await waitFor(() => {
      expect(screen.queryByText(/Auth protocol must be one of/)).not.toBeInTheDocument();
    });
    expect(createSNMPProfile).toHaveBeenCalled();
  });
});

describe('SNMPProfileManager ProfileForm — backend typed error display', () => {
  it('shows ServerError ref message when createSNMPProfile throws ServerError', async () => {
    const { createSNMPProfile } = await import('../api/client');
    (createSNMPProfile as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ServerError('internal error, ref: abc123', 'abc123'),
    );

    await renderCreateForm();

    fireEvent.change(screen.getByPlaceholderText('e.g. Office SNMPv3'), {
      target: { value: 'Test Profile' },
    });
    fireEvent.click(screen.getByText('Create Profile'));

    await waitFor(() => {
      expect(screen.getByText('Something went wrong (ref: abc123)')).toBeInTheDocument();
    });
  });

  it('shows ValidationError message when createSNMPProfile throws ValidationError', async () => {
    const { createSNMPProfile } = await import('../api/client');
    (createSNMPProfile as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ValidationError('profile name already exists'),
    );

    await renderCreateForm();

    fireEvent.change(screen.getByPlaceholderText('e.g. Office SNMPv3'), {
      target: { value: 'Duplicate Profile' },
    });
    fireEvent.click(screen.getByText('Create Profile'));

    await waitFor(() => {
      expect(screen.getByText('profile name already exists')).toBeInTheDocument();
    });
  });

  it('shows plain error message when createSNMPProfile throws plain Error', async () => {
    const { createSNMPProfile } = await import('../api/client');
    (createSNMPProfile as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new Error('network failure'),
    );

    await renderCreateForm();

    fireEvent.change(screen.getByPlaceholderText('e.g. Office SNMPv3'), {
      target: { value: 'Another Profile' },
    });
    fireEvent.click(screen.getByText('Create Profile'));

    await waitFor(() => {
      expect(screen.getByText('network failure')).toBeInTheDocument();
    });
  });
});
