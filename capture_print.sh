#!/bin/bash

set -x 

OUTPUT_FILE="test.pcl"
PDF_PRINTER="PDF"
echo "Listening on /dev/ttyACM1..."
cat /dev/ttyACM1 > "$OUTPUT_FILE"
lp -d "$PDF_PRINTER" "$OUTPUT_FILE"
echo "Saved to ~/PDF/test.pdf"