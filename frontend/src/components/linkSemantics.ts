import type { CSSProperties, MouseEvent as ReactMouseEvent } from 'react';
import type { Link } from '../types/api';
import { type AlertStatus, type LinkMetricsDTO } from '../types/metrics';
import {
  type EdgeBadgeAnchor,
  computeLinkBadgeAnchor,
  measureEdgePathLength,
} from './edgeBadgeAnchors';

export type EdgeEmphasis = 'default' | 'muted' | 'connected';
export type EdgeSemanticState = 'neutral' | 'up' | 'warning' | 'critical';
export type LinkNegotiationState =
  | 'matched'
  | 'mismatch'
  | 'partial'
  | 'unknown'
  | 'not_applicable';
export type LinkBadgeKind = 'rate' | 'throughput';
export type LinkBadgeZoomBand = 'low' | 'medium' | 'high';

export interface LinkEdgeData {
  link?: Link;
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
  metrics?: LinkMetricsDTO | null;
  throughputLabel?: string;
  utilization?: number | null;
  alertStatus?: AlertStatus;
  sourceIfStatus?: string;
  targetIfStatus?: string;
  sourceDeviceStatus?: string;
  targetDeviceStatus?: string;
  areaColor?: string;
  emphasis?: EdgeEmphasis;
  [key: string]: unknown;
}

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

export interface LinkBadgeVisibility {
  zoomBand: LinkBadgeZoomBand;
  showRate: boolean;
  showThroughput: boolean;
}

export interface LinkBadgePresentation {
  anchor: EdgeBadgeAnchor;
  items: LinkBadgeViewModel[];
  opacity: number;
  visibility: LinkBadgeVisibility;
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
const LINK_BADGE_MIN_SCREEN_LENGTH = {
  low: 120,
  medium: 96,
} as const;
const INERT_VIRTUAL_UTIL_WARNING_THRESHOLD = 0.75;
const INERT_VIRTUAL_UTIL_CRITICAL_THRESHOLD = 0.8;

// Centralized zoom matrix for link telemetry badges.
// Low: keep the rate badge always, and keep TX/RX visible while the edge still spans 120px on screen.
// Medium: keep the full RATE+TX/RX stack once the edge spans 96px on screen.
// High: always keep the full RATE+TX/RX stack.
export const LINK_BADGE_ZOOM_THRESHOLDS = {
  medium: 0.92,
  high: 1.2,
} as const;

function formatSpeedBadge(speed: number): string {
  return speed > 0 ? `SPD ${formatBandwidth(speed)}` : 'SPD ?';
}

function isNegotiationWarning(state: LinkNegotiationState | undefined): boolean {
  return state === 'mismatch' || state === 'partial' || state === 'unknown';
}

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

export function normalizeInterfaceStatusForLink(status: string | undefined): string | undefined {
  const normalized = status?.trim().toLowerCase();
  if (!normalized || normalized === 'unknown') {
    return undefined;
  }
  return normalized;
}

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

export function normalizeLinkStateForColor(data: LinkEdgeData | undefined): NormalizedLinkState {
  const inertVirtualLink = data?.inertVirtualLink === true;

  return {
    inertVirtualLink,
    alertStatus: data?.alertStatus,
    sourceDeviceStatus: inertVirtualLink ? undefined : data?.sourceDeviceStatus,
    targetDeviceStatus: inertVirtualLink ? undefined : data?.targetDeviceStatus,
    sourceIfStatus: normalizeInterfaceStatusForLink(data?.sourceIfStatus),
    targetIfStatus: normalizeInterfaceStatusForLink(data?.targetIfStatus),
    utilization: data?.utilization ?? data?.metrics?.utilization ?? null,
    speedMismatch: data?.speedMismatch === true || data?.negotiationState === 'mismatch',
  };
}

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
  const bothDevDown = srcDevDown && tgtDevDown;
  const oneDevDown = (srcDevDown || tgtDevDown) && !bothDevDown;
  const bothDevInactive = srcDevInactive && tgtDevInactive && !bothDevDown;
  const oneDevInactive = srcDevInactive !== tgtDevInactive;

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
    bothDevDown ||
    bothIfDown ||
    inertUtilDown ||
    (inertVirtualLink && singleKnownIfDown)
  ) {
    return {
      color: 'var(--color-edge-critical)',
      width: 2.7,
      haloColor: 'var(--color-edge-critical)',
      labelClassName: 'border-status-down/35 text-status-down',
      semanticState: 'critical',
    };
  }

  if (
    speedMismatch ||
    alertStatus === 'degraded' ||
    oneDevDown ||
    bothDevInactive ||
    oneDevInactive ||
    oneIfDown ||
    inertUtilWarn
  ) {
    return {
      color: 'var(--color-edge-warning)',
      width: 2.35,
      haloColor: 'var(--color-edge-warning)',
      labelClassName: 'border-warning/35 text-warning',
      semanticState: 'warning',
    };
  }

  if (healthyPhysicalLink || healthyInertVirtualLink) {
    return {
      color: 'var(--color-status-up)',
      width: 2.05,
      haloColor: data?.areaColor ?? 'var(--color-edge-active)',
      labelClassName: 'border-status-up/35 text-status-up',
      semanticState: 'up',
    };
  }

  if (inertVirtualLink && utilization !== null) {
    return {
      color: 'var(--color-status-up)',
      width: 2.05,
      haloColor: data?.areaColor ?? 'var(--color-edge-active)',
      labelClassName: 'border-status-up/35 text-status-up',
      semanticState: 'up',
    };
  }

  return {
    color: 'var(--color-edge-default)',
    width: 1.8,
    haloColor: data?.areaColor ?? 'var(--color-edge-active)',
    labelClassName: 'border-outline text-on-bg-secondary',
    semanticState: 'neutral',
  };
}

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
    case 'mismatch':
    case 'partial':
    case 'unknown':
      return 'warning';
    default:
      return 'neutral';
  }
}

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

export function resolveNegotiationIndicatorClassName(): string {
  return 'border-warning/45 bg-warning/12 text-warning';
}

export function resolveLinkBadgeVisibility({
  zoom,
  pathLength,
  bandwidthLabel,
  throughputLabel,
}: ResolveLinkBadgeVisibilityOptions): LinkBadgeVisibility {
  const zoomBand: LinkBadgeZoomBand =
    zoom >= LINK_BADGE_ZOOM_THRESHOLDS.high
      ? 'high'
      : zoom >= LINK_BADGE_ZOOM_THRESHOLDS.medium
        ? 'medium'
        : 'low';

  const screenLength = pathLength > 0 ? pathLength * zoom : 0;
  const showRate = Boolean(bandwidthLabel);
  const showThroughput =
    Boolean(throughputLabel) &&
    (!showRate ||
      zoomBand === 'high' ||
      (zoomBand === 'medium' && screenLength >= LINK_BADGE_MIN_SCREEN_LENGTH.medium) ||
      (zoomBand === 'low' && screenLength >= LINK_BADGE_MIN_SCREEN_LENGTH.low));

  return {
    zoomBand,
    showRate,
    showThroughput,
  };
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
    visibility,
  };
}
