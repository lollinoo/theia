/**
 * Exercises canvas helpers topology canvas behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';

import type { Link } from '../../types/api';
import type { LinkMetricsDTO } from '../../types/metrics';
import { findLinkMetrics } from './canvasHelpers';

function mockLink(overrides: Partial<Link> = {}): Link {
  return {
    id: 'link-1',
    source_device_id: 'dev-1',
    source_if_name: 'ether1',
    target_device_id: 'dev-2',
    target_if_name: 'ether2',
    discovery_protocol: 'lldp',
    source_if_speed: 1_000_000_000,
    source_if_oper_status: 'up',
    target_if_speed: 1_000_000_000,
    target_if_oper_status: 'up',
    ...overrides,
  };
}

function mockLinkMetrics(overrides: Partial<LinkMetricsDTO> = {}): LinkMetricsDTO {
  return {
    link_id: 'runtime-link-1',
    source_device_id: 'dev-1',
    target_device_id: 'dev-2',
    source_if_name: 'ether1',
    target_if_name: 'ether2',
    metrics_status: 'available',
    metrics_reason: 'ok',
    last_collected_at: '2026-04-20T12:00:00Z',
    tx_bps: 1_000,
    rx_bps: 2_000,
    utilization: 0.2,
    ...overrides,
  };
}

describe('findLinkMetrics', () => {
  it('matches normalized source interface names from runtime link records', () => {
    const metrics = findLinkMetrics(
      {
        'dev-1': [mockLinkMetrics({ source_if_name: ' Ether1 ' })],
      },
      mockLink(),
    );

    expect(metrics?.link_id).toBe('runtime-link-1');
  });

  it('falls back to target interface names for virtual-source links', () => {
    const metrics = findLinkMetrics(
      {
        'dev-2': [mockLinkMetrics({ target_if_name: ' Ether2 ' })],
      },
      mockLink(),
    );

    expect(metrics?.link_id).toBe('runtime-link-1');
  });
});
