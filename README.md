# Personal Knowledge Base (KB)

A fast command-line knowledge base for storing and retrieving notes, commands, snippets, and documentation.

## Features

- Markdown entries with YAML frontmatter
- Categories, tags, full-text search, and fast JSON indexing
- AI formatting via Claude or ChatGPT
- Export/import as tarball
- Built-in Markdown rendering fallback for `kb show`
- Single binary with optional prebuilt release downloads

## Installation

Build locally:

```bash
go build -o kb
```

Or download a prebuilt binary from a GitHub release. See [docs/releases.md](/Users/jsaegar/kb/docs/releases.md).

Add the directory containing `kb` to your `PATH`:

```bash
echo 'export PATH="$HOME/kb:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

## Quick Start

```bash
# Initialize a KB
kb init ~/my-kb

# Create an entry
kb add linux/networking-tips

# Search and browse
kb list
kb search "port forwarding"
kb tag linux networking
kb show networking-tips

# AI formatting
kb pretty networking-tips
kb pretty networking-tips --dry-run --diff

# Backup
kb export ~/backup/kb-$(date +%Y%m%d).tar.gz
```

Entries are Markdown files with YAML frontmatter under `entries/<category>/...`.

## Commands

```text
kb init [path]
kb add [category/]title
kb edit <query>
kb show <query>
kb rm <query>
kb list [category]
kb search <text>
kb tag <tag> [<tag2>...]
kb tags
kb pretty <query> [--mode ...] [--provider ...]
kb export [path]
kb import <tarball>
kb stats
kb config
kb doctor
kb rebuild
kb clean
```

## Documentation

- [Configuration and Environment Overrides](/Users/jsaegar/kb/docs/configuration.md)
- [AI Formatting and Spend Reporting](/Users/jsaegar/kb/docs/ai.md)
- [Viewing Entries](/Users/jsaegar/kb/docs/viewer.md)
- [Releases and Prebuilt Binaries](/Users/jsaegar/kb/docs/releases.md)
- [Tests](/Users/jsaegar/kb/README_TESTS.md)
