#!/usr/bin/env bash
# Build a Linux test binary for a package, copy it to the test server and run it.
# Usage: scripts/remote-test.sh internal/topology [test args...]
set -euo pipefail

HOST="${SMTBENCH_HOST:-root@test-host}"
PORT="${SMTBENCH_PORT:-666}"
DIR=/root/smtbench
SSH=(ssh -o ConnectTimeout=15 -o ServerAliveInterval=10 -o ServerAliveCountMax=3 -p "$PORT" "$HOST")
export MSYS_NO_PATHCONV=1

pkg="$1"; shift
name="$(basename "$pkg")"
cd "$(dirname "$0")/.."
mkdir -p dist
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c -o "dist/$name.test" "./$pkg"
"${SSH[@]}" "mkdir -p $DIR && rm -f $DIR/$name.test && cat > $DIR/$name.test && chmod +x $DIR/$name.test" < "dist/$name.test"
"${SSH[@]}" "cd $DIR && ./$name.test -test.v $*"
