# Versions from Conventional Commits via an owner-merged Release Please PR

- Status: accepted
- Date: 20261124
- Source: `docs/work/20261124-release-distribution.md` D3, D4, D5, D7

## Context
The owner does not want to choose version numbers or tag releases by hand, but keeps the decision of when to release.

## Decision
Derive versions and the changelog from Conventional Commits on `main` with Release Please, which maintains one release PR that only the owner merges. Use normal SemVer (breaking changes bump major even before 1.0), start at Release Please's automatic 1.0.0 with no bootstrap tag, and publish assets in the same workflow run with the built-in token.

## Consequences
- Easier: no manual versioning or tagging; releases are a PR merge.
- Harder: every commit landing on `main` needs a Conventional Commit subject to count.
- Forbidden: manual or recurring `release-as` overrides, auto-merging the release PR, tag-triggered chaining, new PATs or App secrets.

## Alternatives considered
- Manual tagging: replaced by commit-derived versions.
- Manual `v0.0.0` bootstrap tag: proposed during delivery, rejected by the owner.
- Tag-triggered workflow chaining: events from the built-in token would not trigger it.
