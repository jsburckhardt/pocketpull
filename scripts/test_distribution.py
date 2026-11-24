#!/usr/bin/env python3
"""Packaging and installer integration tests using real builds and fake downloads."""

import hashlib
import io
import json
import os
from pathlib import Path
import platform
import shutil
import signal
import struct
import subprocess
import sys
import tarfile
import tempfile
import time
import unittest


ROOT = Path(__file__).resolve().parent.parent
PACKAGE = ROOT / "scripts/package-release.sh"
INSTALL = ROOT / "install.sh"
REPO = "https://github.com/jsburckhardt/pocketpull"
ELF_MACHINE = {"amd64": 62, "arm64": 183}
HOST_ARCH = {"x86_64": "amd64", "aarch64": "arm64"}.get(platform.machine())

FAKE_CURL = r'''#!{python}
import json, os, shutil, sys, time
args = sys.argv[1:]
with open(os.environ["FAKE_CURL_LOG"], "a") as log:
    log.write(json.dumps(args) + "\n")
if os.environ.get("FAKE_CURL_HANG"):
    open(os.environ["FAKE_CURL_HANG"], "w").close()
    time.sleep(60)
url = [a for a in args if a.startswith("https://")][-1]
out = args[args.index("--output") + 1] if "--output" in args else None
repo = "{repo}"
if url == repo + "/releases/latest":
    latest = os.environ.get("FAKE_LATEST", "")
    if not latest:
        sys.stderr.write("curl: (22) The requested URL returned error: 404\n")
        sys.exit(22)
    effective = latest if "://" in latest else repo + "/releases/tag/" + latest
    if "--write-out" in args:
        sys.stdout.write(args[args.index("--write-out") + 1].replace("%{{url_effective}}", effective))
    sys.exit(0)
prefix = repo + "/releases/download/"
source = os.path.join(os.environ["FAKE_RELEASES"], url[len(prefix):])
if not url.startswith(prefix) or not os.path.isfile(source):
    sys.stderr.write("curl: (22) The requested URL returned error: 404\n")
    sys.exit(22)
shutil.copyfile(source, out)
'''

LOGGING_STUB = "#!/bin/sh\necho \"$0 $*\" >> \"$FAKE_FORBIDDEN_LOG\"\nexit 1\n"


def elf_info(data):
    """Return (machine, has_interpreter) for a little-endian ELF64 executable."""
    assert data[:4] == b"\x7fELF" and data[4] == 2, "not ELF64"
    machine = struct.unpack_from("<H", data, 18)[0]
    phoff = struct.unpack_from("<Q", data, 32)[0]
    phentsize, phnum = struct.unpack_from("<HH", data, 54)
    types = [struct.unpack_from("<I", data, phoff + i * phentsize)[0] for i in range(phnum)]
    return machine, 3 in types


def add_member(tar, name, data=b"", mode=0o755, kind=tarfile.REGTYPE, link=""):
    info = tarfile.TarInfo(name)
    info.type, info.mode, info.linkname = kind, mode, link
    info.size = len(data) if kind == tarfile.REGTYPE else 0
    tar.addfile(info, io.BytesIO(data) if kind == tarfile.REGTYPE else None)


def make_archive(path, members):
    with tarfile.open(path, "w:gz") as tar:
        for member in members:
            add_member(tar, **member)


def sha256(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


class Distribution(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.shared = Path(tempfile.mkdtemp(prefix="pocketpull-dist-test."))
        cls.releases = {}
        for version in ("0.1.0", "0.2.0"):
            out = cls.shared / f"out {version}"
            result = subprocess.run(
                ["bash", str(PACKAGE), f"v{version}" if version == "0.2.0" else version, str(out)],
                capture_output=True, text=True,
            )
            if result.returncode:
                shutil.rmtree(cls.shared)
                raise AssertionError(f"packaging failed:\n{result.stdout}{result.stderr}")
            cls.releases[version] = out

    @classmethod
    def tearDownClass(cls):
        shutil.rmtree(cls.shared)

    def setUp(self):
        self.work = Path(tempfile.mkdtemp(prefix="pocketpull dist case."))
        self.addCleanup(shutil.rmtree, self.work)
        self.bin = self.work / "fake bin"
        self.bin.mkdir()
        curl = self.bin / "curl"
        curl.write_text(FAKE_CURL.format(python=sys.executable, repo=REPO))
        curl.chmod(0o755)
        for name in ("sudo", "crontab", "su", "doas"):
            stub = self.bin / name
            stub.write_text(LOGGING_STUB)
            stub.chmod(0o755)
        self.home = self.work / "home with space"
        self.home.mkdir()
        self.tmp = self.work / "tmp with space"
        self.tmp.mkdir()
        self.fake_releases = self.work / "releases"
        for version, out in self.releases.items():
            shutil.copytree(out, self.fake_releases / f"v{version}")
        self.log = self.work / "curl.log"
        self.forbidden = self.work / "forbidden.log"
        self.env = {
            "PATH": f"{self.bin}{os.pathsep}{os.environ['PATH']}",
            "HOME": str(self.home),
            "TMPDIR": str(self.tmp),
            "FAKE_CURL_LOG": str(self.log),
            "FAKE_FORBIDDEN_LOG": str(self.forbidden),
            "FAKE_RELEASES": str(self.fake_releases),
            "FAKE_LATEST": "v0.2.0",
        }

    def fake_uname(self, system="Linux", machine="x86_64"):
        stub = self.bin / "uname"
        stub.write_text(
            f'#!/bin/sh\ncase "$1" in -s) echo {system};; -m) echo {machine};; *) exit 1;; esac\n'
        )
        stub.chmod(0o755)

    def install(self, *args, **env):
        return subprocess.run(
            ["bash", str(INSTALL), *args], env={**self.env, **env},
            capture_output=True, text=True, timeout=60,
        )

    def curl_calls(self):
        if not self.log.exists():
            return []
        return [json.loads(line) for line in self.log.read_text().splitlines()]

    def assert_no_side_effects(self):
        self.assertEqual(list(self.tmp.iterdir()), [], "installer left temporary files")
        self.assertFalse(self.forbidden.exists(), self.forbidden.exists() and self.forbidden.read_text())

    # Packaging

    def test_package_produces_checked_static_archives(self):
        for version, out in self.releases.items():
            names = [f"pocketpull_{version}_linux_{arch}.tar.gz" for arch in ("amd64", "arm64")]
            self.assertEqual(sorted(p.name for p in out.iterdir()), sorted(names + ["checksums.txt"]))
            self.assertEqual(
                (out / "checksums.txt").read_text(),
                "".join(f"{sha256(out / name)}  {name}\n" for name in names),
            )
            for arch, name in zip(("amd64", "arm64"), names):
                with tarfile.open(out / name) as tar:
                    members = tar.getmembers()
                    self.assertEqual([(m.name, m.isreg(), m.mode) for m in members],
                                     [("pocketpull", True, 0o755)])
                    data = tar.extractfile(members[0]).read()
                self.assertEqual(elf_info(data), (ELF_MACHINE[arch], False))
                if arch == HOST_ARCH:
                    binary = self.work / f"pocketpull-{version}"
                    binary.write_bytes(data)
                    binary.chmod(0o755)
                    result = subprocess.run([str(binary), "--version"], capture_output=True,
                                            text=True, env={"PATH": ""})
                    self.assertEqual((result.returncode, result.stdout), (0, f"pocketpull {version}\n"))

    def test_package_rejects_bad_versions_and_existing_output(self):
        for version in ("", "1.2", "01.2.3", "1.2.3-rc.1", "1.2.3+meta", "vv1.2.3",
                        "../1.2.3", "1.2.3/x", "latest", " 1.2.3", "1.2.3\n"):
            with self.subTest(version=version):
                out = self.work / "rejected"
                result = subprocess.run(["bash", str(PACKAGE), version, str(out)],
                                        capture_output=True, text=True)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("version", result.stderr)
                self.assertFalse(out.exists())
        existing = self.work / "existing"
        existing.mkdir()
        (existing / "unrelated").write_text("keep")
        result = subprocess.run(["bash", str(PACKAGE), "1.2.3", str(existing)],
                                capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual([p.name for p in existing.iterdir()], ["unrelated"])
        self.assertEqual(sorted(p.name for p in self.work.iterdir()),
                         sorted(["existing", "fake bin", "home with space", "releases", "tmp with space"]))

    def test_failed_build_publishes_no_partial_output(self):
        go = self.bin / "go"
        go.write_text(
            f'#!/bin/sh\nif [ "$1" = build ] && [ "$GOARCH" = arm64 ]; then echo broken >&2; exit 1; fi\n'
            f'exec "{shutil.which("go")}" "$@"\n'
        )
        go.chmod(0o755)
        parent = self.work / "build parent"
        parent.mkdir()
        result = subprocess.run(["bash", str(PACKAGE), "1.2.3", str(parent / "out")],
                                capture_output=True, text=True, env={**os.environ, "PATH": self.env["PATH"]})
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("broken", result.stderr)
        self.assertEqual(list(parent.iterdir()), [])

    # Installer

    def test_round_trip_latest_into_default_directory(self):
        self.assertIsNotNone(HOST_ARCH, "round trip needs a Linux amd64/arm64 host")
        result = self.install()
        self.assertEqual(result.returncode, 0, result.stderr)
        target = self.home / ".local/bin/pocketpull"
        version = subprocess.run([str(target), "--version"], capture_output=True, text=True)
        self.assertEqual(version.stdout, "pocketpull 0.2.0\n")
        self.assertIn("v0.2.0", result.stdout)
        self.assertEqual(sorted(str(p.relative_to(self.home)) for p in self.home.rglob("*")),
                         [".local", ".local/bin", ".local/bin/pocketpull"])
        calls = self.curl_calls()
        urls = [call[-1] for call in calls]
        self.assertEqual(urls, [
            f"{REPO}/releases/latest",
            f"{REPO}/releases/download/v0.2.0/pocketpull_0.2.0_linux_{HOST_ARCH}.tar.gz",
            f"{REPO}/releases/download/v0.2.0/checksums.txt",
        ])
        for call in calls:
            for flag in ("--fail", "--proto", "=https", "--proto-redir", "--max-redirs",
                         "--connect-timeout", "--max-time"):
                self.assertIn(flag, call)
            self.assertFalse({"-k", "--insecure"} & set(call))
        self.assert_no_side_effects()

    def test_exact_versions_upgrade_and_select_arm64(self):
        self.assertIsNotNone(HOST_ARCH, "upgrade check needs a Linux amd64/arm64 host")
        dest = self.work / "install dir"
        for selected, expected in (("v0.1.0", "0.1.0"), ("0.2.0", "0.2.0")):
            result = self.install("--version", selected, "--install-dir", str(dest))
            self.assertEqual(result.returncode, 0, result.stderr)
            version = subprocess.run([str(dest / "pocketpull"), "--version"],
                                     capture_output=True, text=True)
            self.assertEqual(version.stdout, f"pocketpull {expected}\n")
        self.assertNotIn(f"{REPO}/releases/latest", [call[-1] for call in self.curl_calls()])
        self.assertEqual([p.name for p in dest.iterdir()], ["pocketpull"])

        self.fake_uname(machine="aarch64")
        arm = self.work / "arm dest"
        result = self.install("--install-dir", str(arm), "--version", "0.1.0")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(elf_info((arm / "pocketpull").read_bytes())[0], ELF_MACHINE["arm64"])
        self.assertTrue(os.access(arm / "pocketpull", os.X_OK))
        self.assert_no_side_effects()

    def test_failures_preserve_existing_install_without_executing_download(self):
        marker = self.work / "executed"
        payload = f"#!/bin/sh\ntouch '{marker}'\n".encode()
        release = self.fake_releases / "v9.9.9"
        name = "pocketpull_9.9.9_linux_amd64.tar.gz"

        def publish(members=None, checksum=None, raw=None, skip_archive=False, skip_sums=False):
            shutil.rmtree(release, ignore_errors=True)
            release.mkdir()
            if not skip_archive:
                if raw is not None:
                    (release / name).write_bytes(raw)
                else:
                    make_archive(release / name, members or [{"name": "pocketpull", "data": payload}])
            if not skip_sums:
                digest = sha256(release / name) if not skip_archive else "0" * 64
                (release / "checksums.txt").write_text(
                    checksum(digest) if checksum else f"{digest}  {name}\n")

        cases = {
            "wrong checksum": dict(checksum=lambda d: f"{'0' * 64}  {name}\n"),
            "missing entry": dict(checksum=lambda d: f"{d}  pocketpull_9.9.9_linux_arm64.tar.gz\n"),
            "duplicate entry": dict(checksum=lambda d: f"{d}  {name}\n{d}  {name}\n"),
            "path entry": dict(checksum=lambda d: f"{d}  ../{name}\n"),
            "malformed entry": dict(checksum=lambda d: f"{d[:-1]}  {name}\n"),
            "missing archive": dict(skip_archive=True),
            "missing checksums": dict(skip_sums=True),
            "not gzip": dict(raw=b"not an archive"),
            "extra member": dict(members=[{"name": "pocketpull", "data": payload},
                                          {"name": "extra", "data": b"x"}]),
            "traversal": dict(members=[{"name": "../pocketpull", "data": payload}]),
            "nested": dict(members=[{"name": "bin/pocketpull", "data": payload}]),
            "symlink": dict(members=[{"name": "pocketpull", "kind": tarfile.SYMTYPE,
                                      "link": "/bin/sh"}]),
            "hardlink": dict(members=[{"name": "pocketpull", "kind": tarfile.LNKTYPE,
                                       "link": str(marker)}]),
            "directory": dict(members=[{"name": "pocketpull", "kind": tarfile.DIRTYPE}]),
            "not executable": dict(members=[{"name": "pocketpull", "data": payload, "mode": 0o644}]),
        }
        self.fake_uname()
        dest = self.work / "existing install"
        dest.mkdir()
        old = dest / "pocketpull"
        old.write_text("old binary")
        old.chmod(0o755)

        def assert_preserved(result):
            self.assertNotEqual(result.returncode, 0)
            self.assertTrue(result.stderr.splitlines()[-1].startswith("install.sh: "), result.stderr)
            self.assertEqual(old.read_text(), "old binary")
            self.assertEqual([p.name for p in dest.iterdir()], ["pocketpull"])
            self.assertFalse(marker.exists())
            self.assert_no_side_effects()

        for label, case in cases.items():
            with self.subTest(case=label):
                publish(**case)
                assert_preserved(self.install("--version", "9.9.9", "--install-dir", str(dest)))

        for label, args, env, uname in (
            ("unsupported arch", [], {}, ("Linux", "riscv64")),
            ("unsupported os", [], {}, ("Darwin", "arm64")),
            ("prerelease", ["--version", "1.2.3-rc.1"], {}, None),
            ("path version", ["--version", "../v1.2.3"], {}, None),
            ("missing value", ["--version"], {}, None),
            ("unknown option", ["--url", "https://example.invalid"], {}, None),
            ("no release", [], {"FAKE_LATEST": ""}, None),
            ("bad latest tag", [], {"FAKE_LATEST": "v1.0.0-rc.1"}, None),
            ("foreign latest", [], {"FAKE_LATEST": "https://github.com/evil/repo/releases/tag/v1.0.0"}, None),
        ):
            with self.subTest(case=label):
                self.fake_uname(*(uname or ("Linux", "x86_64")))
                assert_preserved(self.install(*args, "--install-dir", str(dest), **env))
        self.assertFalse((self.home / ".local").exists())

    def test_interrupted_download_cleans_up_and_preserves_install(self):
        dest = self.work / "existing install"
        dest.mkdir()
        old = dest / "pocketpull"
        old.write_text("old binary")
        started = self.work / "started"
        process = subprocess.Popen(
            ["bash", str(INSTALL), "--install-dir", str(dest)],
            env={**self.env, "FAKE_CURL_HANG": str(started)},
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True,
        )
        try:
            deadline = time.monotonic() + 20
            while not started.exists():
                self.assertLess(time.monotonic(), deadline, "fake download never started")
                time.sleep(0.05)
            os.killpg(process.pid, signal.SIGTERM)
            self.assertNotEqual(process.wait(timeout=20), 0)
        finally:
            if process.poll() is None:
                os.killpg(process.pid, signal.SIGKILL)
                process.wait()
            process.stdout.close()
            process.stderr.close()
        self.assertEqual(old.read_text(), "old binary")
        self.assertEqual([p.name for p in dest.iterdir()], ["pocketpull"])
        self.assert_no_side_effects()

    def test_help_needs_no_network_or_dependencies(self):
        empty = self.work / "empty path"
        empty.mkdir()
        result = subprocess.run([shutil.which("bash"), str(INSTALL), "--help"],
                                env={"PATH": str(empty), "HOME": str(self.home)},
                                capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("--install-dir", result.stdout)
        self.assertEqual(self.curl_calls(), [])
        missing = subprocess.run([shutil.which("bash"), str(INSTALL)],
                                 env={**self.env, "PATH": str(empty)}, capture_output=True, text=True)
        self.assertNotEqual(missing.returncode, 0)
        self.assertIn("install.sh: ", missing.stderr)
        self.assertEqual(self.curl_calls(), [])


if __name__ == "__main__":
    unittest.main()
