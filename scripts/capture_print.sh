#!/bin/bash

PDF_DIR="/home/cmyk/PDF"
OUTPUT_FILE="$PDF_DIR/test_output"
USBDEV="/dev/ttyACM1"  # Default USB device
VERBOSE=false
REPLAY_QUEUE="${REPLAY_QUEUE:-}"
REPLAY_SERVER="${REPLAY_SERVER:-}"
# HBP gadget prints are often sent as multiple batches with render gaps between
# writes. Use a short idle window, but require multiple consecutive idle windows
# before stopping capture.
IDLE_TIMEOUT_SEC="${IDLE_TIMEOUT_SEC:-10}"
IDLE_WINDOWS_REQUIRED="${IDLE_WINDOWS_REQUIRED:-3}"
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

split_concat_raster_streams() {
    ras_file="$1"
    out_prefix="$2"

    # CUPS raster magic:
    # - little-endian: RaS3
    # - big-endian:    3SaR
    offsets_tmp="$(mktemp /tmp/capture_print.XXXXXX.offsets)"
    parts_tmp="$(mktemp /tmp/capture_print.XXXXXX.parts)"
    trap 'rm -f "$offsets_tmp" "$parts_tmp"' RETURN

    {
        LC_ALL=C grep -abo "RaS3" "$ras_file" 2>/dev/null | cut -d: -f1 || true
        LC_ALL=C grep -abo "3SaR" "$ras_file" 2>/dev/null | cut -d: -f1 || true
    } | sort -n | awk 'BEGIN{last=-1} {if($1!=last){print $1; last=$1}}' > "$offsets_tmp"

    # Not concatenated (or no recognizable header offsets): keep single stream.
    if [ ! -s "$offsets_tmp" ] || [ "$(wc -l < "$offsets_tmp")" -le 1 ]; then
        echo "$ras_file" > "$parts_tmp"
        cat "$parts_tmp"
        return 0
    fi

    mapfile -t offs < "$offsets_tmp"
    total_bytes=$(stat -c%s "$ras_file")
    part_idx=1
    emitted=0

    for i in "${!offs[@]}"; do
        start="${offs[$i]}"
        if [ "$start" -ge "$total_bytes" ]; then
            continue
        fi
        if [ "$i" -lt "$((${#offs[@]} - 1))" ]; then
            next="${offs[$((i + 1))]}"
            count=$((next - start))
        else
            count=$((total_bytes - start))
        fi
        if [ "$count" -le 0 ]; then
            continue
        fi

        part_file="${out_prefix}-part${part_idx}.ras"
        dd if="$ras_file" of="$part_file" bs=1 skip="$start" count="$count" status=none
        if file "$part_file" 2>/dev/null | grep -qi "Cups\? Raster"; then
            echo "$part_file" >> "$parts_tmp"
            part_idx=$((part_idx + 1))
            emitted=$((emitted + 1))
        else
            rm -f "$part_file"
        fi
    done

    if [ "$emitted" -eq 0 ]; then
        echo "$ras_file" > "$parts_tmp"
    fi
    cat "$parts_tmp"
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
IDLE_WINDOWS=0
IDLE_LIMIT=$((IDLE_TIMEOUT_SEC * 2))
while true; do
    sleep 0.5
    CURRENT_SIZE=$(stat -c%s "$OUTPUT_FILE")

    if [ "$CURRENT_SIZE" -eq "$LAST_SIZE" ] && [ "$CURRENT_SIZE" -gt 0 ]; then
        IDLE_TICKS=$((IDLE_TICKS + 1))
        if [ "$IDLE_TICKS" -ge "$IDLE_LIMIT" ]; then
            IDLE_WINDOWS=$((IDLE_WINDOWS + 1))
            IDLE_TICKS=0
            if [ "$IDLE_WINDOWS" -ge "$IDLE_WINDOWS_REQUIRED" ]; then
                total_idle=$((IDLE_TIMEOUT_SEC * IDLE_WINDOWS_REQUIRED))
                echo "No more data received for ${total_idle}s (${IDLE_WINDOWS_REQUIRED}x${IDLE_TIMEOUT_SEC}s), stopping capture."
                kill "$CAPTURE_PID" 2>/dev/null
                break
            fi
        fi
    else
        IDLE_TICKS=0
        IDLE_WINDOWS=0
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
        mapfile -t ras_parts < <(split_concat_raster_streams "$FINAL_FILE" "${FINAL_FILE%.ras}")

        if [ "${#ras_parts[@]}" -gt 1 ]; then
            echo "Detected concatenated raster streams: ${#ras_parts[@]} parts."
            pdf_parts=()
            part_num=1
            for p in "${ras_parts[@]}"; do
                out_pdf="${FINAL_FILE%.ras}-from-ras-part${part_num}.pdf"
                convert_ras_to_pdf "$p" "$out_pdf"
                if [ -s "$out_pdf" ]; then
                    pdf_parts+=("$out_pdf")
                fi
                part_num=$((part_num + 1))
            done
            if [ "${#pdf_parts[@]}" -gt 1 ] && command -v pdfunite >/dev/null 2>&1; then
                merged="${FINAL_FILE%.ras}-from-ras.pdf"
                if pdfunite "${pdf_parts[@]}" "$merged"; then
                    echo "Merged raster PDF saved as $merged"
                fi
            elif [ "${#pdf_parts[@]}" -eq 1 ]; then
                cp "${pdf_parts[0]}" "${FINAL_FILE%.ras}-from-ras.pdf"
            fi
        else
            convert_ras_to_pdf "$FINAL_FILE" "${FINAL_FILE%.ras}-from-ras.pdf"
        fi
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
