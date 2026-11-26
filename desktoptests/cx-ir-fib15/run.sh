#!/bin/sh
# The same program in the CX IR form: cx build appends the serialized
# program to a core of the release, and the built file runs with no parser.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
: "${CX:?set CX}" "${REPO:?set REPO}" "${TOOLS:?set TOOLS}"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"
cp "$REPO/tests/cx-source-fib15/app/main.cx" fib.cx
"$CX" build fib.cx -o fib 2> err.txt
./fib > out.txt
cmp out.txt "$REPO/tests/cx-source-fib15/expected/stdout.txt"
echo "fib 0 to 14 from a $(wc -c < fib) byte built file: the $(wc -c < out.txt) expected bytes"
