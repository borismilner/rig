#!/bin/bash
S=$1; cd $S
export XDG_DATA_HOME=$S/data XDG_CONFIG_HOME=$S/config XDG_CACHE_HOME=$S/cache GSETTINGS_BACKEND=keyfile
gsettings set org.gnome.shell enabled-extensions "['rigcapture@rig']"
gnome-shell --headless --wayland --no-x11 --virtual-monitor 1280x720 --wayland-display wayland-rigspike > $S/shell.log 2>&1 &
SH=$!
for i in $(seq 1 30); do [ -S $XDG_RUNTIME_DIR/wayland-rigspike ] && break; sleep 1; done
sleep 4
WAYLAND_DISPLAY=wayland-rigspike GDK_BACKEND=wayland gjs $S/client.js > $S/client.log 2>&1 &
CL=$!
sleep 4
CALL="gdbus call --session -d org.rig.Capture -o /org/rig/Capture -m org.rig.Capture"
ID() { $CALL.ListWindows | python3 -c "import sys,json,ast;s=sys.stdin.read();j=ast.literal_eval(s)[0];print([w[\"id\"] for w in json.loads(j) if w[\"title\"]==\"$1\"][0])"; }
echo "== list"; $CALL.ListWindows
echo "== covered red"; $CALL.CaptureWindow $(ID rigspike-blue) $S/blue-covered.png
echo "== arrange"; $CALL.Arrange; sleep 2
echo "== list after"; $CALL.ListWindows
ID() { $CALL.ListWindows | python3 -c "import sys,json,ast;s=sys.stdin.read();j=ast.literal_eval(s)[0];print([w['id'] for w in json.loads(j) if w['title']=='$1'][0])"; }
for c in red green blue; do echo "== capture $c"; $CALL.CaptureWindow $(ID rigspike-$c) $S/$c.png; done
kill $CL $SH 2>/dev/null; wait 2>/dev/null
