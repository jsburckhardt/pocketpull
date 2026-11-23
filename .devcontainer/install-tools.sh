#!/usr/bin/env bash
set -euo pipefail

# Expected digests reviewed from official Go downloads and GitHub release metadata.
case "$(dpkg --print-architecture)" in
    amd64)
        go_arch=amd64
        go_sha=7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840
        herdr_arch=x86_64
        herdr_sha=14d9e8bc8b4a9e0118d77c32266f76d461f09b40342eb23752960bd89f59929d
        just_sha=409044617b6f06517c1035e55ca38c8c856ff8b650e41bc2aa1ef82d5903a23e
        ;;
    arm64)
        go_arch=arm64
        go_sha=3392582988a273a87ab5043ee1b2c9c17a2c3c00580bf9326c47a2e0aaaeac92
        herdr_arch=aarch64
        herdr_sha=17d7536f2da50859523680ef279dffcac8e4a235f4e46fe8da6c2ec44141897c
        just_sha=23871b1702bfd550fb5f901778fd4290ecf2ee034a45a889fbe96d89b188086d
        ;;
    *) echo "Unsupported development architecture" >&2; exit 1 ;;
esac

tmp=$(mktemp -d)
trap 'rm -f "$tmp/go.tar.gz" "$tmp/herdr" "$tmp/just.tar.gz" "$tmp/just"; rmdir "$tmp"' EXIT

curl -fsSL --retry 3 "https://go.dev/dl/go1.27.4.linux-${go_arch}.tar.gz" -o "$tmp/go.tar.gz"
echo "$go_sha  $tmp/go.tar.gz" | sha256sum --check
tar -xzf "$tmp/go.tar.gz" -C /usr/local

curl -fsSL --retry 3 "https://github.com/herdrdev/herdr/releases/download/v0.9.5/herdr-linux-${herdr_arch}" -o "$tmp/herdr"
echo "$herdr_sha  $tmp/herdr" | sha256sum --check
install -m 0755 "$tmp/herdr" /usr/local/bin/herdr

curl -fsSL --retry 3 "https://github.com/casey/just/releases/download/1.59.0/just-1.59.0-${herdr_arch}-unknown-linux-musl.tar.gz" -o "$tmp/just.tar.gz"
echo "$just_sha  $tmp/just.tar.gz" | sha256sum --check
tar -xzf "$tmp/just.tar.gz" -C "$tmp" just
install -m 0755 "$tmp/just" /usr/local/bin/just
