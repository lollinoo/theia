import { readFileSync } from 'fs';
import { join } from 'path';
import { describe, expect, it } from 'vitest';

const SRC_DIR = join(__dirname, '../../');

const OPERATIONAL_FILES = [
  'components/NavigationPill.tsx',
  'components/Toolbar.tsx',
  'components/DeviceCard.tsx',
  'components/LinkEdge.tsx',
  'components/AlertsPanel.tsx',
  'components/dashboard/ConfigViewer.tsx',
  'components/dashboard/BackupPanel.tsx',
  'components/dashboard/BulkBackupPanel.tsx',
];

const DISALLOWED_PATTERNS: { pattern: RegExp; reason: string }[] = [
  { pattern: /text-on-bg-secondary\/(?:50|60|70)/, reason: 'semi-transparent secondary text is too weak in light mode' },
  { pattern: /text-on-bg-muted\/(?:50|60|70)/, reason: 'semi-transparent muted text is too weak in light mode' },
  { pattern: /text-yellow-(?:300|400|500)/, reason: 'fixed yellow palette bypasses theme tokens' },
  { pattern: /bg-yellow-(?:400|500|900)/, reason: 'fixed yellow background bypasses theme tokens' },
  { pattern: /border-yellow-(?:200|400|500)/, reason: 'fixed yellow border bypasses theme tokens' },
  { pattern: /tracking-\[0\.2[0-9]em\]/, reason: 'wide tracking hurts compact operational labels' },
  { pattern: /tracking-\[0\.28em\]/, reason: 'wide tracking hurts compact operational labels' },
];

describe('enterprise NOC readability audit', () => {
  it('does not use pale or fixed-palette operational text in key UI files', () => {
    const violations: string[] = [];

    for (const file of OPERATIONAL_FILES) {
      const fullPath = join(SRC_DIR, file);
      const lines = readFileSync(fullPath, 'utf-8').split('\n');

      lines.forEach((line, index) => {
        for (const rule of DISALLOWED_PATTERNS) {
          if (rule.pattern.test(line)) {
            violations.push(`${file}:${index + 1}: ${rule.reason}: ${line.trim()}`);
          }
        }
      });
    }

    if (violations.length > 0) {
      console.error(violations.join('\n'));
    }
    expect(violations).toHaveLength(0);
  });
});
