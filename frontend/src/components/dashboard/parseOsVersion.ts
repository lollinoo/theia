const OS_VERSION_REGEX =
  /\b(RouterOS|Version|IOS(?:-XE)?|JunOS|EOS)\b(?:\s+\S+)*?\s+(\d+(?:\.\d+)+\S*(?:\s*\([^)]+\))?)/i;

export function parseOsVersion(sysDescr: string): string {
  if (!sysDescr) return '';

  const match = sysDescr.match(OS_VERSION_REGEX);
  return match ? `${match[1]} ${match[2]}` : '';
}
