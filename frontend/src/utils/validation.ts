// --- Constants ---

/** Maximum length for general string fields (mirrors Phase 20 backend limit). */
export const MAX_STRING_LENGTH = 255;

/** Allowed SNMP v3 authentication protocols (mirrors Phase 20 backend allowlist). */
export const SNMP_AUTH_PROTOCOLS = ['MD5', 'SHA', 'SHA-224', 'SHA-256', 'SHA-384', 'SHA-512'] as const;

/** Allowed SNMP v3 privacy protocols (mirrors Phase 20 backend allowlist). */
export const SNMP_PRIV_PROTOCOLS = ['DES', 'AES'] as const;

/** Allowed SNMP v3 security levels (mirrors Phase 20 backend allowlist). */
export const SNMP_SECURITY_LEVELS = ['noAuthNoPriv', 'authNoPriv', 'authPriv'] as const;

/** Allowed backup interval values in hours (mirrors Phase 20 backend allowlist). */
export const INTERVAL_ALLOWLIST = [0, 6, 12, 24, 48, 168] as const;

// --- Validators ---

/**
 * Validates that a field is not empty after trimming.
 * Returns an error message string or null if valid.
 */
export function validateRequired(value: string, fieldName: string): string | null {
  if (value.trim() === '') {
    return `${fieldName} is required`;
  }
  return null;
}

/**
 * Validates that a string does not exceed the maximum length.
 * Empty strings are not a length violation (use validateRequired separately).
 * Returns an error message string or null if valid.
 */
export function validateMaxLength(value: string, max: number, fieldName: string): string | null {
  if (value.length > max) {
    return `${fieldName} must be ${max} characters or fewer`;
  }
  return null;
}

/**
 * Validates that a value is a valid IP address (v4 or v6) or RFC 1123 hostname.
 * Returns an error message string or null if valid.
 */
export function validateIPOrHostname(value: string): string | null {
  if (value.trim() === '') {
    return 'IP address or hostname is required';
  }

  // Try IPv4: four octets each 0-255
  const ipv4Regex = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/;
  const ipv4Match = ipv4Regex.exec(value);
  if (ipv4Match) {
    const octets = [ipv4Match[1], ipv4Match[2], ipv4Match[3], ipv4Match[4]];
    const valid = octets.every((o) => {
      const n = parseInt(o, 10);
      return n >= 0 && n <= 255;
    });
    if (valid) return null;
  }

  // Try IPv6: contains ':' and only hex digits plus colons
  if (value.includes(':')) {
    const ipv6Regex = /^[0-9a-fA-F:]+$/;
    if (ipv6Regex.test(value)) {
      return null;
    }
  }

  // Try hostname: total length <= 253, each label starts/ends with alphanumeric,
  // may contain hyphens in the middle, and must contain at least one letter.
  // Purely numeric labels (e.g. "12345") are rejected — they are not valid hostnames.
  if (value.length > 253) {
    return 'Invalid IP address or hostname';
  }

  // Each label must start and end with alphanumeric, may contain hyphens in the middle,
  // and must contain at least one letter (rejects bare numbers like "12345").
  const labelRegex = /^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$/;
  const hasLetter = /[a-zA-Z]/;
  const labels = value.split('.');
  const hostnameValid = labels.every((label) => labelRegex.test(label) && hasLetter.test(label));
  if (hostnameValid) {
    return null;
  }

  return 'Invalid IP address or hostname';
}

/**
 * Validates that a value is a valid TCP port (1-65535).
 * Returns an error message string or null if valid.
 */
export function validatePort(value: string | number, fieldName: string): string | null {
  const strValue = String(value);
  if (strValue.trim() === '') {
    return `${fieldName} is required`;
  }

  const n = parseInt(strValue, 10);
  if (isNaN(n) || String(n) !== strValue.trim()) {
    return `${fieldName} must be a number`;
  }

  if (n < 1 || n > 65535) {
    return `${fieldName} must be between 1 and 65535`;
  }

  return null;
}

/**
 * Validates that a value is a valid http/https URL.
 * Empty values are treated as valid (URLs are optional fields).
 * Returns an error message string or null if valid.
 */
export function validateURL(value: string, _fieldName: string): string | null {
  if (value.trim() === '') {
    return null;
  }

  if (!value.startsWith('http://') && !value.startsWith('https://')) {
    return 'URL must start with http:// or https://';
  }

  try {
    const url = new URL(value);
    if (!url.hostname) {
      return 'URL must have a host';
    }
  } catch (err) {
    // new URL('http://') throws TypeError; distinguish "no host" from "malformed"
    const msg = err instanceof Error ? err.message : '';
    if (msg.toLowerCase().includes('host') || value === 'http://' || value === 'https://') {
      return 'URL must have a host';
    }
    return 'URL must start with http:// or https://';
  }

  return null;
}

/**
 * Validates that a value is an allowed SNMP v3 authentication protocol.
 * Returns an error message string or null if valid.
 */
export function validateSNMPv3Auth(protocol: string): string | null {
  const allowed: readonly string[] = SNMP_AUTH_PROTOCOLS;
  if (!allowed.includes(protocol)) {
    return `Auth protocol must be one of: ${SNMP_AUTH_PROTOCOLS.join(', ')}`;
  }
  return null;
}

/**
 * Validates that a value is an allowed SNMP v3 privacy protocol.
 * Returns an error message string or null if valid.
 */
export function validateSNMPv3Priv(protocol: string): string | null {
  const allowed: readonly string[] = SNMP_PRIV_PROTOCOLS;
  if (!allowed.includes(protocol)) {
    return `Privacy protocol must be one of: ${SNMP_PRIV_PROTOCOLS.join(', ')}`;
  }
  return null;
}

/**
 * Validates that a value is an allowed SNMP v3 security level.
 * Returns an error message string or null if valid.
 */
export function validateSNMPv3SecurityLevel(level: string): string | null {
  const allowed: readonly string[] = SNMP_SECURITY_LEVELS;
  if (!allowed.includes(level)) {
    return `Security level must be one of: ${SNMP_SECURITY_LEVELS.join(', ')}`;
  }
  return null;
}

/**
 * Validates that a coordinate value is a finite number (not NaN or Infinity).
 * Returns an error message string or null if valid.
 */
export function validateCoordinate(value: number, fieldName: string): string | null {
  if (!Number.isFinite(value)) {
    return `${fieldName} must be a valid number`;
  }
  return null;
}

/**
 * Validates that a value is in the allowed backup interval allowlist.
 * Returns an error message string or null if valid.
 */
export function validateIntervalAllowlist(value: string): string | null {
  const n = parseInt(value, 10);
  const allowed: readonly number[] = INTERVAL_ALLOWLIST;
  if (isNaN(n) || !allowed.includes(n)) {
    return `Interval must be one of: ${INTERVAL_ALLOWLIST.join(', ')}`;
  }
  return null;
}

/**
 * Validates that a retention count is a number between 1 and 50.
 * Returns an error message string or null if valid.
 */
export function validateRetentionCount(value: string): string | null {
  const n = parseInt(value, 10);
  if (isNaN(n)) {
    return 'Retention count must be a number';
  }
  if (n < 1 || n > 50) {
    return 'Retention count must be between 1 and 50';
  }
  return null;
}
