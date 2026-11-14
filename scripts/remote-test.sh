#!/usr/bin/env bash
# Build a Linux test binary for a package, copy it to the test server and run it.
# Usage: scripts/remote-test.sh internal/topology [test args...]
set -euo pipefail

HOST="${SMTBENCH_HOST:-root@test-host}"
PORT="${SMTBENCH_PORT:-666}"
DIR=/root/smtbench

pkg="$1"; shift
name="$(basename "$pkg")"
cd "$(dirname "$0")/.."
mkdir -p dist
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c -o "dist/$name.test" "./$pkg"
ssh -p "$PORT" "$HOST" "mkdir -p $DIR"
scp -q -P "$PORT" "dist/$name.test" "$HOST:$DIR/"
ssh -p "$PORT" "$HOST" "cd $DIR && chmod +x $name.test && ./$name.test -test.v $*"
