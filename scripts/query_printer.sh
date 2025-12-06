#!/bin/bash

PRINTER="/dev/usb/lp0"
ESC="\x1b"
UEL="${ESC}%-12345X"
#PJL_COMMAND="${UEL}@PJL INFO STATUS\r\n${UEL}"

PJL_COMMAND="${UEL}@PJL INFO VARIABLES\r\n${UEL}"

if fuser -s "$PRINTER"; then
    echo "Printer is busy. Please wait and try again."
    exit 1
fi

echo "Sending PJL command..."
echo -e "$PJL_COMMAND" > "$PRINTER"

echo "Waiting for response..."
sleep 1

response=$(timeout 2 cat "$PRINTER" 2>/dev/null)

if [ -n "$response" ]; then
    echo "Printer response:"
    echo "$response"
else
    echo "No response received from the printer."
fi

echo -e "$ESC" > "$PRINTER"