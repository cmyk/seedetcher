#!/usr/bin/env bash
set -euo pipefail

PRINTER="${PRINTER:-/dev/usb/lp0}"

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 <file.pcl> [printer_dev]" >&2
  exit 1
fi

PCL_FILE="$1"
if [[ $# -ge 2 ]]; then
  PRINTER="$2"
fi

if [[ ! -f "$PCL_FILE" ]]; then
  echo "PCL file not found: $PCL_FILE" >&2
  exit 1
fi

if ! [ -w "$PRINTER" ]; then
  echo "Printer device not writable: $PRINTER" >&2
  exit 1
fi

# Reset channel
echo -ne "\033%-12345X" >"$PRINTER"
sleep 0.1

# Stream with larger block size for fewer writes
dd if="$PCL_FILE" of="$PRINTER" bs=16k status=none

echo "Sent $PCL_FILE to $PRINTER"
