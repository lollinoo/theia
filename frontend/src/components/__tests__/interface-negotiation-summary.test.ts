import { describe, it, expect } from 'vitest';
import { readFileSync } from 'fs';
import { join } from 'path';

const COMPONENT_PATH = join(__dirname, '..', 'InterfaceStatsPanel.tsx');

describe('InterfaceStatsPanel autonegotiation summary', () => {
  it('renders a dedicated autonegotiation summary card', () => {
    const content = readFileSync(COMPONENT_PATH, 'utf-8');
    expect(content).toContain('function NegotiationSummary');
    expect(content).toContain('Autonegotiation');
    expect(content).toContain('Both interfaces report the same negotiated speed.');
    expect(content).toContain('The two ends report different negotiated speeds.');
  });
});
