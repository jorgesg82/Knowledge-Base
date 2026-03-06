# Viewing Entries

`kb find [query]` renders a canonical note for reading. Without a query, it opens an interactive browser over all notes.

## Modes

- default: render and page the note for terminal reading
- no query: browse every note with paging and numeric selection
- `--raw`: print the materialized Markdown directly
- `--json`: print the selected note and ranked candidates as JSON
- `--synthesize`: answer from the top matching notes using the configured AI provider

## Viewer Resolution

If the configured viewer is available, `kb find` uses it:

- `glow`
- `bat`
- `batcat`
- `mdcat`
- `mdless`

If the viewer is `builtin`, `less`, empty, or unavailable, `kb` renders the Markdown internally and pages it with `less -R` when running in an interactive terminal.

## Built-In Renderer

The built-in path:

- parses the note frontmatter
- shows metadata like summary, topics, and updated time cleanly
- renders the Markdown body to ANSI terminal output
- avoids showing raw YAML frontmatter as part of the document body
