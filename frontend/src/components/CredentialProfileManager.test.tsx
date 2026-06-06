/**
 * Exercises credential profile manager component behavior so refactors preserve the documented contract.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ServerError, ValidationError } from '../api/errors';
import { CredentialProfileManager } from './CredentialProfileManager';

// Mock API calls
vi.mock('../api/client', () => ({
  fetchCredentialProfiles: vi.fn().mockResolvedValue([]),
  createCredentialProfile: vi.fn().mockResolvedValue({
    id: 'new-cp',
    name: 'Test Profile',
    description: '',
    username: 'admin',
    port: 22,
    auth_method: 'password',
    role: 'Admin',
  }),
  updateCredentialProfile: vi.fn().mockResolvedValue({}),
  deleteCredentialProfile: vi.fn().mockResolvedValue(undefined),
}));

beforeEach(() => {
  vi.clearAllMocks();
});

// Helper: navigate to the create form
async function renderCreateForm() {
  render(<CredentialProfileManager />);
  await waitFor(() => {
    expect(screen.queryByText('Loading profiles...')).not.toBeInTheDocument();
  });
  fireEvent.click(screen.getByText('New'));
  await waitFor(() => {
    expect(screen.getByPlaceholderText('e.g. MikroTik Admin')).toBeInTheDocument();
  });
}

// --- CredentialProfileManager blur+submit validation ---

describe('CredentialProfileManager ProfileForm — name required validation', () => {
  it('shows error text when name field is blurred while empty', async () => {
    await renderCreateForm();

    const nameInput = screen.getByPlaceholderText('e.g. MikroTik Admin');
    fireEvent.change(nameInput, { target: { value: '' } });
    fireEvent.blur(nameInput);

    await waitFor(() => {
      expect(screen.getByText('Profile name is required')).toBeInTheDocument();
    });
  });

  it('name input gets border-status-down class on empty blur', async () => {
    await renderCreateForm();

    const nameInput = screen.getByPlaceholderText('e.g. MikroTik Admin');
    fireEvent.change(nameInput, { target: { value: '' } });
    fireEvent.blur(nameInput);

    await waitFor(() => {
      expect(nameInput.className).toContain('border-status-down');
    });
  });
});

describe('CredentialProfileManager ProfileForm — role field default value', () => {
  it('role field renders with default value Admin when creating a new profile', async () => {
    await renderCreateForm();

    const roleInput = screen.getByPlaceholderText('e.g. Admin');
    expect((roleInput as HTMLInputElement).value).toBe('Admin');
  });
});

describe('CredentialProfileManager ProfileForm — username required validation', () => {
  it('shows error text when username is cleared and blurred', async () => {
    await renderCreateForm();

    const usernameInput = screen.getByPlaceholderText('admin');
    fireEvent.change(usernameInput, { target: { value: '' } });
    fireEvent.blur(usernameInput);

    await waitFor(() => {
      expect(screen.getByText('Username is required')).toBeInTheDocument();
    });
  });
});

describe('CredentialProfileManager ProfileForm — port range validation', () => {
  it('shows error text when port 0 is blurred (below range)', async () => {
    await renderCreateForm();

    const portInput = screen.getByPlaceholderText('22');
    fireEvent.change(portInput, { target: { value: '0' } });
    fireEvent.blur(portInput);

    await waitFor(() => {
      expect(screen.getByText('Port must be between 1 and 65535')).toBeInTheDocument();
    });
  });

  it('shows error text when port 65536 is blurred (above range)', async () => {
    await renderCreateForm();

    const portInput = screen.getByPlaceholderText('22');
    fireEvent.change(portInput, { target: { value: '65536' } });
    fireEvent.blur(portInput);

    await waitFor(() => {
      expect(screen.getByText('Port must be between 1 and 65535')).toBeInTheDocument();
    });
  });

  it('accepts port 22 without error', async () => {
    await renderCreateForm();

    const portInput = screen.getByPlaceholderText('22');
    fireEvent.blur(portInput); // value is already '22' from emptyForm()

    await waitFor(() => {
      expect(screen.queryByText('Port must be between 1 and 65535')).not.toBeInTheDocument();
    });
  });
});

describe('CredentialProfileManager ProfileForm — secret required on create', () => {
  it('shows password required error when secret is empty on submit', async () => {
    const { createCredentialProfile } = await import('../api/client');
    await renderCreateForm();

    // Fill required name and username (already defaulted)
    fireEvent.change(screen.getByPlaceholderText('e.g. MikroTik Admin'), {
      target: { value: 'My Credential Profile' },
    });
    // Leave password blank and submit
    fireEvent.click(screen.getByText('Create Profile'));

    await waitFor(() => {
      expect(screen.getByText('Password is required')).toBeInTheDocument();
    });
    expect(createCredentialProfile).not.toHaveBeenCalled();
  });

  it('does not require secret when isEdit=true (edit mode)', async () => {
    const { fetchCredentialProfiles } = await import('../api/client');
    (fetchCredentialProfiles as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      {
        id: 'cp-1',
        name: 'Existing Profile',
        description: '',
        username: 'admin',
        port: 22,
        auth_method: 'password' as const,
        role: 'Admin',
      },
    ]);

    render(<CredentialProfileManager />);
    await waitFor(() => {
      expect(screen.getByText('Existing Profile')).toBeInTheDocument();
    });

    // Click edit button (first button with title "Edit profile")
    fireEvent.click(screen.getByTitle('Edit profile'));

    await waitFor(() => {
      expect(screen.getByDisplayValue('Existing Profile')).toBeInTheDocument();
    });

    // In edit mode the secret field shows placeholder "(unchanged if blank)"
    expect(screen.getByPlaceholderText('(unchanged if blank)')).toBeInTheDocument();
  });
});

describe('CredentialProfileManager ProfileForm — field errors clear on user edit', () => {
  it('clears name error when user types in the name field', async () => {
    await renderCreateForm();

    const nameInput = screen.getByPlaceholderText('e.g. MikroTik Admin');
    fireEvent.change(nameInput, { target: { value: '' } });
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

describe('CredentialProfileManager ProfileForm — backend typed error display', () => {
  it('shows ServerError ref message when createCredentialProfile throws ServerError', async () => {
    const { createCredentialProfile } = await import('../api/client');
    (createCredentialProfile as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ServerError('internal error, ref: xyz789', 'xyz789'),
    );

    await renderCreateForm();

    fireEvent.change(screen.getByPlaceholderText('e.g. MikroTik Admin'), {
      target: { value: 'My Credential Profile' },
    });
    // Fill password
    fireEvent.change(screen.getByPlaceholderText('Enter password'), {
      target: { value: 'secret123' },
    });
    fireEvent.click(screen.getByText('Create Profile'));

    await waitFor(() => {
      expect(screen.getByText('Something went wrong (ref: xyz789)')).toBeInTheDocument();
    });
  });

  it('shows ValidationError message when createCredentialProfile throws ValidationError', async () => {
    const { createCredentialProfile } = await import('../api/client');
    (createCredentialProfile as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ValidationError('username contains invalid characters'),
    );

    await renderCreateForm();

    fireEvent.change(screen.getByPlaceholderText('e.g. MikroTik Admin'), {
      target: { value: 'Credential Profile' },
    });
    fireEvent.change(screen.getByPlaceholderText('Enter password'), {
      target: { value: 'pass' },
    });
    fireEvent.click(screen.getByText('Create Profile'));

    await waitFor(() => {
      expect(screen.getByText('username contains invalid characters')).toBeInTheDocument();
    });
  });
});
