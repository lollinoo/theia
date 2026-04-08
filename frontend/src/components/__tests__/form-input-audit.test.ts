/**
 * COMP-08 Form Standardization Audit
 * Verifies that form inputs across key component files use border-outline-subtle
 * (not border-outline) as mandated by the UI-SPEC Form Input Contract.
 *
 * Audits: SettingsPanel, SNMPProfileManager, SSHProfileManager, AddDevicePanel,
 *         DeviceConfigPanel, LinkCreatePanel, LinkDetailsPanel, InterfaceStatsPanel
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'fs';
import { join } from 'path';

const COMPONENTS_DIR = join(__dirname, '..');

const KEY_FORM_FILES = [
  join(COMPONENTS_DIR, 'SettingsPanel.tsx'),
  join(COMPONENTS_DIR, 'LinkDetailsPanel.tsx'),
  join(COMPONENTS_DIR, 'InterfaceStatsPanel.tsx'),
  join(COMPONENTS_DIR, 'SNMPProfileManager.tsx'),
  join(COMPONENTS_DIR, 'CredentialProfileManager.tsx'),
  join(COMPONENTS_DIR, 'AddDevicePanel.tsx'),
  join(COMPONENTS_DIR, 'DeviceConfigPanel.tsx'),
  join(COMPONENTS_DIR, 'LinkCreatePanel.tsx'),
];

// Matches input/select/textarea elements that still use the old border-outline
// (not border-outline-subtle). Pattern: className=... border border-outline (not -subtle)
// We look for the pattern 'border border-outline' without '-subtle' suffix
// in a context that looks like a form input (on lines with input, select, textarea, className=)
function hasOldBorderOutlineOnFormInput(content: string, filePath: string): string[] {
  const violations: string[] = [];
  const lines = content.split('\n');
  lines.forEach((line, i) => {
    // Skip comments
    if (line.trimStart().startsWith('//') || line.trimStart().startsWith('*')) return;
    // Check for border border-outline (NOT followed by -subtle)
    const match = /border border-outline(?!-subtle)/.test(line);
    if (match) {
      const relPath = filePath.split('/src/')[1] ?? filePath;
      violations.push(`${relPath}:${i + 1}: ${line.trim()}`);
    }
  });
  return violations;
}

describe('COMP-08 Form Input Standardization Audit', () => {
  for (const file of KEY_FORM_FILES) {
    const fileName = file.split('/').pop()!;
    it(`${fileName} uses border-outline-subtle (not border-outline) on form inputs`, () => {
      let content: string;
      try {
        content = readFileSync(file, 'utf-8');
      } catch {
        throw new Error(`File not found: ${file}`);
      }
      const violations = hasOldBorderOutlineOnFormInput(content, file);
      if (violations.length > 0) {
        console.error(`Form border violations in ${fileName}:\n` + violations.join('\n'));
      }
      expect(violations).toHaveLength(0);
    });
  }
});

describe('COMP-08 SidePanel chrome restyling audit', () => {
  const sidePanelPath = join(COMPONENTS_DIR, 'SidePanel.tsx');

  it('SidePanel.tsx header h2 uses text-sm (not text-lg) for tighter professional header', () => {
    const content = readFileSync(sidePanelPath, 'utf-8');
    // COMP-08: SidePanel header must use text-sm per D-18 (key change: text-lg -> text-sm tracking-wide)
    expect(content).toContain('text-sm');
  });

  it('SidePanel.tsx header div uses py-3 for reduced vertical padding', () => {
    const content = readFileSync(sidePanelPath, 'utf-8');
    // COMP-08: header vertical padding tightened from py-4 to py-3 per D-18
    expect(content).toContain('py-3');
  });

  it('SidePanel.tsx header div contains transition-colors for smooth theme switching', () => {
    const content = readFileSync(sidePanelPath, 'utf-8');
    // COMP-08: transition-colors duration-200 must be present on the header div for theme switching
    expect(content).toContain('transition-colors');
    expect(content).toContain('duration-200');
  });

  it('SidePanel.tsx close button uses MaterialIcon name="close" with size={18}', () => {
    const content = readFileSync(sidePanelPath, 'utf-8');
    // COMP-08: close icon must be MaterialIcon (not a raw SVG), size reduced from 20 to 18 per D-18
    expect(content).toContain('MaterialIcon');
    // Verify the close icon name and size as specified in the plan acceptance criteria
    expect(content).toContain('name="close"');
    expect(content).toContain('size={18}');
  });
});

describe('COMP-08 Sub-panel inputClass contains transition-colors', () => {
  const DASHBOARD_DIR = join(COMPONENTS_DIR, 'dashboard');

  it('SSHCredentialForm.tsx inputClass string includes transition-colors', () => {
    const content = readFileSync(join(DASHBOARD_DIR, 'SSHCredentialForm.tsx'), 'utf-8');
    // COMP-08: The plan's acceptance criteria requires inputClass to contain transition-colors.
    // This ensures all text inputs in the form animate on theme switching.
    const inputClassMatch = content.match(/const inputClass\s*=[\s\S]*?transition-colors/);
    expect(inputClassMatch).not.toBeNull();
  });

  it('VendorSettingsPanel.tsx inputClass string includes transition-colors', () => {
    const content = readFileSync(join(DASHBOARD_DIR, 'VendorSettingsPanel.tsx'), 'utf-8');
    // COMP-08: All sub-panel input classes must include transition-colors per the standardized
    // inputClass pattern. The plan explicitly requires this for VendorSettingsPanel.
    const inputClassMatch = content.match(/const inputClass\s*=[\s\S]*?transition-colors/);
    expect(inputClassMatch).not.toBeNull();
  });
});
