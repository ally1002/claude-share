# claude-share

Convert [Claude Code](https://docs.anthropic.com/en/docs/claude-code) conversation sessions into shareable, self-contained HTML files.

## Install

```bash
go install github.com/aly/claude-share@latest
```

Or build from source:

```bash
git clone https://github.com/aly/claude-share.git
cd claude-share
go build -o claude-share .
cp claude-share ~/.local/bin/
```

## Update

If installed via `go install`:

```bash
go install github.com/aly/claude-share@latest
```

If built from source:

```bash
git pull && go build -o claude-share . && cp claude-share ~/.local/bin/
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

Claude Code stores conversation history as JSONL files under `~/.claude/`. This tool reads those files, reconstructs the conversation (grouping streamed assistant messages, parsing tool calls, thinking blocks, etc.), and renders everything into a single HTML file with no external dependencies.

The exported HTML includes:

- Markdown rendering with syntax-highlighted code blocks
- Collapsible tool call and thinking sections
- Dark theme with responsive layout
- Session metadata (project, date, message count)

## Testing

```bash
go test ./...
```
