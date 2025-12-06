#!/usr/bin/env bash
set -euo pipefail
# Run tests excluding hardware/CGO-dependent packages.
pkgs=$(go list ./... | grep -v 'driver/libcamera' | grep -v 'driver/wshat')
go test "$@" $pkgs
