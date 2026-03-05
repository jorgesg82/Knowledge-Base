# Personal Knowledge Base (KB)

A fast, portable, command-line knowledge base for storing and retrieving notes, commands, snippets, and documentation.

## Features

- 📝 **Markdown entries** with YAML frontmatter
- 🏷️ **Tag-based organization** plus hierarchical categories
- 🔍 **Multiple search modes**: by tag, category, or full-text
- 📦 **Export/import** as tarball for portability
- ⚡ **Single binary** - no runtime dependencies
- 🎯 **Fast indexing** with JSON cache
- 🎨 **Colorized output** for better readability
- 📖 **Markdown viewer** - uses `glow`/`bat`/`mdcat` for beautiful rendering

## Installation

Build the binary from this repository:

```bash
go build -o kb
```

### Add to PATH

Option 1 - Add the repository directory to your shell profile:
```bash
echo 'export PATH="$HOME/kb:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

Option 2 - Create a symlink (if `~/bin` is in PATH):
```bash
ln -sf "$HOME/kb/kb" "$HOME/bin/kb"
```

## Quick Start

### Creating Entries

```bash
# Create entry with default category (misc)
kb add my-notes

# Create entry with specific category
kb add linux/networking-tips

# This will:
# 1. Create file: entries/linux/networking-tips.md
# 2. Open it in $EDITOR with template
# 3. Update index automatically
```

### Entry Format

Each entry is a markdown file with YAML frontmatter:

```yaml
---
title: Networking Tips
tags: [linux, networking, tcp]
category: linux
created: 2026-03-05T10:00:00+01:00
updated: 2026-03-05T10:00:00+01:00
---

# Networking Tips

## Check open ports
netstat -tuln

## Monitor network traffic
iftop -i eth0
```

### Searching and Viewing

```bash
# List all entries
kb list

# List by category
kb list linux

# Search by tag (single tag)
kb tag linux

# Search by multiple tags (AND)
kb tag linux networking

# Full-text search
kb search "port forwarding"

# Show entry
kb show networking-tips
kb show linux-networking-tips  # by ID
```

### Editing and Deleting

```bash
# Edit entry
kb edit networking-tips

# Delete entry
kb rm networking-tips
```

### AI-Powered Formatting

Format and improve entries using Claude or ChatGPT:

```bash
# Prettify single entry (uses default mode from config)
kb pretty networking-tips

# Prettify with specific mode
kb pretty networking-tips --mode conservative
kb pretty networking-tips --mode moderate
kb pretty networking-tips --mode aggressive

# Use ChatGPT instead of Claude for a run
kb pretty networking-tips --provider chatgpt

# Prettify all entries
kb pretty --all

# Preview changes before applying (override auto-apply)
kb pretty networking-tips --confirm

# Show a unified diff without writing changes
kb pretty networking-tips --dry-run --diff
```

**Modes:**
- **conservative**: Minimal changes - only fix markdown syntax and spacing
- **moderate** (default): Format + improve clarity, add brief clarifications
- **aggressive**: Format + expand explanations, add examples and best practices

**Configuration:**
```yaml
pretty_provider: auto           # auto | claude | chatgpt
pretty_mode: moderate          # Default mode (conservative|moderate|aggressive)
pretty_auto_apply: true        # Auto-apply without confirmation
```

**Provider environment variables:**
- `claude`: `ANTHROPIC_API_KEY` or `ANTHROPIC_CUSTOM_HEADERS`, plus optional `ANTHROPIC_BASE_URL` and `ANTHROPIC_MODEL`
- `chatgpt`: `OPENAI_API_KEY`, plus optional `OPENAI_BASE_URL` and `OPENAI_MODEL`
- `kb stats` can show OpenAI API spend if `OPENAI_ADMIN_KEY` is set; use `OPENAI_PROJECT_ID` to scope it to one project

**Platform defaults:**
- `pretty_provider: auto` resolves to `chatgpt` on macOS and `claude` on Linux
- OpenAI defaults to `gpt-5-mini` if `OPENAI_MODEL` is unset
- Claude defaults to `claude-sonnet-4-6` if `ANTHROPIC_MODEL` is unset

**Typical workflow:**
1. Quickly jot down notes: `kb add quick/new-thing`
2. Add rough content in nvim
3. Prettify it: `kb pretty new-thing`
4. View the improved result: `kb show new-thing`

### Other Commands

```bash
# List all tags with counts
kb tags

# Show statistics
kb stats

# Show configuration
kb config

# Check local environment and KB health
kb doctor

# Export KB
kb export ~/backup/kb-$(date +%Y%m%d).tar.gz

# Import KB
kb import ~/backup/kb-20260305.tar.gz
```

## Directory Structure

```
~/kb/
├── kb                      # Binary
├── README.md               # This file
├── .kb/
│   ├── config.yml          # Configuration
│   └── index.json          # Entry index (auto-generated)
└── entries/
    ├── linux/
    │   ├── ssh-tunneling.md
    │   └── find-commands.md
    ├── programming/
    │   └── go-snippets.md
    └── misc/
        └── git-aliases.md
```

## Configuration

Configuration is stored in `.kb/config.yml`:

```yaml
kb_path: /path/to/kb           # KB root path
editor: nvim                  # Editor for add/edit (nvim, vim, nano)
viewer: glow                  # Viewer for show (glow, bat, batcat, mdcat, less)
default_category: misc        # Default category for new entries
auto_update_index: true       # Auto-update index after changes
pretty_provider: auto         # AI provider (auto, claude, chatgpt)
pretty_mode: moderate         # AI formatting mode (conservative, moderate, aggressive)
pretty_auto_apply: true       # Auto-apply prettify changes without confirmation
```

**Machine-local overrides:**
These environment variables override the portable `.kb/config.yml` on a per-host basis:
- `KB_PATH`
- `KB_EDITOR`
- `KB_VIEWER`
- `KB_DEFAULT_CATEGORY`
- `KB_AUTO_UPDATE_INDEX`
- `KB_PRETTY_PROVIDER`
- `KB_PRETTY_MODE`
- `KB_PRETTY_AUTO_APPLY`

**Viewer auto-detection:**
The `kb init` command automatically detects the best markdown viewer available:
1. `glow` - Beautiful markdown rendering with colors and formatting
2. `bat` - Syntax highlighting with line numbers
3. `mdcat` - Terminal markdown rendering
4. `mdless` - Markdown pager
5. `less` - Fallback plain text viewer

**Edit vs Show:**
- `kb edit <query>` - Opens in your editor (for editing raw markdown)
- `kb show <query>` - Opens in your viewer (for reading rendered markdown)

## Tips

### Use Descriptive Tags

Good tagging makes searching easier:
```yaml
tags: [linux, networking, ssh, security, tunnel]
```

### Category Organization

Categories create a basic hierarchy:
- `linux/` - Linux commands and config
- `programming/` - Code snippets and patterns
- `misc/` - Everything else

### Quick Access

Add shell functions to your shell profile:
```bash
# Quick search and show
ks() {
    kb search "$@"
}

# Quick tag search
kt() {
    kb tag "$@"
}
```

### Integration with Tools

Pipe kb output to other tools:
```bash
# Copy entry to clipboard
kb show ssh-tunneling | xclip -selection clipboard

# Count entries
kb list | wc -l

# Search and edit first result
kb tag linux | head -1 | awk '{print $2}' | xargs kb edit
```

## Portability

### Backup

```bash
# Regular backup
kb export ~/backups/kb-$(date +%Y%m%d).tar.gz

# Automated daily backup (add to cron)
0 2 * * * cd ~/kb && ./kb export ~/backups/kb-$(date +\%Y\%m\%d).tar.gz
```

### Transfer to Another Machine

On source machine:
```bash
kb export /tmp/my-kb.tar.gz
scp /tmp/my-kb.tar.gz user@remote:~/
```

On destination machine:
```bash
# Copy binary
scp user@remote:~/kb/kb ~/kb/

# Initialize
~/kb/kb init ~/kb

# Import
cd ~/kb
./kb import ~/my-kb.tar.gz
```

## Development

Source code lives in this repository:
- `main.go` - CLI commands
- `config.go` - Configuration management
- `entry.go` - Entry parsing and writing
- `index.go` - Index management
- `search.go` - Search functionality

To rebuild:
```bash
cd /path/to/kb
go build -o kb
```

## License

Personal use.
