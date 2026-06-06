/**
 * Exercises hello hook lifecycle behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';
import { buildCanvasHelloPayload } from './hello';

describe('buildCanvasHelloPayload', () => {
  it('includes runtime and alert versions when a runtime base is available', () => {
    expect(
      buildCanvasHelloPayload({
        topologyVersion: 'topology-7',
        hasRuntimeSnapshot: true,
        runtimeVersion: 42,
        runtimeIdentity: 'rt-sha256:abc',
        alertVersion: 9,
        detailDeviceId: 'device-1',
      }),
    ).toEqual({
      canvas_schema_version: 1,
      topology_version: 'topology-7',
      runtime_version: 42,
      runtime_identity: 'rt-sha256:abc',
      alert_version: 9,
      subscriptions: {
        runtime: true,
        topology: true,
        alerts: true,
        details_device_id: 'device-1',
      },
    });
  });

  it('leaves runtime identity and version undefined when no runtime base exists', () => {
    const payload = buildCanvasHelloPayload({
      topologyVersion: undefined,
      hasRuntimeSnapshot: false,
      runtimeVersion: 42,
      runtimeIdentity: 'rt-sha256:abc',
      alertVersion: null,
      detailDeviceId: null,
    });

    expect(payload).toEqual({
      canvas_schema_version: 1,
      topology_version: undefined,
      runtime_version: undefined,
      runtime_identity: undefined,
      alert_version: undefined,
      subscriptions: {
        runtime: true,
        topology: true,
        alerts: true,
        details_device_id: null,
      },
    });
  });
});
