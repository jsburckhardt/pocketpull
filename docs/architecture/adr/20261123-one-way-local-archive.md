# One-way latest-state local archive of full detail JSON

- Status: accepted
- Date: 20261123
- Source: `docs/work/20261123-collector.md` D1, D2, D8

## Context
A downstream wiki agent consumes the collected recordings. It needs source content (transcripts, metadata and summaries Pocket already returns), and the local copy must survive upstream deletions, title changes and interrupted writes.

## Decision
Store each recording as `<raw-dir>/<recording-id>/recording.json` holding the entire detail `data` object, replaced atomically only when content changes. Never write to Pocket, never prune local snapshots when a recording disappears or returns 404, and keep only the latest state, not revision history. Exclude audio and any collector-generated processing.

## Consequences
- Easier: downstream readers get raw, stable-ID input; failed or interrupted fetches never damage prior snapshots.
- Harder: deleted upstream recordings stay locally until someone removes them; edits are only as fresh as the refresh policy.
- Forbidden: remote edits/deletions, local pruning, audio downloads, transcription, summarisation or generated Markdown.

## Alternatives considered
- Two-way sync with delete semantics: rejected to protect locally collected data.
- Revision history: out of scope; latest state is enough for the wiki input.
