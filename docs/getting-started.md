# Getting started

`pocketpull` is a native run-once Go CLI, not a scheduler. It lists every account
page from `https://public.heypocketai.com/api/v1/public/recordings?page=1&limit=100`,
increments `page` while `pagination.has_more` is true, deduplicates IDs, and
fetches `/public/recordings/<id>` with Bearer authentication. Details include
transcript and summarizations by the API's defaults. No audio route, date/tag
filter, extra content service, remote edit, or local pruning is used.

## Build from source

Build into a temporary directory and invoke with an explicitly exported secret:

```sh
build_dir="$(mktemp -d)"
trap 'rm -f -- "$build_dir/pocketpull"; rmdir -- "$build_dir"' EXIT
CGO_ENABLED=0 go build -o "$build_dir/pocketpull" ./cmd/pocketpull
"$build_dir/pocketpull" --help
"$build_dir/pocketpull" --version
# Bash: supply your key without placing it in shell history.
read -r -s -p 'Pocket API key: ' POCKETPULL_API_KEY
printf '\n'
export POCKETPULL_API_KEY
"$build_dir/pocketpull" --raw-dir "$HOME/pocket-raw"
unset POCKETPULL_API_KEY
```

The binary needs no Go/Python/Node runtime on the collection host. To build
either Linux target, use `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o
pocketpull-amd64 ./cmd/pocketpull` or the same command with `GOARCH=arm64` and
`-o pocketpull-arm64`. Build tools are needed only on the build host; choose the
binary matching the collection host's architecture.

## Install a release

> **Status:** no pocketpull release has been published yet. Packaging, the
> installer and release publication are tested locally against fake release
> downloads and a fake `gh`; the first real release exists only after the owner
> merges the first Release Please PR (see [Releases](development/ci-cd.md#releases)). Until then
> `install.sh` fails; build from source as shown above.

## Installing a release

`install.sh` installs the Linux amd64 or arm64 binary (`x86_64`/`aarch64` are
normalised) from this repository's
[GitHub Releases](https://github.com/jsburckhardt/pocketpull/releases):

```sh
curl -fsSL --proto '=https' -o install.sh \
  https://raw.githubusercontent.com/jsburckhardt/pocketpull/main/install.sh
less install.sh   # review before running
bash install.sh                                  # latest into ~/.local/bin
bash install.sh --version v1.2.3                 # exact release (1.2.3 also accepted)
bash install.sh --install-dir "$HOME/my tools"   # custom destination
```

Interface: `install.sh [--version vX.Y.Z|X.Y.Z|latest] [--install-dir DIRECTORY]`,
defaulting to `latest` and `~/.local/bin`; `--help` exits before any dependency,
platform or network check. Prerelease and path-like versions are rejected.
`latest` is resolved once to an exact `vX.Y.Z` tag; that release's archive and
`checksums.txt` are then downloaded over HTTPS only (finite redirects, connect
and total timeouts, HTTP errors fail). Requirements: Linux amd64/arm64, bash,
curl, GNU tar, gzip, `sha256sum` and coreutils (`mktemp`, `stat`, `mv`, …). The
installed binary needs no Go, Python or other application runtime.

Before anything is executed or installed, the installer requires exactly one
`checksums.txt` entry for the exact archive name, compares its SHA-256, and
checks that the archive contains only one regular executable named
`pocketpull` (no paths, links, directories or extra members). Download,
verification and extraction happen in a private `$TMPDIR` directory removed on
exit or interruption. The binary is staged in the destination directory and
atomically renamed over `pocketpull`, so a failed download, checksum, archive or
copy leaves an existing installation unchanged. It never uses sudo and never
writes credentials, configuration, shell profiles or cron entries; if the
destination is not on `PATH` it only prints a note, and an unwritable
destination fails (choose another `--install-dir`).

Checksum trust: `checksums.txt` is fetched from the same GitHub Release as the
archive, so verification proves integrity relative to that repository release
(corruption, truncation, mismatched asset). It does not independently prove
publisher identity; there is no separate signing infrastructure. Trust rests on
GitHub HTTPS and this repository's release permissions.

## Versions

Releases are tagged `vX.Y.Z` (no prereleases). Archive names and the reported
version omit the `v`: `pocketpull --version` prints `pocketpull X.Y.Z` for
release `vX.Y.Z`, while source builds report `pocketpull dev`. Include that
output and your architecture (`uname -m`) when reporting problems.
