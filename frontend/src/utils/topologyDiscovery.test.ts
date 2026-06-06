/**
 * Exercises topology discovery utility behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';

import { formatTopologyDiscoveryResult } from './topologyDiscovery';

describe('formatTopologyDiscoveryResult', () => {
  it('renders partial discovery failures as an operational failure label', () => {
    expect(formatTopologyDiscoveryResult('partial_discovery_failed')).toBe(
      'Partial discovery failed',
    );
  });
});
