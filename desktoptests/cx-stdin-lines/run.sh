#!/bin/sh
# Three lines on standard input reach CX read() in order.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
: "${CX:?set CX}" "${REPO:?set REPO}" "${TOOLS:?set TOOLS}"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"
printf "hello cx\n2 + 3\nlast line\n" | "$CX" "$here/echo.cx" > out.txt 2> err.txt
cmp out.txt "$here/expected/stdout.txt"
echo "three lines read and answered in order"
