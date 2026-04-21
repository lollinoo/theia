import { readFileSync } from 'fs';
import { join } from 'path';
/**
 * Structural smoke tests for LinkEdge.
 * The labels render through React Flow portals, so the styling assertions
 * read the source file directly.
 */
import { describe, expect, it } from 'vitest';
import {
  normalizeInterfaceStatusForLink,
  normalizeLinkStateForColor,
  resolveEdgeTone,
} from './linkSemantics';

const LINK_EDGE_PATH = join(__dirname, 'LinkEdge.tsx');
const LINK_SEMANTICS_PATH = join(__dirname, 'linkSemantics.ts');

describe('LinkEdge', () => {
  it('treats unknown interface status as neutral for link coloring', () => {
    expect(normalizeInterfaceStatusForLink('unknown')).toBeUndefined();
    expect(normalizeInterfaceStatusForLink(' UNKNOWN ')).toBeUndefined();
    expect(normalizeInterfaceStatusForLink(undefined)).toBeUndefined();
    expect(normalizeInterfaceStatusForLink('up')).toBe('up');
    expect(normalizeInterfaceStatusForLink('down')).toBe('down');
  });

  it('ignores device status but preserves physical interface telemetry for inert virtual links', () => {
    expect(
      normalizeLinkStateForColor({
        inertVirtualLink: true,
        alertStatus: 'degraded',
        sourceDeviceStatus: 'down',
        targetDeviceStatus: 'probing',
        sourceIfStatus: 'down',
        targetIfStatus: 'up',
        utilization: 0.9,
      }),
    ).toEqual({
      inertVirtualLink: true,
      alertStatus: 'degraded',
      sourceDeviceStatus: undefined,
      targetDeviceStatus: undefined,
      sourceIfStatus: 'down',
      targetIfStatus: 'up',
      utilization: 0.9,
      speedMismatch: false,
    });
  });

  it('renders healthy links with an operational up tone instead of a neutral base color', () => {
    expect(
      resolveEdgeTone({
        sourceDeviceStatus: 'up',
        targetDeviceStatus: 'up',
        sourceIfStatus: 'up',
        targetIfStatus: 'up',
      }),
    ).toMatchObject({
      color: 'var(--color-status-up)',
      semanticState: 'up',
    });
  });

  it('keeps warning and critical tones tied to link health semantics', () => {
    expect(
      resolveEdgeTone({
        sourceDeviceStatus: 'up',
        targetDeviceStatus: 'up',
        sourceIfStatus: 'down',
        targetIfStatus: 'up',
      }),
    ).toMatchObject({
      color: 'var(--color-edge-warning)',
      semanticState: 'warning',
    });

    expect(
      resolveEdgeTone({
        alertStatus: 'down',
        sourceDeviceStatus: 'up',
        targetDeviceStatus: 'up',
        sourceIfStatus: 'up',
        targetIfStatus: 'up',
      }),
    ).toMatchObject({
      color: 'var(--color-edge-critical)',
      semanticState: 'critical',
    });
  });

  it('uses the new high-contrast label surface token', () => {
    const content = readFileSync(LINK_EDGE_PATH, 'utf-8');
    expect(content).toContain('bg-surface-container-high');
    expect(content).not.toContain('bg-bg/95');
  });

  it('animates label pills with opacity and transform transitions', () => {
    const content = readFileSync(LINK_EDGE_PATH, 'utf-8');
    expect(content).toContain('transition-[opacity,transform]');
    expect(content).toContain('transition-[border-color,color]');
  });

  it('uses outline border tone for neutral label pills', () => {
    const content = readFileSync(LINK_SEMANTICS_PATH, 'utf-8');
    expect(content).toContain("return 'border-outline text-on-bg-secondary';");
  });

  it('uses computed path anchors instead of fixed throughput offsets', () => {
    const content = readFileSync(LINK_EDGE_PATH, 'utf-8');
    expect(content).toContain('const labelYOffset = labelY + labelOffsetY;');
    expect(content).toContain('resolveLinkBadgePresentation({');
    expect(content).not.toContain('y: labelYOffset + 20');
  });

  it('keeps the main stroke bound to semantic tone and only uses halo color for emphasis', () => {
    const content = readFileSync(LINK_EDGE_PATH, 'utf-8');
    expect(content).toContain('stroke: tone.color');
    expect(content).toContain('stroke: haloColor');
  });

  it('renders a stacked negotiated-rate and throughput group without a standalone AUTO pill', () => {
    const content = readFileSync(LINK_EDGE_PATH, 'utf-8');
    expect(content).toContain('data?.bandwidthLabel');
    expect(content).toContain('data?.throughputLabel');
    expect(content).not.toContain('data?.autonegLabel');
    expect(content).toContain('badgePresentation.items.map((badge) =>');
    expect(content).toContain('badge.warningIndicator');
  });
});
