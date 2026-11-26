#!/bin/sh
# provider-test.sh PROVIDER MEMO records femtoclaw and zclaw on the inbound
# messages of their before-after tests through backendstore serve -provider
# PROVIDER, with the stub in that provider's format as the model and the
# store on (MEMO=on) or off (MEMO=off). It checks that two replays of the
# tape print the recorded output, that the tape has the record kinds of the
# committed anthropic tape in the same order, so the agent made the same
# calls, and with the store on that the unmodified original gets every
# request answered by it. mimiclaw is left out because its recording makes
# a live HEAD request for the clock.
set -eu
provider=$1
memo=$2
here=$(cd "$(dirname "$0")/.." && pwd)
: "${CX:?set CX}" "${BACKENDSTORE:?set BACKENDSTORE}" "${FEMTOCLAW_BEFORE:?set FEMTOCLAW_BEFORE}" "${ZCLAW_BEFORE:?set ZCLAW_BEFORE}"
work=$(mktemp -d)
pids=
cleanup() { [ -z "$pids" ] || kill $pids 2> /dev/null || true; rm -rf "$work"; }
trap cleanup EXIT

kinds() { cut -f1 "$1"; }

for agent in femtoclaw zclaw; do
	test_dir=$here/$agent/tests/before-after
	run=$work/$agent
	mkdir -p "$run"
	cd "$run"
	[ -d "$test_dir/spiffs" ] && cp -r "$test_dir/spiffs" spiffs
	cp "$test_dir/inbound.txt" inbound.txt
	"$BACKENDSTORE" stub -format "$provider" -address-file stub.url &
	stub_pid=$!
	while [ ! -s stub.url ]; do sleep 0.1; done
	store_args="-store store.bolt -mode record"
	[ "$memo" = on ] || store_args="-memo=false"
	"$BACKENDSTORE" serve $store_args -provider "$provider" -listen 127.0.0.1:18080 \
		-upstream "$(cat stub.url)/v1/messages" -api-key-env "" -address-file store.url &
	store_pid=$!
	pids="$stub_pid $store_pid"
	while [ ! -s store.url ]; do sleep 0.1; done
	"$CX" --enable=file,net,process "$here/$agent/port" ++mode=record ++tape=tape.txt ++inbound=inbound.txt \
		"++client=$BACKENDSTORE" "++backend=$(cat store.url)" > recorded.txt 2> record-stderr.txt || {
		cat recorded.txt record-stderr.txt; echo "$agent: the recording failed" >&2; exit 1; }
	"$BACKENDSTORE" stop -url "$(cat store.url)"
	"$BACKENDSTORE" stop -url "$(cat stub.url)"
	wait $store_pid $stub_pid
	pids=
	for replay in first second; do
		"$CX" --enable=file,net,process "$here/$agent/port" ++mode=replay ++tape=tape.txt > "$replay.txt" 2> "$replay-stderr.txt" || {
			cat "$replay-stderr.txt"; echo "$agent: the $replay replay failed" >&2; exit 1; }
	done
	cmp first.txt second.txt
	cmp first.txt recorded.txt
	kinds tape.txt > kinds.txt
	kinds "$test_dir/tape.txt" > kinds-anthropic.txt
	cmp kinds.txt kinds-anthropic.txt || { echo "$agent: the tape's records differ from the anthropic tape's" >&2; exit 1; }
	if [ "$memo" = off ]; then
		[ ! -e store.bolt ] || { echo "$agent: -memo=false wrote a store" >&2; exit 1; }
		continue
	fi
	mkdir before
	[ -d "$test_dir/spiffs" ] && cp -r "$test_dir/spiffs" before/spiffs
	"$BACKENDSTORE" serve -store store.bolt -mode replay -provider "$provider" -address-file replay.url 2> replay-store-stderr.txt &
	store_pid=$!
	pids=$store_pid
	while [ ! -s replay.url ]; do sleep 0.1; done
	before_bin=$FEMTOCLAW_BEFORE
	[ "$agent" = zclaw ] && before_bin=$ZCLAW_BEFORE
	before_status=0
	(cd before && "$before_bin" ../tape.txt "$(cat ../replay.url)" "$BACKENDSTORE") > before.txt 2> before-stderr.txt || before_status=$?
	"$BACKENDSTORE" stop -url "$(cat replay.url)"
	store_status=0
	wait $store_pid || store_status=$?
	pids=
	[ "$before_status" -eq 0 ] || { cat before.txt; echo "$agent: the original exited with $before_status" >&2; exit 1; }
	[ "$store_status" -eq 0 ] || { cat replay-store-stderr.txt; echo "$agent: the $provider store missed requests of the original" >&2; exit 1; }
	cmp before.txt first.txt
done
checked="two identical replays and the anthropic record kinds"
[ "$memo" = on ] && checked="$checked, and the original answered by the $provider store"
[ "$memo" = off ] && checked="$checked, and no store written"
echo "$provider stub, store $memo: femtoclaw and zclaw recorded, $checked"
