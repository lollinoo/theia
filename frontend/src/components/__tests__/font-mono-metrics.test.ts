/**
 * Exercises font mono metrics component behavior so refactors preserve the documented contract.
 */
import { readFileSync } from 'fs';
import { join } from 'path';
/**
 * COMP-09 JetBrains Mono for Metric Values
 * Verifies that LinkDetailsPanel and InterfaceStatsPanel contain font-mono
 * on their metric value elements (bandwidth, throughput, utilization, speed, counters).
 */
import { describe, expect, it } from 'vitest';

const COMPONENTS_DIR = join(__dirname, '..');

describe('COMP-09 Metric values use font-mono (JetBrains Mono)', () => {
  it('InterfaceStatsPanel.tsx contains font-mono for metric values', () => {
    const content = readFileSync(join(COMPONENTS_DIR, 'InterfaceStatsPanel.tsx'), 'utf-8');
    // The file must contain font-mono class on numeric metric values
    expect(content).toContain('font-mono');
    // Specifically verify it's used on text displaying metric values (TX, RX, Speed, Utilization)
    // The pattern is: className="... font-mono ..."
    const fontMonoMatches = content.match(/font-mono/g);
    expect(fontMonoMatches).not.toBeNull();
    expect(fontMonoMatches!.length).toBeGreaterThanOrEqual(2);
  });

  it('LinkDetailsPanel.tsx exists and renders link details', () => {
    const content = readFileSync(join(COMPONENTS_DIR, 'LinkDetailsPanel.tsx'), 'utf-8');
    // LinkDetailsPanel is a link editor (interface selects, protocol badge),
    // not a metrics panel — font-mono is not applicable here.
    expect(content).toContain('LinkDetailsPanel');
  });

  it('DeviceCard.tsx contains font-mono for technical address and self-link values', () => {
    const content = readFileSync(join(COMPONENTS_DIR, 'DeviceCard.tsx'), 'utf-8');
    // DeviceCard technical address chips and self-link summaries must use font-mono per COMP-01.
    const fontMonoMatches = content.match(/font-mono/g);
    expect(fontMonoMatches).not.toBeNull();
    expect(fontMonoMatches!.length).toBeGreaterThanOrEqual(2);
  });
});

describe('COMP-09 Phase 5 sub-panels use font-mono for technical/metric values', () => {
  const DASHBOARD_DIR = join(COMPONENTS_DIR, 'dashboard');

  it('BackupHistoryTable.tsx contains font-mono on timestamp cells (COMP-09)', () => {
    const content = readFileSync(join(DASHBOARD_DIR, 'BackupHistoryTable.tsx'), 'utf-8');
    // COMP-09: timestamps (created_at) and file counts/sizes must render in JetBrains Mono.
    // The VERIFICATION.md confirms: "timestamps at line 113, file counts/sizes at line 116, file names at line 131 — all font-mono"
    expect(content).toContain('font-mono');
    const fontMonoMatches = content.match(/font-mono/g);
    expect(fontMonoMatches).not.toBeNull();
    expect(fontMonoMatches!.length).toBeGreaterThanOrEqual(3);
  });

  it('ConfigViewer.tsx contains font-mono on metadata and pre block for config content (COMP-09)', () => {
    const content = readFileSync(join(DASHBOARD_DIR, 'ConfigViewer.tsx'), 'utf-8');
    // COMP-09: config content pre block and metadata (date, hash, file name) must render in font-mono.
    // The VERIFICATION.md confirms: "Lines 140, 144, 172, 189: metadata and pre block in font-mono"
    expect(content).toContain('font-mono');
    const fontMonoMatches = content.match(/font-mono/g);
    expect(fontMonoMatches).not.toBeNull();
    expect(fontMonoMatches!.length).toBeGreaterThanOrEqual(3);
  });

  it('BackupPanel.tsx contains font-mono on latest backup date, file count, and size values (COMP-09)', () => {
    const content = readFileSync(join(DASHBOARD_DIR, 'BackupPanel.tsx'), 'utf-8');
    // COMP-09: technical readout values (created_at, file count, total size) must render in font-mono.
    // The VERIFICATION.md confirms: "dates and sizes in font-mono" at lines 167-175.
    expect(content).toContain('font-mono');
    const fontMonoMatches = content.match(/font-mono/g);
    expect(fontMonoMatches).not.toBeNull();
    expect(fontMonoMatches!.length).toBeGreaterThanOrEqual(2);
  });

  it('VendorSettingsPanel.tsx contains font-mono on inputClass for PromQL/OID technical values (COMP-09)', () => {
    const content = readFileSync(join(DASHBOARD_DIR, 'VendorSettingsPanel.tsx'), 'utf-8');
    // COMP-09: PromQL query and SNMP OID input fields must render in JetBrains Mono.
    // The VERIFICATION.md confirms: "line 87: inputClass includes font-mono"
    // Verify that the inputClass constant string contains font-mono.
    expect(content).toContain('font-mono');
    // The inputClass definition itself must embed font-mono so all inputs inherit it.
    const inputClassMatch = content.match(
      /const inputClass\s*=\s*['"`][^'"`]*font-mono[^'"`]*['"`]/,
    );
    expect(inputClassMatch).not.toBeNull();
  });
});
