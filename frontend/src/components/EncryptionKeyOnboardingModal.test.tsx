/**
 * Exercises EncryptionKeyOnboardingModal behavior so one-time bootstrap secrets are shown and acknowledged safely.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { EncryptionKeyOnboardingModal } from './EncryptionKeyOnboardingModal';

const fetchEncryptionKeyStatusMock = vi.fn();
const acknowledgeEncryptionKeyMock = vi.fn();

vi.mock('../api/client', () => ({
  fetchEncryptionKeyStatus: () => fetchEncryptionKeyStatusMock(),
  acknowledgeEncryptionKey: () => acknowledgeEncryptionKeyMock(),
}));

describe('EncryptionKeyOnboardingModal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('does not render when key is already acknowledged', async () => {
    fetchEncryptionKeyStatusMock.mockResolvedValueOnce({
      active_key_id: 'k-20261018-11223344',
      key_ids: ['k-20261018-11223344'],
      acknowledged: true,
      managed_by_env: false,
    });

    render(<EncryptionKeyOnboardingModal enabled={true} />);

    await waitFor(() => {
      expect(fetchEncryptionKeyStatusMock).toHaveBeenCalledOnce();
    });
    expect(screen.queryByText('Save Your Instance Encryption Key')).not.toBeInTheDocument();
  });

  it('renders pending secret and hides modal after acknowledgement', async () => {
    fetchEncryptionKeyStatusMock.mockResolvedValueOnce({
      active_key_id: 'k-20261018-11223344',
      key_ids: ['k-20261018-11223344'],
      acknowledged: false,
      managed_by_env: false,
      pending_secret: 'super-secret-value-12345',
      env_snippet:
        'THEIA_ENCRYPTION_KEY_ID=k-20261018-11223344\nTHEIA_ENCRYPTION_KEYS=k-20261018-11223344=super-secret-value-12345',
    });
    acknowledgeEncryptionKeyMock.mockResolvedValueOnce({
      active_key_id: 'k-20261018-11223344',
      key_ids: ['k-20261018-11223344'],
      acknowledged: true,
      managed_by_env: false,
    });

    render(<EncryptionKeyOnboardingModal enabled={true} />);

    await waitFor(() => {
      expect(screen.getByText('Save Your Instance Encryption Key')).toBeInTheDocument();
    });
    expect(screen.getByText(/super-secret-value-12345/)).toBeInTheDocument();

    const confirmBtn = screen.getByRole('button', { name: 'I Saved the Key' });
    expect(confirmBtn).toBeDisabled();

    fireEvent.click(screen.getByRole('checkbox'));
    expect(confirmBtn).not.toBeDisabled();

    fireEvent.click(confirmBtn);

    await waitFor(() => {
      expect(acknowledgeEncryptionKeyMock).toHaveBeenCalledOnce();
      expect(screen.queryByText('Save Your Instance Encryption Key')).not.toBeInTheDocument();
    });
  });
});
