#!/bin/bash
set -x
OUTPUT_FILE="test.pdf"  # Rename to .pdf
PDF_DIR="/home/cmyk/PDF"
USBDEV="${1:-/dev/ttyACM1}"
echo "Listening on $USBDEV..."
timeout 10 cat "$USBDEV" > "$OUTPUT_FILE"
if [ -s "$OUTPUT_FILE" ]; then
    mkdir -p "$PDF_DIR"
    if head -c 4 "$OUTPUT_FILE" | grep -q "%PDF"; then
        cp "$OUTPUT_FILE" "$PDF_DIR/test.pdf"
        echo "Captured PDF saved to $PDF_DIR/test.pdf"
    else
        echo "Captured data is not a PDF"
        cat "$OUTPUT_FILE"  # Debug non-PDF content
    fi
    ls -lh "$PDF_DIR/test.pdf" || true
else
    echo "No data captured in $OUTPUT_FILE"
fi