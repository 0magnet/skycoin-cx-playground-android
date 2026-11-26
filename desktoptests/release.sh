#!/bin/sh
# release.sh VERSION downloads the skycoin-cx release archive of VERSION for
# this host from GitHub, checks it against the SHA256SUMS of the release,
# unpacks it under build/cx-release/VERSION and prints the path of its cx.
set -eu
version=${1:?usage: release.sh VERSION}
here=$(cd "$(dirname "$0")" && pwd)
os=$(go env GOOS)
arch=$(go env GOARCH)
name=cx-$version-$os-$arch.tar.gz
[ "$os" = windows ] && name=cx-$version-$os-$arch.zip
dir=$here/../build/cx-release/$version
mkdir -p "$dir"
cd "$dir"
if [ ! -x "cx-$version-$os-$arch/bin/cx" ]; then
	base=https://github.com/skycoin/skycoin-cx/releases/download/$version
	curl -fsSLO "$base/$name"
	curl -fsSLO "$base/SHA256SUMS"
	grep " $name\$" SHA256SUMS | sha256sum -c - >&2
	case $name in
	*.zip) unzip -q "$name" ;;
	*) tar xzf "$name" ;;
	esac
fi
echo "$dir/cx-$version-$os-$arch/bin/cx"
