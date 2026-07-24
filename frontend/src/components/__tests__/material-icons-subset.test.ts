/**
 * Exercises material icons subset component behavior so refactors preserve the documented contract.
 */
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

const subsetScript = readFileSync(
  join(__dirname, '../../../scripts/subset-material-icons.sh'),
  'utf-8',
);

const REQUIRED_TOPOLOGY_HUB_ICONS = [
  'add_location_alt',
  'check',
  'close_fullscreen',
  'content_copy',
  'expand_less',
  'map',
  'open_in_full',
  'public',
] as const;

const REQUIRED_CANVAS_TOOLBAR_ICONS = [
  'add',
  'build',
  'close',
  'edit',
  'grid_4x4',
  'link',
  'notifications',
  'search',
] as const;

const REQUIRED_DASHBOARD_DEVICE_ACTION_ICONS = ['description', 'history'] as const;

const REQUIRED_AUTH_ADMIN_ICONS = [
  'admin_panel_settings',
  'block',
  'lock_reset',
  'logout',
  'refresh',
  'visibility',
  'visibility_off',
] as const;

const REQUIRED_USER_SETTINGS_ICONS = ['download', 'lock', 'more_vert', 'person', 'sync'] as const;

const REQUIRED_ADMIN_SETTINGS_ICONS = [
  'account_tree',
  'badge',
  'info',
  'settings_ethernet',
  'speed',
  'tune',
] as const;

function iconNamesFromScript(): Set<string> {
  const match = subsetScript.match(/ICON_NAMES=\(\r?\n(?<body>[\s\S]*?)\r?\n\)/);
  if (!match?.groups?.body) {
    throw new Error('Missing ICON_NAMES array in subset-material-icons.sh');
  }

  return new Set(
    [...match.groups.body.matchAll(/^\s*([a-z0-9_]+)\s*$/gm)].map((entry) => entry[1]),
  );
}

function inputCodePointsFromScript(): Set<number> {
  const match = subsetScript.match(/^INPUT_UNICODES="(?<ranges>[^"]+)"$/m);
  if (!match?.groups?.ranges) {
    throw new Error('Missing INPUT_UNICODES declaration in subset-material-icons.sh');
  }

  const codePoints = new Set<number>();
  for (const range of match.groups.ranges.split(',')) {
    const [start, end] = range
      .split('-')
      .map((value) => Number.parseInt(value.replace('U+', ''), 16));
    if (start === undefined) {
      throw new Error(`Invalid INPUT_UNICODES range: ${range}`);
    }
    for (let codePoint = start; codePoint <= (end ?? start); codePoint += 1) {
      codePoints.add(codePoint);
    }
  }

  return codePoints;
}

describe('Material Symbols subset contract', () => {
  it('declares every canvas toolbar icon in the generated subset inputs', () => {
    const iconNames = iconNamesFromScript();
    for (const iconName of REQUIRED_CANVAS_TOOLBAR_ICONS) {
      expect(iconNames.has(iconName), iconName).toBe(true);
    }
  });

  it('retains every character needed to shape canvas toolbar ligatures', () => {
    const inputCodePoints = inputCodePointsFromScript();
    for (const iconName of REQUIRED_CANVAS_TOOLBAR_ICONS) {
      for (const character of iconName) {
        const codePoint = character.codePointAt(0);
        if (codePoint === undefined) {
          throw new Error(`Missing code point for ${iconName} input character`);
        }
        const label = `${iconName} input U+${codePoint.toString(16).toUpperCase().padStart(4, '0')}`;
        expect(inputCodePoints.has(codePoint), label).toBe(true);
      }
    }
  });

  it('declares every Topology Hub icon in the generated subset inputs', () => {
    const iconNames = iconNamesFromScript();

    for (const iconName of REQUIRED_TOPOLOGY_HUB_ICONS) {
      expect(iconNames.has(iconName), iconName).toBe(true);
    }
  });

  it('declares every Devices table action icon in the generated subset inputs', () => {
    const iconNames = iconNamesFromScript();

    for (const iconName of REQUIRED_DASHBOARD_DEVICE_ACTION_ICONS) {
      expect(iconNames.has(iconName), iconName).toBe(true);
    }
  });

  it('declares every auth and admin icon in the generated subset inputs', () => {
    const iconNames = iconNamesFromScript();

    for (const iconName of REQUIRED_AUTH_ADMIN_ICONS) {
      expect(iconNames.has(iconName), iconName).toBe(true);
    }
  });

  it('declares every User Settings icon in the generated subset inputs', () => {
    const iconNames = iconNamesFromScript();

    for (const iconName of REQUIRED_USER_SETTINGS_ICONS) {
      expect(iconNames.has(iconName), iconName).toBe(true);
    }
  });

  it('declares every Admin Settings panel icon in the generated subset inputs', () => {
    const iconNames = iconNamesFromScript();

    for (const iconName of REQUIRED_ADMIN_SETTINGS_ICONS) {
      expect(iconNames.has(iconName), iconName).toBe(true);
    }
  });
});
