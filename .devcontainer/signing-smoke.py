#!/usr/bin/env python3
"""Disposable-key integration check; never uses the developer's SSH directory."""

import errno
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import uuid


ROOT = Path(__file__).resolve().parent.parent
KEY_NAME = "pocketpull-disposable-signing-key"


def inside():
    home = Path.home()
    before = [(home / rc).read_bytes() for rc in (".bashrc", ".zshrc")]
    for _ in range(2):
        subprocess.run(["bash", ".devcontainer/post-create.sh"], check=True)
    assert before == [(home / rc).read_bytes() for rc in (".bashrc", ".zshrc")]
    probe = home / ".ssh/pocketpull-readonly-probe"
    try:
        probe.write_text("must not be writable\n")
    except OSError as error:
        assert error.errno == errno.EROFS, error
    else:
        probe.unlink()
        raise AssertionError("SSH mount is writable")

    key = home / ".ssh" / KEY_NAME
    assert key.is_file(), "Disposable signing key is missing"
    with tempfile.TemporaryDirectory(prefix="pocketpull-git-signing-") as directory:
        repo = Path(directory)
        allowed = repo / "allowed_signers"
        allowed.write_text("signing-test@example.invalid " + key.with_suffix(".pub").read_text())

        def git(*args):
            subprocess.run(["git", "-C", directory, *args], check=True)

        git("init", "--quiet")
        for name, value in {
            "user.name": "Disposable signing test",
            "user.email": "signing-test@example.invalid",
            "gpg.format": "ssh",
            "user.signingkey": str(key),
            "gpg.ssh.allowedSignersFile": str(allowed),
            "commit.gpgsign": "true",
        }.items():
            git("config", name, value)
        git("commit", "--allow-empty", "--quiet", "-m", "Disposable SSH signing check")
        git("verify-commit", "HEAD")
    print("PASS: idempotent post-create; SSH mount rejects writes; signed commit verifies")


def host():
    label = "pocketpull.signing-smoke=" + uuid.uuid4().hex
    existing_volumes = set(subprocess.check_output(
        ["docker", "volume", "ls", "--format", "{{.Name}}"], text=True
    ).splitlines())
    volumes = []
    with tempfile.TemporaryDirectory(prefix="pocketpull-signing-") as directory:
        home = Path(directory)
        ssh = home / ".ssh"
        ssh.mkdir(mode=0o700)
        key = ssh / KEY_NAME
        subprocess.run(
            ["ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C",
             "pocketpull-disposable-test", "-f", str(key)], check=True
        )
        original_key = key.read_bytes()
        env = os.environ | {"HOME": directory}
        for name in ("SSH_AUTH_SOCK", "GH_TOKEN", "GITHUB_TOKEN", "COPILOT_GITHUB_TOKEN"):
            env.pop(name, None)
        base = ["--workspace-folder", str(ROOT), "--id-label", label]
        try:
            result = subprocess.run(
                ["devcontainer", "up", *base], env=env, text=True,
                stdout=subprocess.PIPE, check=True
            )
            container = json.loads(result.stdout)["containerId"]
            inspection = json.loads(subprocess.check_output(
                ["docker", "inspect", container], text=True
            ))[0]
            volumes = [mount["Name"] for mount in inspection["Mounts"]
                       if mount["Type"] == "volume"]
            assert not inspection["HostConfig"]["Privileged"]
            assert not inspection["HostConfig"]["CapAdd"]
            assert not inspection["HostConfig"]["SecurityOpt"]
            assert inspection["Config"]["User"] == "vscode"
            binds = {mount["Destination"]: mount for mount in inspection["Mounts"]
                     if mount["Type"] == "bind"}
            assert len(binds) == 2, list(binds)
            ssh_mount = binds["/home/vscode/.ssh"]
            assert ssh_mount["Source"] == str(ssh) and not ssh_mount["RW"]
            assert any(mount["Source"] == str(ROOT) for mount in binds.values())
            print("PASS: inspected non-root, unprivileged container; only workspace/SSH binds",
                  flush=True)
            for command in (
                ["just", "dev-check"],
                ["just", "dev-smoke"],
                ["python3", ".devcontainer/signing-smoke.py", "--inside"],
            ):
                subprocess.run(["devcontainer", "exec", *base, *command],
                               env=env, check=True)
            negative = subprocess.run(
                ["docker", "exec", "--user", "root", "-w",
                 inspection["Config"]["WorkingDir"] or "/", container,
                 "bash", str(Path(json.loads(result.stdout)["remoteWorkspaceFolder"])
                             / ".devcontainer/dev-smoke.sh")],
                text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE
            )
            assert negative.returncode != 0, "Smoke check incorrectly accepts root"
            assert key.read_bytes() == original_key
            assert sorted(path.name for path in ssh.iterdir()) == [KEY_NAME, KEY_NAME + ".pub"]
            print("PASS: root rejected; disposable host SSH directory unchanged", flush=True)
        finally:
            owned = subprocess.check_output(
                ["docker", "ps", "-aq", "--filter", "label=" + label], text=True
            ).splitlines()
            for container in owned:
                # Discover volumes even when post-create failed before `up` returned.
                inspection = json.loads(subprocess.check_output(
                    ["docker", "inspect", container], text=True
                ))[0]
                volumes.extend(mount["Name"] for mount in inspection["Mounts"]
                               if mount["Type"] == "volume")
                subprocess.run(["docker", "rm", "--force", container], check=True)
            for volume in sorted(set(volumes) - existing_volumes):
                subprocess.run(["docker", "volume", "rm", volume], check=True)


if __name__ == "__main__":
    if sys.argv[1:] == ["--inside"]:
        inside()
    elif not sys.argv[1:]:
        host()
    else:
        raise SystemExit("usage: signing-smoke.py [--inside]")
