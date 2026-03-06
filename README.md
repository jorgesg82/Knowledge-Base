# Personal Knowledge Base (KB)

A fast command-line knowledge base for capturing and retrieving technical knowledge.

## Features

- Fast capture with `kb add`
- Retrieval and browsing with `kb find`
- AI-assisted organization via Claude or ChatGPT
- Canonical notes materialized as Markdown with YAML frontmatter
- Export/import as tarball
- Built-in Markdown rendering fallback for `kb find`
- Single binary with optional prebuilt release downloads

## Installation

Build locally:

```bash
go build -o kb
```

Or download a prebuilt binary from a GitHub release. See [docs/releases.md](docs/releases.md).

Add the directory containing `kb` to your `PATH`:

```bash
echo 'export PATH="$HOME/kb:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

## Quick Start

```bash
# Initialize a KB
kb init ~/my-kb

# Capture something quickly
kb add "how to inspect open ports on macos"
kb add   # opens your editor
kb add --clipboard
kb add --file ~/Downloads/snippet.txt
kb add --url https://example.com/article

# Retrieve it later
kb find
kb find open ports
kb find macos

# Backup
kb export ~/backup/kb-$(date +%Y%m%d).tar.gz
```

Materialized notes live under `entries/`. Internal captures, canonical note records, and operation logs live under `.kb/`.

## Commands

Core workflow:

```text
kb init [path]
kb add [text]
kb find [query]
```

Maintenance:

```text
kb doctor
kb stats
kb config
kb export [path]
kb import <tarball>
kb rebuild
kb clean
```

`kb add` supports `--file`, `--url`, `--clipboard`, `--dry-run`, and `--json`.

`kb find` without a query opens an interactive browser over all notes. Use note numbers to open, `n`/`p` to move between pages, and `q` to quit.

## Documentation

- [Architecture and Workflow](docs/architecture.md)
- [Configuration and Environment Overrides](docs/configuration.md)
- [AI Organization and Spend Reporting](docs/ai.md)
- [Viewing Entries](docs/viewer.md)
- [Releases and Prebuilt Binaries](docs/releases.md)
- [Tests](README_TESTS.md)
