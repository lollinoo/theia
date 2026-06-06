/**
 * Provides topology discovery utility behavior shared by frontend workflows.
 * Keeps non-UI policy and formatting rules reusable across components.
 */
import type { TopologyBootstrapState, TopologyDiscoveryMode } from '../types/api';

const TOPOLOGY_DISCOVERY_FOLLOWUP_DELAY_SECONDS = 20;

/** Defines topology discovery mode options constants and helper contracts for the shared frontend utility layer. */
export const TOPOLOGY_DISCOVERY_MODE_OPTIONS: Array<{
  value: TopologyDiscoveryMode;
  label: string;
}> = [
  { value: 'inherit', label: 'Use global default' },
  { value: 'off', label: 'Off' },
  { value: 'lldp', label: 'LLDP only' },
  { value: 'lldp_cdp', label: 'LLDP + CDP' },
  { value: 'bootstrap_once', label: 'Bootstrap once' },
];

/** Defines topology discovery default options constants and helper contracts for the shared frontend utility layer. */
export const TOPOLOGY_DISCOVERY_DEFAULT_OPTIONS: Array<{
  value: Exclude<TopologyDiscoveryMode, 'inherit'>;
  label: string;
}> = TOPOLOGY_DISCOVERY_MODE_OPTIONS.filter(
  (option): option is { value: Exclude<TopologyDiscoveryMode, 'inherit'>; label: string } =>
    option.value !== 'inherit',
);

function humanizeSnakeCase(value: string): string {
  return value
    .split('_')
    .filter(Boolean)
    .map((chunk) => chunk.charAt(0).toUpperCase() + chunk.slice(1))
    .join(' ');
}

/** Formats topology discovery mode for the shared frontend utility layer. */
export function formatTopologyDiscoveryMode(mode?: TopologyDiscoveryMode): string {
  return (
    TOPOLOGY_DISCOVERY_MODE_OPTIONS.find((option) => option.value === mode)?.label ??
    'Use global default'
  );
}

/** Formats topology bootstrap state for the shared frontend utility layer. */
export function formatTopologyBootstrapState(state?: TopologyBootstrapState): string {
  switch (state) {
    case 'pending':
      return 'Pending';
    case 'followup_scheduled':
      return 'Follow-up queued';
    case 'completed':
      return 'Completed';
    default:
      return 'Idle';
  }
}

/** Formats topology discovery result for the shared frontend utility layer. */
export function formatTopologyDiscoveryResult(result?: string): string {
  if (!result) {
    return 'Never run';
  }
  switch (result) {
    case 'partial_discovery_failed':
      return 'Partial discovery failed';
    case 'neighbors_found':
      return 'Neighbors found';
    case 'ports_pending':
      return 'Waiting for port details';
    case 'no_neighbors':
      return 'No neighbors found';
    default:
      return humanizeSnakeCase(result);
  }
}

/** Formats topology discovery timestamp for the shared frontend utility layer. */
export function formatTopologyDiscoveryTimestamp(value?: string | null): string {
  if (!value) {
    return 'Never';
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toLocaleString();
}

/** Formats topology followup expectation for the shared frontend utility layer. */
export function formatTopologyFollowupExpectation(
  state?: TopologyBootstrapState,
  lastDiscoveryAt?: string | null,
): string | null {
  if (state !== 'followup_scheduled') {
    return null;
  }
  if (!lastDiscoveryAt) {
    return 'Automatic follow-up queued shortly.';
  }
  return `Automatic follow-up runs about ${TOPOLOGY_DISCOVERY_FOLLOWUP_DELAY_SECONDS}s after last discovery.`;
}
