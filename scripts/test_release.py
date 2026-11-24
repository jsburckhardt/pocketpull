#!/usr/bin/env python3
"""Release asset publication/recovery tests: real packaging, a fake gh and a real install."""

import base64
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
from test_distribution import FAKE_CURL, HOST_ARCH, INSTALL, PACKAGE, REPO  # noqa: E402

PUBLISH = Path(__file__).resolve().parent / "release-assets.py"
SHA = "a" * 40
OTHER_SHA = "b" * 40
NAMES = ["pocketpull_0.1.0_linux_amd64.tar.gz", "pocketpull_0.1.0_linux_arm64.tar.gz",
         "checksums.txt"]

# Models the GitHub REST responses and gh release subcommands the script uses.
FAKE_GH = r'''#!{python}
import base64, fnmatch, json, os, shutil, sys
args = sys.argv[1:]
state_path = os.environ["FAKE_GH_STATE"]
state = json.load(open(state_path))
with open(os.environ["FAKE_GH_LOG"], "a") as log:
    log.write(json.dumps(args) + "\n")
assert os.environ.get("GH_REPO") == "jsburckhardt/pocketpull", "GH_REPO not set"
def fail(msg, code=1):
    sys.stderr.write(msg + "\n"); sys.exit(code)
def save():
    json.dump(state, open(state_path, "w"))
store = os.environ["FAKE_GH_STORE"]
releases = state["releases"]
if args[:1] == ["api"]:
    path = args[1]
    prefix = "repos/{{owner}}/{{repo}}/"
    if not path.startswith(prefix):
        fail("unexpected api path " + path)
    path = path[len(prefix):]
    if path.startswith("git/ref/tags/"):
        tag = path[len("git/ref/tags/"):]
        if tag not in state["tags"]:
            fail("gh: Not Found (HTTP 404)")
        print(json.dumps({{"ref": "refs/tags/" + tag, "object": state["tags"][tag]}}))
    elif path.startswith("git/tags/"):
        obj = state["annotated"].get(path[len("git/tags/"):])
        if not obj:
            fail("gh: Not Found (HTTP 404)")
        print(json.dumps({{"object": obj}}))
    elif path.startswith("compare/"):
        base, head = path[len("compare/"):].split("...")
        assert head == "main", head
        print(json.dumps({{"status": state["compare"].get(base, "diverged")}}))
    elif path.startswith("contents/.release-please-manifest.json?ref="):
        ref = path.split("=", 1)[1]
        text = state["manifests"].get(ref)
        if text is None:
            fail("gh: Not Found (HTTP 404)")
        print(json.dumps({{"encoding": "base64",
                          "content": base64.encodebytes(text.encode()).decode()}}))
    else:
        fail("unexpected api path " + path)
elif args[:2] == ["release", "view"]:
    tag = args[2]
    if tag not in releases:
        fail("release not found")
    rel = releases[tag]
    print(json.dumps({{"tagName": tag, "isDraft": rel["draft"], "isPrerelease": False,
                      "assets": [{{"name": n}} for n in rel["assets"]]}}))
elif args[:2] == ["release", "list"]:
    print(json.dumps([{{"tagName": t, "isDraft": r["draft"], "isPrerelease": False}}
                      for t, r in releases.items()]))
elif args[:2] == ["release", "download"]:
    tag = args[2]
    pattern = args[args.index("--pattern") + 1]
    target = args[args.index("--dir") + 1]
    names = [n for n in releases[tag]["assets"] if fnmatch.fnmatch(n, pattern)]
    if not names:
        fail("no assets match the file pattern")
    for n in names:
        shutil.copyfile(os.path.join(store, tag, n), os.path.join(target, n))
elif args[:2] == ["release", "upload"]:
    tag = args[2]
    if os.environ.get("FAKE_GH_UPLOAD_FAIL"):
        fail("HTTP 502: upload failed")
    if "--clobber" in args:
        fail("clobber is not allowed in this fixture")
    os.makedirs(os.path.join(store, tag), exist_ok=True)
    for f in args[3:]:
        name = os.path.basename(f)
        if name in releases[tag]["assets"]:
            fail("asset under the same name already exists: " + name)
        shutil.copyfile(f, os.path.join(store, tag, name))
        releases[tag]["assets"].append(name)
        save()
elif args[:2] == ["release", "edit"]:
    tag = args[2]
    for a in args[3:]:
        if a == "--draft=false":
            releases[tag]["draft"] = False
        elif a.startswith("--latest"):
            releases[tag]["latest"] = a != "--latest=false"
        else:
            fail("unexpected edit flag " + a)
    save()
else:
    fail("unexpected gh invocation: " + " ".join(args))
'''

WRITES = ("upload", "edit")


class ReleaseAssets(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.shared = Path(tempfile.mkdtemp(prefix="pocketpull-release-test."))
        cls.assets = cls.shared / "assets"
        result = subprocess.run(["bash", str(PACKAGE), "v0.1.0", str(cls.assets)],
                                capture_output=True, text=True)
        if result.returncode:
            shutil.rmtree(cls.shared)
            raise AssertionError(f"packaging failed:\n{result.stdout}{result.stderr}")

    @classmethod
    def tearDownClass(cls):
        shutil.rmtree(cls.shared)

    def setUp(self):
        self.work = Path(tempfile.mkdtemp(prefix="pocketpull release case."))
        self.addCleanup(shutil.rmtree, self.work)
        self.bin = self.work / "fake bin"
        self.bin.mkdir()
        gh = self.bin / "gh"
        gh.write_text(FAKE_GH.format(python=sys.executable))
        gh.chmod(0o755)
        self.store = self.work / "store"
        self.store.mkdir()
        self.state_path = self.work / "state.json"
        self.log = self.work / "gh.log"
        self.tmp = self.work / "tmp dir"
        self.tmp.mkdir()
        self.state = {
            "releases": {"v0.1.0": {"draft": True, "assets": []}},
            "tags": {"v0.1.0": {"type": "commit", "sha": SHA}},
            "annotated": {},
            "compare": {SHA: "ahead"},
            "manifests": {SHA: '{\n  ".": "0.1.0"\n}\n'},
        }
        self.save()
        self.env = {
            "PATH": f"{self.bin}{os.pathsep}{os.environ['PATH']}",
            "GH_REPO": "jsburckhardt/pocketpull",
            "TMPDIR": str(self.tmp),
            "FAKE_GH_STATE": str(self.state_path),
            "FAKE_GH_LOG": str(self.log),
            "FAKE_GH_STORE": str(self.store),
        }

    def save(self):
        self.state_path.write_text(json.dumps(self.state))

    def load(self):
        return json.loads(self.state_path.read_text())

    def run_script(self, *args, **env):
        return subprocess.run([sys.executable, str(PUBLISH), *args], env={**self.env, **env},
                              capture_output=True, text=True, timeout=60)

    def publish(self, assets=None, tag="v0.1.0", sha=SHA, **env):
        return self.run_script("publish", tag, "--sha", sha, "--assets", str(assets or self.assets),
                               **env)

    def gh_calls(self):
        if not self.log.exists():
            return []
        return [json.loads(line) for line in self.log.read_text().splitlines()]

    def writes(self):
        return [c for c in self.gh_calls() if c[:1] == ["release"] and c[1] in WRITES]

    def seed(self, tag, names, data=None):
        (self.store / tag).mkdir(exist_ok=True)
        for name in names:
            target = self.store / tag / name
            if data is None:
                shutil.copyfile(self.assets / name, target)
            else:
                target.write_bytes(data)
        self.state["releases"][tag]["assets"] = list(names)
        self.save()

    def assert_clean(self):
        self.assertEqual(list(self.tmp.iterdir()), [], "script left temporary files")

    def test_publish_round_trip_installs_released_binary(self):
        self.assertIsNotNone(HOST_ARCH, "round trip needs a Linux amd64/arm64 host")
        result = self.publish()
        self.assertEqual(result.returncode, 0, result.stderr)
        release = self.load()["releases"]["v0.1.0"]
        self.assertEqual(sorted(release["assets"]), sorted(NAMES))
        self.assertEqual((release["draft"], release["latest"]), (False, True))
        for name in NAMES:
            self.assertEqual((self.store / "v0.1.0" / name).read_bytes(),
                             (self.assets / name).read_bytes())
        self.assert_clean()

        # The same files, served as GitHub release downloads, install and report the version.
        releases = self.work / "downloads"
        shutil.copytree(self.store, releases)
        curl = self.bin / "curl"
        curl.write_text(FAKE_CURL.format(python=sys.executable, repo=REPO))
        curl.chmod(0o755)
        home = self.work / "home"
        home.mkdir()
        installed = subprocess.run(
            ["bash", str(INSTALL)], capture_output=True, text=True, timeout=60,
            env={**self.env, "HOME": str(home), "FAKE_RELEASES": str(releases),
                 "FAKE_LATEST": "v0.1.0", "FAKE_CURL_LOG": str(self.work / "curl.log")})
        self.assertEqual(installed.returncode, 0, installed.stderr)
        version = subprocess.run([str(home / ".local/bin/pocketpull"), "--version"],
                                 capture_output=True, text=True)
        self.assertEqual(version.stdout, "pocketpull 0.1.0\n")

        # Rerunning against the complete published release changes nothing.
        before = len(self.writes())
        again = self.publish()
        self.assertEqual(again.returncode, 0, again.stderr)
        self.assertIn("already complete", again.stdout)
        self.assertEqual(len(self.writes()), before)

    def test_recovery_uploads_only_missing_assets_then_publishes(self):
        self.seed("v0.1.0", [NAMES[0]])
        result = self.publish()
        self.assertEqual(result.returncode, 0, result.stderr)
        uploads = [c for c in self.writes() if c[1] == "upload"]
        self.assertEqual(len(uploads), 1)
        self.assertEqual(sorted(Path(f).name for f in uploads[0][3:]), sorted(NAMES[1:]))
        release = self.load()["releases"]["v0.1.0"]
        self.assertFalse(release["draft"])
        self.assertEqual(sorted(release["assets"]), sorted(NAMES))
        self.assert_clean()

    def test_failed_upload_leaves_draft_and_retry_completes(self):
        failed = self.publish(FAKE_GH_UPLOAD_FAIL="1")
        self.assertNotEqual(failed.returncode, 0)
        self.assertIn("upload failed", failed.stderr)
        self.assertTrue(self.load()["releases"]["v0.1.0"]["draft"])
        self.assertFalse([c for c in self.writes() if c[1] == "edit"])
        self.assert_clean()
        retried = self.publish()
        self.assertEqual(retried.returncode, 0, retried.stderr)
        self.assertFalse(self.load()["releases"]["v0.1.0"]["draft"])

    def test_existing_different_asset_is_never_replaced(self):
        self.seed("v0.1.0", [NAMES[1]], data=b"other bytes")
        result = self.publish()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(NAMES[1], result.stderr)
        self.assertIn("differs", result.stderr)
        self.assertEqual(self.writes(), [])
        self.assertEqual((self.store / "v0.1.0" / NAMES[1]).read_bytes(), b"other bytes")
        self.assertTrue(self.load()["releases"]["v0.1.0"]["draft"])
        self.assert_clean()

    def test_older_release_is_published_without_becoming_latest(self):
        self.state["releases"]["v0.2.0"] = {"draft": False, "assets": []}
        self.save()
        result = self.publish()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.load()["releases"]["v0.1.0"]["latest"], False)

    def test_incomplete_or_mismatched_asset_directory_is_rejected_before_writes(self):
        def variant(label, mutate):
            target = self.work / label
            shutil.copytree(self.assets, target)
            mutate(target)
            return target

        cases = {
            "missing archive": lambda d: (d / NAMES[1]).unlink(),
            "extra file": lambda d: (d / "notes.txt").write_text("x"),
            "corrupt archive": lambda d: (d / NAMES[0]).write_bytes(b"corrupt"),
            "foreign checksum entry": lambda d: (d / "checksums.txt").write_text(
                (d / "checksums.txt").read_text() + f"{'0' * 64}  other.tar.gz\n"),
            "wrong version": lambda d: [p.rename(d / p.name.replace("0.1.0", "0.2.0"))
                                        for p in list(d.iterdir()) if p.name.endswith(".tar.gz")],
        }
        for label, mutate in cases.items():
            with self.subTest(case=label):
                result = self.publish(variant(label, mutate))
                self.assertNotEqual(result.returncode, 0)
                self.assertTrue(result.stderr.startswith("release-assets.py: "), result.stderr)
                self.assertEqual(self.writes(), [])
                self.assert_clean()
        self.assertTrue(self.load()["releases"]["v0.1.0"]["draft"])

    def test_resolve_accepts_only_reviewed_release_tags_on_main(self):
        ok = self.run_script("resolve", "v0.1.0")
        self.assertEqual(ok.returncode, 0, ok.stderr)
        self.assertEqual(ok.stdout, f"tag=v0.1.0\nversion=0.1.0\nsha={SHA}\n")
        self.assertEqual(self.run_script("resolve", "v0.1.0", "--sha", SHA).stdout, ok.stdout)

        self.state["tags"]["v0.1.0"] = {"type": "tag", "sha": "c" * 40}
        self.state["annotated"]["c" * 40] = {"type": "commit", "sha": SHA}
        self.save()
        self.assertEqual(self.run_script("resolve", "v0.1.0").stdout, ok.stdout)

        rejected = {
            "short tag": ("v0.1",),
            "missing v": ("0.1.0",),
            "prerelease": ("v0.1.0-rc.1",),
            "branch": ("main",),
            "shell": ("v0.1.0;id",),
            "bad sha": ("v0.1.0", "--sha", "HEAD"),
            "other sha": ("v0.1.0", "--sha", OTHER_SHA),
            "no release": ("v9.9.9",),
        }
        for label, args in rejected.items():
            with self.subTest(case=label):
                result = self.run_script("resolve", *args)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(result.stdout, "")
                self.assertTrue(result.stderr.startswith("release-assets.py: "), result.stderr)

        for label, change in (
            ("off main", lambda s: s["compare"].update({SHA: "diverged"})),
            ("manifest mismatch", lambda s: s["manifests"].update({SHA: '{".": "0.0.9"}'})),
            ("no manifest", lambda s: s["manifests"].clear()),
            ("no tag", lambda s: s["tags"].clear()),
        ):
            with self.subTest(case=label):
                self.state = json.loads(json.dumps(self.state))
                saved = json.loads(json.dumps(self.state))
                change(self.state)
                self.save()
                result = self.run_script("resolve", "v0.1.0")
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(result.stdout, "")
                self.state = saved
                self.save()
        self.assertEqual(self.writes(), [])

    def test_publish_revalidates_tag_before_writing(self):
        self.state["tags"]["v0.1.0"] = {"type": "commit", "sha": OTHER_SHA}
        self.state["compare"][OTHER_SHA] = "ahead"
        self.save()
        result = self.publish()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(OTHER_SHA, result.stderr)
        self.assertEqual(self.writes(), [])


if __name__ == "__main__":
    unittest.main()
