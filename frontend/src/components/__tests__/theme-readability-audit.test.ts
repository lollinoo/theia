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

const CLASS_START = "(?:^|[\\s\"'`])";
const CLASS_END = "(?=$|[\\s\"'`>])";
const VARIANT_PREFIX = String.raw`(?:[a-z0-9_-]+:)*`;

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
