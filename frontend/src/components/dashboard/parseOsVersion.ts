/**
 * Defines parse os version behavior for the operations dashboard.
 * Keeps table, backup, and device-management responsibilities isolated by module.
 */
const OS_VERSION_REGEX =
  /\b(RouterOS|Version|IOS(?:-XE)?|JunOS|EOS)\b(?:\s+\S+)*?\s+(\d+(?:\.\d+)+\S*(?:\s*\([^)]+\))?)/i;
const TRAILING_DOTTED_VERSION_REGEX = /\b(\d+(?:\.\d+){2,}\S*(?:\s*\([^)]+\))?)\s*$/i;

/** Parses OS version for the operations dashboard. */
export function parseOsVersion(sysDescr: string): string {
  if (!sysDescr) return '';

  const match = sysDescr.match(OS_VERSION_REGEX);
  if (match) {
    return `${match[1]} ${match[2]}`;
  }

  const trailingVersionMatch = sysDescr.match(TRAILING_DOTTED_VERSION_REGEX);
  return trailingVersionMatch ? trailingVersionMatch[1] : '';
}

/** Resolves OS version for the operations dashboard. */
export function resolveOsVersion(osVersion: string | undefined, sysDescr: string): string {
  const normalized = osVersion?.trim();
  return normalized || parseOsVersion(sysDescr);
}
