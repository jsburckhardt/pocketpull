# One strict YAML file plus explicit CLI overrides

- Status: accepted
- Date: 20261123
- Source: `docs/work/20261123-collector.md` D5, D16

## Context
Configuration must be predictable under cron, where the working directory and environment differ from an interactive shell. Hidden multi-file merging makes it hard to know which value applied.

## Decision
Select exactly one configuration file (explicit `--config`, else `./pocketpull.yaml`, else the XDG config path), never merge files, and let explicit CLI flags override it. Parse YAML strictly (six known keys, one document, no aliases) and resolve file paths relative to the file. Parse flags with the standard library `flag` package and YAML node-by-node with `go.yaml.in/yaml/v3`, not Viper. The brief suggested Cobra as a delegated choice; the delivered CLI did not need it.

## Consequences
- Easier: one place to look for each value; invalid files fail loudly instead of falling back.
- Harder: adding a setting means changing the strict schema, flags and docs together.
- Forbidden: implicit discovery/merging, unknown keys, shell expansion inside YAML.

## Alternatives considered
- Viper: its implicit discovery and merging conflict with this contract.
- Cobra: suggested in the brief (D16) as a delegated option; unnecessary for a single command with seven flags.
