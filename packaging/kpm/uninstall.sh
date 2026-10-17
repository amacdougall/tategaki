#!/bin/sh
# KPM uninstall hook, also run before every upgrade ("$1" = "upgrade").
# Never touches user data in /mnt/us/tategaki/. KPM deletes the package
# directory itself.

SCRIPTLET=/mnt/us/documents/tategaki.sh
if [ -f "$SCRIPTLET" ] && [ "$(md5sum < "$SCRIPTLET")" = "$(md5sum < ./scriptlets/tategaki.sh)" ]; then
    echo "Removing Library scriptlet"
    rm -f "$SCRIPTLET"
fi
exit 0
