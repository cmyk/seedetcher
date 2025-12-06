#!/bin/bash

PDF_DIR="/home/cmyk/PDF"
OUTPUT_FILE="$PDF_DIR/test_output"
USBDEV="/dev/ttyACM1"  # Default USB device
VERBOSE=false

# Parse command-line options
while getopts "v" opt; do
  case "$opt" in
    v) VERBOSE=true ;;
    *) echo "Usage: $0 [-v] [USBDEV]"; exit 1 ;;
  esac
done
shift $((OPTIND - 1))

# Enable debugging only if verbose mode is enabled
if [ "$VERBOSE" = true ]; then
    set -x
fi

# Ensure USBDEV is correctly set if provided as an argument
if [ $# -gt 0 ]; then
    USBDEV="$1"
fi

echo "Listening on $USBDEV..."

# Ensure the output directory exists
mkdir -p "$PDF_DIR"

# Clear buffer to avoid stale data
stty -F "$USBDEV" raw -echo
cat "$USBDEV" > /dev/null 2>/dev/null &
PID=$!
sleep 1
kill $PID 2>/dev/null || true
echo "Buffer cleared" >&2

# Start capturing as soon as data arrives
echo "Waiting for print job at $(date '+%H:%M:%S')..." >&2

if [ "$VERBOSE" = true ]; then
    cat "$USBDEV" | tee "$OUTPUT_FILE" | xxd -g 1 -c 16 &
else
    cat "$USBDEV" > "$OUTPUT_FILE" &
fi

CAPTURE_PID=$!

# Monitor file growth and stop when data stops arriving
LAST_SIZE=0
while true; do
    sleep 0.5
    CURRENT_SIZE=$(stat -c%s "$OUTPUT_FILE")

    if [ "$CURRENT_SIZE" -eq "$LAST_SIZE" ] && [ "$CURRENT_SIZE" -gt 0 ]; then
        echo "No more data received, stopping capture."
        kill "$CAPTURE_PID" 2>/dev/null
        break
    fi
    LAST_SIZE=$CURRENT_SIZE
done

# Ensure processes exit properly
sync
sleep 1

# Check if data was captured
if [ -s "$OUTPUT_FILE" ]; then
    echo "Captured data saved to $OUTPUT_FILE"

    # Detect MIME type
    MIME_TYPE=$(file --mime-type -b "$OUTPUT_FILE")
    echo "Detected MIME type: $MIME_TYPE"

    # Detect full file type
    FILE_TYPE=$(file "$OUTPUT_FILE")
    echo "Detected file type: $FILE_TYPE"

    # Determine extension, only allow PDF, PS, and PCL
    case "$FILE_TYPE" in
        *"PDF document"*) EXT="pdf" ;;
        *"PostScript"*) EXT="ps" ;;
        *"PCL printer data"*) EXT="pcl" ;;  # Now properly detects PCL
        *) 
            echo "Unknown format, ignoring."
            rm -f "$OUTPUT_FILE"
            exit 1
            ;;
    esac
    
    FINAL_FILE="$PDF_DIR/test.$EXT"
    mv "$OUTPUT_FILE" "$FINAL_FILE"
    echo "Saved as $FINAL_FILE"

    # Extra diagnostics (only in verbose mode)
    if [ "$VERBOSE" = true ]; then
        echo "Captured file content (Hex Dump):"
        xxd -g 1 -c 16 "$FINAL_FILE" | head -n 20
    fi

    # cp "$FINAL_FILE" "$PDF_DIR/test_raw.$EXT"
    # echo "Saved raw copy as $PDF_DIR/test_raw.$EXT"
    # ls -lh "$FINAL_FILE" "$PDF_DIR/test_raw.$EXT"

    # Exit cleanly
    exit 0
else
    echo "No data received, exiting."
    exit 1
fi