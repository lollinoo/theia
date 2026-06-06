/**
 * Exercises diagnostics hook lifecycle behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';

import { getRawWebSocketMessageType } from './diagnostics';

describe('websocket diagnostics helpers', () => {
  it('extracts a raw message type only when the payload shape supports it', () => {
    expect(getRawWebSocketMessageType({ type: 'runtime_delta' })).toBe('runtime_delta');
    expect(getRawWebSocketMessageType({ type: 123 })).toBeNull();
    expect(getRawWebSocketMessageType(null)).toBeNull();
    expect(getRawWebSocketMessageType('runtime_delta')).toBeNull();
  });
});
