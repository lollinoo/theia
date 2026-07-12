/**
 * Exercises theme readability audit component behavior so refactors preserve the documented contract.
 */
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

const TASK7_SWEEP_FILES = [
  'components/SidePanel.tsx',
  'components/SettingsPanel.tsx',
  'components/DeviceConfigPanel.tsx',
  'components/LinkCreatePanel.tsx',
  'components/LinkDetailsPanel.tsx',
  'components/InterfaceStatsPanel.tsx',
  'components/dashboard/DeviceTable.tsx',
  'components/dashboard/DeviceRow.tsx',
  'components/dashboard/FilterSelect.tsx',
];

const DAILY_USE_FILES = [
  'components/NavigationPill.tsx',
  'components/DeviceCard.tsx',
  'components/SidePanel.tsx',
  'components/Dashboard.tsx',
  'components/InterfaceStatsPanel.tsx',
  'components/LinkLabelLayer.tsx',
];

const CLASS_START = '(?:^|[\\s"\'`])';
const CLASS_END = '(?=$|[\\s"\'`>])';
const VARIANT_PREFIX = '(?:[a-z0-9_-]+:)*';

const DISALLOWED_PATTERNS: { pattern: RegExp; reason: string }[] = [
  {
    pattern: new RegExp(
      `${CLASS_START}${VARIANT_PREFIX}text-on-bg-secondary/(?:[1-9]|[1-9][0-9])${CLASS_END}`,
    ),
    reason: 'semi-transparent secondary text is too weak in light mode',
  },
  {
    pattern: new RegExp(
      `${CLASS_START}${VARIANT_PREFIX}text-on-bg-muted/(?:[1-9]|[1-9][0-9])${CLASS_END}`,
    ),
    reason: 'semi-transparent muted text is too weak in light mode',
  },
  {
    pattern: new RegExp(
      `${CLASS_START}${VARIANT_PREFIX}text-yellow-(?:50|100|200|300|400|500|600|700|800|900|950)(?:/(?:[1-9]|[1-9][0-9]|100))?${CLASS_END}`,
    ),
    reason: 'fixed yellow palette bypasses theme tokens',
  },
  {
    pattern: new RegExp(
      `${CLASS_START}${VARIANT_PREFIX}bg-yellow-(?:50|100|200|300|400|500|600|700|800|900|950)(?:/(?:[1-9]|[1-9][0-9]|100))?${CLASS_END}`,
    ),
    reason: 'fixed yellow background bypasses theme tokens',
  },
  {
    pattern: new RegExp(
      `${CLASS_START}${VARIANT_PREFIX}border-yellow-(?:50|100|200|300|400|500|600|700|800|900|950)(?:/(?:[1-9]|[1-9][0-9]|100))?${CLASS_END}`,
    ),
    reason: 'fixed yellow border bypasses theme tokens',
  },
  {
    pattern: new RegExp(
      `${CLASS_START}${VARIANT_PREFIX}text-(?:red|green|blue)-(?:50|100|200|300|400|500|600|700|800|900|950)(?:/(?:[1-9]|[1-9][0-9]|100))?${CLASS_END}`,
    ),
    reason: 'fixed red/green/blue text palette bypasses theme tokens',
  },
  {
    pattern: new RegExp(
      `${CLASS_START}${VARIANT_PREFIX}bg-(?:red|green|blue)-(?:50|100|200|300|400|500|600|700|800|900|950)(?:/(?:[1-9]|[1-9][0-9]|100))?${CLASS_END}`,
    ),
    reason: 'fixed red/green/blue background palette bypasses theme tokens',
  },
  {
    pattern: new RegExp(
      `${CLASS_START}${VARIANT_PREFIX}border-(?:red|green|blue)-(?:50|100|200|300|400|500|600|700|800|900|950)(?:/(?:[1-9]|[1-9][0-9]|100))?${CLASS_END}`,
    ),
    reason: 'fixed red/green/blue border palette bypasses theme tokens',
  },
  {
    pattern: new RegExp(
      `${CLASS_START}${VARIANT_PREFIX}tracking-\\[(?:0\\.[2-9]\\d*|[1-9]\\d*(?:\\.\\d+)?)em\\]${CLASS_END}`,
    ),
    reason: 'wide tracking hurts compact operational labels',
  },
];

describe('enterprise NOC readability audit', () => {
  it('flags weak on-background text opacity variants without matching full-strength tokens', () => {
    const weakTextPatterns = DISALLOWED_PATTERNS.filter((rule) =>
      rule.reason.includes('semi-transparent'),
    ).map((rule) => rule.pattern);

    for (const className of [
      'text-on-bg-secondary/10',
      'text-on-bg-secondary/40',
      'text-on-bg-secondary/90',
      'hover:text-on-bg-secondary/50',
      'dark:hover:text-on-bg-secondary/50',
      'disabled:text-on-bg-muted/40',
      'text-on-bg-muted/20',
      'text-on-bg-muted/80',
    ]) {
      expect(weakTextPatterns.some((pattern) => pattern.test(className))).toBe(true);
    }

    for (const className of [
      'text-on-bg-secondary',
      'text-on-bg-muted',
      'hover:text-on-bg-secondary',
      'text-on-bg-secondaryity/50',
    ]) {
      expect(weakTextPatterns.some((pattern) => pattern.test(className))).toBe(false);
    }
  });

  it('flags fixed yellow Tailwind palette classes without blocking warning tokens', () => {
    const yellowPatterns = DISALLOWED_PATTERNS.filter((rule) =>
      rule.reason.includes('fixed yellow'),
    ).map((rule) => rule.pattern);

    for (const className of [
      'text-yellow-300',
      'text-yellow-600/80',
      'bg-yellow-300',
      'bg-yellow-500/10',
      'hover:bg-yellow-500/10',
      'dark:hover:bg-yellow-500/10',
      'border-yellow-600',
      'border-yellow-700/30',
      'focus:border-yellow-600',
    ]) {
      expect(yellowPatterns.some((pattern) => pattern.test(className))).toBe(true);
    }

    for (const className of ['text-warning', 'bg-warning/10', 'border-warning/30']) {
      expect(yellowPatterns.some((pattern) => pattern.test(className))).toBe(false);
    }
  });

  it('flags fixed red green and blue Tailwind palette classes without blocking semantic tokens', () => {
    const rgbPatterns = DISALLOWED_PATTERNS.filter((rule) =>
      rule.reason.includes('fixed red/green/blue'),
    ).map((rule) => rule.pattern);

    for (const className of [
      'text-red-300',
      'text-red-300/70',
      'text-green-400',
      'text-blue-400',
      'bg-red-500/8',
      'bg-green-500/5',
      'hover:bg-blue-500/10',
      'border-red-500/25',
      'border-green-500/20',
      'focus:border-blue-600',
    ]) {
      expect(rgbPatterns.some((pattern) => pattern.test(className))).toBe(true);
    }

    for (const className of [
      'text-status-down',
      'text-critical',
      'text-status-up',
      'text-primary',
      'text-warning',
      'bg-status-down/5',
      'bg-critical/10',
      'bg-status-up/5',
      'bg-primary/5',
      'bg-warning/10',
      'border-status-down/20',
      'border-critical/25',
      'border-status-up/20',
      'border-primary/20',
      'border-warning/30',
    ]) {
      expect(rgbPatterns.some((pattern) => pattern.test(className))).toBe(false);
    }
  });

  it('flags arbitrary tracking values at 0.20em and above', () => {
    const trackingPatterns = DISALLOWED_PATTERNS.filter((rule) =>
      rule.reason.includes('wide tracking'),
    ).map((rule) => rule.pattern);

    for (const className of [
      'tracking-[0.20em]',
      'tracking-[0.28em]',
      'tracking-[0.30em]',
      'sm:tracking-[0.30em]',
      'dark:hover:tracking-[0.30em]',
    ]) {
      expect(trackingPatterns.some((pattern) => pattern.test(className))).toBe(true);
    }

    for (const className of ['tracking-[0.14em]', 'tracking-[0.18em]', 'tracking-wide']) {
      expect(trackingPatterns.some((pattern) => pattern.test(className))).toBe(false);
    }
  });

  it('does not use pale or fixed-palette operational text in key UI files', () => {
    const violations: string[] = [];

    for (const file of [...OPERATIONAL_FILES, ...TASK7_SWEEP_FILES]) {
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

  it('keeps daily-use operational labels legible and compact', () => {
    const violations: string[] = [];
    const undersized = /text-\[(?:9|10)px\]/;
    const expandedTracking = /tracking-(?:\[[^\]]+\]|wide|wider|widest)/;

    for (const file of DAILY_USE_FILES) {
      const lines = readFileSync(join(SRC_DIR, file), 'utf-8').split('\n');

      lines.forEach((line, index) => {
        if (undersized.test(line) || expandedTracking.test(line)) {
          violations.push(`${file}:${index + 1}: ${line.trim()}`);
        }
      });
    }

    if (violations.length > 0) {
      console.error(violations.join('\n'));
    }
    expect(violations).toHaveLength(0);
  });
});
