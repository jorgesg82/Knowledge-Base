# Viewing Entries

`kb show <query>` renders an entry for reading.

## Viewer Resolution

If the configured viewer is available, `kb show` uses it:

- `glow`
- `bat`
- `batcat`
- `mdcat`
- `mdless`

If the viewer is `builtin`, `less`, empty, or unavailable, `kb` renders the Markdown internally and pages it with `less -R` when running in an interactive terminal.

## Built-In Renderer

The built-in path:

- parses the note frontmatter
- shows metadata like category, tags, and updated time cleanly
- renders the Markdown body to ANSI terminal output
- avoids showing raw YAML frontmatter as part of the document body

## Edit vs Show

- `kb edit <query>` opens the raw file in your editor.
- `kb show <query>` opens the rendered note for reading.
