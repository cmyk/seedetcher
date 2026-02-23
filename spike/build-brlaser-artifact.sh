#!/usr/bin/env bash
set -euo pipefail

# Build a SeedEtcher-compatible brlaser drop-in artifact:
#   spike/brlaser-root.tar.gz
#
# Expected archive layout:
#   brlaser-root/
#     lib/cups/filter/rastertobrlaser
#     share/cups/drv/brlaser.drv
#     lib/*.so*              (optional, for dynamic runtime)
#
# Run this INSIDE the environment where rastertobrlaser is known-good for target.
#
# Example:
#   BRLASER_FILTER_BIN=/path/to/rastertobrlaser \
#   BRLASER_DRV_FILE=/path/to/brlaser.drv \
#   LIB_SEARCH_DIRS="/path/to/lib1:/path/to/lib2" \
#   ./spike/build-brlaser-artifact.sh
#
# Optional:
#   OUT=spike/brlaser-root.tar.gz
#   STAGE_DIR=/tmp/brlaser-stage
#   INCLUDE_NEEDED_LIBS=1 (default)

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${OUT:-$ROOT_DIR/spike/brlaser-root.tar.gz}"
STAGE_DIR="${STAGE_DIR:-/tmp/seedetcher-brlaser-artifact}"
INCLUDE_NEEDED_LIBS="${INCLUDE_NEEDED_LIBS:-1}"

BRLASER_FILTER_BIN="${BRLASER_FILTER_BIN:-}"
BRLASER_DRV_FILE="${BRLASER_DRV_FILE:-}"
LIB_SEARCH_DIRS="${LIB_SEARCH_DIRS:-}"

if [[ -z "$BRLASER_FILTER_BIN" || -z "$BRLASER_DRV_FILE" ]]; then
  echo "error: BRLASER_FILTER_BIN and BRLASER_DRV_FILE must be set" >&2
  exit 2
fi
if [[ ! -f "$BRLASER_FILTER_BIN" ]]; then
  echo "error: filter not found: $BRLASER_FILTER_BIN" >&2
  exit 2
fi
if [[ ! -f "$BRLASER_DRV_FILE" ]]; then
  echo "error: drv not found: $BRLASER_DRV_FILE" >&2
  exit 2
fi

rm -rf "$STAGE_DIR"
mkdir -p "$STAGE_DIR/brlaser-root/lib/cups/filter" "$STAGE_DIR/brlaser-root/share/cups/drv" "$STAGE_DIR/brlaser-root/lib"

cp -a "$BRLASER_FILTER_BIN" "$STAGE_DIR/brlaser-root/lib/cups/filter/rastertobrlaser"
chmod 0555 "$STAGE_DIR/brlaser-root/lib/cups/filter/rastertobrlaser"
cp -a "$BRLASER_DRV_FILE" "$STAGE_DIR/brlaser-root/share/cups/drv/brlaser.drv"

if [[ "$INCLUDE_NEEDED_LIBS" == "1" ]]; then
  if ! command -v readelf >/dev/null 2>&1; then
    echo "error: readelf is required when INCLUDE_NEEDED_LIBS=1" >&2
    exit 2
  fi
  if [[ -z "$LIB_SEARCH_DIRS" ]]; then
    echo "error: LIB_SEARCH_DIRS is required when INCLUDE_NEEDED_LIBS=1" >&2
    exit 2
  fi

  mapfile -t NEEDED < <(readelf -d "$BRLASER_FILTER_BIN" 2>/dev/null | sed -n 's@.*Shared library: \[\(.*\)\]@\1@p' | sort -u)
  if [[ "${#NEEDED[@]}" -eq 0 ]]; then
    echo "warn: no NEEDED libs detected via readelf"
  fi

  IFS=':' read -r -a SEARCH_DIRS <<< "$LIB_SEARCH_DIRS"
  for lib in "${NEEDED[@]}"; do
    found=""
    for dir in "${SEARCH_DIRS[@]}"; do
      [[ -n "$dir" && -d "$dir" ]] || continue
      if [[ -f "$dir/$lib" ]]; then
        cp -a "$dir/$lib" "$STAGE_DIR/brlaser-root/lib/"
        found="1"
        break
      fi
    done
    if [[ -z "$found" ]]; then
      echo "warn: needed lib not found in LIB_SEARCH_DIRS: $lib"
    fi
  done
fi

MANIFEST="$STAGE_DIR/brlaser-root/MANIFEST.txt"
{
  echo "artifact: brlaser-root.tar.gz"
  echo "created_utc: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "filter: $BRLASER_FILTER_BIN"
  echo "drv: $BRLASER_DRV_FILE"
  echo "include_needed_libs: $INCLUDE_NEEDED_LIBS"
  echo "lib_search_dirs: $LIB_SEARCH_DIRS"
  echo
  echo "[file]"
  file "$STAGE_DIR/brlaser-root/lib/cups/filter/rastertobrlaser" || true
  echo
  echo "[readelf interpreter]"
  readelf -l "$STAGE_DIR/brlaser-root/lib/cups/filter/rastertobrlaser" 2>/dev/null | sed -n 's@.*Requesting program interpreter: \(.*\)]@\1@p' || true
  echo
  echo "[readelf needed]"
  readelf -d "$STAGE_DIR/brlaser-root/lib/cups/filter/rastertobrlaser" 2>/dev/null | sed -n 's@.*Shared library: \[\(.*\)\]@\1@p' || true
  echo
  echo "[payload]"
  (cd "$STAGE_DIR" && find brlaser-root -type f | sort)
} > "$MANIFEST"

mkdir -p "$(dirname "$OUT")"
tar -C "$STAGE_DIR" -czf "$OUT" brlaser-root

echo "wrote: $OUT"
echo "manifest: $MANIFEST"
echo "next: copy artifact to SD boot partition as brlaser-root.tar.gz"
