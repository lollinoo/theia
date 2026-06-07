/**
 * Exercises validation utility behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';
import {
  INTERVAL_ALLOWLIST,
  MAX_STRING_LENGTH,
  SNMP_AUTH_PROTOCOLS,
  SNMP_PRIV_PROTOCOLS,
  SNMP_SECURITY_LEVELS,
  validateCoordinate,
  validateIntervalAllowlist,
  validateIPOrHostname,
  validateMaxLength,
  validatePort,
  validateRequired,
  validateRetentionCount,
  validateSNMPv3Auth,
  validateSNMPv3Priv,
  validateSNMPv3SecurityLevel,
  validateURL,
} from './validation';

// --- validateRequired ---

describe('validateRequired', () => {
  it('returns error for empty string', () => {
    expect(validateRequired('', 'Name')).toBe('Name is required');
  });

  it('returns error for whitespace-only string', () => {
    expect(validateRequired('  ', 'Name')).toBe('Name is required');
  });

  it('returns null for non-empty string', () => {
    expect(validateRequired('hello', 'Name')).toBeNull();
  });

  it('returns null for string with content surrounded by whitespace', () => {
    expect(validateRequired('  hello  ', 'Name')).toBeNull();
  });

  it('uses the fieldName in the error message', () => {
    expect(validateRequired('', 'Username')).toBe('Username is required');
  });
});

// --- validateMaxLength ---

describe('validateMaxLength', () => {
  it('returns null for string within limit', () => {
    expect(validateMaxLength('abc', 255, 'Name')).toBeNull();
  });

  it('returns error for string exceeding limit', () => {
    expect(validateMaxLength('a'.repeat(256), 255, 'Name')).toBe(
      'Name must be 255 characters or fewer',
    );
  });

  it('returns null for empty string (empty is not a length violation)', () => {
    expect(validateMaxLength('', 255, 'Name')).toBeNull();
  });

  it('returns null for string at exactly the limit', () => {
    expect(validateMaxLength('a'.repeat(255), 255, 'Name')).toBeNull();
  });

  it('uses the fieldName in the error message', () => {
    expect(validateMaxLength('a'.repeat(10), 5, 'Description')).toBe(
      'Description must be 5 characters or fewer',
    );
  });
});

// --- validateIPOrHostname ---

describe('validateIPOrHostname', () => {
  it('accepts valid IPv4 address', () => {
    expect(validateIPOrHostname('192.168.1.1')).toBeNull();
  });

  it('accepts another valid IPv4 address', () => {
    expect(validateIPOrHostname('10.0.0.1')).toBeNull();
  });

  it('accepts IPv6 loopback', () => {
    expect(validateIPOrHostname('::1')).toBeNull();
  });

  it('accepts valid IPv6 address', () => {
    expect(validateIPOrHostname('fe80::1')).toBeNull();
  });

  it('accepts valid hostname with dot', () => {
    expect(validateIPOrHostname('router-01.local')).toBeNull();
  });

  it('accepts simple hostname', () => {
    expect(validateIPOrHostname('my-host')).toBeNull();
  });

  it('returns error for empty string', () => {
    expect(validateIPOrHostname('')).toBe('IP address or hostname is required');
  });

  it('returns error for string with invalid characters', () => {
    expect(validateIPOrHostname('not valid!!')).toBe('Invalid IP address or hostname');
  });

  it('returns error for hostname exceeding 253 chars', () => {
    expect(validateIPOrHostname('a'.repeat(254))).toBe('Invalid IP address or hostname');
  });

  it('returns error for hostname starting with dash', () => {
    expect(validateIPOrHostname('-invalid')).toBe('Invalid IP address or hostname');
  });

  it('returns error for purely numeric value (not a valid IP or hostname)', () => {
    expect(validateIPOrHostname('12345')).toBe('Invalid IP address or hostname');
  });

  it('returns error for another purely numeric value', () => {
    expect(validateIPOrHostname('999')).toBe('Invalid IP address or hostname');
  });

  it('returns error for multi-label purely numeric value', () => {
    expect(validateIPOrHostname('123.456')).toBe('Invalid IP address or hostname');
  });

  it('accepts alphanumeric hostname with trailing digit', () => {
    expect(validateIPOrHostname('router1')).toBeNull();
  });

  it('accepts hostname label starting with digit but containing letter', () => {
    expect(validateIPOrHostname('1e100.net')).toBeNull();
  });
});

// --- validatePort ---

describe('validatePort', () => {
  it('returns null for port 22', () => {
    expect(validatePort('22', 'Port')).toBeNull();
  });

  it('returns null for port 1', () => {
    expect(validatePort('1', 'Port')).toBeNull();
  });

  it('returns null for port 65535', () => {
    expect(validatePort('65535', 'Port')).toBeNull();
  });

  it('returns error for port 0', () => {
    expect(validatePort('0', 'Port')).toBe('Port must be between 1 and 65535');
  });

  it('returns error for port 65536', () => {
    expect(validatePort('65536', 'Port')).toBe('Port must be between 1 and 65535');
  });

  it('returns error for non-numeric port', () => {
    expect(validatePort('abc', 'Port')).toBe('Port must be a number');
  });

  it('returns error for empty port', () => {
    expect(validatePort('', 'Port')).toBe('Port is required');
  });

  it('accepts numeric value', () => {
    expect(validatePort(22, 'Port')).toBeNull();
  });
});

// --- validateURL ---

describe('validateURL', () => {
  it('returns null for empty string (URLs are optional)', () => {
    expect(validateURL('', 'URL')).toBeNull();
  });

  it('accepts valid http URL', () => {
    expect(validateURL('http://localhost:3000', 'URL')).toBeNull();
  });

  it('accepts valid https URL', () => {
    expect(validateURL('https://grafana.example.com', 'URL')).toBeNull();
  });

  it('returns error for ftp URL', () => {
    expect(validateURL('ftp://invalid', 'URL')).toBe('URL must start with http:// or https://');
  });

  it('returns error for plain string', () => {
    expect(validateURL('not-a-url', 'URL')).toBe('URL must start with http:// or https://');
  });

  it('returns error for http:// with no host', () => {
    expect(validateURL('http://', 'URL')).toBe('URL must have a host');
  });
});

// --- validateSNMPv3Auth ---

describe('validateSNMPv3Auth', () => {
  it('accepts MD5', () => {
    expect(validateSNMPv3Auth('MD5')).toBeNull();
  });

  it('accepts SHA', () => {
    expect(validateSNMPv3Auth('SHA')).toBeNull();
  });

  it('accepts SHA-224', () => {
    expect(validateSNMPv3Auth('SHA-224')).toBeNull();
  });

  it('accepts SHA-256', () => {
    expect(validateSNMPv3Auth('SHA-256')).toBeNull();
  });

  it('accepts SHA-384', () => {
    expect(validateSNMPv3Auth('SHA-384')).toBeNull();
  });

  it('accepts SHA-512', () => {
    expect(validateSNMPv3Auth('SHA-512')).toBeNull();
  });

  it('returns error for invalid protocol', () => {
    expect(validateSNMPv3Auth('INVALID')).toBe(
      'Auth protocol must be one of: MD5, SHA, SHA-224, SHA-256, SHA-384, SHA-512',
    );
  });
});

// --- validateSNMPv3Priv ---

describe('validateSNMPv3Priv', () => {
  it('accepts DES', () => {
    expect(validateSNMPv3Priv('DES')).toBeNull();
  });

  it('accepts AES', () => {
    expect(validateSNMPv3Priv('AES')).toBeNull();
  });

  it('returns error for invalid protocol', () => {
    expect(validateSNMPv3Priv('INVALID')).toBe('Privacy protocol must be one of: DES, AES');
  });
});

// --- validateSNMPv3SecurityLevel ---

describe('validateSNMPv3SecurityLevel', () => {
  it('accepts noAuthNoPriv', () => {
    expect(validateSNMPv3SecurityLevel('noAuthNoPriv')).toBeNull();
  });

  it('accepts authNoPriv', () => {
    expect(validateSNMPv3SecurityLevel('authNoPriv')).toBeNull();
  });

  it('accepts authPriv', () => {
    expect(validateSNMPv3SecurityLevel('authPriv')).toBeNull();
  });

  it('returns error for invalid level', () => {
    expect(validateSNMPv3SecurityLevel('invalid')).toBe(
      'Security level must be one of: noAuthNoPriv, authNoPriv, authPriv',
    );
  });
});

// --- validateCoordinate ---

describe('validateCoordinate', () => {
  it('returns null for positive number', () => {
    expect(validateCoordinate(100, 'X')).toBeNull();
  });

  it('returns null for negative number', () => {
    expect(validateCoordinate(-500, 'Y')).toBeNull();
  });

  it('returns null for zero', () => {
    expect(validateCoordinate(0, 'X')).toBeNull();
  });

  it('returns error for NaN', () => {
    expect(validateCoordinate(NaN, 'X')).toBe('X must be a valid number');
  });

  it('returns error for positive Infinity', () => {
    expect(validateCoordinate(Infinity, 'X')).toBe('X must be a valid number');
  });

  it('returns error for negative Infinity', () => {
    expect(validateCoordinate(-Infinity, 'Y')).toBe('Y must be a valid number');
  });
});

// --- validateIntervalAllowlist ---

describe('validateIntervalAllowlist', () => {
  it('accepts 0', () => {
    expect(validateIntervalAllowlist('0')).toBeNull();
  });

  it('accepts 6', () => {
    expect(validateIntervalAllowlist('6')).toBeNull();
  });

  it('accepts 12', () => {
    expect(validateIntervalAllowlist('12')).toBeNull();
  });

  it('accepts 24', () => {
    expect(validateIntervalAllowlist('24')).toBeNull();
  });

  it('accepts 48', () => {
    expect(validateIntervalAllowlist('48')).toBeNull();
  });

  it('accepts 168', () => {
    expect(validateIntervalAllowlist('168')).toBeNull();
  });

  it('returns error for 7 (not in allowlist)', () => {
    expect(validateIntervalAllowlist('7')).toBe('Interval must be one of: 0, 6, 12, 24, 48, 168');
  });

  it('returns error for non-numeric string', () => {
    expect(validateIntervalAllowlist('abc')).toBe('Interval must be one of: 0, 6, 12, 24, 48, 168');
  });
});

// --- validateRetentionCount ---

describe('validateRetentionCount', () => {
  it('accepts 1', () => {
    expect(validateRetentionCount('1')).toBeNull();
  });

  it('accepts 365', () => {
    expect(validateRetentionCount('365')).toBeNull();
  });

  it('returns error for 0', () => {
    expect(validateRetentionCount('0')).toBe('Retention count must be between 1 and 365');
  });

  it('returns error for 366', () => {
    expect(validateRetentionCount('366')).toBe('Retention count must be between 1 and 365');
  });

  it('returns error for non-numeric string', () => {
    expect(validateRetentionCount('abc')).toBe('Retention count must be a number');
  });
});

// --- Constants ---

describe('constants', () => {
  it('MAX_STRING_LENGTH equals 255', () => {
    expect(MAX_STRING_LENGTH).toBe(255);
  });

  it('SNMP_AUTH_PROTOCOLS contains all expected values', () => {
    expect(SNMP_AUTH_PROTOCOLS).toEqual(['MD5', 'SHA', 'SHA-224', 'SHA-256', 'SHA-384', 'SHA-512']);
  });

  it('SNMP_PRIV_PROTOCOLS contains expected values', () => {
    expect(SNMP_PRIV_PROTOCOLS).toEqual(['DES', 'AES']);
  });

  it('SNMP_SECURITY_LEVELS contains expected values', () => {
    expect(SNMP_SECURITY_LEVELS).toEqual(['noAuthNoPriv', 'authNoPriv', 'authPriv']);
  });

  it('INTERVAL_ALLOWLIST contains expected values', () => {
    expect(INTERVAL_ALLOWLIST).toEqual([0, 6, 12, 24, 48, 168]);
  });
});
