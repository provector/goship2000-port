#!/usr/bin/env bash
# Regular files keep early failures and avoid pipe backpressure from native logging.
set -euo pipefail
target=$1
exe=$2
shift 2
cd "$MAC_SMOKE_CWD"
probe="${1:-$exe}"
label="$target/$exe $probe"
status=0
swift "$GITHUB_WORKSPACE/test/macos/window-smoke.swift" "$MAC_SMOKE_APP/$exe" "$@" \
    >"$probe.stdout" 2>"$probe.stderr" || status=$?
for stream in stdout stderr; do
    echo "[$label $stream]"
    cat "$probe.$stream"
    printf '\n'
done
echo "[$label] probe exit $status"
exit "$status"
