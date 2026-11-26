#!/bin/sh
# The cx-forth translation of the CX IR example fib prints the bytes of the
# CX source fib program.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
: "${CX:?set CX}" "${REPO:?set REPO}" "${TOOLS:?set TOOLS}"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"
"$TOOLS/cxforth" -cx fib > out.txt
cmp out.txt "$REPO/tests/cx-source-fib15/expected/stdout.txt"
echo "cxforth -cx fib prints the bytes cx prints for the source"
