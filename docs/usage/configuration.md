# Configuration and credentials

The public collection flags are `--config`, `--raw-dir`, `--api-key-file`,
`--state-dir`, `--refresh-after`, `--log-format`, and `--log-level`. Help/version
read no configuration or credentials and send no requests; the unversioned
development build reports `pocketpull dev`. There is no literal-key flag or
public API endpoint override.

Select **exactly one** configuration file: explicit `--config FILE`, otherwise
`./pocketpull.yaml`, otherwise `$XDG_CONFIG_HOME/pocketpull/config.yaml`
(or `$HOME/.config/pocketpull/config.yaml` when the XDG base is unset/empty).
No files are merged. A missing explicit file is an error. An unreadable or
invalid selected/discovered file is an error, never permission to fall back.
If neither discovery file exists, flags and defaults may supply configuration.
Explicit flags override the selected file, including `--refresh-after 0`.
File-configured paths resolve against that file's directory; CLI paths resolve
against the working directory. Paths are not shell-expanded: use absolute paths
or relative paths, not literal `~` or `$HOME` inside YAML.

YAML must be one strict mapping/document with only these six keys:

```yaml
raw_dir: ../../pocket-raw
api_key_file: api-key
state_dir: ../../.local/state/pocketpull
refresh_after: 24h
log_format: json
log_level: info
```

These example paths assume the file is in `$HOME/.config/pocketpull/`.
[`examples/pocketpull.yaml`](../../examples/pocketpull.yaml) is a copyable version.
All supplied values must be nonempty strings; unknown/duplicate keys, multiple
documents, malformed YAML, aliases and incorrect types are rejected, even when
a CLI override would replace an invalid file value. `raw_dir` is mandatory:
there is no implicit output folder. `refresh_after` defaults to `24h` and uses
Go duration syntax, such as `30m`, `1h30m`, or `"0"` (quote YAML zero to keep it
a string). Zero selects full refresh; negative/invalid durations are rejected.
The default state base is `$XDG_STATE_HOME/pocketpull`, or
`$HOME/.local/state/pocketpull` when the XDG base is unset/empty.

Read exactly one nonempty secret source: `POCKETPULL_API_KEY` **or** the selected
`api_key_file`/`--api-key-file`. Both sources conflict even if equal, empty, or
whitespace-only; an explicitly empty environment key or unusable file never
falls back to another source. Leading/trailing whitespace is trimmed; embedded
newlines are rejected. Secret files must be regular, usable, owned by the
effective user, and have no group/other read permission. Provision the secret
through your secret workflow and use `chmod 600 /path/to/api-key`; the collector
does not change permissions. Do not place a literal `api_key` in YAML.

No `.env` file is automatically loaded. A shell or development tool may
explicitly export the environment key, but keep `.env` gitignored. Keep all
secrets out of command arguments, tracked files, images and build arguments.
Configuration/parser/file errors report only redacted operational categories,
not supplied values or paths.
