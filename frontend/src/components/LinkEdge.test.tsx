/**
 * COMP-10 LinkEdge label pill styling tests.
 * EdgeLabelRenderer uses ReactFlow portals that require a full ReactFlow canvas context
 * to render label pills. Since label pills can't be asserted via DOM in unit tests,
 * we verify the class presence in the source file — a structural smoke test
 * that confirms the implementation matches the requirement.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'fs';
import { join } from 'path';
import { normalizeInterfaceStatusForLink, normalizeLinkStateForColor } from './LinkEdge';

const LINK_EDGE_PATH = join(__dirname, 'LinkEdge.tsx');

describe('LinkEdge (COMP-10)', () => {
  it('treats unknown interface status as neutral for link coloring', () => {
    expect(normalizeInterfaceStatusForLink('unknown')).toBeUndefined();
    expect(normalizeInterfaceStatusForLink(' UNKNOWN ')).toBeUndefined();
    expect(normalizeInterfaceStatusForLink(undefined)).toBeUndefined();
    expect(normalizeInterfaceStatusForLink('up')).toBe('up');
    expect(normalizeInterfaceStatusForLink('down')).toBe('down');
  });

  it('ignores device status but preserves physical interface telemetry for inert virtual links', () => {
    expect(normalizeLinkStateForColor({
      inertVirtualLink: true,
      alertStatus: 'degraded',
      sourceDeviceStatus: 'down',
      targetDeviceStatus: 'probing',
      sourceIfStatus: 'down',
      targetIfStatus: 'up',
      utilization: 0.9,
    })).toEqual({
      inertVirtualLink: true,
      alertStatus: 'degraded',
      sourceDeviceStatus: undefined,
      targetDeviceStatus: undefined,
      sourceIfStatus: 'down',
      targetIfStatus: 'up',
      utilization: 0.9,
    });
  });

  it('label pills use bg-surface token (not bg-bg/95)', () => {
    const content = readFileSync(LINK_EDGE_PATH, 'utf-8');
    // Label pill containers should have bg-surface
    expect(content).toContain('bg-surface');
    // Old value bg-bg/95 must not be present
    expect(content).not.toContain('bg-bg/95');
  });

  it('label pills have transition-colors class', () => {
    const content = readFileSync(LINK_EDGE_PATH, 'utf-8');
    expect(content).toContain('transition-colors');
  });

  it('label pills use border-outline-subtle (ghost border, not layout separator)', () => {
    const content = readFileSync(LINK_EDGE_PATH, 'utf-8');
    // Label pills use outline-subtle not the old border-outline
    const pillSection = content.substring(
      content.indexOf('EdgeLabelRenderer'),
      content.lastIndexOf('EdgeLabelRenderer') + 500,
    );
    expect(pillSection).toContain('outline-subtle');
  });

  it('positions bandwidth label at edge midpoint and throughput label below', () => {
    const content = readFileSync(LINK_EDGE_PATH, 'utf-8');
    expect(content).toContain('labelY + labelOffsetY}px)');
    expect(content).toContain('labelY + labelOffsetY + 20}px)');
  });
});
