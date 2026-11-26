#!/bin/sh
# The announce and request network of programnetwork ("DHT prototype
# (#18)" in docs/ProgramDistribution.md) between processes over UDP on
# 127.0.0.1: one node publishes the fib app, a thin node gets it by address,
# a second node subscribes and fetches it from the have, and cx runs it.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
: "${CX:?set CX}" "${REPO:?set REPO}" "${TOOLS:?set TOOLS}"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"
seed="desktop test seed"
signer=$("$TOOLS/programblocks" signer -seed "$seed")
mkdir pub
cp -r "$REPO/tests/cx-source-fib15/app" pub/fib
"$TOOLS/programnetwork" node -addr 127.0.0.1:47310 -store store1 -publish pub -seed "$seed" > node1.log 2>&1 &
node1=$!
node2=
trap 'kill $node1 $node2 2> /dev/null; rm -rf "$work"' EXIT
until grep -q "node listening" node1.log; do sleep 0.1; done
until "$TOOLS/programnetwork" store -store store1 list | grep -q cached; do sleep 0.1; done
address=$("$TOOLS/programnetwork" store -store store1 list | awk "{print \$2}")
"$TOOLS/programnetwork" get -signer "$signer" -out got "$address" 127.0.0.1:47310 > get.txt
grep -q "verified, trusted" get.txt
"$CX" got/main.cx > thin.txt 2> err.txt
cmp thin.txt "$REPO/tests/cx-source-fib15/expected/stdout.txt"
"$TOOLS/programnetwork" node -addr 127.0.0.1:47311 -store store2 -signer "$signer" -subscribe 127.0.0.1:47310 > node2.log 2>&1 &
node2=$!
tries=0
until "$TOOLS/programnetwork" store -store store2 list 2> /dev/null | grep -q "$address"; do
	tries=$((tries + 1))
	[ $tries -lt 100 ] || { cat node2.log; exit 1; }
	sleep 0.1
done
echo "published, fetched by a thin node and by a subscriber, verified, and run: $(head -1 get.txt | cut -d, -f1)"
