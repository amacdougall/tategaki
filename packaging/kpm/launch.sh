#!/bin/sh
# KPM launch hook: `kpm launch tategaki [args]`. Runs in the package directory.
#
# Pauses the Kindle UI, runs the reader in the foreground, and restores the UI
# on any exit, including a crash of the reader or a signal to this script.
# The pause/restore sequence follows KOReader's platform/kindle/koreader.sh
# for FW >= 5.12.4: disable pillow, SIGSTOP awesome, and on exit restore the
# framebuffer, SIGCONT awesome, re-enable pillow and reopen the home booklet.

PKG_DIR="$(pwd)"
DATA=/mnt/us/tategaki
mkdir -p "$DATA"
exec >>"$DATA/log.txt" 2>&1 </dev/null

log() { echo "$(date '+%Y-%m-%d %H:%M:%S') launch.sh: $*"; }

PIDFILE=/var/tmp/tategaki.pid
if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then
    log "already running as $(cat "$PIDFILE"), not starting another"
    exit 0
fi
echo $$ >"$PIDFILE"

FBDUMP=/var/tmp/tategaki-fb.dump
RESTORED=no
restore() {
    [ "$RESTORED" = yes ] && return
    RESTORED=yes
    log "restoring Kindle UI"
    killall -CONT awesome
    if [ -f "$FBDUMP" ]; then
        cat "$FBDUMP" >/dev/fb0
        rm -f "$FBDUMP"
    fi
    lipc-set-prop com.lab126.pillow disableEnablePillow enable
    lipc-set-prop com.lab126.appmgrd start app://com.lab126.booklet.home
    rm -f "$PIDFILE"
    log "done"
}
trap restore EXIT
trap 'exit 130' INT TERM HUP

log "starting (package $PKG_DIR, args: $*)"
cat /dev/fb0 >"$FBDUMP"
lipc-set-prop com.lab126.pillow disableEnablePillow disable
killall -STOP awesome

"$PKG_DIR/bin/tategaki" -fbink "$PKG_DIR/bin/fbink" "$@"
log "tategaki exited with status $?"
