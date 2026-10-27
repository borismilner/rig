#!/bin/sh
# Install the rig capture extension for this user (plan section 58).
# Usage: shellext/install.sh [--uninstall]
set -eu

uuid="rigcapture@rig"
here=$(cd "$(dirname "$0")" && pwd)
dest="${XDG_DATA_HOME:-$HOME/.local/share}/gnome-shell/extensions/$uuid"

if [ "${1:-}" = "--uninstall" ]; then
    command -v gnome-extensions >/dev/null 2>&1 && gnome-extensions disable "$uuid" 2>/dev/null || true
    if command -v gsettings >/dev/null 2>&1; then
        cur=$(gsettings get org.gnome.shell enabled-extensions 2>/dev/null || true)
        new=$(printf '%s' "$cur" | sed "s/, '$uuid'//; s/'$uuid', //; s/'$uuid'//")
        [ -n "$cur" ] && [ "$new" != "$cur" ] && gsettings set org.gnome.shell enabled-extensions "$new"
    fi
    rm -rf -- "$dest"
    echo "removed $dest"
    exit 0
fi

[ -f "$here/$uuid/extension.js" ] || { echo "install: $here/$uuid/extension.js is missing" >&2; exit 1; }
mkdir -p -- "$dest"
cp -- "$here/$uuid/extension.js" "$here/$uuid/metadata.json" "$dest/"
echo "installed $dest"

# A running Shell has not scanned the new directory, so "gnome-extensions
# enable" fails; the list in gsettings is what the next Shell reads.
if command -v gsettings >/dev/null 2>&1; then
    cur=$(gsettings get org.gnome.shell enabled-extensions 2>/dev/null || echo "@as []")
    case "$cur" in
    *"'$uuid'"*) ;;
    "@as []" | "[]") gsettings set org.gnome.shell enabled-extensions "['$uuid']" ;;
    *) gsettings set org.gnome.shell enabled-extensions "${cur%]}, '$uuid']" ;;
    esac
fi
if command -v gnome-extensions >/dev/null 2>&1 &&
    gnome-extensions info "$uuid" 2>/dev/null | grep -q 'State: .*ACTIVE'; then
    echo "the extension is active"
    exit 0
fi
echo "Log out and log in once: Wayland cannot load a new extension into a running Shell."
