# KB Architecture

`kb` is built around two primary entry points:

- `kb add`
- `kb find`

The user should not need to choose categories, tags, filenames, or note structure up front. The model organizes the capture. The application preserves the raw input, validates the plan, and applies deterministic changes.

## Core Workflow

### `kb add`

Supported inputs:

- positional text: `kb add "how to inspect open ports on macos"`
- stdin: `pbpaste | kb add`
- file input: `kb add --file notes.txt`
- URL input: `kb add --url https://...`
- clipboard: `kb add --clipboard`
- no args: `kb add` opens the editor

Behavior:

1. capture the raw input exactly as provided
2. persist it as a capture record under `.kb/captures`
3. rank relevant existing notes
4. ask the provider for a structured add plan
5. validate and normalize that plan
6. apply deterministic note updates and materialize Markdown under `entries/`

If the provider is unavailable or returns an invalid plan, `kb` falls back to local heuristics instead of dropping the capture.

### `kb find`

`kb find` ranks canonical notes by:

- exact ID/title/path matches
- aliases
- topics
- summary/body hits
- token overlap

Output modes:

- no query: browse all notes with paging and numeric selection
- default: readable terminal rendering
- `--raw`: raw materialized Markdown
- `--json`: machine-readable selected note and candidates
- `--synthesize`: synthesized answer across the top matching notes

## Data Model

```text
.kb/
  config.yml
  captures/
  notes/
  ops/
  state.json

entries/
  *.md
```

Principles:

- captures are immutable
- canonical notes are the internal source of truth
- Markdown files are human-facing materializations
- operation logs record each applied add plan
- indexes are derived data and can be rebuilt

## AI Contract

The provider is never allowed to write files directly. It must return structured actions such as:

- `create_note`
- `update_note`

The app validates action types, note targets, payload sizes, and path safety before materializing anything.

## Compatibility

The current workflow is intentionally not source-compatible with the old v1 UX.

Removed commands:

- `kb edit`
- `kb rm`
- `kb list`
- `kb search`
- `kb tag`
- `kb tags`
- `kb pretty`

If you need the old workflow, use the v1 release.
