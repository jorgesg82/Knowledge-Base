# Configuration

KB stores configuration in `.kb/config.yml` at the root of each knowledge base.

## Config File

```yaml
kb_path: /path/to/kb
editor: nvim
viewer: builtin
default_category: misc
auto_update_index: true
pretty_provider: auto
pretty_mode: moderate
pretty_auto_apply: true
```

## Fields

- `kb_path`: KB root path.
- `editor`: editor used by `kb add` and `kb edit`.
- `viewer`: viewer used by `kb show`. Supports `builtin`, `glow`, `bat`, `batcat`, `mdcat`, `mdless`, and `less`.
- `default_category`: category used when `kb add` omits one.
- `auto_update_index`: whether write operations refresh `.kb/index.json`.
- `pretty_provider`: `auto`, `claude`, or `chatgpt`.
- `pretty_mode`: `conservative`, `moderate`, or `aggressive`.
- `pretty_auto_apply`: whether `kb pretty` applies changes without confirmation.

## Machine-Local Overrides

These environment variables override the portable config on a per-host basis:

- `KB_PATH`
- `KB_EDITOR`
- `KB_VIEWER`
- `KB_DEFAULT_CATEGORY`
- `KB_AUTO_UPDATE_INDEX`
- `KB_PRETTY_PROVIDER`
- `KB_PRETTY_MODE`
- `KB_PRETTY_AUTO_APPLY`

## Notes

- `pretty_provider: auto` resolves to `chatgpt` on macOS and `claude` on Linux.
- If no external Markdown viewer is available, `kb show` falls back to the built-in renderer.
