#!/bin/sh
# The udp client and server of skycoin-cx tests/test-net-udp.cx in one
# program on loopback, live and from the replay record.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
: "${CX:?set CX}" "${REPO:?set REPO}" "${TOOLS:?set TOOLS}"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"
"$CX" --enable=net "$here/test-net-udp.cx" > live.txt 2> err.txt
cmp live.txt "$here/expected/stdout.txt"
"$CX" --enable=net --net-replay="$here/test-net-udp.netreplay" "$here/test-net-udp.cx" > replay.txt 2> err.txt
cmp replay.txt "$here/expected/stdout.txt"
echo "udp on loopback, live and replayed, the expected output"
