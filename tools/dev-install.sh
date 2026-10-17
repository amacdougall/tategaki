#!/bin/sh
# Build the KPM package and (re)install it on the Kindle over SSH.
#
# KPM 0.2.2 refuses to reinstall a package at the same version, so this
# uninstalls first. Uninstalling never touches /mnt/us/tategaki user data.
# Usage: tools/dev-install.sh [ssh-host]
set -e
HOST="${1:-kindle}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PKG="$(python3 "$ROOT/packaging/kpm/build.py")"
NAME="$(basename "$PKG")"
KPM=/var/local/kmc/bin/kpm

scp -q "$PKG" "$HOST:/mnt/us/tategaki/$NAME"
ssh "$HOST" "
    $KPM -y uninstall tategaki </dev/null >/dev/null 2>&1
    $KPM -y install file:///mnt/us/tategaki/$NAME </dev/null 2>&1 | sed 's/\x1b\[[0-9;]*m//g' | grep -E 'ERR|succesfully|successfully'
    rm -f /mnt/us/tategaki/$NAME
"
