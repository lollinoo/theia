#!/usr/bin/env bash
set -euo pipefail
profile_path="${1:?usage: check-go-cover.sh <coverprofile> <min-percent>}"
min_percent="${2:?usage: check-go-cover.sh <coverprofile> <min-percent>}"
total_line="$(go tool cover -func="$profile_path" | tail -n 1)"
total_percent="$(printf '%s\n' "$total_line" | sed -E 's/^total:\s+\(statements\)\s+([0-9.]+)%.*/\1/')"
python3 - "$total_percent" "$min_percent" <<'PY'
import sys
actual = float(sys.argv[1])
minimum = float(sys.argv[2])
if actual < minimum:
    print(f"backend coverage {actual:.1f}% is below required {minimum:.1f}%", file=sys.stderr)
    raise SystemExit(1)
print(f"backend coverage {actual:.1f}% meets required {minimum:.1f}%")
PY
