#!/usr/bin/env bash
# seed-check: the seeder gate as one command (plan/48 acceptance 7).
#
# Builds docket's rigseed, starts a throwaway `development` rigd whose state
# and sockets live in temporary directories, seeds it from this tree's plan
# and README and the logbook's BACKLOG and DECISIONS, runs `rigseed --check`,
# stops that rigd and exits with the check's code. The real estates are never
# opened: RIG_ROOT and XDG_RUNTIME_DIR point elsewhere, and the script stops
# if rigd reports any other state path.
set -euo pipefail

here=$(cd "$(dirname "$0")/.." && pwd)
docket=$(realpath "${DOCKET:-$here/../docket}")
logbook=$(realpath "${LOGBOOK:-$here/../logbook/projects/rig}")
rig=$here/build/rig
rigd=$here/build/rigd

for f in "$rig" "$rigd"; do
	[[ -x $f ]] || { echo "seed-check: $f is not built; run make build" >&2; exit 2; }
done
[[ -d $docket/cmd/rigseed ]] || { echo "seed-check: no rigseed under $docket" >&2; exit 2; }
[[ -f $logbook/BACKLOG.md ]] || { echo "seed-check: no BACKLOG.md under $logbook" >&2; exit 2; }

root=$(mktemp -d)
run=$(mktemp -d /tmp/sc.XXXX) # short: a unix socket path has a 107-byte ceiling
pid=
cleanup() {
	if [[ -n $pid ]] && kill -0 "$pid" 2>/dev/null; then
		kill "$pid"
		for _ in $(seq 50); do kill -0 "$pid" 2>/dev/null || break; sleep 0.1; done
	fi
	rm -rf "$root" "$run"
}
trap cleanup EXIT

(cd "$docket" && go build -o "$root/rigseed" ./cmd/rigseed)

env -u XDG_STATE_HOME RIG_ROOT="$root" XDG_RUNTIME_DIR="$run" \
	"$rigd" --estate=development >"$root/rigd.log" 2>&1 &
pid=$!
opened=
for _ in $(seq 100); do
	opened=$(grep -m1 'estate state opened' "$root/rigd.log" || true)
	[[ -n $opened ]] && break
	kill -0 "$pid" 2>/dev/null || break
	sleep 0.1
done
if [[ -z $opened ]]; then
	echo "seed-check: the scratch rigd did not start:" >&2
	tail -5 "$root/rigd.log" >&2
	exit 2
fi
if [[ $opened != *"$root"* ]]; then
	echo "seed-check: rigd opened state outside $root, stopping:" >&2
	echo "$opened" >&2
	exit 2
fi

args=(--rig "$rig" --estate development
	--backlog "$logbook/BACKLOG.md" --decisions "$logbook/DECISIONS.md"
	--plan-dir "$here/plan" --readme "$here/README.md")
export XDG_RUNTIME_DIR=$run
if ! "$root/rigseed" "${args[@]}" >"$root/seed.log" 2>&1; then
	echo "seed-check: seeding failed:" >&2
	tail -20 "$root/seed.log" >&2
	exit 2
fi
rc=0
"$root/rigseed" --check "${args[@]}" || rc=$?
exit "$rc"
