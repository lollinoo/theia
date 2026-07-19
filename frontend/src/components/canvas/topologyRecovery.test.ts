/**
 * Exercises topology recovery topology canvas behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';

import {
  buildTopologyRecoveryFailureNotice,
  buildTopologyRecoveryNotice,
  measurementTriggerForCauses,
} from './topologyRecovery';

describe('topologyRecovery', () => {
  it('selects backend reconnect measurement for reconnect and resync causes', () => {
    expect(measurementTriggerForCauses(new Set(['backend-reconnected']))).toBe(
      'backend_reconnected',
    );
    expect(measurementTriggerForCauses(new Set(['backend-resync-required']))).toBe(
      'backend_reconnected',
    );
  });

  it('selects topology change measurement when no backend recovery cause is present', () => {
    expect(measurementTriggerForCauses(new Set(['topology-changed']))).toBe('topology_changed');
  });

  it('builds success notices with structural wording for retained recovery causes', () => {
    expect(buildTopologyRecoveryNotice(new Set(['backend-reconnected']))).toEqual({
      tone: 'success',
      message: 'Topology refreshed after reconnect',
    });
    expect(buildTopologyRecoveryNotice(new Set(['backend-resync-required']))).toEqual({
      tone: 'success',
      message: 'Topology refreshed after backend resync',
    });
    expect(
      buildTopologyRecoveryNotice(
        new Set(['backend-reconnected', 'backend-resync-required', 'topology-changed']),
      ),
    ).toEqual({
      tone: 'success',
      message: 'Topology refreshed',
    });
  });

  it('never describes structural refresh success as live topology resync', () => {
    const notices = [
      buildTopologyRecoveryNotice(new Set(['backend-reconnected'])),
      buildTopologyRecoveryNotice(new Set(['backend-resync-required'])),
      buildTopologyRecoveryNotice(new Set(['backend-reconnected', 'topology-changed'])),
    ];

    expect(notices.every((notice) => notice?.message !== 'Live topology resynced')).toBe(true);
  });

  it('does not show a recovery notice for topology-only refreshes', () => {
    expect(buildTopologyRecoveryNotice(new Set(['topology-changed']))).toBeNull();
  });

  it('builds the delayed refresh retry notice', () => {
    expect(buildTopologyRecoveryFailureNotice()).toEqual({
      tone: 'warning',
      message: 'Live topology refresh delayed',
      actionLabel: 'Retry topology refresh',
    });
  });
});
