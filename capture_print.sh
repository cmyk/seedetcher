#!/bin/bash
set -x
PDF_DIR="/home/cmyk/PDF"
OUTPUT_FILE="$PDF_DIR/test.pdf"
USBDEV="${1:-/dev/ttyACM1}"

echo "Listening on $USBDEV..."

# Clear buffer
stty -F "$USBDEV" raw -echo
cat "$USBDEV" > /dev/null 2>/dev/null &
PID=$!
sleep 1
kill $PID 2>/dev/null || true
echo "Buffer cleared" >&2

# Capture with timeout
echo "Waiting for print job at $(date '+%H:%M:%S')..." >&2
mkdir -p "$PDF_DIR"
timeout 5 tee "$OUTPUT_FILE" < "$USBDEV" > /dev/null || true

# Process
if [ -s "$OUTPUT_FILE" ]; then
    echo "Saved test.pdf to $PDF_DIR/test.pdf"
    if grep -q "%PDF" "$OUTPUT_FILE"; then
        echo "Contains %PDF"
    else
        echo "No %PDF, first 20 lines:"
        xxd "$OUTPUT_FILE" | head -n 20
    fi
    cp "$OUTPUT_FILE" "$PDF_DIR/test_raw.pdf"
    echo "Saved test_raw.pdf to $PDF_DIR/test_raw.pdf"
    ls -lh "$PDF_DIR/test.pdf" "$PDF_DIR/test_raw.pdf"
else
    echo "No data captured" >&2
    exit 1
fi