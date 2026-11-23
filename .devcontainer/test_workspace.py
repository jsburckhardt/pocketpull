#!/usr/bin/env python3
"""Host-side contract checks; runtime behaviour is covered by dev-smoke."""

import json
from pathlib import Path
import subprocess
import unittest


ROOT = Path(__file__).resolve().parent.parent


class WorkspaceContract(unittest.TestCase):
    def setUp(self):
        self.config = json.loads((ROOT / ".devcontainer/devcontainer.json").read_text())

    def test_explicit_development_boundary(self):
        self.assertEqual(self.config["remoteUser"], "vscode")
        self.assertEqual(self.config["containerUser"], "vscode")
        self.assertNotIn("privileged", self.config)
        self.assertNotIn("runArgs", self.config)
        self.assertNotIn("capAdd", self.config)
        self.assertNotIn("securityOpt", self.config)
        mounts = self.config["mounts"]
        binds = [mount for mount in mounts if "type=bind" in mount]
        self.assertEqual(binds, [
            "source=${localEnv:HOME}/.ssh,target=/home/vscode/.ssh,type=bind,readonly"
        ])
        self.assertEqual(len(mounts), 4)
        for mount in mounts:
            self.assertNotIn("docker.sock", mount)
        for key in ("containerEnv", "remoteEnv"):
            self.assertFalse(any(
                name.startswith(("OTEL_", "COPILOT_OTEL_"))
                for name in self.config.get(key, {})
            ))
        self.assertEqual(
            self.config["postCreateCommand"], "bash .devcontainer/post-create.sh"
        )
        self.assertNotIn("postStartCommand", self.config)
        self.assertNotIn("postAttachCommand", self.config)

    def test_feature_lock_and_exact_versions(self):
        lock = json.loads((ROOT / ".devcontainer/devcontainer-lock.json").read_text())
        self.assertEqual(set(lock["features"]), set(self.config["features"]))
        for feature, options in self.config["features"].items():
            resolved = lock["features"][feature]
            self.assertRegex(resolved["integrity"], r"^sha256:[0-9a-f]{64}$")
            self.assertIn("@sha256:", resolved["resolved"])
            if "version" in options:
                self.assertRegex(options["version"], r"^\d+\.\d+\.\d+$")

    def test_shell_syntax_and_recipes(self):
        for script in sorted((ROOT / ".devcontainer").glob("*.sh")):
            subprocess.run(["bash", "-n", str(script)], check=True)
        subprocess.run(["just", "--list"], cwd=ROOT, check=True, capture_output=True)


if __name__ == "__main__":
    unittest.main()
