#!/bin/bash
# Runs inside dbus-run-session: a headless Shell with the real extension and
# three coloured windows, then the Go end-to-end test against it.
S=$1; REPO=$2; cd "$S"
export XDG_DATA_HOME=$S/data XDG_CONFIG_HOME=$S/config XDG_CACHE_HOME=$S/cache GSETTINGS_BACKEND=keyfile
mkdir -p "$XDG_DATA_HOME" "$XDG_CONFIG_HOME" "$XDG_CACHE_HOME"
XDG_DATA_HOME=$XDG_DATA_HOME "$REPO/shellext/install.sh" >/dev/null
gsettings set org.gnome.shell enabled-extensions "['rigcapture@rig']"
gnome-shell --headless --wayland --no-x11 --virtual-monitor 1280x720 --wayland-display wayland-rigtest > "$S/shell.log" 2>&1 &
SH=$!
for _ in $(seq 1 30); do [ -S "$XDG_RUNTIME_DIR/wayland-rigtest" ] && break; sleep 1; done
sleep 4
WAYLAND_DISPLAY=wayland-rigtest GDK_BACKEND=wayland gjs "$REPO/tools/shellcapture-e2e/client.js" > "$S/client.log" 2>&1 &
CL=$!
sleep 4
(cd "$REPO" && RIG_SHELL_E2E=1 go test ./internal/capture -run TestRealShell -count=1 -v)
rc=$?
kill $CL $SH 2>/dev/null; wait 2>/dev/null
exit $rc
