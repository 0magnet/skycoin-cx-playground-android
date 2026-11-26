#!/bin/sh
# run-tests.sh runs every test folder of desktoptests against a cx on the
# host: $CX when set, else the release archive of the skycoin-cx version
# that go.mod pins, which release.sh downloads and checks. It builds the
# rig tools the tests use into build/desktoptests. TESTS is a glob of the
# folders to run, default *.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/.." && pwd)
build=$repo/build/desktoptests
mkdir -p "$build"
if [ -z "${CX:-}" ]; then
	CX=$("$here/release.sh" "$(cd "$repo" && go list -m -f '{{.Version}}' github.com/skycoin/skycoin-cx)")
fi
(cd "$repo" && go build -o "$build/programnetwork" ./cmd/programnetwork &&
	go build -o "$build/programblocks" ./cmd/programblocks &&
	go build -o "$build/net-echo-server" ./cmd/net-echo-server)
# cxforth builds in the pinned cx-forth module with its own go.sum, as the
# vendor tree of this repository holds only the packages the app imports.
forth=$(cd "$repo" && GOFLAGS=-mod=mod go list -m -f '{{.Dir}}' github.com/skycoin/skycoin-cx-forth)
(cd "$forth" && go build -o "$build/cxforth" ./cmd/cxforth)
export CX REPO="$repo" TOOLS="$build"
echo "cx: $("$CX" --version 2>&1 | head -1), $CX"
failed=0
for test in "$here"/${TESTS:-*}/; do
	[ -x "$test/run.sh" ] || continue
	name=$(basename "$test")
	if output=$("$test/run.sh" 2>&1); then
		echo "PASS $name: $(echo "$output" | tail -1)"
	else
		echo "FAIL $name"
		echo "$output" | sed 's/^/    /'
		failed=$((failed + 1))
	fi
done
[ "$failed" -eq 0 ] || { echo "$failed test(s) failed"; exit 1; }
echo "all tests passed"
