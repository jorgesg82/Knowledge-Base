# KB Test Suite

The suite is focused on the current capture/retrieval engine: capture, planning, canonical note materialization, retrieval, archive safety, rendering, and platform behavior.

## Running Tests

```bash
go test ./...
go test -race ./...
go vet ./...
```

## Main Areas

### Core Workflow

- `add_test.go`: add option parsing and editor-first capture behavior
- `capture_sources_test.go`: file, URL, clipboard, and stdin capture sources
- `capture_quality_test.go`: cleanup and canonicalization of captured content
- `organizer_test.go`: heuristic and provider-backed add planning
- `store_test.go`: capture/note/op persistence and materialization
- `find_test.go`: note ranking and retrieval behavior

### CLI / UX

- `cli_integration_test.go`: black-box tests for `init`, `add`, `find`, `doctor`, `stats`, `clean`, removed-command failures, and viewer fallback
- `viewer_test.go`: built-in Markdown rendering and paging behavior
- `doctor_test.go`: environment/provider diagnostics

### Storage / Safety

- `config_test.go`: config defaults, overrides, aliases, and path detection
- `entry_test.go`: frontmatter parsing and materialized note round-trips
- `index_test.go`: index persistence and rebuild behavior
- `archive_test.go`: export/import safety
- `edge_cases_test.go`: malformed input, path traversal, corruption, concurrency, and large-content cases

## Notes

- The suite intentionally mixes unit tests and binary-level CLI tests.
- Coverage is strongest on capture and retrieval paths, which are the critical behaviors in the current workflow.
- The old v1 `pretty`/manual-taxonomy workflows are no longer part of the tested surface.
