/**
 * Exercises no line audit component behavior so refactors preserve the documented contract.
 */
import { readdirSync, readFileSync, statSync } from 'fs';
import { join } from 'path';
/**
 * COMP-12 No-Line Rule Audit
 * Scans all component source files for layout border patterns that violate the no-line rule.
 * Layout border separators (border-t border-outline, border-b border-outline, border-l border-outline)
 * must be zero. Ghost/functional borders (border-outline-subtle, border-glass-border, ring-outline) are allowed.
 * The SidePanel slide-over edge is an explicit panel boundary, not an internal layout separator.
 */
import { describe, expect, it } from 'vitest';

const COMPONENTS_DIR = join(__dirname, '..');

function getAllTsxFiles(dir: string): string[] {
  const files: string[] = [];
  const entries = readdirSync(dir);
  for (const entry of entries) {
    const fullPath = join(dir, entry);
    const stat = statSync(fullPath);
    if (stat.isDirectory() && entry !== '__tests__' && entry !== 'node_modules') {
      files.push(...getAllTsxFiles(fullPath));
    } else if (entry.endsWith('.tsx') && !entry.endsWith('.test.tsx')) {
      files.push(fullPath);
    }
  }
  return files;
}

// Layout separator patterns that must be absent (the no-line rule violations)
// Matches: border-t border-outline, border-b border-outline, border-l border-outline
// but NOT border-outline-subtle (that's the allowed form input ghost border)
const LAYOUT_SEPARATOR_PATTERNS = [
  /border-t border-outline(?!-subtle)/,
  /border-b border-outline(?!-subtle)/,
  /border-l border-outline(?!-subtle)/,
];

// Exception: known retained functional/semantic borders that are allowed
// Spinner animations (border-t-primary on rotating elements) and prometheus status semantic borders
// are excluded from the no-line rule per SUMMARY files.
function isExceptionLine(line: string): boolean {
  // Spinner animation: border-t-primary (not border-t border-outline)
  if (line.includes('border-t-primary')) return true;
  // Semantic status borders: border-red-, border-yellow- (prometheus status cards)
  if (/border-(red|yellow)-/.test(line)) return true;
  // React handle borders that are layout-needed
  if (line.includes('!border-')) return true;
  // Explicit right-side slide-over panel edge
  if (line.includes('fixed right-0 top-0') && line.includes('border-l border-outline')) return true;
  return false;
}

describe('COMP-12 No-Line Rule Audit', () => {
  it('no layout border separators remain across all component files', () => {
    const files = getAllTsxFiles(COMPONENTS_DIR);
    const violations: string[] = [];

    for (const file of files) {
      const content = readFileSync(file, 'utf-8');
      const lines = content.split('\n');

      lines.forEach((line, lineIndex) => {
        if (isExceptionLine(line)) return;
        for (const pattern of LAYOUT_SEPARATOR_PATTERNS) {
          if (pattern.test(line)) {
            const relPath = file.replace(COMPONENTS_DIR + '/', '');
            violations.push(`${relPath}:${lineIndex + 1}: ${line.trim()}`);
          }
        }
      });
    }

    if (violations.length > 0) {
      console.error('Layout border separator violations found:\n' + violations.join('\n'));
    }
    expect(violations).toHaveLength(0);
  });
});
