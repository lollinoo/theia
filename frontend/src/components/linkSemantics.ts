/**
 * Renders link semantics UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
import type { CSSProperties, MouseEvent as ReactMouseEvent } from 'react';
import type { Link, LinkRoute } from '../types/api';
import { type AlertStatus, type DeviceMetricsDTO, type LinkMetricsDTO } from '../types/metrics';
import {
  computeLinkBadgeAnchor,
  type EdgeBadgeAnchor,
  measureEdgePathLength,
} from './edgeBadgeAnchors';

/** Describes the edge emphasis contract used by the UI component boundary. */
export type EdgeEmphasis = 'default' | 'muted' | 'connected';
/** Describes the edge semantic state contract used by the UI component boundary. */
export type EdgeSemanticState = 'neutral' | 'up' | 'warning' | 'critical';
/** Describes the link negotiation state contract used by the UI component boundary. */
export type LinkNegotiationState =
  | 'matched'
  | 'mismatch'
  | 'partial'
  | 'unknown'
  | 'not_applicable';
/** Describes the link badge kind contract used by the UI component boundary. */
export type LinkBadgeKind = 'rate' | 'throughput';
/** Describes the link badge zoom band contract used by the UI component boundary. */
export type LinkBadgeZoomBand = 'low' | 'medium' | 'high';
/** Describes the link badge semantic priority contract used by the UI component boundary. */
export type LinkBadgeSemanticPriority = 'normal' | 'active' | 'alert';
type DeviceEndpointHealth = DeviceMetricsDTO['health'];
type DeviceEndpointPrimaryHealth = DeviceMetricsDTO['primary_health'];
type DeviceEndpointReachability = DeviceMetricsDTO['reachability'];
type DeviceEndpointReachabilityEvidence = DeviceMetricsDTO['network_reachable'];

interface DeviceEndpointRuntimeState {
  health?: DeviceEndpointHealth;
  primaryHealth?: DeviceEndpointPrimaryHealth;
  reachability?: DeviceEndpointReachability;
  networkReachable?: DeviceEndpointReachabilityEvidence;
  snmpReachable?: DeviceEndpointReachabilityEvidence;
}

/** Describes the link edge data contract used by the UI component boundary. */
export interface LinkEdgeData {
  link?: Link;
  interactionMode?: 'idle' | 'interactive';
  bandwidthLabel?: string;
  speedLabel?: string;
  negotiationTitle?: string;
  autonegTitle?: string;
  speedMismatch?: boolean;
  negotiationState?: LinkNegotiationState;
  inertVirtualLink?: boolean;
  manual?: boolean;
  parallelIndex?: number;
  onContextMenu?: (event: MouseEvent | ReactMouseEvent<SVGPathElement>, edgeID: string) => void;
  route?: LinkRoute;
  routeEditable?: boolean;
  onRouteCommit?: (edgeId: string, route: LinkRoute | null) => void;
  metrics?: LinkMetricsDTO | null;
  throughputLabel?: string;
  utilization?: number | null;
  alertStatus?: AlertStatus;
  sourceIfStatus?: string;
  targetIfStatus?: string;
  sourceDeviceStatus?: string;
  targetDeviceStatus?: string;
  sourceDeviceAlertStatus?: AlertStatus;
  targetDeviceAlertStatus?: AlertStatus;
  sourceDeviceHealth?: DeviceEndpointHealth;
  targetDeviceHealth?: DeviceEndpointHealth;
  sourceDevicePrimaryHealth?: DeviceEndpointPrimaryHealth;
  targetDevicePrimaryHealth?: DeviceEndpointPrimaryHealth;
  sourceDeviceReachability?: DeviceEndpointReachability;
  targetDeviceReachability?: DeviceEndpointReachability;
  sourceDeviceNetworkReachable?: DeviceEndpointReachabilityEvidence;
  targetDeviceNetworkReachable?: DeviceEndpointReachabilityEvidence;
  sourceDeviceSnmpReachable?: DeviceEndpointReachabilityEvidence;
  targetDeviceSnmpReachable?: DeviceEndpointReachabilityEvidence;
  sourceIsVirtual?: boolean;
  targetIsVirtual?: boolean;
  areaColor?: string;
  emphasis?: EdgeEmphasis;
  [key: string]: unknown;
}

/** Describes the link badge view model contract used by the UI component boundary. */
export interface LinkBadgeViewModel {
  key: string;
  text: string;
  title?: string;
  className: string;
  style?: CSSProperties;
  warningIndicator?: {
    text: string;
    title?: string;
    className: string;
  };
}

/** Describes the link badge visibility contract used by the UI component boundary. */
export interface LinkBadgeVisibility {
  zoomBand: LinkBadgeZoomBand;
  showRate: boolean;
  showThroughput: boolean;
}

/** Describes the link badge presentation contract used by the UI component boundary. */
export interface LinkBadgePresentation {
  anchor: EdgeBadgeAnchor;
  items: LinkBadgeViewModel[];
  opacity: number;
  scale: number;
  visibility: LinkBadgeVisibility;
  semanticState: EdgeSemanticState;
  semanticPriority: LinkBadgeSemanticPriority;
}

interface LinkTelemetryInput {
  sourceSpeed: number;
  targetSpeed: number;
  isVirtualLink: boolean;
  sourceIsVirtual: boolean;
}

interface NormalizedLinkState {
  inertVirtualLink: boolean;
  alertStatus: AlertStatus | undefined;
  sourceDeviceStatus: string | undefined;
  targetDeviceStatus: string | undefined;
  sourceDeviceAlertStatus: AlertStatus | undefined;
  targetDeviceAlertStatus: AlertStatus | undefined;
  sourceDeviceRuntime: DeviceEndpointRuntimeState;
  targetDeviceRuntime: DeviceEndpointRuntimeState;
  sourceIfStatus: string | undefined;
  targetIfStatus: string | undefined;
  utilization: number | null;
  speedMismatch: boolean;
}

interface ResolveLinkBadgeVisibilityOptions {
  zoom: number;
  pathLength: number;
  bandwidthLabel?: string;
  throughputLabel?: string;
}

interface ResolveLinkBadgePresentationOptions {
  data: LinkEdgeData | undefined;
  zoom: number;
  path: string;
  fallbackX: number;
  fallbackY: number;
  edgeTone: ReturnType<typeof resolveEdgeTone>;
  parallelIndex?: number;
  isActive: boolean;
  isConnected: boolean;
  isMuted: boolean;
}

const LINK_BADGE_STACK_ORDER: readonly LinkBadgeKind[] = ['rate', 'throughput'];
const INERT_VIRTUAL_UTIL_WARNING_THRESHOLD = 0.75;
const INERT_VIRTUAL_UTIL_CRITICAL_THRESHOLD = 0.8;

// Centralized zoom matrix for link telemetry badges.
// Throughput remains visible whenever runtime telemetry is available; the band is
// still exposed for callers that need zoom metadata.
export const LINK_BADGE_ZOOM_THRESHOLDS = {
  medium: 0.92,
  high: 1.2,
} as const;
const LINK_BADGE_SCALE_START_ZOOM = 0.9;
const LINK_BADGE_MIN_SCALE_ZOOM = 0.6;
const LINK_BADGE_MAX_SCALE = 1.2;

function formatSpeedBadge(speed: number): string {
  return speed > 0 ? `SPD ${formatBandwidth(speed)}` : 'SPD ?';
}

function isNegotiationWarning(state: LinkNegotiationState | undefined): boolean {
  return state === 'mismatch' || state === 'partial' || state === 'unknown';
}

/** Formats bandwidth for the UI component boundary. */
export function formatBandwidth(speed: number): string {
  if (!speed || speed <= 0) {
    return 'Unknown';
  }

  if (speed >= 1_000_000_000) {
    return `${Math.round(speed / 1_000_000_000)} Gbps`;
  }

  if (speed >= 1_000_000) {
    return `${Math.round(speed / 1_000_000)} Mbps`;
  }

  if (speed >= 1_000) {
    return `${Math.round(speed / 1_000)} Kbps`;
  }

  return `${speed} bps`;
}

/** Normalizes interface status for link state in the UI component boundary. */
export function normalizeInterfaceStatusForLink(status: string | undefined): string | undefined {
  const normalized = status?.trim().toLowerCase();
  if (!normalized || normalized === 'unknown') {
    return undefined;
  }
  return normalized;
}

function normalizeEndpointRuntimeState({
  suppressed,
  health,
  primaryHealth,
  reachability,
  networkReachable,
  snmpReachable,
}: DeviceEndpointRuntimeState & { suppressed: boolean }): DeviceEndpointRuntimeState {
  if (suppressed) {
    return {};
  }

  const runtime: DeviceEndpointRuntimeState = {};
  if (health !== undefined) runtime.health = health;
  if (primaryHealth !== undefined) runtime.primaryHealth = primaryHealth;
  if (reachability !== undefined) runtime.reachability = reachability;
  if (networkReachable !== undefined) runtime.networkReachable = networkReachable;
  if (snmpReachable !== undefined) runtime.snmpReachable = snmpReachable;
  return runtime;
}

function isEndpointRuntimeCritical(runtime: DeviceEndpointRuntimeState): boolean {
  return (
    runtime.health === 'critical' ||
    runtime.primaryHealth === 'unreachable' ||
    runtime.primaryHealth === 'quarantined' ||
    runtime.reachability === 'hard_down' ||
    runtime.networkReachable === 'false'
  );
}

function isEndpointNetworkReachable(runtime: DeviceEndpointRuntimeState): boolean {
  return runtime.networkReachable === 'true' || runtime.reachability === 'up';
}

function isEndpointRuntimeWarning(runtime: DeviceEndpointRuntimeState): boolean {
  if (isEndpointNetworkReachable(runtime)) {
    return runtime.health === 'warning';
  }

  return (
    runtime.health === 'warning' ||
    runtime.primaryHealth === 'snmp_degraded' ||
    runtime.reachability === 'soft_down' ||
    runtime.snmpReachable === 'false'
  );
}

/** Builds link telemetry badges for the UI component boundary. */
export function buildLinkTelemetryBadges({
  sourceSpeed,
  targetSpeed,
  isVirtualLink,
  sourceIsVirtual,
}: LinkTelemetryInput): Pick<
  LinkEdgeData,
  | 'bandwidthLabel'
  | 'speedLabel'
  | 'negotiationTitle'
  | 'autonegTitle'
  | 'speedMismatch'
  | 'negotiationState'
> {
  if (isVirtualLink) {
    const realSpeed = sourceIsVirtual ? targetSpeed : sourceSpeed;
    const speedLabel = realSpeed > 0 ? formatSpeedBadge(realSpeed) : undefined;

    return {
      bandwidthLabel: realSpeed > 0 ? formatBandwidth(realSpeed) : undefined,
      speedLabel,
      negotiationTitle: undefined,
      autonegTitle: undefined,
      speedMismatch: false,
      negotiationState: 'not_applicable',
    };
  }

  if (sourceSpeed > 0 && targetSpeed > 0) {
    if (sourceSpeed !== targetSpeed) {
      const negotiatedSpeed = Math.min(sourceSpeed, targetSpeed);
      const interfaceSpeed = Math.max(sourceSpeed, targetSpeed);
      const title = `Negotiation mismatch: ${formatBandwidth(sourceSpeed)} vs ${formatBandwidth(targetSpeed)}.`;

      return {
        bandwidthLabel: formatBandwidth(negotiatedSpeed),
        speedLabel: formatSpeedBadge(interfaceSpeed),
        negotiationTitle: title,
        autonegTitle: title,
        speedMismatch: true,
        negotiationState: 'mismatch',
      };
    }

    const title = `Autonegotiation matched at ${formatBandwidth(sourceSpeed)}.`;

    return {
      bandwidthLabel: formatBandwidth(sourceSpeed),
      speedLabel: formatSpeedBadge(sourceSpeed),
      negotiationTitle: title,
      autonegTitle: title,
      speedMismatch: false,
      negotiationState: 'matched',
    };
  }

  if (sourceSpeed > 0 || targetSpeed > 0) {
    const detectedSpeed = sourceSpeed > 0 ? sourceSpeed : targetSpeed;
    const title =
      'Autonegotiation is only partially visible because one side did not expose negotiated speed.';

    return {
      bandwidthLabel: formatBandwidth(detectedSpeed),
      speedLabel: formatSpeedBadge(detectedSpeed),
      negotiationTitle: title,
      autonegTitle: title,
      speedMismatch: false,
      negotiationState: 'partial',
    };
  }

  const title = 'Autonegotiation data is not available on either interface.';

  return {
    bandwidthLabel: 'SPD ?',
    speedLabel: undefined,
    negotiationTitle: title,
    autonegTitle: title,
    speedMismatch: false,
    negotiationState: 'unknown',
  };
}

/** Normalizes link state for color for the UI component boundary. */
export function normalizeLinkStateForColor(data: LinkEdgeData | undefined): NormalizedLinkState {
  const inertVirtualLink = data?.inertVirtualLink === true;
  const suppressSourceVirtualEndpoint = inertVirtualLink && data?.sourceIsVirtual === true;
  const suppressTargetVirtualEndpoint = inertVirtualLink && data?.targetIsVirtual === true;

  return {
    inertVirtualLink,
    alertStatus: data?.alertStatus,
    sourceDeviceStatus: suppressSourceVirtualEndpoint ? undefined : data?.sourceDeviceStatus,
    targetDeviceStatus: suppressTargetVirtualEndpoint ? undefined : data?.targetDeviceStatus,
    sourceDeviceAlertStatus: suppressSourceVirtualEndpoint
      ? undefined
      : data?.sourceDeviceAlertStatus,
    targetDeviceAlertStatus: suppressTargetVirtualEndpoint
      ? undefined
      : data?.targetDeviceAlertStatus,
    sourceDeviceRuntime: normalizeEndpointRuntimeState({
      suppressed: suppressSourceVirtualEndpoint,
      health: data?.sourceDeviceHealth,
      primaryHealth: data?.sourceDevicePrimaryHealth,
      reachability: data?.sourceDeviceReachability,
      networkReachable: data?.sourceDeviceNetworkReachable,
      snmpReachable: data?.sourceDeviceSnmpReachable,
    }),
    targetDeviceRuntime: normalizeEndpointRuntimeState({
      suppressed: suppressTargetVirtualEndpoint,
      health: data?.targetDeviceHealth,
      primaryHealth: data?.targetDevicePrimaryHealth,
      reachability: data?.targetDeviceReachability,
      networkReachable: data?.targetDeviceNetworkReachable,
      snmpReachable: data?.targetDeviceSnmpReachable,
    }),
    sourceIfStatus: normalizeInterfaceStatusForLink(data?.sourceIfStatus),
    targetIfStatus: normalizeInterfaceStatusForLink(data?.targetIfStatus),
    utilization: data?.utilization ?? data?.metrics?.utilization ?? null,
    speedMismatch: data?.speedMismatch === true || data?.negotiationState === 'mismatch',
  };
}

/** Resolves edge tone for the UI component boundary. */
export function resolveEdgeTone(data: LinkEdgeData | undefined): {
  color: string;
  width: number;
  labelClassName: string;
  haloColor: string;
  semanticState: EdgeSemanticState;
} {
  const {
    inertVirtualLink,
    alertStatus,
    sourceDeviceStatus,
    targetDeviceStatus,
    sourceDeviceAlertStatus,
    targetDeviceAlertStatus,
    sourceDeviceRuntime,
    targetDeviceRuntime,
    sourceIfStatus,
    targetIfStatus,
    utilization,
    speedMismatch,
  } = normalizeLinkStateForColor(data);

  const srcDevDown = sourceDeviceStatus === 'down';
  const tgtDevDown = targetDeviceStatus === 'down';
  const srcDevProbing = sourceDeviceStatus === 'probing';
  const tgtDevProbing = targetDeviceStatus === 'probing';
  const srcDevInactive = srcDevDown || srcDevProbing;
  const tgtDevInactive = tgtDevDown || tgtDevProbing;
  const sourceDeviceAlertDown = sourceDeviceAlertStatus === 'down';
  const targetDeviceAlertDown = targetDeviceAlertStatus === 'down';
  const sourceDeviceAlertWarn = sourceDeviceAlertStatus === 'degraded';
  const targetDeviceAlertWarn = targetDeviceAlertStatus === 'degraded';
  const deviceAlertWarning = sourceDeviceAlertWarn || targetDeviceAlertWarn;
  const endpointRuntimeCritical =
    isEndpointRuntimeCritical(sourceDeviceRuntime) ||
    isEndpointRuntimeCritical(targetDeviceRuntime);
  const endpointRuntimeWarning =
    isEndpointRuntimeWarning(sourceDeviceRuntime) || isEndpointRuntimeWarning(targetDeviceRuntime);
  const bothDevDown = srcDevDown && tgtDevDown;
  const oneDevDown = (srcDevDown || tgtDevDown) && !bothDevDown;
  const bothDevInactive = srcDevInactive && tgtDevInactive && !bothDevDown;
  const oneDevInactive = srcDevInactive !== tgtDevInactive;
  const inertDeviceDown = inertVirtualLink && (srcDevDown || tgtDevDown);
  const inertDeviceWarning = inertVirtualLink && (srcDevProbing || tgtDevProbing);

  const sourceIfKnown = sourceIfStatus != null;
  const targetIfKnown = targetIfStatus != null;
  const sourceUp = sourceIfStatus === 'up' || !sourceIfKnown;
  const targetUp = targetIfStatus === 'up' || !targetIfKnown;
  const singleKnownIf = sourceIfKnown !== targetIfKnown;
  const singleKnownIfUp = sourceIfKnown ? sourceUp : targetIfKnown ? targetUp : false;
  const singleKnownIfDown = singleKnownIf && !singleKnownIfUp;
  const oneIfDown =
    (sourceIfKnown || targetIfKnown) && ((sourceUp && !targetUp) || (!sourceUp && targetUp));
  const bothIfDown = sourceIfKnown && targetIfKnown && !sourceUp && !targetUp;
  const inertUtilDown =
    inertVirtualLink && utilization !== null && utilization > INERT_VIRTUAL_UTIL_CRITICAL_THRESHOLD;
  const inertUtilWarn =
    inertVirtualLink &&
    utilization !== null &&
    utilization >= INERT_VIRTUAL_UTIL_WARNING_THRESHOLD &&
    utilization <= INERT_VIRTUAL_UTIL_CRITICAL_THRESHOLD;
  const devicesHealthy =
    (!sourceDeviceStatus || sourceDeviceStatus === 'up') &&
    (!targetDeviceStatus || targetDeviceStatus === 'up');
  const hasOperationalTelemetry =
    sourceIfKnown || targetIfKnown || sourceDeviceStatus === 'up' || targetDeviceStatus === 'up';
  const healthyPhysicalLink =
    !inertVirtualLink &&
    hasOperationalTelemetry &&
    devicesHealthy &&
    sourceUp &&
    targetUp &&
    !speedMismatch;
  const healthyInertVirtualLink = inertVirtualLink && singleKnownIfUp;

  if (
    alertStatus === 'down' ||
    sourceDeviceAlertDown ||
    targetDeviceAlertDown ||
    endpointRuntimeCritical ||
    inertDeviceDown ||
    bothDevDown ||
    bothIfDown ||
    inertUtilDown ||
    (inertVirtualLink && singleKnownIfDown)
  ) {
    return {
      color: 'var(--color-edge-critical)',
      width: 10.7,
      haloColor: 'var(--color-edge-critical)',
      labelClassName: 'border-status-down/35 text-status-down',
      semanticState: 'critical',
    };
  }

  if (
    speedMismatch ||
    alertStatus === 'degraded' ||
    deviceAlertWarning ||
    endpointRuntimeWarning ||
    inertDeviceWarning ||
    oneDevDown ||
    bothDevInactive ||
    oneDevInactive ||
    oneIfDown ||
    inertUtilWarn
  ) {
    return {
      color: 'var(--color-edge-warning)',
      width: 10.35,
      haloColor: 'var(--color-edge-warning)',
      labelClassName: 'border-warning/35 text-warning',
      semanticState: 'warning',
    };
  }

  if (healthyPhysicalLink || healthyInertVirtualLink) {
    return {
      color: 'var(--color-status-up)',
      width: 10.05,
      haloColor: data?.areaColor ?? 'var(--color-edge-active)',
      labelClassName: 'border-status-up/35 text-status-up',
      semanticState: 'up',
    };
  }

  if (inertVirtualLink && utilization !== null) {
    return {
      color: 'var(--color-status-up)',
      width: 10.05,
      haloColor: data?.areaColor ?? 'var(--color-edge-active)',
      labelClassName: 'border-status-up/35 text-status-up',
      semanticState: 'up',
    };
  }

  return {
    color: 'var(--color-edge-default)',
    width: 9.8,
    haloColor: data?.areaColor ?? 'var(--color-edge-active)',
    labelClassName: 'border-outline text-on-bg-secondary',
    semanticState: 'neutral',
  };
}

/** Resolves inline badge tone for the UI component boundary. */
export function resolveInlineBadgeTone(
  edgeState: EdgeSemanticState,
  badgeKind: LinkBadgeKind,
  data: LinkEdgeData | undefined,
): EdgeSemanticState {
  if (edgeState === 'critical' || edgeState === 'warning') {
    return edgeState;
  }

  if (badgeKind === 'throughput') {
    return edgeState === 'up' ? 'up' : 'neutral';
  }

  if (badgeKind !== 'rate') {
    return 'neutral';
  }

  switch (data?.negotiationState) {
    case 'matched':
      return 'up';
    case 'not_applicable':
      return edgeState === 'up' ? 'up' : 'neutral';
    case 'mismatch':
    case 'partial':
    case 'unknown':
      return 'warning';
    default:
      return 'neutral';
  }
}

/** Resolves badge class name for the UI component boundary. */
export function resolveBadgeClassName(tone: EdgeSemanticState): string {
  switch (tone) {
    case 'up':
      return 'border-status-up/35 text-status-up';
    case 'warning':
      return 'border-warning/35 text-warning';
    case 'critical':
      return 'border-status-down/35 text-status-down';
    default:
      return 'border-outline text-on-bg-secondary';
  }
}

/** Resolves negotiation indicator class name for the UI component boundary. */
export function resolveNegotiationIndicatorClassName(): string {
  return 'border-warning/45 bg-warning/12 text-warning';
}

/** Resolves link badge visibility for the UI component boundary. */
export function resolveLinkBadgeVisibility({
  zoom,
  bandwidthLabel,
  throughputLabel,
}: ResolveLinkBadgeVisibilityOptions): LinkBadgeVisibility {
  const zoomBand: LinkBadgeZoomBand =
    zoom >= LINK_BADGE_ZOOM_THRESHOLDS.high
      ? 'high'
      : zoom >= LINK_BADGE_ZOOM_THRESHOLDS.medium
        ? 'medium'
        : 'low';

  const showRate = Boolean(bandwidthLabel);
  const showThroughput = Boolean(throughputLabel);

  return {
    zoomBand,
    showRate,
    showThroughput,
  };
}

/** Resolves link badge scale for the UI component boundary. */
export function resolveLinkBadgeScale(zoom: number): number {
  const safeZoom = Number.isFinite(zoom) && zoom > 0 ? zoom : 1;

  if (safeZoom >= 1) {
    return 1;
  }

  const zoomRange = LINK_BADGE_SCALE_START_ZOOM - LINK_BADGE_MIN_SCALE_ZOOM;
  const scaleRange = LINK_BADGE_MAX_SCALE - 1;
  const progress = Math.min(1, Math.max(0, (LINK_BADGE_SCALE_START_ZOOM - safeZoom) / zoomRange));
  const scale = 1 + progress * scaleRange;
  return Number(scale.toFixed(2));
}

function buildStackedLinkBadgeItems(
  data: LinkEdgeData | undefined,
  visibility: LinkBadgeVisibility,
  edgeTone: ReturnType<typeof resolveEdgeTone>,
): LinkBadgeViewModel[] {
  const negotiationTitle = data?.negotiationTitle ?? data?.autonegTitle;
  const badgeByKind: Partial<Record<LinkBadgeKind, LinkBadgeViewModel>> = {};

  if (visibility.showRate && data?.bandwidthLabel) {
    badgeByKind.rate = {
      key: 'rate',
      text: data.bandwidthLabel,
      title: negotiationTitle,
      className: resolveBadgeClassName(
        resolveInlineBadgeTone(edgeTone.semanticState, 'rate', data),
      ),
      warningIndicator: isNegotiationWarning(data?.negotiationState)
        ? {
            text: '!',
            title: negotiationTitle,
            className: resolveNegotiationIndicatorClassName(),
          }
        : undefined,
    };
  }

  if (visibility.showThroughput && data?.throughputLabel) {
    badgeByKind.throughput = {
      key: 'throughput',
      text: data.throughputLabel,
      className: resolveBadgeClassName(
        resolveInlineBadgeTone(edgeTone.semanticState, 'throughput', data),
      ),
    };
  }

  return LINK_BADGE_STACK_ORDER.flatMap((kind) => {
    const badge = badgeByKind[kind];
    return badge ? [badge] : [];
  });
}

function resolveLinkBadgeSemanticPriority({
  semanticState,
  isActive,
  isConnected,
}: {
  semanticState: EdgeSemanticState;
  isActive: boolean;
  isConnected: boolean;
}): LinkBadgeSemanticPriority {
  if (semanticState === 'warning' || semanticState === 'critical') {
    return 'alert';
  }

  if (isActive || isConnected) {
    return 'active';
  }

  return 'normal';
}

/** Resolves link badge presentation for the UI component boundary. */
export function resolveLinkBadgePresentation({
  data,
  zoom,
  path,
  fallbackX,
  fallbackY,
  edgeTone,
  parallelIndex,
  isActive,
  isConnected,
  isMuted,
}: ResolveLinkBadgePresentationOptions): LinkBadgePresentation {
  const pathLength = measureEdgePathLength(path);
  const visibility = resolveLinkBadgeVisibility({
    zoom,
    pathLength,
    bandwidthLabel: data?.bandwidthLabel,
    throughputLabel: data?.throughputLabel,
  });
  const items = buildStackedLinkBadgeItems(data, visibility, edgeTone);

  return {
    anchor: computeLinkBadgeAnchor({
      path,
      fallbackX,
      fallbackY,
      parallelIndex,
    }),
    items,
    opacity: isMuted ? 0.5 : isConnected ? 1 : isActive ? 0.96 : 0.9,
    scale: resolveLinkBadgeScale(zoom),
    visibility,
    semanticState: edgeTone.semanticState,
    semanticPriority: resolveLinkBadgeSemanticPriority({
      semanticState: edgeTone.semanticState,
      isActive,
      isConnected,
    }),
  };
}
