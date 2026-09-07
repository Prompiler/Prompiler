#!/usr/bin/env bash
# End-to-end smoke tests for the promptpiler CLI binary.
#
# Usage: scripts/e2e.sh [path-to-promptpiler-binary]
set -euo pipefail

BIN="${1:-./bin/prompiler}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

echo "== list =="
"$BIN" list -root "$ROOT/examples/feature/interpolation" | grep -q "Interpolate"

echo "== check (valid) =="
"$BIN" check -root "$ROOT/examples/feature/interpolation"

echo "== check (type error) =="
if "$BIN" check -root "$ROOT/examples/errors/type-mismatch" >/dev/null 2>&1; then
  echo "expected 'check' to fail on type-mismatch" >&2
  exit 1
fi

echo "== run =="
out="$("$BIN" run -root "$ROOT/examples/feature/interpolation" Interpolate)"
grep -q "Name: Ada" <<<"$out"
grep -q "Greeting: hello" <<<"$out"

echo "e2e smoke tests passed"
