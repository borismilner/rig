#!/bin/sh
# Real-Shell check of the capture service. Never touches the live session:
# a private bus, a private headless Shell and private XDG directories.
set -eu
repo=$(cd "$(dirname "$0")/../.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
dbus-run-session -- bash "$repo/tools/shellcapture-e2e/inner.sh" "$work" "$repo"
