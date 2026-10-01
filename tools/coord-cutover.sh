#!/usr/bin/env bash
# coord-cutover.sh - the one-off move of every live coord.db from bbolt to
# SQLite, and the deploy of the build that reads it (decision 0251, plan/48
# slice 4). It leaves the repository with cmd/coordconvert.
#
# WHY NOT `make deploy` ALONE: the new rigd refuses a bbolt coord.db, the old
# one cannot read a converted file, and rig-team.service runs the same binary
# on its own estate. So both units stop, all three files convert, the new
# binaries go in, and only then does anything start.
#
# ANY FAILURE AFTER THE STOP PUTS EVERYTHING BACK: each original returns from
# coord.db.bbolt, the old binaries return, and the units that were running
# start again. rigwindow.service is never stopped.
set -Eeuo pipefail
cd "$(dirname "$0")/.."

BIN="$HOME/.local/bin"
FILES=(
	"$HOME/.local/state/rig/estates/production/coord.db"
	"$HOME/.local/state/rig/estates/development/coord.db"
	"$HOME/.local/state/rig-team/rig/estates/production/coord.db"
)
UNITS=(rig-team.service rigd.service)

step() { printf '\n== %s\n' "$*"; }
die() { printf '\ncoord-cutover: %s\n' "$*" >&2; exit 1; }
is_bolt() { [ -s "$1" ] && [ "$(head -c 15 "$1" | tr -d '\0')" != "SQLite format 3" ]; }

step "1/8 preflight - nothing live is touched yet"
[ -z "$(git status --porcelain --untracked-files=no)" ] ||
	die "this tree has uncommitted changes. Nothing was touched."
git fetch -q origin main
[ "$(git merge-base HEAD origin/main)" = "$(git rev-parse origin/main)" ] ||
	die "origin/main has commits this tree lacks: git pull --ff-only first. Nothing was touched."
todo=()
for f in "${FILES[@]}"; do
	if is_bolt "$f"; then
		todo+=("$f")
		echo "  bbolt, will convert: $f"
	else
		echo "  not bbolt, left alone: $f"
	fi
done

step "2/8 build"
make --no-print-directory build >/dev/null
go build -o build/coordconvert ./cmd/coordconvert
VERSION="$(build/rig version | awk '/^product/ {print $2}')"
[ -n "$VERSION" ] || die "the new rig did not report a version. Nothing was touched."
echo "  built $VERSION"

step "3/8 push main, which make deploy requires"
git push origin HEAD:main

step "4/8 stop the daemons that hold the files"
SAVE="$(mktemp -d "${XDG_RUNTIME_DIR:-/tmp}/coord-cutover.XXXXXX")"
cp -p "$BIN/rig" "$BIN/rigd" "$SAVE/"
echo "  current binaries saved in $SAVE"
active=()
for u in "${UNITS[@]}"; do
	if systemctl --user is-active --quiet "$u"; then
		active+=("$u")
		systemctl --user stop "$u"
		echo "  stopped $u"
	fi
done

converted=()
restore() {
	trap - ERR
	printf '\n  FAILED - PUTTING EVERYTHING BACK\n' >&2
	for u in "${UNITS[@]}"; do systemctl --user stop "$u" 2>/dev/null || true; done
	for f in "${converted[@]}"; do
		mv -f "$f" "$f.sqlite-failed" && mv -f "$f.bbolt" "$f" &&
			echo "  restored $f (the new file is $f.sqlite-failed)" >&2
	done
	cp -p "$SAVE/rig" "$SAVE/rigd" "$BIN/"
	for u in "${active[@]}"; do systemctl --user start "$u" || true; done
	echo "  old binaries back; restarted: ${active[*]:-none}" >&2
	exit 1
}
trap restore ERR

step "5/8 convert"
for f in "${todo[@]}"; do
	build/coordconvert "$f"
	converted+=("$f")
done

step "6/8 install the new binaries (units are stopped, so nothing starts)"
make --no-print-directory install >/dev/null

step "7/8 start and verify by dialling"
for u in rigd.service rig-team.service; do
	[[ " ${active[*]} " == *" $u "* ]] || continue
	systemctl --user start "$u"
done
sleep 3
live="$("$BIN/rig" estate | awk '/^daemon/ {print $2; exit}')"
[ "$live" = "$VERSION" ] || { echo "  rigd answers '$live', not $VERSION" >&2; false; }
echo "  rigd       $live"
"$BIN/rig" store list | grep -E '^ *coord' | sed 's/^/  /' || true
if [[ " ${active[*]} " == *" rig-team.service "* ]]; then
	team="$(XDG_RUNTIME_DIR="$XDG_RUNTIME_DIR/rigteam" "$BIN/rig" estate | awk '/^daemon/ {print $2; exit}')"
	[ "$team" = "$VERSION" ] || { echo "  rig-team answers '$team', not $VERSION" >&2; false; }
	echo "  rig-team   $team"
fi
trap - ERR

step "8/8 make deploy - window, tray and rigged programs, verified live"
make --no-print-directory deploy

printf '\nDONE. coord is on SQLite in every estate. The originals are kept:\n'
for f in "${converted[@]}"; do echo "  $f.bbolt"; done
echo "Delete them once you are happy; bbolt then leaves go.mod."
