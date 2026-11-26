#!/bin/sh
# Records through the anthropic stub with the store off, see ../../provider-test.sh.
exec "$(dirname "$0")/../../provider-test.sh" anthropic off
