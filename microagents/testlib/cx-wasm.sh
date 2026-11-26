#!/bin/sh
# cx-wasm.sh runs cx built for js/wasm under Node with the Go toolchain's
# wasm_exec_node.js, so CX=testlib/cx-wasm.sh runs the tests on cx.wasm.
# CX_WASM names the cx.wasm. js/wasm limits the arguments and the
# environment together to 12 KB, so only the variables cx reads are passed.
set -eu
exec_node=$(go env GOROOT)/lib/wasm/wasm_exec_node.js
exec env -i PATH="$PATH" HOME="$HOME" CXPATH="${CXPATH:-$HOME/cx}" node "$exec_node" "${CX_WASM:?set CX_WASM to a cx.wasm}" "$@"
