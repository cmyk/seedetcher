#!/bin/bash

PRINTER="/dev/usb/lp0"
ESC="\x1b"
UEL="${ESC}%-12345X"

if [ -z "$1" ]; then
    echo "Usage: $0 <file.ps or file.pcl>"
    exit 1
fi

FILE="$1"

if [ ! -f "$FILE" ]; then
    echo "File $FILE not found!"
    exit 1
fi

if fuser -s "$PRINTER"; then
    echo "Printer is busy. Please wait and try again."
    exit 1
fi

case "$FILE" in
    *.ps)
        PJL_HEADER="${UEL}@PJL ENTER LANGUAGE = POSTSCRIPT\r\n"
        MODE="PostScript"
        ;;
    *.pcl)
        PJL_HEADER="${UEL}@PJL ENTER LANGUAGE = PCL\r\n"
        PCL_FORMAT="${ESC}E${ESC}&l26A${ESC}&l0O${ESC}&l0E${ESC}&a0L${ESC}&u600D${ESC}&l0S${ESC}*p0x0Y${ESC}&l0M${ESC}&f5950I${ESC}&f8420J${ESC}9${ESC}&l0H${ESC}&l0F"
        MODE="PCL"
        ;;
    *)
        echo "Unsupported file extension. Use .ps or .pcl."
        exit 1
        ;;
esac

echo "Resetting printer..."
echo -e "$ESC" > "$PRINTER"
sleep 0.1

echo "Sending $MODE job: $FILE..."
echo -e "$PJL_HEADER" > "$PRINTER"
if [ "$MODE" = "PCL" ]; then
    echo -e "$PCL_FORMAT" > "$PRINTER"
fi
cat "$FILE" > "$PRINTER"

sleep 2

echo "Checking printer status..."
echo -e "${UEL}@PJL INFO STATUS\r\n${UEL}" > "$PRINTER"
sleep 1
response=$(timeout 2 cat "$PRINTER" 2>/dev/null)
echo "Printer status:"
echo "$response"

echo -e "$ESC" > "$PRINTER"
echo "Job sent in $MODE mode!"