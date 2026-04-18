import type {
  TopologyBootstrapState,
  TopologyDiscoveryMode,
} from '../types/api';

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

export function formatTopologyDiscoveryMode(mode?: TopologyDiscoveryMode): string {
  return (
    TOPOLOGY_DISCOVERY_MODE_OPTIONS.find((option) => option.value === mode)?.label ?? 'Use global default'
  );
}

export function formatTopologyBootstrapState(state?: TopologyBootstrapState): string {
  switch (state) {
    case 'pending':
      return 'Pending';
    case 'followup_scheduled':
      return 'Follow-up scheduled';
    case 'completed':
      return 'Completed';
    default:
      return 'Idle';
  }
}

export function formatTopologyDiscoveryResult(result?: string): string {
  if (!result) {
    return 'Never run';
  }
  switch (result) {
    case 'neighbors_found':
      return 'Neighbors found';
    case 'ports_pending':
      return 'Ports still resolving';
    case 'no_neighbors':
      return 'No neighbors found';
    default:
      return humanizeSnakeCase(result);
  }
}

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
