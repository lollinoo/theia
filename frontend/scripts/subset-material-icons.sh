#!/usr/bin/env bash
set -euo pipefail
# Regenerates the Material Symbols Rounded self-hosted woff2 subset.
# Prerequisites: pip install fonttools brotli
# Usage: bash frontend/scripts/subset-material-icons.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FONT_DIR="$SCRIPT_DIR/../public/fonts"
WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT

# All icon codepoints used in the project (Material Symbols Rounded ligatures).
# Update this list when adding new MaterialIcon usages.
# Format: U+XXXX  icon_name
UNICODES="U+E145,U+E250,U+E2BD,U+E326,U+E518,U+E51C,U+E5CD,U+E5CF,U+E7F5,U+E864,U+E873,U+E875,U+E894,U+E8B3,U+E8B6,U+E8B8,U+E8FF,U+E900,U+E92E,U+E9F4,U+EA10,U+EB8E,U+F097,U+F0BE,U+0020,U+005F,0061-007A"
# ASCII letters a-z (U+0061-U+007A), underscore (U+005F), and space (U+0020)
# are required as ligature input glyphs for Material Symbols icon name lookup.
# Codepoint reference:
#   E145=add  E250=link  E2BD=cloud  E326=devices  E518=edit  E51C=content_copy
#   E5CD=close  E5CF=expand_more  E7F5=notifications  E864=backup  E873=history
#   E875=dns  E894=language  E8B3=filter_list  E8B6=search  E8B8=settings
#   E8FF=zoom_in  E900=zoom_out  E92E=delete  E9F4=hub  EA10=fit_screen
#   EB8E=check_circle  F097=swap_vert  F0BE=arrow_upward

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

echo "Subsetting font with $(echo "$UNICODES" | tr ',' '\n' | wc -l) icon codepoints..."

pyftsubset "$FULL_FONT" \
  --output-file="$FONT_DIR/material-symbols-rounded-subset.woff2" \
  --flavor=woff2 \
  --unicodes="$UNICODES" \
  --layout-features='liga,clig' \
  --no-hinting \
  --desubroutinize

FILESIZE=$(wc -c < "$FONT_DIR/material-symbols-rounded-subset.woff2")
echo "Done. Output: $FONT_DIR/material-symbols-rounded-subset.woff2 ($FILESIZE bytes)"
