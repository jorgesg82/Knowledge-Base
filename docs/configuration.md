# Configuration

KB stores configuration in `.kb/config.yml` at the root of each knowledge base.

## Config File

```yaml
kb_path: /path/to/kb
editor: nvim
viewer: builtin
default_category: misc
auto_update_index: true
ai_provider: auto
```

## Fields

- `kb_path`: KB root path.
- `editor`: editor used by `kb add` when no input is passed.
- `viewer`: viewer used by `kb find`. Supports `builtin`, `glow`, `bat`, `batcat`, `mdcat`, `mdless`, and `less`.
- `default_category`: fallback category for materialized notes and index entries.
- `auto_update_index`: whether write operations refresh `.kb/index.json`.
- `ai_provider`: `auto`, `claude`, or `chatgpt`.

## Machine-Local Overrides

These environment variables override the portable config on a per-host basis:

- `KB_PATH`
- `KB_EDITOR`
- `KB_VIEWER`
- `KB_DEFAULT_CATEGORY`
- `KB_AUTO_UPDATE_INDEX`
- `KB_AI_PROVIDER`

## Notes

- `ai_provider: auto` resolves to `chatgpt` on macOS and `claude` on Linux.
- `pretty_provider` and `KB_PRETTY_PROVIDER` are still accepted as read-only compatibility aliases for older setups.
- `builtin` is the default viewer.
- If no external Markdown viewer is available, `kb find` falls back to the built-in renderer.
