import { describe, expect, it } from 'vitest';
import {
  buildLinkTelemetryBadges,
  resolveEdgeTone,
  resolveInlineBadgeTone,
  resolveLinkBadgePresentation,
  resolveLinkBadgeVisibility,
} from './linkSemantics';

describe('linkSemantics', () => {
  it('derives stacked rate and speed badges for matched physical links', () => {
    expect(
      buildLinkTelemetryBadges({
        sourceSpeed: 1_000_000_000,
        targetSpeed: 1_000_000_000,
        isVirtualLink: false,
        sourceIsVirtual: false,
      }),
    ).toMatchObject({
      bandwidthLabel: '1 Gbps',
      speedLabel: 'SPD 1 Gbps',
      speedMismatch: false,
      negotiationState: 'matched',
    });
  });

  it('marks mismatched negotiated speed as warning data and keeps the speed badge readable', () => {
    expect(
      buildLinkTelemetryBadges({
        sourceSpeed: 1_000_000_000,
        targetSpeed: 100_000_000,
        isVirtualLink: false,
        sourceIsVirtual: false,
      }),
    ).toMatchObject({
      bandwidthLabel: '100 Mbps',
      speedLabel: 'SPD 1 Gbps',
      speedMismatch: true,
      negotiationState: 'mismatch',
    });
  });

  it('keeps a warning-capable rate badge visible when only one side exposes negotiated speed', () => {
    expect(
      buildLinkTelemetryBadges({
        sourceSpeed: 1_000_000_000,
        targetSpeed: 0,
        isVirtualLink: false,
        sourceIsVirtual: false,
      }),
    ).toMatchObject({
      bandwidthLabel: '1 Gbps',
      speedLabel: 'SPD 1 Gbps',
      speedMismatch: false,
      negotiationState: 'partial',
    });
  });

  it('falls back to the primary rate signal when both physical sides lack negotiated speed', () => {
    expect(
      buildLinkTelemetryBadges({
        sourceSpeed: 0,
        targetSpeed: 0,
        isVirtualLink: false,
        sourceIsVirtual: false,
      }),
    ).toMatchObject({
      bandwidthLabel: 'SPD ?',
      speedLabel: undefined,
      speedMismatch: false,
      negotiationState: 'unknown',
    });
  });

  it('suppresses negotiation warnings for virtual links without altering the backend payload model', () => {
    expect(
      buildLinkTelemetryBadges({
        sourceSpeed: 0,
        targetSpeed: 1_000_000_000,
        isVirtualLink: true,
        sourceIsVirtual: true,
      }),
    ).toMatchObject({
      bandwidthLabel: '1 Gbps',
      speedLabel: 'SPD 1 Gbps',
      speedMismatch: false,
      negotiationState: 'not_applicable',
    });
  });

  it('never resolves a speed mismatch to an up/green edge tone', () => {
    expect(
      resolveEdgeTone({
        sourceDeviceStatus: 'up',
        targetDeviceStatus: 'up',
        sourceIfStatus: 'up',
        targetIfStatus: 'up',
        speedMismatch: true,
        negotiationState: 'mismatch',
      }),
    ).toMatchObject({
      color: 'var(--color-edge-warning)',
      semanticState: 'warning',
    });
  });

  it('keeps inert virtual links green below the 75% utilization warning threshold', () => {
    expect(
      resolveEdgeTone({
        inertVirtualLink: true,
        sourceIfStatus: 'up',
        utilization: 0.74,
      }),
    ).toMatchObject({
      color: 'var(--color-status-up)',
      semanticState: 'up',
    });
  });

  it('turns inert virtual links warning once utilization reaches 75%', () => {
    expect(
      resolveEdgeTone({
        inertVirtualLink: true,
        utilization: 0.76,
      }),
    ).toMatchObject({
      color: 'var(--color-edge-warning)',
      semanticState: 'warning',
    });
  });

  it('turns inert virtual links critical above the high-utilization ceiling', () => {
    expect(
      resolveEdgeTone({
        inertVirtualLink: true,
        utilization: 0.81,
      }),
    ).toMatchObject({
      color: 'var(--color-edge-critical)',
      semanticState: 'critical',
    });
  });

  it('keeps inline badges aligned with warning and critical edge states', () => {
    expect(resolveInlineBadgeTone('warning', 'rate', { negotiationState: 'matched' })).toBe(
      'warning',
    );
    expect(
      resolveInlineBadgeTone('critical', 'throughput', { throughputLabel: 'TX: 500M / RX: 300M' }),
    ).toBe('critical');
    expect(
      resolveInlineBadgeTone('up', 'throughput', { throughputLabel: 'TX: 500M / RX: 300M' }),
    ).toBe('up');
  });

  it('uses a deterministic zoom visibility matrix for the stacked badge group', () => {
    expect(
      resolveLinkBadgeVisibility({
        zoom: 0.8,
        pathLength: 220,
        bandwidthLabel: '1 Gbps',
        throughputLabel: 'TX: 500M / RX: 300M',
      }),
    ).toMatchObject({
      zoomBand: 'low',
      showRate: true,
      showThroughput: true,
    });

    expect(
      resolveLinkBadgeVisibility({
        zoom: 0.6,
        pathLength: 160,
        bandwidthLabel: '1 Gbps',
        throughputLabel: 'TX: 500M / RX: 300M',
      }),
    ).toMatchObject({
      zoomBand: 'low',
      showRate: true,
      showThroughput: false,
    });

    expect(
      resolveLinkBadgeVisibility({
        zoom: 1,
        pathLength: 220,
        bandwidthLabel: '1 Gbps',
        throughputLabel: 'TX: 500M / RX: 300M',
      }),
    ).toMatchObject({
      zoomBand: 'medium',
      showRate: true,
      showThroughput: true,
    });
  });

  it('never materializes a third inline badge even when throughput telemetry exists', () => {
    const edgeTone = resolveEdgeTone({
      sourceDeviceStatus: 'up',
      targetDeviceStatus: 'up',
      sourceIfStatus: 'up',
      targetIfStatus: 'up',
      negotiationState: 'matched',
    });

    const presentation = resolveLinkBadgePresentation({
      data: {
        bandwidthLabel: '1 Gbps',
        speedLabel: 'SPD 1 Gbps',
        throughputLabel: 'TX: 500M / RX: 300M',
        negotiationState: 'matched',
        sourceDeviceStatus: 'up',
        targetDeviceStatus: 'up',
        sourceIfStatus: 'up',
        targetIfStatus: 'up',
      },
      zoom: 1.3,
      path: 'M0 0 C0 0 200 0 200 0',
      fallbackX: 100,
      fallbackY: 0,
      edgeTone,
      isActive: false,
      isConnected: false,
      isMuted: false,
    });

    expect(presentation.items.map((item) => item.key)).toEqual(['rate', 'throughput']);
  });
});
