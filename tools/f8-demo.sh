#!/usr/bin/env bash
# The F8 owner check in one command: every pane tier the window has, on screen,
# against a throwaway estate, cleaned up afterwards.
#
#   tools/f8-demo.sh              build, start, open the window, wait for Enter
#   tools/f8-demo.sh --seconds 60 the same, but close by itself after 60 s
#   tools/f8-demo.sh --no-build   skip `make build build-rigwindow`
#
# WHAT IT NEVER TOUCHES. rigd runs as a `development` estate, but under a
# PRIVATE XDG_STATE_HOME and XDG_RUNTIME_DIR made by mktemp, so it cannot see
# or reach the real development or production estate: section 37 keys both the
# socket and the store to those two directories and nothing else. The check
# below refuses to start if either one is not inside the temp directory.
#
# WHAT IT KILLS. Only the pids it started, each recorded as it is started, on
# every exit path (Enter, the timer, Ctrl-C, or a failure half-way). It never
# kills by name, so a real rigd, ledger or window on this machine is safe.
#
# Needs a desktop session (a real display, or Xvfb) and the window's build
# dependencies: gtk4 and webkitgtk-6.0 headers, and the frontend's npm packages
# (`make deps-frontend`).
set -euo pipefail

# The whole body is one function, read before any of it runs, so editing or
# replacing this file while a demo is up cannot change what the running copy
# does next (bash otherwise reads a script as it executes it).
main() {

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

seconds=0
build=1
while [ $# -gt 0 ]; do
  case "$1" in
    --seconds) seconds=${2:?--seconds needs a number}; shift 2 ;;
    --no-build) build=0; shift ;;
    -h|--help) sed -n '2,20p' "$0"; exit 0 ;;
    *) echo "f8-demo: unknown argument $1" >&2; exit 2 ;;
  esac
done
case "$seconds" in (*[!0-9]*) echo "f8-demo: --seconds takes a whole number" >&2; exit 2 ;; esac

if [ -z "${DISPLAY:-}" ] && [ -z "${WAYLAND_DISPLAY:-}" ]; then
  echo "f8-demo: no display. Run it in a desktop session, or under xvfb-run." >&2
  exit 2
fi

if [ "$build" = 1 ]; then
  make build build-rigwindow >/dev/null
fi
for b in rigd rig fakeapp ledger abacus rigwindow; do
  [ -x "build/$b" ] || { echo "f8-demo: build/$b is missing; run without --no-build" >&2; exit 1; }
done

demo=$(mktemp -d "${TMPDIR:-/tmp}/rig-f8.XXXXXX")
pids="$demo/pids"
: >"$pids"

cleanup() {
  # Newest first, so the window goes before the programs and they before rigd.
  if [ -s "$pids" ]; then
    tac "$pids" | while read -r pid name; do
      if kill -0 "$pid" 2>/dev/null; then
        kill "$pid" 2>/dev/null || true
        for _ in 1 2 3 4 5 6 7 8 9 10; do kill -0 "$pid" 2>/dev/null || break; sleep 0.2; done
        kill -0 "$pid" 2>/dev/null && kill -9 "$pid" 2>/dev/null || true
        echo "f8-demo: stopped $name (pid $pid)"
      fi
    done
  fi
  rm -rf "$demo"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

export XDG_STATE_HOME="$demo/state" XDG_RUNTIME_DIR="$demo/run"
mkdir -m 0700 -p "$XDG_STATE_HOME" "$XDG_RUNTIME_DIR"
case "$XDG_STATE_HOME:$XDG_RUNTIME_DIR" in
  "$demo"/*:"$demo"/*) ;;
  *) echo "f8-demo: refusing: the XDG directories are not private" >&2; exit 1 ;;
esac

start() { # start <name> <command...>: run it in the background and record its pid
  local name=$1; shift
  "$@" >"$demo/$name.log" 2>&1 &
  echo "$! $name" >>"$pids"
}

start rigd build/rigd --estate development
for _ in $(seq 1 50); do build/rig estate >/dev/null 2>&1 && break; sleep 0.2; done
build/rig estate >/dev/null 2>&1 || { echo "f8-demo: rigd did not answer; log:" >&2; cat "$demo/rigd.log" >&2; exit 1; }

# Ports away from ledger's and abacus's defaults (7451, 7452), so a copy
# already running on this machine does not collide with the demo's.
start fakeapp build/fakeapp
start ledger build/ledger --addr 127.0.0.1:17451
start abacus build/abacus --addr 127.0.0.1:17452
for _ in $(seq 1 50); do
  n=$( (build/rig apps list --json 2>/dev/null || true) | { grep -o '"id": *"\(fakeapp\|ledger\|abacus\)"' || true; } | wc -l)
  [ "$n" -ge 3 ] && break
  sleep 0.2
done
start window build/rigwindow --window

cat <<'EOF'

rig F8 demo: a private development estate, three programs, the window.

The window opens on the Dashboard: "3 programs registered", the three in
"The estate", and "What is deployed" saying the daemon and the window are the
same build (this script builds both from one tree).

The rail on the left lists abacus, fakeapp and ledger (ab, fa, le). Click each:

  fakeapp  GENERATED tier. rig draws the pane itself from what the program
           declared: its identity, version, coverage, services and commands,
           ending "This program declared no pane of its own, which is what
           puts it on the generated tier." No HTML comes from the program.

  ledger   KIT tier. The program's own page (pull-report's shape) in a frame:
           headings, a lead paragraph, a toolbar with a search box and a live
           count, and tables filled by script, drawn with rig's kit elements
           and in the window's theme. Switch light/dark in Settings: the page
           follows, because the window hands it the token set.

  abacus   KIT tier, the hard case (dispatch's CI board): pill toggles that
           filter, skeleton bars while loading, chips and a build matrix
           rather than text cells, and an empty message that changes with the
           filter. Also follows the theme switch.

  EMBEDDED tier: no demo program exists. A program there serves its own HTML
           with its own components and receives only the token set; the
           window side is the same frame and handoff the kit tier uses.

Also check: closing the window ends only the window (the tray is a separate
process, not started by this demo), and the status strip names the
development estate.
EOF

if [ "$seconds" -gt 0 ]; then
  echo "Closing by itself in $seconds s."
  sleep "$seconds"
else
  read -r -p "Press Enter to stop everything and clean up. " _ || true
fi
}

main "$@"
