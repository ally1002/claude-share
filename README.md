# claude-share

Export [Claude Code](https://docs.anthropic.com/en/docs/claude-code) conversations to self-contained, shareable HTML files.

<img width="1237" height="665" alt="image" src="https://github.com/user-attachments/assets/1093e892-337e-49c9-af68-792186427160" />

## Features

- Markdown rendering with syntax-highlighted code blocks
- Collapsible tool call and thinking sections
- Dark theme with responsive layout
- Session metadata (project, date, message count)
- Single HTML file with zero external dependencies

## Install

```bash
go install github.com/ally1002/claude-share@latest
```

Or build from source:

```bash
git clone https://github.com/ally1002/claude-share.git
cd claude-share
go build -o claude-share .
cp claude-share ~/.local/bin/
```

## Usage

### List sessions

```bash
claude-share list
```

Filter by project:

```bash
claude-share list --project myapp
```

### Export a session

```bash
claude-share export <session-id> -o conversation.html
```

Include tool calls and thinking blocks:

```bash
claude-share export <session-id> -o conversation.html --include-tools --include-thinking
```

Output to stdout (pipe-friendly):

```bash
claude-share export <session-id> > conversation.html
```

## How it works

Claude Code stores conversation history as JSONL files under `~/.claude/`. This tool reads those files, reconstructs the conversation (grouping streamed messages, parsing tool calls, thinking blocks, etc.), and renders everything into a single HTML file.

## Testing

```bash
go test ./...
```

## License

[WTFPL](LICENSE)
