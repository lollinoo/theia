/**
 * Renders device credentials section controls within the device configuration workflow.
 * Keeps this section focused on one editable device responsibility.
 */
import { useEffect, useRef, useState } from 'react';
import {
  assignCredentialProfile,
  clearWinBoxProfile,
  fetchCredentialProfiles,
  fetchDeviceCredentialProfiles,
  setWinBoxProfile,
  unassignCredentialProfile,
} from '../../api/client';
import type { CredentialProfile, Device, DeviceCredentialProfile } from '../../types/api';
import { MaterialIcon } from '../MaterialIcon';

interface DeviceCredentialsSectionProps {
  device: Device;
  readOnly?: boolean;
  isVirtual?: boolean;
  onWinBoxAvailabilityChange?: (hasWinboxProfile: boolean) => void;
}

/** Renders the DeviceCredentialsSection component within the device configuration workflow. */
export function DeviceCredentialsSection({
  device,
  readOnly = false,
  isVirtual,
  onWinBoxAvailabilityChange,
}: DeviceCredentialsSectionProps) {
  const [credentialProfiles, setCredentialProfiles] = useState<CredentialProfile[]>([]);
  const [assignments, setAssignments] = useState<DeviceCredentialProfile[]>([]);
  const [assignmentsLoading, setAssignmentsLoading] = useState(false);
  const [showAddSelect, setShowAddSelect] = useState(false);
  const [removingId, setRemovingId] = useState<string | null>(null);
  const winBoxAvailabilityCallbackRef = useRef(onWinBoxAvailabilityChange);
  const assignmentsGenerationRef = useRef(0);

  useEffect(() => {
    winBoxAvailabilityCallbackRef.current = onWinBoxAvailabilityChange;
  }, [onWinBoxAvailabilityChange]);

  async function loadAssignments(
    deviceId = device.id,
    generation = assignmentsGenerationRef.current,
  ) {
    setAssignmentsLoading(true);
    try {
      const nextAssignments = await fetchDeviceCredentialProfiles(deviceId);
      if (assignmentsGenerationRef.current !== generation) return;
      setAssignments(nextAssignments);
      winBoxAvailabilityCallbackRef.current?.(
        nextAssignments.some((assignment) => assignment.is_winbox),
      );
    } catch {
      // non-fatal — section shows empty
    } finally {
      if (assignmentsGenerationRef.current === generation) {
        setAssignmentsLoading(false);
      }
    }
  }

  useEffect(() => {
    if (isVirtual) return;
    let cancelled = false;
    fetchCredentialProfiles()
      .then((nextProfiles) => {
        if (!cancelled) setCredentialProfiles(nextProfiles);
      })
      .catch(() => {
        /* non-fatal */
      });
    return () => {
      cancelled = true;
    };
  }, [isVirtual]);

  useEffect(() => {
    const generation = assignmentsGenerationRef.current + 1;
    assignmentsGenerationRef.current = generation;
    setShowAddSelect(false);
    setRemovingId(null);

    if (isVirtual) {
      setAssignments([]);
      setAssignmentsLoading(false);
      winBoxAvailabilityCallbackRef.current?.(false);
      return;
    }

    setAssignments([]);
    void loadAssignments(device.id, generation);
  }, [device.id, isVirtual]);

  useEffect(() => {
    return () => {
      assignmentsGenerationRef.current += 1;
    };
  }, []);

  if (isVirtual) {
    return null;
  }

  async function handleAssign(profileId: string) {
    if (readOnly) return;
    const generation = assignmentsGenerationRef.current;
    try {
      await assignCredentialProfile(device.id, profileId);
      if (assignmentsGenerationRef.current !== generation) return;
      setShowAddSelect(false);
      void loadAssignments(device.id, generation);
    } catch {
      // non-fatal
    }
  }

  async function handleUnassign(profileId: string) {
    if (readOnly) return;
    const generation = assignmentsGenerationRef.current;
    try {
      await unassignCredentialProfile(device.id, profileId);
      if (assignmentsGenerationRef.current !== generation) return;
      setRemovingId(null);
      void loadAssignments(device.id, generation);
    } catch {
      // non-fatal
    }
  }

  async function handleToggleWinBox(profileId: string, currentlyDesignated: boolean) {
    if (readOnly) return;
    const generation = assignmentsGenerationRef.current;
    try {
      if (currentlyDesignated) {
        await clearWinBoxProfile(device.id);
      } else {
        await setWinBoxProfile(device.id, profileId);
      }
      if (assignmentsGenerationRef.current !== generation) return;
      void loadAssignments(device.id, generation);
    } catch {
      // non-fatal
    }
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
          Credentials
        </p>
        <button
          type="button"
          onClick={() => setShowAddSelect((v) => !v)}
          className="px-2 py-1 text-xs rounded bg-surface-high text-on-bg-secondary hover:text-on-bg"
        >
          + Add
        </button>
      </div>

      {showAddSelect && (
        <div className="flex items-center gap-2">
          <select
            defaultValue=""
            onChange={(e) => {
              if (e.target.value) {
                void handleAssign(e.target.value);
              }
            }}
            className="flex-1 rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
          >
            <option value="" disabled>
              Select a profile...
            </option>
            {credentialProfiles
              .filter((p) => !assignments.some((a) => a.profile_id === p.id))
              .map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
          </select>
          <button
            type="button"
            onClick={() => setShowAddSelect(false)}
            className="px-2 py-1 text-xs rounded bg-surface-high text-on-bg-secondary hover:text-on-bg"
          >
            Dismiss
          </button>
        </div>
      )}

      {assignmentsLoading && <p className="text-xs text-on-bg-secondary">Loading credentials...</p>}

      {!assignmentsLoading && assignments.length === 0 && (
        <p className="text-xs text-on-bg-secondary">
          No credentials assigned. Add a profile to enable WinBox launch.
        </p>
      )}

      {!assignmentsLoading &&
        assignments.map((assignment) => (
          <div key={assignment.profile_id} className="rounded-lg bg-surface-high p-3">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2 min-w-0">
                <span className="text-sm font-medium text-on-bg truncate">{assignment.name}</span>
                <span className="text-xs font-medium px-2 py-0.5 bg-surface rounded-full text-on-bg-secondary shrink-0">
                  {assignment.role}
                </span>
              </div>
              <div className="flex items-center gap-1 shrink-0 ml-2">
                <button
                  type="button"
                  title={
                    assignment.is_winbox
                      ? 'Clear WinBox designation'
                      : 'Designate as WinBox profile'
                  }
                  onClick={() => {
                    void handleToggleWinBox(assignment.profile_id, assignment.is_winbox);
                  }}
                  className={`p-1 rounded-md transition-colors${assignment.is_winbox ? ' text-primary' : ' text-on-bg-secondary hover:text-on-bg'}`}
                >
                  <MaterialIcon name="key" size={18} />
                </button>
                <button
                  type="button"
                  title="Remove assignment"
                  onClick={() => setRemovingId(assignment.profile_id)}
                  className="p-1 rounded-md text-on-bg-secondary hover:text-status-down transition-colors"
                >
                  <MaterialIcon name="remove" size={18} />
                </button>
              </div>
            </div>

            {removingId === assignment.profile_id && (
              <div className="mt-2 border border-status-down/30 bg-status-down/10 rounded-lg px-3 py-2 flex items-center justify-between">
                <p className="text-xs text-status-down">Delete this profile?</p>
                <div className="flex gap-2">
                  <button
                    type="button"
                    onClick={() => setRemovingId(null)}
                    className="px-2 py-1 text-xs rounded bg-surface-high text-on-bg hover:bg-elevated"
                  >
                    Keep Profile
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      void handleUnassign(assignment.profile_id);
                    }}
                    className="px-2 py-1 text-xs rounded bg-status-down text-white hover:opacity-90"
                  >
                    Delete
                  </button>
                </div>
              </div>
            )}
          </div>
        ))}
    </div>
  );
}
