#!/usr/bin/env bash
#
# Restart the window so a redeployment reaches the screen.
#
# PLAN.md section 28: "Re-deployment should elegantly gracefully close the live
# instances and deploy the new ones instead of them" - Boris, 2026-09-18.
#
# ⛔ THE DEFECT THIS EXISTS FOR, MEASURED ON HIS MACHINE THE DAY HE ASKED.
# `make install-window` wrote a new binary at 17:09 and the tray kept running
# the one it started with at 11:04: `/proc/<pid>/exe` read
# `.../rigwindow (deleted)`. Linux holds the old inode open, so the window ran
# on for six hours with no path on disk and nothing said so - the installed
# file was current and the screen was not. An install that reports `installed`
# and leaves the old process running is the daemon skew defect with the
# surfaces swapped.
#
# ⛔ WHAT IT WILL NOT DO. It does not START a window that was not running, for
# the same reason `install` does not enable a unit: replacing what is deployed
# and deciding to deploy are different acts. It does not touch any unit but the
# one named, because rig-team and a peer's private daemon are separate
# deployments and stopping "the window" by pattern would take them with it.
#
# WHAT WAS LOOKED FOR FIRST (section 38). `systemctl restart` is the graceful
# path and already exists: it sends SIGTERM, waits, and starts the unit from
# the file on disk. Nothing needed writing except the CHECK that the new
# process is not on a deleted inode, which is the half that was missing.
set -euo pipefail

UNIT="${1:?unit}"

say() { printf '  %s\n' "$*"; }

if ! command -v systemctl >/dev/null 2>&1; then
	say "no systemctl here, so the window is not under a unit: restart it by hand."
	exit 0
fi

# exe_of prints the executable a pid is running, with Linux's " (deleted)"
# suffix intact - that suffix is the whole measurement.
exe_of() {
	local pid="$1"
	[ -n "$pid" ] && [ "$pid" != "0" ] || return 0
	readlink "/proc/$pid/exe" 2>/dev/null || true
}

state="$(systemctl --user is-active "$UNIT" 2>/dev/null || true)"
before_pid="$(systemctl --user show "$UNIT" -p MainPID --value 2>/dev/null || echo 0)"
before_exe="$(exe_of "$before_pid")"

if [ "$state" != "active" ]; then
	say "$UNIT is $state - nothing live to replace, and starting it is not this"
	say "target's call. The new binary is installed and will be used next start."
	exit 0
fi

say "$UNIT is active, pid $before_pid"
case "$before_exe" in
*"(deleted)") say "  and it is running a DELETED inode: the screen is older than the file" ;;
"") say "  (its executable could not be read)" ;;
*) say "  running $before_exe" ;;
esac

say "stopping it gracefully and starting the new one"
systemctl --user restart "$UNIT"

# ⛔ BOUNDED, AND A TIMEOUT IS A RESULT. A restart that has not produced a new
# main pid is not a restart, and reporting one would be the exact claim this
# script exists to stop being made without evidence.
after_pid=""
for _ in $(seq 1 50); do
	after_pid="$(systemctl --user show "$UNIT" -p MainPID --value 2>/dev/null || echo 0)"
	if [ -n "$after_pid" ] && [ "$after_pid" != "0" ] && [ "$after_pid" != "$before_pid" ]; then
		break
	fi
	sleep 0.2
done

if [ -z "$after_pid" ] || [ "$after_pid" = "0" ] || [ "$after_pid" = "$before_pid" ]; then
	say "⛔ $UNIT did not come back with a new pid within 10s."
	say "   systemctl --user status $UNIT"
	exit 1
fi

after_exe="$(exe_of "$after_pid")"
case "$after_exe" in
*"(deleted)")
	say "⛔ the new pid $after_pid is ALSO on a deleted inode, so it did not pick"
	say "   up the file that was just installed."
	exit 1
	;;
esac
say "restarted: pid $before_pid -> $after_pid, running $after_exe"
