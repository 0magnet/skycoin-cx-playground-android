#!/bin/sh
# The fib program of tests/cx-source-fib15 run from source by cx.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
: "${CX:?set CX}" "${REPO:?set REPO}" "${TOOLS:?set TOOLS}"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"
"$CX" "$REPO/tests/cx-source-fib15/app/main.cx" > out.txt 2> err.txt
cmp out.txt "$REPO/tests/cx-source-fib15/expected/stdout.txt"
echo "fib 0 to 14 from source: the $(wc -c < out.txt) expected bytes"
