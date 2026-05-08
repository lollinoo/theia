import { readFileSync } from 'fs';
import { join } from 'path';
import { describe, expect, it } from 'vitest';

const subsetScript = readFileSync(
  join(__dirname, '../../../scripts/subset-material-icons.sh'),
  'utf-8',
);

const REQUIRED_TOPOLOGY_HUB_ICONS = [
  'add_location_alt',
  'check',
  'content_copy',
  'expand_less',
  'map',
  'open_in_full',
  'public',
] as const;

function iconNamesFromScript(): Set<string> {
  const match = subsetScript.match(/ICON_NAMES=\(\n(?<body>[\s\S]*?)\n\)/);
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
});
