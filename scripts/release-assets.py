#!/usr/bin/env python3
"""Validate a Release Please release and attach its verified Linux assets.

Usage:
  release-assets.py baseline --sha SHA
  release-assets.py resolve TAG [--sha SHA]
  release-assets.py publish TAG --sha SHA --assets DIR

baseline succeeds when .release-please-manifest.json at SHA is past the 0.0.0
bootstrap, or when the one-time v0.0.0 baseline tag (never a release) points to a
commit on main, so Release Please derives the first version from history instead
of proposing its fixed initial 1.0.0.

resolve prints tag=, version= and sha= lines (GITHUB_OUTPUT format) only when TAG
is a vX.Y.Z GitHub Release whose tag commit is on main, matches SHA if given, and
records X.Y.Z in .release-please-manifest.json, i.e. a merged release PR.

publish revalidates TAG/SHA, requires DIR to hold exactly the two archives and
checksums.txt for that version with matching SHA-256 sums, uploads only missing
assets (never replacing one; an existing asset must be byte-identical), and then
publishes a draft release, marking it latest only if it is the highest stable
version. Rerunning is safe and finishes a partial upload.

Uses the gh CLI with GH_REPO and GH_TOKEN from the environment.
"""

import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile

BRANCH = "main"
TAG = re.compile(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)")
SHA = re.compile(r"[0-9a-f]{40}")
ARCHES = ("amd64", "arm64")
BASELINE = "0.0.0"
BASELINE_TAG = f"v{BASELINE}"


class Failure(Exception):
    pass


def gh(*args):
    result = subprocess.run(["gh", *args], capture_output=True, text=True)
    if result.returncode:
        raise Failure(f"gh {' '.join(args[:2])} failed: {result.stderr.strip() or result.returncode}")
    return result.stdout


def gh_json(*args):
    try:
        return json.loads(gh(*args))
    except json.JSONDecodeError as err:
        raise Failure(f"gh {' '.join(args[:2])} returned invalid JSON: {err}") from None


def release_info(tag):
    return gh_json("release", "view", tag, "--json", "tagName,isDraft,isPrerelease,assets")


def tag_commit(tag):
    obj = gh_json("api", f"repos/{{owner}}/{{repo}}/git/ref/tags/{tag}")["object"]
    for _ in range(5):
        if obj["type"] != "tag":
            break
        obj = gh_json("api", f"repos/{{owner}}/{{repo}}/git/tags/{obj['sha']}")["object"]
    sha = obj["sha"]
    if obj["type"] != "commit" or not SHA.fullmatch(sha):
        raise Failure(f"{tag} does not point to a commit")
    return sha


def require_on_main(tag, sha):
    status = gh_json("api", f"repos/{{owner}}/{{repo}}/compare/{sha}...{BRANCH}").get("status")
    if status not in ("ahead", "identical"):
        raise Failure(f"{tag} commit {sha} is not on {BRANCH} (compare status {status!r})")


def manifest_version(sha):
    content = gh_json("api", f"repos/{{owner}}/{{repo}}/contents/.release-please-manifest.json?ref={sha}")
    try:
        return json.loads(base64.b64decode(content["content"])).get(".")
    except (KeyError, ValueError, AttributeError) as err:
        raise Failure(f"unreadable .release-please-manifest.json at {sha}: {err}") from None


def require_sha(sha):
    if not SHA.fullmatch(sha):
        raise Failure(f"sha must be a full 40-character lowercase commit SHA: {sha!r}")


def baseline(sha):
    require_sha(sha)
    version = manifest_version(sha)
    if version != BASELINE:
        print(f"Manifest at {sha} records {version}; the v{BASELINE} baseline tag is no longer used")
        return
    try:
        root = tag_commit(BASELINE_TAG)
    except Failure as err:
        raise Failure(
            f"manifest is at the {BASELINE} bootstrap but the {BASELINE_TAG} baseline tag is "
            f"unusable ({err}); without it Release Please ignores history and proposes 1.0.0. "
            f"Create it once on the root commit (see README Releases) and rerun") from None
    require_on_main(BASELINE_TAG, root)
    print(f"Bootstrap baseline {BASELINE_TAG} at {root}; first version derives from later commits")


def resolve(tag, expected_sha=None):
    match = TAG.fullmatch(tag)
    if not match:
        raise Failure(f"tag must be vX.Y.Z without prerelease/build metadata: {tag!r}")
    if tag == BASELINE_TAG:
        raise Failure(f"{tag} is the bootstrap baseline tag, never a release")
    if expected_sha is not None:
        require_sha(expected_sha)
    version = tag[1:]
    release = release_info(tag)
    if release.get("tagName") != tag or release.get("isPrerelease"):
        raise Failure(f"no stable GitHub Release exists for {tag}")
    sha = tag_commit(tag)
    if expected_sha is not None and sha != expected_sha:
        raise Failure(f"{tag} points to {sha}, not the expected release commit {expected_sha}")
    require_on_main(tag, sha)
    found = manifest_version(sha)
    if found != version:
        raise Failure(f"{sha} is not the Release Please commit for {version} (manifest records {found!r})")
    return version, sha


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def check_assets(version, directory):
    archives = [f"pocketpull_{version}_linux_{arch}.tar.gz" for arch in ARCHES]
    names = archives + ["checksums.txt"]
    directory = Path(directory)
    if not directory.is_dir():
        raise Failure(f"asset directory not found: {directory}")
    found = sorted(p.name for p in directory.iterdir())
    if found != sorted(names) or any(not (directory / n).is_file() or (directory / n).is_symlink()
                                     for n in names):
        raise Failure(f"{directory} must contain exactly {', '.join(names)}; found {', '.join(found)}")
    expected = "".join(f"{digest(directory / name)}  {name}\n" for name in archives)
    if (directory / "checksums.txt").read_text() != expected:
        raise Failure("checksums.txt does not match the archives exactly")
    return [directory / name for name in names]


def stable_key(tag):
    match = TAG.fullmatch(tag)
    return tuple(int(part) for part in match.groups()) if match else None


def publish(tag, sha, directory):
    version, sha = resolve(tag, sha)
    files = check_assets(version, directory)
    release = release_info(tag)
    present = {asset["name"] for asset in release.get("assets", [])}
    missing = []
    with tempfile.TemporaryDirectory(prefix="pocketpull-release-assets.") as scratch:
        for path in files:
            if path.name not in present:
                missing.append(path)
                continue
            download = Path(scratch) / path.name
            gh("release", "download", tag, "--pattern", path.name, "--dir", scratch)
            if not download.is_file() or download.read_bytes() != path.read_bytes():
                raise Failure(f"existing release asset {path.name} differs from the verified build; "
                              "not replacing it (inspect and delete it manually before retrying)")
            download.unlink()
    if missing:
        print(f"Uploading {', '.join(p.name for p in missing)} to {tag}")
        gh("release", "upload", tag, *map(str, missing))
        present = {asset["name"] for asset in release_info(tag).get("assets", [])}
        absent = [p.name for p in files if p.name not in present]
        if absent:
            raise Failure(f"{tag} is still missing {', '.join(absent)} after upload")
    if release.get("isDraft"):
        others = [r["tagName"] for r in gh_json("release", "list", "--json",
                                                 "tagName,isDraft,isPrerelease", "--limit", "1000")
                  if not r.get("isDraft") and not r.get("isPrerelease") and r["tagName"] != tag]
        newest = all(stable_key(tag) > key for key in map(stable_key, others) if key)
        gh("release", "edit", tag, "--draft=false", "--latest" if newest else "--latest=false")
        print(f"Published {tag} ({sha}) with {len(files)} verified assets"
              f"{' as latest' if newest else ''}")
    elif missing:
        print(f"Completed assets of published release {tag} ({sha})")
    else:
        print(f"{tag} is already complete; nothing to do")


def main(argv):
    parser = argparse.ArgumentParser(prog="release-assets.py", description=__doc__.split("\n")[0])
    sub = parser.add_subparsers(dest="command", required=True)
    sub.add_parser("baseline").add_argument("--sha", required=True)
    resolve_cmd = sub.add_parser("resolve")
    resolve_cmd.add_argument("tag")
    resolve_cmd.add_argument("--sha")
    publish_cmd = sub.add_parser("publish")
    publish_cmd.add_argument("tag")
    publish_cmd.add_argument("--sha", required=True)
    publish_cmd.add_argument("--assets", required=True)
    args = parser.parse_args(argv)
    if not os.environ.get("GH_REPO"):
        raise Failure("GH_REPO must name the repository (owner/name)")
    if args.command == "baseline":
        baseline(args.sha)
    elif args.command == "resolve":
        version, sha = resolve(args.tag, args.sha)
        sys.stdout.write(f"tag={args.tag}\nversion={version}\nsha={sha}\n")
    else:
        publish(args.tag, args.sha, args.assets)


if __name__ == "__main__":
    try:
        main(sys.argv[1:])
    except Failure as err:
        sys.stderr.write(f"release-assets.py: {err}\n")
        sys.exit(1)
