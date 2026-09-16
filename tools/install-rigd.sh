#!/usr/bin/env bash
#
# Replace a live rigd deployment without losing anything.
#
# PLAN.md section 28 carries the requirement and the seven clauses. Boris,
# 2026-09-17: "make install must be robust, meaning if there is already one
# that is deployed it should close it elegantly and replace; Use all
# precautions and make sure this is 100% robust and doesn't cause any losses
# or problems."
#
# WHY A SCRIPT AND NOT RECIPE LINES. Every step below has a failure branch and
# one of them is a ROLLBACK, which a Makefile recipe cannot express: each line
# is its own shell and `set -e` ends the target rather than running a repair.
# A deployment that can half-fail needs somewhere to put the other half.
#
# WHAT WAS LOOKED FOR FIRST, because section 38 says to say so. There is no
# library for this: it is three tools that already exist - `install`, rename(2)
# via `mv`, and `systemctl` - and the job is using them in the right ORDER with
# the right checks. `systemd-run`, `systemctl reload-or-restart` and deb/rpm
# packaging were all considered and none of them covers verify-then-roll-back
# for a user unit. `cmd/pkgdeb` builds a package and is a different question.
set -euo pipefail

PREFIX="${1:?prefix}"
UNITDIR="${2:?unitdir}"
BUILD="${3:?build dir}"
VERSION="${4:?version}"

# ⛔ THE UNIT NAME IS A VARIABLE FOR ONE REASON AND IT IS NOT CONFIGURABILITY.
# The rollback below is the branch that matters and it is the branch nobody
# ever runs: a safety net that has never been watched catching anything is not
# evidence of a safety net. RIG_INSTALL_UNIT lets the test point this script at
# a throwaway unit and prove the rollback restores a working deployment.
# Clause 7 is unaffected - it is still ONE exact unit name and never a pattern.
UNIT="${RIG_INSTALL_UNIT:-rigd.service}"
BINDIR="$PREFIX/bin"
STOP_TIMEOUT=30
START_TIMEOUT=30

say() { printf '  %s\n' "$*"; }
die() { printf '\n  REFUSED: %s\n' "$*" >&2; exit 1; }

have_systemd() { command -v systemctl >/dev/null 2>&1 && systemctl --user show-environment >/dev/null 2>&1; }

# ---------------------------------------------------------------- clause 1
# VERIFY THE NEW BUILD BEFORE TOUCHING THE OLD ONE. A broken build must not be
# able to remove a working deployment, so nothing is replaced until the new
# binaries exist and the daemon answers for its own version.
for b in rig rigd; do
	[ -x "$BUILD/$b" ] || die "$BUILD/$b is missing or not executable, so there is nothing to install. The deployment is untouched."
done

got="$("$BUILD/rigd" --version 2>/dev/null | awk '/^product/ {print $2; exit}')" || true
[ -n "$got" ] || die "the new rigd did not answer --version, so it cannot be verified. The deployment is untouched."
[ "$got" = "$VERSION" ] || die "the new rigd reports $got and this build is $VERSION. A binary that disagrees with its own build stamp is not installed."
say "clause 1: new build verified, rigd answers $got"

# ---------------------------------------------------------------- clause 7
# TOUCH ONLY rigd.service, AND BY EXACT UNIT NAME. rig-team.service is a
# separate deployment on its own XDG_STATE_HOME and peers run private daemons
# on their own XDG_RUNTIME_DIR. Stopping "the daemon" by pattern rather than by
# unit name would take a peer's work with it, which is why no pgrep, pkill or
# glob appears anywhere in this file.
was_active=no
if have_systemd && systemctl --user is-active --quiet "$UNIT"; then
	was_active=yes
fi
say "clause 7: acting on $UNIT alone; it is currently active=$was_active"

# ---------------------------------------------------------- clauses 2 and 3
# STOP GRACEFULLY, NEVER KILL, AND WAIT FOR IT. rigd drains on SIGTERM through
# signal.NotifyContext, and that drain is what closes the record store, releases
# the flock and removes the socket. `systemctl stop` sends SIGTERM, so it IS the
# elegant path. A timeout here is a REFUSAL to install, never a licence to
# replace a binary underneath a daemon that is still writing.
if [ "$was_active" = yes ]; then
	say "clause 2: stopping $UNIT with SIGTERM and waiting for the drain"
	systemctl --user stop "$UNIT" || die "systemctl stop $UNIT failed. Nothing has been replaced."
	waited=0
	while systemctl --user is-active --quiet "$UNIT"; do
		[ "$waited" -ge "$STOP_TIMEOUT" ] && die "$UNIT did not stop within ${STOP_TIMEOUT}s. It may be mid-write, so NOTHING has been replaced. Investigate before retrying."
		sleep 1
		waited=$((waited + 1))
	done
	say "clause 3: drained after ${waited}s"
fi

# ---------------------------------------------------------------- clause 4
# REPLACE ATOMICALLY: write beside, then rename(2). `install` over a running
# executable can return ETXTBSY or leave a file that is neither version. A
# rename is atomic and leaves any still-running process holding its own inode,
# so there is no instant at which $BINDIR/rigd is a partial file.
#
# The temporary is in the SAME DIRECTORY on purpose: rename is only atomic
# within one filesystem, and a $TMPDIR on another mount would silently degrade
# this to a copy.
mkdir -p "$BINDIR"
for b in rig rigd; do
	if [ -e "$BINDIR/$b" ]; then
		cp -p "$BINDIR/$b" "$BINDIR/.$b.prev" # clause 5's rollback copy
	fi
	install -Dm755 "$BUILD/$b" "$BINDIR/.$b.new"
	mv -f "$BINDIR/.$b.new" "$BINDIR/$b"
done
say "clause 4: rig and rigd replaced by rename; previous kept as .rig.prev/.rigd.prev"

if have_systemd; then
	[ -f packaging/"$UNIT" ] && install -Dm644 packaging/"$UNIT" "$UNITDIR/$UNIT"
	systemctl --user daemon-reload 2>/dev/null || true
fi

# ------------------------------------------------------- clauses 5 and 6
# START, THEN VERIFY BY DIALLING, AND ROLL BACK IF IT DOES NOT COME UP.
#
# AN `active` UNIT IS NOT EVIDENCE, and this project has the scar: a daemon ran
# happily fourteen commits behind its own tree while every document said the new
# verbs answered. The check is what `rig estate` REPORTS, compared to what was
# just installed.
#
# The one outcome that is not allowed is a machine with no working daemon.
rollback() {
	printf '\n  ROLLING BACK: %s\n' "$1" >&2
	for b in rig rigd; do
		[ -e "$BINDIR/.$b.prev" ] && mv -f "$BINDIR/.$b.prev" "$BINDIR/$b"
	done
	systemctl --user start "$UNIT" 2>/dev/null || true
	printf '  the previous binaries are back and %s was restarted.\n' "$UNIT" >&2
	exit 1
}

if [ "$was_active" = yes ]; then
	systemctl --user start "$UNIT" || rollback "$UNIT did not start on the new binary"
	waited=0
	until systemctl --user is-active --quiet "$UNIT"; do
		[ "$waited" -ge "$START_TIMEOUT" ] && rollback "$UNIT did not become active within ${START_TIMEOUT}s"
		sleep 1
		waited=$((waited + 1))
	done

	dialled=""
	for _ in 1 2 3 4 5 6 7 8 9 10; do
		dialled="$("$BINDIR/rig" estate 2>/dev/null | awk '/^daemon/ {print $2; exit}')" || true
		[ -n "$dialled" ] && break
		sleep 1
	done
	[ -n "$dialled" ] || rollback "the new daemon is active but did not answer rig estate"
	[ "$dialled" = "$VERSION" ] || rollback "the live daemon reports $dialled and $VERSION was just installed - the replacement did not take"
	say "clauses 5+6: dialled, and the live daemon reports $dialled"
fi

# Only now is the rollback copy dead weight. Reaching this line IS the success
# condition: rollback() exits, so there is no path here that rolled back, and a
# flag saying so would be dead code that reads as a live check.
rm -f "$BINDIR/.rig.prev" "$BINDIR/.rigd.prev"

if [ "$was_active" = no ]; then
	say "nothing was deployed, so nothing was started. Enabling stays your call:"
	say "    systemctl --user enable --now $UNIT"
fi
