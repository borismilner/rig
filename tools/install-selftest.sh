#!/usr/bin/env bash
#
# Prove install-rigd.sh's ROLLBACK actually catches, on a throwaway unit.
#
# ⛔ WHY THIS EXISTS. The rollback is the branch that protects the one outcome
# section 28 forbids - a machine with no working daemon - and it is the branch a
# successful install never runs. A safety net nobody has watched catch anything
# is not evidence of a safety net, it is the tenth instance of this project's
# most-paid-for defect wearing a reassuring name.
#
# ⛔ IT TOUCHES NOTHING REAL. Its own PREFIX under a temp dir, its own unit name
# via RIG_INSTALL_UNIT, its own XDG_RUNTIME_DIR. rigd.service, rigwindow.service
# and rig-team.service are never named and never signalled. The unit is removed
# on exit whichever way this ends.
set -euo pipefail

UNIT=rigd-selftest.service
UNITDIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
WORK="$(mktemp -d)"
RUNDIR="$(mktemp -d -p /run/user/"$(id -u)" rig-selftest-XXXX)"
FAILED=0

cleanup() {
	systemctl --user stop "$UNIT" 2>/dev/null || true
	rm -f "$UNITDIR/$UNIT"
	systemctl --user daemon-reload 2>/dev/null || true
	rm -rf "$WORK" "$RUNDIR"
}
trap cleanup EXIT

ok()  { printf '  PASS  %s\n' "$*"; }
bad() { printf '  FAIL  %s\n' "$*"; FAILED=$((FAILED + 1)); }

# ⛔ check IS if-then-else AND THAT MATTERS. `cond && ok ... || bad ...` runs
# `bad` whenever `ok` returns non-zero, so a reporting helper's exit status
# would silently become part of the verdict. shellcheck SC2015 names it.
check() { # check <message> <command...>
	msg="$1"; shift
	if "$@"; then ok "$msg"; else bad "$msg"; fi
}
same() { [ "$1" = "$2" ]; }

mkdir -p "$WORK/bin" "$WORK/good" "$WORK/bad"

# The GOOD deployment: the real binaries, so the daemon this test rolls back TO
# is a daemon that genuinely works.
cp build/rig build/rigd "$WORK/good/"
cp build/rig build/rigd "$WORK/bin/"
GOOD_SUM="$(sha256sum "$WORK/bin/rigd" | cut -d' ' -f1)"
VERSION="$("$WORK/good/rigd" --version | awk '/^product/ {print $2}')"

# The BAD build: it answers --version correctly, so it PASSES clause 1, and
# then refuses to serve. That is the shape the rollback exists for - a binary
# that looks installable and is not - and it is the shape a version check alone
# would wave through.
cat > "$WORK/bad/rigd" <<EOF
#!/bin/sh
[ "\$1" = "--version" ] && { echo "product $VERSION"; echo "wire    v1"; exit 0; }
exit 1
EOF
cp "$WORK/bad/rigd" "$WORK/bad/rig"
chmod +x "$WORK/bad/rigd" "$WORK/bad/rig"

cat > "$UNITDIR/$UNIT" <<EOF
[Unit]
Description=rig install self-test - throwaway, removed by tools/install-selftest.sh
[Service]
Environment=XDG_RUNTIME_DIR=$RUNDIR
ExecStart=$WORK/bin/rigd --estate=development
Restart=no
EOF
systemctl --user daemon-reload
systemctl --user start "$UNIT"
sleep 2
systemctl --user is-active --quiet "$UNIT" || { echo "the self-test's own daemon did not start; the test did not run"; exit 1; }
ok "a working deployment is up on $UNIT"

# ---------------------------------------------------------------------------
echo
echo "CASE 1 - a build that verifies and then will not serve MUST roll back"
set +e
RIG_INSTALL_UNIT="$UNIT" tools/install-rigd.sh "$WORK" "$UNITDIR" "$WORK/bad" "$VERSION" > "$WORK/out.log" 2>&1
RC=$?
set -e
check "install refused (exit $RC) rather than deploying a daemon that cannot serve" test "$RC" -ne 0
check "it announced the rollback" grep -q "ROLLING BACK" "$WORK/out.log"

NOW_SUM="$(sha256sum "$WORK/bin/rigd" | cut -d' ' -f1)"
check "the WORKING rigd is back, byte-identical" same "$NOW_SUM" "$GOOD_SUM"

sleep 2
check "the deployment is running again, so the machine is never left without a daemon" systemctl --user is-active --quiet "$UNIT"

# ---------------------------------------------------------------------------
echo
echo "CASE 2 - a missing build MUST refuse without touching the deployment"
set +e
RIG_INSTALL_UNIT="$UNIT" tools/install-rigd.sh "$WORK" "$UNITDIR" "$WORK/nonexistent" "$VERSION" > "$WORK/out2.log" 2>&1
RC=$?
set -e
check "install refused a build directory that does not exist (exit $RC)" test "$RC" -ne 0
check "the refusal said what it left behind" grep -q "untouched" "$WORK/out2.log"
check "the deployed rigd is unchanged by the refusal" same "$(sha256sum "$WORK/bin/rigd" | cut -d' ' -f1)" "$GOOD_SUM"

# ---------------------------------------------------------------------------
echo
echo "CASE 3 - a build whose stamp disagrees with the version MUST refuse"
set +e
RIG_INSTALL_UNIT="$UNIT" tools/install-rigd.sh "$WORK" "$UNITDIR" "$WORK/good" "v9.9.9-not-this-build" > "$WORK/out3.log" 2>&1
RC=$?
set -e
check "install refused a binary disagreeing with its own build stamp (exit $RC)" test "$RC" -ne 0
check "the deployed rigd is unchanged by the refusal" same "$(sha256sum "$WORK/bin/rigd" | cut -d' ' -f1)" "$GOOD_SUM"

echo
if [ "$FAILED" -eq 0 ]; then
	echo "  install-selftest: every case passed."
	echo
	echo "  The cases were watched FAILING before they were trusted: breaking"
	echo "  install-rigd.sh's rollback so it does not restore turns CASE 1 red"
	echo "  on the byte-identical check and on the still-running check. Re-run"
	echo "  that mutation if you ever change either script - a self-test nobody"
	echo "  has seen fail is the thing this file exists to refuse."
else
	echo "  install-selftest: $FAILED FAILED"
	exit 1
fi
