#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

version="${1:-}"
if [ -z "$version" ]; then
  version="$(sed -nE 's/^const Tag = "([^"]+)".*/\1/p' "$ROOT_DIR/version/version.go" | head -n1 || true)"
fi
if [ -z "$version" ]; then
  version="dev"
fi

output_path="${2:-$ROOT_DIR/release/SeedEtcher-Workflow-${version}.pdf}"
mkdir -p "$(dirname "$output_path")"

ROOT_DIR="$ROOT_DIR" OUTPUT_PATH="$output_path" nix shell nixpkgs#pandoc nixpkgs#python3Packages.weasyprint -c bash -lc '
  cd "$ROOT_DIR/docs"
  pandoc SeedEtcher-Workflow.md -f gfm \
    -o "$OUTPUT_PATH" \
    --pdf-engine=weasyprint \
    --css="$ROOT_DIR/docs/assets/workflow/workflow-pdf.css"
'

echo "Generated: $output_path"
