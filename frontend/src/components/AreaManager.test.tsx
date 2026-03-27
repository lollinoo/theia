import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { AreaManager } from './AreaManager';

vi.mock('../contexts/ThemeContext', () => ({
  useTheme: () => ({ theme: 'dark' as const, resolvedTheme: 'dark' as const, setTheme: vi.fn() }),
  adaptAreaColor: (hex: string) => hex,
}));

// Mock API calls
const mockFetchAreas = vi.fn();
const mockCreateArea = vi.fn();
const mockUpdateArea = vi.fn();
const mockDeleteArea = vi.fn();
const mockFetchDevices = vi.fn();
const mockUpdateDevice = vi.fn();

vi.mock('../api/client', () => ({
  fetchAreas: (...args: unknown[]) => mockFetchAreas(...args),
  createArea: (...args: unknown[]) => mockCreateArea(...args),
  updateArea: (...args: unknown[]) => mockUpdateArea(...args),
  deleteArea: (...args: unknown[]) => mockDeleteArea(...args),
  fetchDevices: (...args: unknown[]) => mockFetchDevices(...args),
  updateDevice: (...args: unknown[]) => mockUpdateDevice(...args),
}));

beforeEach(() => {
  vi.clearAllMocks();
  mockFetchAreas.mockResolvedValue([]);
  mockFetchDevices.mockResolvedValue([]);
});

describe('AreaManager', () => {
  it('renders empty state when no areas exist', async () => {
    render(<AreaManager />);
    await waitFor(() => {
      expect(screen.getByText(/no areas yet/i)).toBeInTheDocument();
    });
  });

  it('renders area list with names and device counts', async () => {
    mockFetchAreas.mockResolvedValue([
      { id: 'a1', name: 'Backbone', description: 'Core', color: '#2979FF', device_count: 3, created_at: '', updated_at: '' },
      { id: 'a2', name: 'Edge', description: '', color: '#00E676', device_count: 0, created_at: '', updated_at: '' },
    ]);

    render(<AreaManager />);
    await waitFor(() => {
      expect(screen.getByText('Backbone')).toBeInTheDocument();
    });
    expect(screen.getByText('Edge')).toBeInTheDocument();
    // Device count badge should be visible
    expect(screen.getByText('3 devices')).toBeInTheDocument();
  });

  it('switches to create mode when New is clicked', async () => {
    render(<AreaManager />);
    await waitFor(() => {
      expect(screen.getByText(/no areas yet/i)).toBeInTheDocument();
    });

    const newBtn = screen.getByRole('button', { name: /new/i });
    await userEvent.click(newBtn);

    // Should show create form with "Create Area" button
    await waitFor(() => {
      expect(screen.getByText(/create area/i)).toBeInTheDocument();
    });
  });

  it('calls createArea with form data on submit', async () => {
    mockCreateArea.mockResolvedValue({ id: 'new-1', name: 'NewArea', description: '', color: '#00E676', device_count: 0, created_at: '', updated_at: '' });

    render(<AreaManager />);
    await waitFor(() => {
      expect(screen.getByText(/no areas yet/i)).toBeInTheDocument();
    });

    await userEvent.click(screen.getByRole('button', { name: /new/i }));

    await waitFor(() => {
      expect(screen.getByPlaceholderText(/backbone/i)).toBeInTheDocument();
    });

    await userEvent.type(screen.getByPlaceholderText(/backbone/i), 'NewArea');
    await userEvent.click(screen.getByText(/create area/i));

    await waitFor(() => {
      expect(mockCreateArea).toHaveBeenCalledWith(
        expect.objectContaining({ name: 'NewArea' }),
      );
    });
  });

  it('shows delete confirmation with device count', async () => {
    mockFetchAreas.mockResolvedValue([
      { id: 'a1', name: 'ToDelete', description: '', color: '#FF1744', device_count: 5, created_at: '', updated_at: '' },
    ]);

    render(<AreaManager />);
    await waitFor(() => {
      expect(screen.getByText('ToDelete')).toBeInTheDocument();
    });

    // Click the delete button via its aria-label
    const deleteButton = screen.getByRole('button', { name: /delete area/i });
    await userEvent.click(deleteButton);

    // Should show confirmation with device count
    await waitFor(() => {
      expect(screen.getByText(/5 devices will be unassigned/i)).toBeInTheDocument();
    });
  });
});
