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

describe('Material Symbols subset contract', () => {
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
