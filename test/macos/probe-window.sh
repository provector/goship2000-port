#!/usr/bin/env bash
# Regular files keep early failures and avoid pipe backpressure from native logging.
set -euo pipefail
target=$1
exe=$2
shift 2
cd "$MAC_SMOKE_CWD"
label="$target/$exe"
status=0
swift "$GITHUB_WORKSPACE/test/macos/window-smoke.swift" "$MAC_SMOKE_APP/$exe" "$@" \
    >"$exe.stdout" 2>"$exe.stderr" || status=$?
for stream in stdout stderr; do
    echo "[$label $stream]"
    cat "$exe.$stream"
    printf '\n'
done
echo "[$label] probe exit $status"
exit "$status"
