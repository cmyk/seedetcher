#!/bin/sh
echo "SeedEtcher minimal shell"
while true; do
    echo -n "Enter command: "
    read cmd
    $cmd
done