#!/bin/bash

PDF_DIR="/home/cmyk/PDF"
OUTPUT_FILE="$PDF_DIR/test_output"
USBDEV="/dev/ttyACM1"  # Default USB device
VERBOSE=false
REPLAY_QUEUE="${REPLAY_QUEUE:-}"
REPLAY_SERVER="${REPLAY_SERVER:-}"
IDLE_TIMEOUT_SEC="${IDLE_TIMEOUT_SEC:-3}"
CONVERT_RAS_TO_PDF="${CONVERT_RAS_TO_PDF:-1}"

# Parse command-line options
while getopts "vq:s:" opt; do
  case "$opt" in
    v) VERBOSE=true ;;
    q) REPLAY_QUEUE="$OPTARG" ;;
    s) REPLAY_SERVER="$OPTARG" ;;
    *) echo "Usage: $0 [-v] [-q QUEUE] [-s CUPS_SERVER] [USBDEV]"; exit 1 ;;
  esac
done
shift $((OPTIND - 1))

convert_ras_to_pdf() {
    ras_file="$1"
    pdf_out="$2"
    pwg_tmp="$(mktemp /tmp/capture_print.XXXXXX.pwg)"

    if ! [ -x /usr/lib/cups/filter/rastertopwg ] || ! [ -x /usr/lib/cups/filter/pwgtopdf ]; then
        echo "Skipping RAS->PDF conversion: rastertopwg/pwgtopdf not available."
        rm -f "$pwg_tmp"
        return 0
    fi

    if CONTENT_TYPE=application/vnd.cups-raster \
        /usr/lib/cups/filter/rastertopwg 1 "${USER:-capture}" test 1 "media=A4" \
        "$ras_file" > "$pwg_tmp"; then
        if CONTENT_TYPE=image/pwg-raster \
            /usr/lib/cups/filter/pwgtopdf 1 "${USER:-capture}" test 1 "media=A4" \
            "$pwg_tmp" > "$pdf_out"; then
            echo "Converted raster PDF saved as $pdf_out"
            rm -f "$pwg_tmp"
            return 0
        fi
    fi

    echo "RAS->PDF conversion failed (kept raster at $ras_file)."
    rm -f "$pwg_tmp"
    return 0
}

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

# Monitor file growth and stop only after sustained idle period.
LAST_SIZE=0
IDLE_TICKS=0
IDLE_LIMIT=$((IDLE_TIMEOUT_SEC * 2))
while true; do
    sleep 0.5
    CURRENT_SIZE=$(stat -c%s "$OUTPUT_FILE")

    if [ "$CURRENT_SIZE" -eq "$LAST_SIZE" ] && [ "$CURRENT_SIZE" -gt 0 ]; then
        IDLE_TICKS=$((IDLE_TICKS + 1))
        if [ "$IDLE_TICKS" -ge "$IDLE_LIMIT" ]; then
            echo "No more data received for ${IDLE_TIMEOUT_SEC}s, stopping capture."
            kill "$CAPTURE_PID" 2>/dev/null
            break
        fi
    else
        IDLE_TICKS=0
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

    # Determine extension, allow PDF/PS/PCL/CUPS-raster.
    RAS_MAGIC="$(dd if="$OUTPUT_FILE" bs=4 count=1 2>/dev/null | od -An -tx1 | tr -d ' \n')"
    case "$FILE_TYPE" in
        *"PDF document"*) EXT="pdf" ;;
        *"PostScript"*) EXT="ps" ;;
        *"PCL printer data"*) EXT="pcl" ;;  # Now properly detects PCL
        *"CUPS Raster"*|*"Cups Raster"*|*"cups raster"*) EXT="ras" ;;
        *) 
            if [ "$RAS_MAGIC" = "52615332" ] || [ "$RAS_MAGIC" = "52615333" ]; then
                EXT="ras"
            else
                echo "Unknown format, ignoring."
                rm -f "$OUTPUT_FILE"
                exit 1
            fi
            ;;
    esac
    
    FINAL_FILE="$PDF_DIR/test.$EXT"
    mv "$OUTPUT_FILE" "$FINAL_FILE"
    echo "Saved as $FINAL_FILE"

    if [ "$EXT" = "ras" ] && [ "$CONVERT_RAS_TO_PDF" != "0" ]; then
        convert_ras_to_pdf "$FINAL_FILE" "${FINAL_FILE%.ras}-from-ras.pdf"
    fi

    if [ "$EXT" = "ras" ] && [ -n "$REPLAY_QUEUE" ]; then
        echo "Replaying CUPS raster to queue '$REPLAY_QUEUE'..."
        if [ -n "$REPLAY_SERVER" ]; then
            lp -h "$REPLAY_SERVER" -d "$REPLAY_QUEUE" \
                -o document-format=application/vnd.cups-raster \
                "$FINAL_FILE"
        else
            lp -d "$REPLAY_QUEUE" \
                -o document-format=application/vnd.cups-raster \
                "$FINAL_FILE"
        fi
    fi

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
