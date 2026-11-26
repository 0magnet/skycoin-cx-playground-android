#!/bin/sh
# Records through the openai stub with the store off, see ../../provider-test.sh.
exec "$(dirname "$0")/../../provider-test.sh" openai off
