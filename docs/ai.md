# AI Formatting

`kb pretty` formats and improves entries using Claude or ChatGPT while preserving the note as Markdown.

## Common Usage

```bash
kb pretty networking-tips
kb pretty networking-tips --mode conservative
kb pretty networking-tips --provider chatgpt
kb pretty networking-tips --dry-run --diff
kb pretty --all
```

## Modes

- `conservative`: fix Markdown and spacing only.
- `moderate`: improve formatting and clarity with minimal rewriting.
- `aggressive`: expand explanations and structure more heavily.

## Providers

### Claude

Environment:

- `ANTHROPIC_API_KEY` or `ANTHROPIC_CUSTOM_HEADERS`
- optional `ANTHROPIC_BASE_URL`
- optional `ANTHROPIC_MODEL`

Default model:

- `claude-sonnet-4-6`

### ChatGPT

Environment:

- `OPENAI_API_KEY`
- optional `OPENAI_BASE_URL`
- optional `OPENAI_MODEL`

Default model:

- `gpt-5-mini`

## Spend Reporting

`kb stats` can show OpenAI API spend when:

- `OPENAI_ADMIN_KEY` is set
- optional `OPENAI_PROJECT_ID` is set to scope costs to one project

Output includes:

- total
- last 30 days
- today
