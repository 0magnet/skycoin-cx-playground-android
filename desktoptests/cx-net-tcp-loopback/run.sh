#!/bin/sh
# The tcp client and server of skycoin-cx tests/test-net-tcp.cx in one
# program on loopback, live and from the replay record.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
: "${CX:?set CX}" "${REPO:?set REPO}" "${TOOLS:?set TOOLS}"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"
"$CX" --enable=net "$here/test-net-tcp.cx" > live.txt 2> err.txt
cmp live.txt "$here/expected/stdout.txt"
"$CX" --enable=net --net-replay="$here/test-net-tcp.netreplay" "$here/test-net-tcp.cx" > replay.txt 2> err.txt
cmp replay.txt "$here/expected/stdout.txt"
echo "tcp on loopback, live and replayed, the expected output"
