#!/bin/bash

set -x

OUTPUT_FILE="test.pcl"
PDF_DIR="/home/cmyk/PDF"

echo "Listening on /dev/ttyACM1..."
timeout 5 cat /dev/ttyACM1 > "$OUTPUT_FILE"  # 10-second timeout
if [ -s "$OUTPUT_FILE" ]; then
    mkdir -p "$PDF_DIR"
    gpcl6 -dBATCH -dNOPAUSE -sDEVICE=pdfwrite -sOutputFile="$PDF_DIR/test.pdf" "$OUTPUT_FILE"
    echo "Saved to $PDF_DIR/test.pdf"
else
    echo "No data captured in $OUTPUT_FILE"
fi