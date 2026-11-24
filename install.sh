#!/usr/bin/env bash
# Install pocketpull from this repository's GitHub Releases without sudo.
set -euo pipefail

readonly repo_url="https://github.com/jsburckhardt/pocketpull"

usage() {
    local text
    IFS= read -r -d '' text <<'EOF' || true
Usage: install.sh [--version vX.Y.Z|X.Y.Z|latest] [--install-dir DIRECTORY]

Install a checksum-verified Linux amd64/arm64 pocketpull release from
https://github.com/jsburckhardt/pocketpull/releases.

  --version VERSION      Exact release (vX.Y.Z or X.Y.Z) or latest (default).
  --install-dir DIR      Destination directory (default: ~/.local/bin).
  -h, --help             Show this help.

Requires bash, curl, tar, gzip, sha256sum and coreutils. Never uses sudo and
never writes credentials, configuration, shell profiles or cron entries.
EOF
    printf '%s' "$text"
}

die() { printf 'install.sh: %s\n' "$*" >&2; exit 1; }

semver='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'
version=latest
install_dir=""
while (($#)); do
    case $1 in
        -h | --help) usage; exit 0 ;;
        --version)
            (($# >= 2)) || die "--version requires a value"
            version=$2; shift 2 ;;
        --install-dir)
            (($# >= 2)) || die "--install-dir requires a value"
            [[ -n $2 ]] || die "--install-dir must not be empty"
            install_dir=$2; shift 2 ;;
        *) die "unknown argument: $1 (see --help)" ;;
    esac
done
if [[ $version != latest ]]; then
    version=${version#v}
    [[ $version =~ $semver ]] || die "version must be latest, vX.Y.Z or X.Y.Z (no prereleases)"
fi
if [[ -z $install_dir ]]; then
    [[ -n ${HOME:-} ]] || die "HOME is not set; pass --install-dir"
    install_dir="$HOME/.local/bin"
fi

for tool in uname curl tar gzip sha256sum mktemp mkdir cp chmod mv rm stat; do
    command -v "$tool" > /dev/null 2>&1 || die "required command not found: $tool"
done

os=$(uname -s)
[[ $os == Linux ]] || die "unsupported operating system: $os (Linux only)"
machine=$(uname -m)
case $machine in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) die "unsupported architecture: $machine (amd64 or arm64 only)" ;;
esac

scratch=""
staged=""
cleanup() {
    if [[ -n $staged ]]; then rm -f -- "$staged"; fi
    if [[ -n $scratch ]]; then rm -rf -- "$scratch"; fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

fetch() {
    curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
        --tlsv1.2 --max-redirs 5 --connect-timeout 15 --max-time 300 --retry 2 "$@"
}

if [[ $version == latest ]]; then
    effective=$(fetch --output /dev/null --write-out '%{url_effective}' "$repo_url/releases/latest") \
        || die "could not resolve the latest release"
    tag=${effective#"$repo_url/releases/tag/v"}
    [[ $tag != "$effective" && $tag =~ $semver ]] \
        || die "latest release did not resolve to a vX.Y.Z tag of $repo_url"
    version=$tag
fi

archive="pocketpull_${version}_linux_${arch}.tar.gz"
base="$repo_url/releases/download/v$version"
scratch=$(mktemp -d -t pocketpull-install.XXXXXXXX)
printf 'Downloading pocketpull v%s for linux/%s\n' "$version" "$arch"
fetch --output "$scratch/archive.tar.gz" "$base/$archive" || die "download failed: $base/$archive"
fetch --output "$scratch/checksums.txt" "$base/checksums.txt" || die "download failed: $base/checksums.txt"

expected=""
matches=0
while IFS= read -r line || [[ -n $line ]]; do
    if [[ $line == *"$archive" ]]; then
        matches=$((matches + 1))
        if [[ $line =~ ^([0-9a-f]{64})\ \ (.*)$ && ${BASH_REMATCH[2]} == "$archive" ]]; then
            expected=${BASH_REMATCH[1]}
        fi
    fi
done < "$scratch/checksums.txt"
[[ $matches == 1 && -n $expected ]] || die "checksums.txt must contain exactly one valid entry for $archive"
actual=$(sha256sum < "$scratch/archive.tar.gz")
actual=${actual%% *}
[[ $actual == "$expected" ]] || die "checksum mismatch for $archive; nothing installed"

members=$(tar -tzf "$scratch/archive.tar.gz" 2> /dev/null) || die "$archive is not a readable tar.gz"
[[ $members == pocketpull ]] || die "$archive must contain only the pocketpull executable"
listing=$(LC_ALL=C tar -tvzf "$scratch/archive.tar.gz" 2> /dev/null)
[[ $listing =~ ^-..x ]] || die "$archive member pocketpull is not a regular executable file"
mkdir -- "$scratch/extract"
tar -xzf "$scratch/archive.tar.gz" -C "$scratch/extract" --no-same-owner -- pocketpull \
    || die "could not extract $archive"
shopt -s dotglob nullglob
extracted=("$scratch/extract"/*)
shopt -u dotglob nullglob
binary="$scratch/extract/pocketpull"
[[ ${#extracted[@]} == 1 && -f $binary && ! -L $binary && $(stat -c %h -- "$binary") == 1 ]] \
    || die "$archive did not extract to a single regular pocketpull file"

mkdir -p -- "$install_dir" || die "could not create $install_dir"
[[ -d $install_dir && -w $install_dir ]] \
    || die "$install_dir is not a writable directory; choose --install-dir (sudo is never used)"
target="$install_dir/pocketpull"
[[ ! -d $target || -L $target ]] || die "$target is a directory"
staged=$(mktemp -- "$install_dir/.pocketpull.install.XXXXXXXX") || die "could not stage in $install_dir"
cp -- "$binary" "$staged" || die "could not stage pocketpull in $install_dir"
chmod 0755 -- "$staged"
mv -f -T -- "$staged" "$target" || die "could not replace $target"
staged=""

printf 'Installed pocketpull v%s (linux/%s) to %s\n' "$version" "$arch" "$target"
case ":${PATH:-}:" in
    *":$install_dir:"*) ;;
    *) printf 'Note: %s is not on PATH; add it in your shell configuration if desired.\n' "$install_dir" >&2 ;;
esac
