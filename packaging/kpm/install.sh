#!/bin/sh
# KPM install hook. Runs in the unpacked package directory; "$1" is
# "upgrade" during an upgrade. Writes only under /mnt/us (no rootfs writes).

mkdir -p /mnt/us/tategaki/books
chmod +x ./launch.sh ./bin/* 2>/dev/null

echo "Installing Library scriptlet"
cp -f ./scriptlets/tategaki.sh /mnt/us/documents/tategaki.sh
chmod +x /mnt/us/documents/tategaki.sh

echo "Tategaki installed. User data lives in /mnt/us/tategaki/."
exit 0
