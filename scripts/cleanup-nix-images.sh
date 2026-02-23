#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/cleanup-nix-images.sh [options]

Remove old SeedEtcher disk-image outputs from /nix/store.
Defaults to dry-run.

Options:
  --apply            Actually delete paths (default: dry-run)
  --no-keep-result   Do not automatically keep the current ./result target
  --keep PATH        Keep an extra /nix/store path (repeatable)
  -h, --help         Show this help

Examples:
  scripts/cleanup-nix-images.sh
  scripts/cleanup-nix-images.sh --apply
EOF
}

APPLY=0
KEEP_RESULT=1
declare -a KEEP_PATHS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --apply)
      APPLY=1
      shift
      ;;
    --no-keep-result)
      KEEP_RESULT=0
      shift
      ;;
    --keep)
      [[ $# -ge 2 ]] || { echo "error: --keep needs a path" >&2; exit 2; }
      KEEP_PATHS+=("$(readlink -f "$2")")
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown option: $1" >&2
      usage
      exit 2
      ;;
  esac
done

if [[ $KEEP_RESULT -eq 1 && -L result ]]; then
  KEEP_PATHS+=("$(readlink -f result)")
fi

in_keep_list() {
  local needle="$1"
  local p
  for p in "${KEEP_PATHS[@]}"; do
    [[ "$needle" == "$p" ]] && return 0
  done
  return 1
}

is_seedetcher_disk_image_dir() {
  local d="$1"
  compgen -G "$d/seedetcher*.img" >/dev/null
}

mapfile -t ALL_DISK_IMAGE_DIRS < <(find /nix/store -maxdepth 1 -type d -name '*-disk-image' | sort)

if [[ ${#ALL_DISK_IMAGE_DIRS[@]} -eq 0 ]]; then
  echo "No /nix/store/*-disk-image paths found."
  exit 0
fi

declare -a CANDIDATES=()
declare -a SKIPPED_KEEP=()
declare -a SKIPPED_ALIVE=()
declare -a DELETABLE=()

for path in "${ALL_DISK_IMAGE_DIRS[@]}"; do
  if ! is_seedetcher_disk_image_dir "$path"; then
    continue
  fi
  CANDIDATES+=("$path")
  if in_keep_list "$path"; then
    SKIPPED_KEEP+=("$path")
    continue
  fi
  roots="$(nix-store --query --roots "$path" 2>/dev/null || true)"
  if [[ -n "$roots" ]]; then
    SKIPPED_ALIVE+=("$path")
    continue
  fi
  DELETABLE+=("$path")
done

echo "SeedEtcher disk-image paths found: ${#CANDIDATES[@]}"
echo "Kept explicitly: ${#SKIPPED_KEEP[@]}"
echo "Still alive (GC roots): ${#SKIPPED_ALIVE[@]}"
echo "Deletable: ${#DELETABLE[@]}"
echo

if [[ ${#DELETABLE[@]} -eq 0 ]]; then
  echo "Nothing deletable."
  exit 0
fi

echo "Deletable paths:"
printf '  %s\n' "${DELETABLE[@]}"

if [[ $APPLY -eq 0 ]]; then
  echo
  echo "Dry run only. Re-run with --apply to delete."
  exit 0
fi

echo
echo "Deleting..."
for path in "${DELETABLE[@]}"; do
  nix store delete "$path" || echo "warn: failed to delete $path"
done

echo "Done."
