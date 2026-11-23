#!/usr/bin/env python3
"""Bounded real Copilot OTLP check using only synthetic input and a clean profile."""

import gzip
from http.server import BaseHTTPRequestHandler, HTTPServer
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import tempfile
import threading
import uuid


def fields(data):
    """Read protobuf wire fields without installing a runtime/schema toolchain."""
    offset = 0

    def varint():
        nonlocal offset
        value = 0
        for shift in range(0, 70, 7):
            if offset >= len(data):
                raise ValueError("Truncated protobuf varint")
            byte = data[offset]
            offset += 1
            value |= (byte & 127) << shift
            if byte < 128:
                return value
        raise ValueError("Oversized protobuf varint")

    result = {}
    while offset < len(data):
        tag = varint()
        number, wire = tag >> 3, tag & 7
        if not number:
            raise ValueError("Invalid protobuf field")
        if wire == 0:
            value = varint()
        elif wire in (1, 2, 5):
            size = varint() if wire == 2 else {1: 8, 5: 4}[wire]
            value = data[offset:offset + size]
            if len(value) != size:
                raise ValueError("Truncated protobuf field")
            offset += size
        else:
            raise ValueError("Unsupported protobuf wire type")
        result.setdefault(number, []).append((wire, value))
    return result


def messages(message, number):
    values = message.get(number, [])
    assert all(wire == 2 for wire, _ in values), "Expected protobuf message"
    return [fields(value) for _, value in values]


def strings(data):
    # Attribute keys/values are UTF-8 length-delimited fields. Walk all nested
    # messages too, including resource, scope, event and exemplar attributes.
    for values in fields(data).values():
        for wire, value in values:
            if wire != 2:
                continue
            try:
                yield value.decode("utf-8")
            except UnicodeDecodeError:
                pass
            try:
                yield from strings(value)
            except ValueError:
                # Strings and opaque byte IDs are not necessarily submessages.
                pass


def signal_count(path, data):
    count = 0
    for resource in messages(fields(data), 1):
        assert messages(resource, 1), "Missing OTLP resource"
        for scope in messages(resource, 2):
            assert messages(scope, 1), "Missing instrumentation scope"
            for item in messages(scope, 2):
                if path == "/v1/traces":
                    assert len(item[1][0][1]) == 16, "Invalid trace ID"
                    assert len(item[2][0][1]) == 8, "Invalid span ID"
                    assert item[5][0][1], "Missing span name"
                    count += 1
                else:
                    assert item[1][0][1], "Missing metric name"
                    points = [point for kind in (5, 7, 9, 10, 11)
                              for metric in messages(item, kind)
                              for point in messages(metric, 1)]
                    assert points, "Missing metric data points"
                    count += len(points)
    assert count, "Empty OTLP export"
    return count


def run_cli(directory, env, prompt, response):
    command = [
        "copilot", "--no-auto-update", "--no-custom-instructions",
        "--disable-builtin-mcps", "--disable-client-telemetry",
        "--no-ask-user", "--no-bash-env",
        "--deny-tool", "shell", "--deny-tool", "read", "--deny-tool", "write",
        "--deny-tool", "url", "--deny-tool", "memory",
        "--available-tools", "--trust-folder", "--silent",
        "--stream", "off", "-p", prompt,
    ]
    with subprocess.Popen(command, cwd=directory, env=env, stdin=subprocess.DEVNULL,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                          start_new_session=True) as process:
        try:
            stdout, stderr = process.communicate(timeout=150)
            if process.returncode != 0:
                diagnostic = (stderr + stdout).decode("utf-8", errors="replace")
                for key in ("GH_TOKEN", "GITHUB_TOKEN", "COPILOT_GITHUB_TOKEN"):
                    if env.get(key):
                        diagnostic = diagnostic.replace(env[key], "[redacted]")
                raise RuntimeError(f"Copilot exited {process.returncode}: {diagnostic[:4000]}")
            assert response.encode() in stdout, "Synthetic response missing"
        finally:
            # Also terminate any child left behind by a failed/timed-out CLI.
            try:
                os.killpg(process.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            if process.poll() is None:
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    os.killpg(process.pid, signal.SIGKILL)
                    process.wait()


def main():
    assert os.getuid() != 0, "Run inside the non-root devcontainer"
    version = subprocess.check_output(["copilot", "--version"], text=True)
    assert re.search(r"\b1\.0\.114\b", version), "Expected pinned Copilot 1.0.114"
    assert os.environ.get("OTEL_EXPORTER_OTLP_PROTOCOL") == "http/protobuf"
    assert os.environ.get("OTEL_INSTRUMENTATION_GENAI_CAPTURE_CONTENT") == "NO_CONTENT"
    records = []

    class Receiver(BaseHTTPRequestHandler):
        def do_POST(self):
            records.append((self.path, dict(self.headers),
                            self.rfile.read(int(self.headers["Content-Length"]))))
            self.send_response(200)
            self.send_header("Content-Type", "application/x-protobuf")
            self.send_header("Content-Length", "0")
            self.end_headers()

        def log_message(self, *_):
            pass

    server = HTTPServer(("127.0.0.1", 0), Receiver)
    server.timeout = 5
    thread = threading.Thread(target=server.serve_forever)
    thread.start()
    try:
        with tempfile.TemporaryDirectory(prefix="pocketpull-telemetry-") as directory:
            root = Path(directory)
            # Do not inherit host/user telemetry, provider, instruction or tool settings.
            env = {key: value for key, value in os.environ.items()
                   if key in ("PATH", "LANG", "SSL_CERT_FILE", "SSL_CERT_DIR",
                              "GH_TOKEN", "GITHUB_TOKEN", "COPILOT_GITHUB_TOKEN")}
            env |= {
                "HOME": directory, "XDG_CONFIG_HOME": str(root / "config"),
                "XDG_CACHE_HOME": str(root / "cache"),
                "COPILOT_AUTO_UPDATE": "false",
                "COPILOT_DISABLE_LOGIN_SHELL_ENV": "true",
                "USE_TGREP": "false",
                "OTEL_EXPORTER_OTLP_PROTOCOL": os.environ["OTEL_EXPORTER_OTLP_PROTOCOL"],
                "OTEL_INSTRUMENTATION_GENAI_CAPTURE_CONTENT":
                    os.environ["OTEL_INSTRUMENTATION_GENAI_CAPTURE_CONTENT"],
            }
            # Reuse an existing development gh login without copying its token to disk.
            if not any(env.get(key) for key in
                       ("GH_TOKEN", "GITHUB_TOKEN", "COPILOT_GITHUB_TOKEN")):
                token = subprocess.run(["gh", "auth", "token"], capture_output=True,
                                       text=True, timeout=15)
                assert token.returncode == 0 and token.stdout.strip(), (
                    "No development credential: run gh auth login inside the container"
                )
                env["GH_TOKEN"] = token.stdout.strip()
            prompt_marker = "PROMPT_PRIVATE_" + uuid.uuid4().hex
            response_marker = "RESPONSE_PRIVATE_" + uuid.uuid4().hex
            prompt = (f"Synthetic telemetry check {prompt_marker}. Do not use any tools. "
                      f"Reply with exactly this text and nothing else: {response_marker}")
            for name in ("unconfigured", "configured"):
                cwd = root / name
                cwd.mkdir()
                profile = root / (name + "-profile")
                profile.mkdir()
                (profile / "settings.json").write_text(
                    json.dumps({
                        "disableAllHooks": True,
                        "extensions": {"mode": "disabled"},
                        "telemetry": {"captureContent": False, "capture": {
                            key: False for key in ("identity", "prompts", "responses",
                                                   "toolArguments", "toolOutput",
                                                   "policyDetail")
                        }},
                    }) + "\n"
                )
                current = env | {"COPILOT_HOME": str(profile)}
                if name == "configured":
                    current |= {
                        "OTEL_EXPORTER_OTLP_ENDPOINT":
                            f"http://127.0.0.1:{server.server_port}",
                        "OTEL_EXPORTER_OTLP_HEADERS": "x-pocketpull-smoke=synthetic-only",
                    }
                else:
                    # Dev Container localEnv substitutions are empty when unset.
                    current |= {"OTEL_EXPORTER_OTLP_ENDPOINT": "",
                                "OTEL_EXPORTER_OTLP_HEADERS": ""}
                before = len(records)
                run_cli(cwd, current, prompt, response_marker)
                secrets = [env[key].encode() for key in
                           ("GH_TOKEN", "GITHUB_TOKEN", "COPILOT_GITHUB_TOKEN")
                           if env.get(key)]
                for path in root.rglob("*"):
                    if path.is_file() and not path.is_symlink():
                        contents = path.read_bytes()
                        assert not any(secret in contents for secret in secrets), (
                            "Development credential unexpectedly persisted in scratch state"
                        )
                if name == "unconfigured":
                    assert len(records) == before, "Unexpected unconfigured export"
                    print("PASS: clean unconfigured Copilot prompt succeeds; no OTLP activation",
                          flush=True)

            totals = {"/v1/traces": 0, "/v1/metrics": 0}
            leaked = set()
            forbidden = re.compile(
                r"^(user\.name|user\.id|process\.user\.name|host\.name|"
                r"enduser\.(?!pseudo\.id$).*|gen_ai\.(input\.messages|output\.messages|"
                r"system_instructions|prompt|completion|tool\.(call\.arguments|"
                r"call\.result|definitions)).*|.*(?:tool_arguments|tool_output|"
                r"policy_detail|policy\.details).*)$"
            )
            for path, headers, body in records:
                assert path in totals, f"Unexpected OTLP path: {path}"
                headers = {key.lower(): value for key, value in headers.items()}
                assert headers.get("content-type") == "application/x-protobuf"
                assert headers.get("x-pocketpull-smoke") == "synthetic-only"
                if headers.get("content-encoding") == "gzip":
                    body = gzip.decompress(body)
                else:
                    assert headers.get("content-encoding", "identity") == "identity"
                totals[path] += signal_count(path, body)
                assert prompt_marker.encode() not in body, "Prompt content exported"
                assert response_marker.encode() not in body, "Response content exported"
                values = set(strings(body))
                assert "github.copilot" in values, "Missing actual Copilot scope"
                leaked.update(value for value in values if forbidden.fullmatch(value))
                assert not any("execute_tool" in value for value in values), (
                    "Unexpected tool execution during synthetic check"
                )
            assert all(totals.values()), f"Missing actual trace/metric export: {totals}"
            print(f"Observed real OTLP/HTTP protobuf exports: {totals}", flush=True)
            assert not leaked, (
                "Prohibited content/identity attribute keys exported "
                f"(only enduser.pseudo.id permitted): {sorted(leaked)}"
            )
            print(f"PASS: Copilot 1.0.114 OTLP/HTTP protobuf {totals}; synthetic header "
                  "received; prompt/response sentinels and prohibited content/identity "
                  "keys absent; only enduser.pseudo.id permitted")
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=10)
        assert not thread.is_alive(), "Loopback receiver did not stop"


if __name__ == "__main__":
    main()
