#!/usr/bin/env bash
# Build Linux release archives and checksums for one exact version.
# Usage: scripts/package-release.sh VERSION OUTPUT_DIR
# VERSION is X.Y.Z or vX.Y.Z; OUTPUT_DIR must not exist and appears only when complete.
set -euo pipefail

die() { printf 'package-release.sh: %s\n' "$*" >&2; exit 1; }

(($# == 2)) || die "usage: scripts/package-release.sh VERSION OUTPUT_DIR"
semver='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'
version=${1#v}
[[ $version =~ $semver ]] || die "version must be X.Y.Z or vX.Y.Z without prerelease/build metadata: '$1'"
out=$2
[[ -n $out ]] || die "output directory must not be empty"
[[ ! -e $out && ! -L $out ]] || die "output path already exists; choose a new directory: $out"
parent=$(dirname -- "$out")
[[ -d $parent ]] || die "output parent directory does not exist: $parent"

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
expected_go="go$(sed -n 's/^go //p' "$root/go.mod")"
export GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=linux GOFLAGS=-mod=readonly
actual_go=$(go env GOVERSION) || die "go is required"
[[ $actual_go == "$expected_go" ]] || die "need pinned $expected_go, found $actual_go"

staging=""
cleanup() {
    if [[ -n $staging ]]; then rm -rf -- "$staging"; fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

staging=$(mktemp -d -- "$parent/.pocketpull-release.XXXXXXXX")
staging=$(cd -- "$staging" && pwd)
archives=()
for arch in amd64 arm64; do
    mkdir -- "$staging/$arch"
    GOARCH=$arch go build -C "$root" -trimpath \
        -ldflags "-s -w -X github.com/jsburckhardt/pocketpull/internal/cli.version=$version" \
        -o "$staging/$arch/pocketpull" ./cmd/pocketpull
    archive="pocketpull_${version}_linux_${arch}.tar.gz"
    tar --sort=name --owner=0 --group=0 --numeric-owner --mode=0755 \
        --mtime="@${SOURCE_DATE_EPOCH:-0}" -C "$staging/$arch" -cf - pocketpull \
        | gzip -n -9 > "$staging/$archive"
    rm -rf -- "${staging:?}/$arch"
    archives+=("$archive")
done
(cd -- "$staging" && sha256sum -- "${archives[@]}" > checksums.txt)
chmod 0755 -- "$staging"
mv -T -- "$staging" "$out"
staging=""
printf 'Packaged pocketpull %s into %s:\n' "$version" "$out"
(cd -- "$out" && cat checksums.txt)
