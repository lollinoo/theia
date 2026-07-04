#!/bin/bash
set -euo pipefail

if [ "$#" -ne 3 ]; then
  echo "Usage: $0 <mode> <api_base> <output_dir>"
  echo "Modes: synthetic | wisp"
  exit 1
fi

MODE="$1"
API_BASE="$2"
OUTPUT_DIR="$3"

mkdir -p "$OUTPUT_DIR"

write_readme() {
  local scale_files
  case "$MODE" in
    synthetic)
      scale_files=$'- `scale-300-baseline.json`\n- `scale-300-burst-adds.json`'
      ;;
    wisp)
      scale_files='- `scale-wisp-hybrid.json`'
      ;;
    *)
      scale_files='- mode-specific scale-lab outputs'
      ;;
  esac

  cat >"$OUTPUT_DIR/README.md" <<EOF
# Scale-Lab Validation Evidence

- Mode: ${MODE}
- API base: ${API_BASE}
- Output directory: ${OUTPUT_DIR}

## Evidence Files

${scale_files}
- \`metrics.prom\`

## Required Evidence Surfaces

- \`theia_refresh_snapshot_build_seconds\`
- \`theia_refresh_topology_reload_total\`
- \`theia_state_changes_dropped_total\`
- \`window.__THEIA_CANVAS_METRICS__\`
EOF
}

run_synthetic() {
  go run ./cmd/theia-scale-lab -profile 300 -scenario baseline -out "${OUTPUT_DIR}/scale-300-baseline.json" >/dev/null
  go run ./cmd/theia-scale-lab -profile 300 -scenario burst-adds -out "${OUTPUT_DIR}/scale-300-burst-adds.json" >/dev/null
}

run_wisp() {
  go run ./cmd/theia-scale-lab -profile 300 -scenario baseline -fixture internal/scalelab/testdata/wisp-hybrid.json -out "${OUTPUT_DIR}/scale-wisp-hybrid.json" >/dev/null
}

write_readme

case "$MODE" in
  synthetic)
    run_synthetic
    ;;
  wisp)
    run_wisp
    ;;
  *)
    echo "Unsupported mode: $MODE"
    echo "Modes: synthetic | wisp"
    exit 1
    ;;
esac

curl -fsS "${API_BASE}/metrics" -o "${OUTPUT_DIR}/metrics.prom"

for metric in \
  theia_refresh_snapshot_build_seconds \
  theia_refresh_topology_reload_total \
  theia_state_changes_dropped_total
do
  if ! grep -q "^${metric}" "${OUTPUT_DIR}/metrics.prom"; then
    echo "Missing required metric family: ${metric}" >&2
    exit 1
  fi
done

echo "Saved scale-lab ${MODE} evidence to ${OUTPUT_DIR}"
