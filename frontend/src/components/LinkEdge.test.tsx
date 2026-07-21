/**
 * Exercises link edge component behavior so refactors preserve the documented contract.
 */
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
const LINK_LABEL_LAYER_PATH = join(__dirname, 'LinkLabelLayer.tsx');
const LINK_SEMANTICS_PATH = join(__dirname, 'linkSemantics.ts');

function sourceTemplateVariable(name: string): string {
  return ['$', '{', name, '}'].join('');
}

describe('LinkEdge', () => {
  it('treats unknown interface status as neutral for link coloring', () => {
    expect(normalizeInterfaceStatusForLink('unknown')).toBeUndefined();
    expect(normalizeInterfaceStatusForLink(' UNKNOWN ')).toBeUndefined();
    expect(normalizeInterfaceStatusForLink(undefined)).toBeUndefined();
    expect(normalizeInterfaceStatusForLink('up')).toBe('up');
    expect(normalizeInterfaceStatusForLink('down')).toBe('down');
  });

  it('preserves physical endpoint status and interface telemetry for inert virtual links', () => {
    expect(
      normalizeLinkStateForColor({
        inertVirtualLink: true,
        sourceIsVirtual: false,
        targetIsVirtual: true,
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
      sourceDeviceStatus: 'down',
      targetDeviceStatus: undefined,
      sourceDeviceAlertStatus: undefined,
      targetDeviceAlertStatus: undefined,
      sourceDeviceRuntime: {},
      targetDeviceRuntime: {},
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
    const content = readFileSync(LINK_LABEL_LAYER_PATH, 'utf-8');
    expect(content).toContain('bg-surface-container-high');
    expect(content).not.toContain('bg-bg/95');
  });

  it('keeps telemetry label pills on a solid tokenized surface without repeated shadows', () => {
    const content = readFileSync(LINK_LABEL_LAYER_PATH, 'utf-8');
    expect(content).toContain('bg-surface-container-high');
    expect(content).toContain('topology-link-badge');
    expect(content).not.toContain('shadow-pill');
    expect(content).toContain(
      `data-testid={\`${sourceTemplateVariable('edgeId')}-badge-${sourceTemplateVariable('badge.key')}\`}`,
    );
  });

  it('delegates telemetry label DOM to the centralized link label layer', () => {
    const edgeContent = readFileSync(LINK_EDGE_PATH, 'utf-8');
    const labelLayerContent = readFileSync(LINK_LABEL_LAYER_PATH, 'utf-8');

    expect(edgeContent).not.toContain('EdgeLabelRenderer');
    expect(edgeContent).toContain('registerLinkLabel');
    expect(labelLayerContent).toContain('EdgeLabelRenderer');
    expect(labelLayerContent).toContain(
      `data-testid={\`${sourceTemplateVariable('label.edgeId')}-badge-stack\`}`,
    );
  });

  it('animates label pills with opacity and transform transitions', () => {
    const content = readFileSync(LINK_LABEL_LAYER_PATH, 'utf-8');
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

  it('subscribes only to its two endpoint nodes for floating geometry', () => {
    const content = readFileSync(LINK_EDGE_PATH, 'utf-8');

    expect(content).toContain('useInternalNode<DeviceNode>(source)');
    expect(content).toContain('useInternalNode<DeviceNode>(target)');
    expect(content).toContain('buildEditableLinkPath({');
    expect(content).not.toContain('useNodes(');
    expect(content).not.toContain('getNodes(');
  });

  it('uses the stable React Flow coordinate API without a viewport-store subscription', () => {
    const content = readFileSync(LINK_EDGE_PATH, 'utf-8');

    expect(content).toContain('useReactFlow');
    expect(content).toContain('screenToFlowPosition');
    expect(content).not.toContain('useStore');
  });

  it('keeps route editing inputs in the custom edge memo boundary', () => {
    const content = readFileSync(LINK_EDGE_PATH, 'utf-8');

    expect(content).toContain('prev.data?.route === next.data?.route');
    expect(content).toContain('prev.data?.routeEditable === next.data?.routeEditable');
    expect(content).toContain('prev.data?.onRouteCommit === next.data?.onRouteCommit');
  });

  it('keeps the main stroke bound to semantic tone and only uses halo color for emphasis', () => {
    const content = readFileSync(LINK_EDGE_PATH, 'utf-8');
    expect(content).toContain('stroke: tone.color');
    expect(content).toContain('stroke: haloColor');
  });

  it('includes endpoint runtime health fields in the memo comparator', () => {
    const content = readFileSync(LINK_EDGE_PATH, 'utf-8');
    expect(content).toContain('prev.data?.sourceDeviceHealth === next.data?.sourceDeviceHealth');
    expect(content).toContain(
      'prev.data?.sourceDevicePrimaryHealth === next.data?.sourceDevicePrimaryHealth',
    );
    expect(content).toContain(
      'prev.data?.sourceDeviceReachability === next.data?.sourceDeviceReachability',
    );
    expect(content).toContain(
      'prev.data?.sourceDeviceNetworkReachable === next.data?.sourceDeviceNetworkReachable',
    );
    expect(content).toContain(
      'prev.data?.sourceDeviceSnmpReachable === next.data?.sourceDeviceSnmpReachable',
    );
    expect(content).toContain('prev.data?.targetDeviceHealth === next.data?.targetDeviceHealth');
    expect(content).toContain(
      'prev.data?.targetDevicePrimaryHealth === next.data?.targetDevicePrimaryHealth',
    );
    expect(content).toContain(
      'prev.data?.targetDeviceReachability === next.data?.targetDeviceReachability',
    );
    expect(content).toContain(
      'prev.data?.targetDeviceNetworkReachable === next.data?.targetDeviceNetworkReachable',
    );
    expect(content).toContain(
      'prev.data?.targetDeviceSnmpReachable === next.data?.targetDeviceSnmpReachable',
    );
  });

  it('keeps parallel lane changes in the memo comparator', () => {
    const content = readFileSync(LINK_EDGE_PATH, 'utf-8');
    expect(content).toContain('prev.data?.parallelIndex === next.data?.parallelIndex');
  });

  it('memoizes lane orientation from stable endpoint ids', () => {
    const content = readFileSync(LINK_EDGE_PATH, 'utf-8');

    expect(content).toContain('const laneOrientation = source <= target ? 1 : -1;');
    expect(content).toMatch(/parallelIndex: index,\s+laneOrientation,/);
    expect(content).toMatch(/\[\s+index,\s+laneOrientation,/);
    expect(content).toContain('prev.source === next.source');
    expect(content).toContain('prev.target === next.target');
  });

  it('renders a stacked negotiated-rate and throughput group without a standalone AUTO pill', () => {
    const edgeContent = readFileSync(LINK_EDGE_PATH, 'utf-8');
    const labelLayerContent = readFileSync(LINK_LABEL_LAYER_PATH, 'utf-8');
    expect(edgeContent).toContain('data?.bandwidthLabel');
    expect(edgeContent).toContain('data?.throughputLabel');
    expect(edgeContent).not.toContain('data?.autonegLabel');
    expect(labelLayerContent).toContain('presentation.items.map((badge) =>');
    expect(labelLayerContent).toContain('badge.warningIndicator');
  });
});
