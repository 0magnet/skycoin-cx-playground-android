#!/bin/sh
# A CX tcp client talks to net-echo-server, the rig's echo server, in
# another process on 127.0.0.1:47301 over the host's real sockets.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
: "${CX:?set CX}" "${REPO:?set REPO}" "${TOOLS:?set TOOLS}"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"
"$TOOLS/net-echo-server" -addr 127.0.0.1:47301 > server.log 2>&1 &
server=$!
trap 'kill $server; rm -rf "$work"' EXIT
until grep -q echoing server.log; do sleep 0.1; done
"$CX" --enable=net "$here/client.cx" > out.txt 2> err.txt
cmp out.txt "$here/expected/stdout.txt"
echo "tcp through the echo server in another process: $(tail -1 out.txt)"
