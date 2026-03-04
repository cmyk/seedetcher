#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
GO_BIN="$(go env GOPATH 2>/dev/null)/bin"

timestamp="$(date +%Y%m%d-%H%M%S)"
OUT_DIR="${OUT_DIR:-$ROOT/.tmp/go-bloat-$timestamp}"
CYCLO_OVER="${CYCLO_OVER:-12}"
DUPL_THRESHOLD="${DUPL_THRESHOLD:-80}"

mkdir -p "$OUT_DIR"

info() {
  printf '[analyze] %s\n' "$*"
}

warn() {
  printf '[analyze] WARN: %s\n' "$*" >&2
}

have() {
  command -v "$1" >/dev/null 2>&1 || [ -x "$GO_BIN/$1" ]
}

tool_path() {
  if command -v "$1" >/dev/null 2>&1; then
    command -v "$1"
    return 0
  fi
  if [ -x "$GO_BIN/$1" ]; then
    printf '%s\n' "$GO_BIN/$1"
    return 0
  fi
  return 1
}

run_capture() {
  local name="$1"
  shift
  local log="$OUT_DIR/$name.log"
  info "running $name"
  set +e
  "$@" >"$log" 2>&1
  local rc=$?
  set -e
  printf '%s %d\n' "$name" "$rc" >>"$OUT_DIR/status.txt"
}

# Same package exclusions as scripts/test-lite.sh.
mapfile -t PKGS < <(go list ./... | grep -v 'driver/libcamera' | grep -v 'driver/wshat')
if [ "${#PKGS[@]}" -eq 0 ]; then
  warn "no packages found"
  exit 1
fi
MODULE_PATH="$(go list -m)"
LINT_PKGS=()
for p in "${PKGS[@]}"; do
  rel="${p#"$MODULE_PATH"}"
  if [ "$rel" = "$p" ]; then
    LINT_PKGS+=("$p")
    continue
  fi
  if [ -z "$rel" ]; then
    LINT_PKGS+=(".")
  else
    LINT_PKGS+=(".$rel")
  fi
done

find . -name '*.go' -not -path './vendor/*' -print0 \
  | xargs -0 wc -l \
  | sort -nr \
  >"$OUT_DIR/top_files_loc.txt"

if have gocyclo; then
  GOCYCLO_BIN="$(tool_path gocyclo)"
  set +e
  find . -name '*.go' -not -path './vendor/*' -print0 \
    | xargs -0 "$GOCYCLO_BIN" -over "$CYCLO_OVER" \
    | sort -nr \
    >"$OUT_DIR/top_cyclo.txt"
  rc=$?
  set -e
  printf '%s %d\n' "gocyclo" "$rc" >>"$OUT_DIR/status.txt"
else
  warn "gocyclo not installed; skipping complexity ranking"
fi

if have dupl; then
  DUPL_BIN="$(tool_path dupl)"
  run_capture dupl "$DUPL_BIN" -threshold "$DUPL_THRESHOLD" .
else
  warn "dupl not installed; skipping duplicate-block scan"
fi

if have staticcheck; then
  STATICCHECK_BIN="$(tool_path staticcheck)"
  run_capture staticcheck "$STATICCHECK_BIN" "${PKGS[@]}"
else
  warn "staticcheck not installed; skipping"
fi

if have gocritic; then
  GOCRITIC_BIN="$(tool_path gocritic)"
  run_capture gocritic "$GOCRITIC_BIN" check "${PKGS[@]}"
else
  warn "gocritic not installed; skipping"
fi

if have golangci-lint; then
  GOLANGCI_BIN="$(tool_path golangci-lint)"
  run_capture golangci_lint "$GOLANGCI_BIN" run "${LINT_PKGS[@]}"
else
  warn "golangci-lint not installed; skipping"
fi

run_capture go_vet go vet "${PKGS[@]}"

{
  echo "Go bloat analysis"
  echo "repo: $ROOT"
  echo "out: $OUT_DIR"
  echo
  echo "tool status (name rc):"
  if [ -f "$OUT_DIR/status.txt" ]; then
    cat "$OUT_DIR/status.txt"
  else
    echo "none"
  fi
  echo

  echo "largest go files (top 20):"
  head -n 20 "$OUT_DIR/top_files_loc.txt"
  echo

  if [ -f "$OUT_DIR/top_cyclo.txt" ]; then
    echo "complexity hotspots (gocyclo, top 30):"
    head -n 30 "$OUT_DIR/top_cyclo.txt"
    echo
  fi

  if [ -f "$OUT_DIR/dupl.log" ]; then
    clones="$(grep -Ec '^found [0-9]+ clones:' "$OUT_DIR/dupl.log" || true)"
    echo "dupl clone-groups: $clones (see dupl.log)"
    echo
  fi

  for t in staticcheck gocritic golangci_lint go_vet; do
    if [ -f "$OUT_DIR/$t.log" ]; then
      count="$(grep -cvE '^[[:space:]]*$' "$OUT_DIR/$t.log" || true)"
      echo "$t non-empty lines: $count (see $t.log)"
    fi
  done
} >"$OUT_DIR/summary.txt"

info "done"
info "summary: $OUT_DIR/summary.txt"
