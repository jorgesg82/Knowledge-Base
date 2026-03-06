# AI Organization

`kb` uses AI in two places:

- `kb add`: organize a new capture into one or more canonical notes
- `kb find --synthesize`: answer a retrieval query from the best matching notes

If the configured provider is unavailable, `kb add` falls back to deterministic local heuristics instead of dropping the capture.

## Common Usage

```bash
kb add "how to inspect open ports on macos"
kb add --dry-run "ssh tunnels with ssh -L"
kb add --provider chatgpt --file ~/Downloads/snippet.txt
kb find --synthesize "open ports on mac"
```

## Providers

### Claude

Environment:

- `ANTHROPIC_API_KEY` or `ANTHROPIC_CUSTOM_HEADERS`
- optional `ANTHROPIC_BASE_URL`
- optional `ANTHROPIC_MODEL`
- optional `ANTHROPIC_ADMIN_KEY` for spend reporting

Default model:

- `claude-sonnet-4-5-20250929`

This is the default because Claude Sonnet is Anthropic's recommended balance of capability, speed, and cost for most production use cases.

### ChatGPT

Environment:

- `OPENAI_API_KEY`
- optional `OPENAI_BASE_URL`
- optional `OPENAI_MODEL`

Default model:

- `gpt-5-mini`

## Spend Reporting

`kb stats` can show provider spend when the matching admin key is available:

- `OPENAI_ADMIN_KEY` is set
- optional `OPENAI_PROJECT_ID` is set to scope costs to one project
- `ANTHROPIC_ADMIN_KEY` is set for Claude usage

Output includes:

- total
- last 30 days
- today
