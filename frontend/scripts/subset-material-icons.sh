#!/usr/bin/env bash
set -euo pipefail
# Regenerates the Material Symbols Rounded self-hosted woff2 subset.
# Prerequisites: pip install fonttools brotli
# Usage: bash frontend/scripts/subset-material-icons.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FONT_DIR="$SCRIPT_DIR/../public/fonts"
WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT

# All icon ligatures used or intentionally available in the project.
# Update this list when adding new MaterialIcon usages. Codepoints are resolved
# from the downloaded Material Symbols font because Google can move ligatures
# between releases.
ICON_NAMES=(
  account_tree
  add
  add_location_alt
  admin_panel_settings
  backup
  badge
  block
  build
  check
  check_circle
  close
  close_fullscreen
  cloud
  content_copy
  dark_mode
  delete
  description
  devices
  dns
  download
  edit
  expand_less
  expand_more
  fit_screen
  grid_4x4
  history
  hub
  info
  key
  language
  light_mode
  link
  lock
  lock_reset
  logout
  map
  notifications
  open_in_full
  open_in_new
  more_vert
  person
  public
  refresh
  remove
  search
  settings
  settings_ethernet
  speed
  sync
  terminal
  tune
  visibility
  visibility_off
  zoom_in
  zoom_out
)

# ASCII letters a-z (U+0061-U+007A), underscore (U+005F), and space (U+0020)
# are required as ligature input glyphs for Material Symbols icon name lookup.
INPUT_UNICODES="U+0020,U+005F,U+0061-007A"

# Download the full variable font from Google Fonts (Material Symbols Rounded)
FULL_FONT="$WORK_DIR/MaterialSymbolsRounded.woff2"
CSS_FILE="$WORK_DIR/material-symbols.css"

# First fetch the CSS to extract the woff2 URL
curl -fsSL -o "$CSS_FILE" \
  "https://fonts.googleapis.com/icon?family=Material+Symbols+Rounded" \
  --header "User-Agent: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36"

WOFF2_URL=$(grep -oP 'url\(\K[^)]+' "$CSS_FILE" | head -1)
if [ -z "$WOFF2_URL" ]; then
  echo "ERROR: Could not extract woff2 URL from Google Fonts CSS"
  exit 1
fi

echo "Downloading full font from: $WOFF2_URL"
curl -fsSL -o "$FULL_FONT" "$WOFF2_URL"

ICON_NAMES_FILE="$WORK_DIR/material-symbols-icon-names.txt"
printf '%s\n' "${ICON_NAMES[@]}" > "$ICON_NAMES_FILE"

ICON_UNICODES=$(
  python3 - "$FULL_FONT" "$ICON_NAMES_FILE" <<'PY'
from fontTools.ttLib import TTFont
import sys

font_path, names_path = sys.argv[1], sys.argv[2]
font = TTFont(font_path)
best_cmap = font.getBestCmap()
glyph_to_codepoint = {glyph: codepoint for codepoint, glyph in best_cmap.items()}
glyph_to_char = {glyph: chr(codepoint) for codepoint, glyph in best_cmap.items()}

ligatures = {}
if "GSUB" in font:
    lookup_list = font["GSUB"].table.LookupList
    for lookup in lookup_list.Lookup:
        for subtable in lookup.SubTable:
            subst = getattr(subtable, "ExtSubTable", subtable)
            if not hasattr(subst, "ligatures"):
                continue
            for first, ligature_set in subst.ligatures.items():
                for ligature in ligature_set:
                    name = "".join(
                        [glyph_to_char.get(first, ""), *[glyph_to_char.get(component, "") for component in ligature.Component]]
                    )
                    ligatures[name] = ligature.LigGlyph

icon_names = [line.strip() for line in open(names_path, encoding="utf-8") if line.strip()]
missing = []
codepoints = set()
for icon_name in icon_names:
    glyph = ligatures.get(icon_name)
    codepoint = glyph_to_codepoint.get(glyph) if glyph else None
    if codepoint is None:
        missing.append(icon_name)
        continue
    codepoints.add(codepoint)

if missing:
    print(f"ERROR: Missing Material Symbols ligatures: {', '.join(missing)}", file=sys.stderr)
    sys.exit(1)

print(",".join(f"U+{codepoint:04X}" for codepoint in sorted(codepoints)))
PY
)
UNICODES="$ICON_UNICODES,$INPUT_UNICODES"

echo "Subsetting font with ${#ICON_NAMES[@]} icon ligatures..."

if command -v pyftsubset >/dev/null 2>&1; then
  SUBSETTER=(pyftsubset)
else
  SUBSETTER=(python3 -m fontTools.subset)
fi

"${SUBSETTER[@]}" "$FULL_FONT" \
  --output-file="$FONT_DIR/material-symbols-rounded-subset.woff2" \
  --flavor=woff2 \
  --unicodes="$UNICODES" \
  --layout-features='rlig' \
  --no-layout-closure \
  --no-hinting \
  --desubroutinize

FILESIZE=$(wc -c < "$FONT_DIR/material-symbols-rounded-subset.woff2")
echo "Done. Output: $FONT_DIR/material-symbols-rounded-subset.woff2 ($FILESIZE bytes)"
