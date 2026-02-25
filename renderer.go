package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"regexp"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/gomarkdown/markdown"
	mkhtml "github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

type SessionMeta struct {
	SessionID    string
	Project      string
	Date         string
	MessageCount int
	FirstPrompt  string
}

type RenderOpts struct {
	IncludeTools    bool
	IncludeThinking bool
}

func RenderHTML(messages []Message, meta SessionMeta, opts RenderOpts) (string, error) {
	type renderedBlock struct {
		Type     string
		HTML     template.HTML
		ToolName string
		IsError  bool
	}
	type renderedMessage struct {
		Role   string
		Blocks []renderedBlock
	}

	var rendered []renderedMessage
	for _, msg := range messages {
		rm := renderedMessage{Role: msg.Role}
		hasVisible := false
		for _, b := range msg.Blocks {
			switch b.Type {
			case "text":
				rm.Blocks = append(rm.Blocks, renderedBlock{
					Type: "text",
					HTML: template.HTML(renderMarkdown(b.Text)),
				})
				hasVisible = true
			case "thinking":
				rm.Blocks = append(rm.Blocks, renderedBlock{
					Type: "thinking",
					HTML: template.HTML(renderMarkdown(b.Text)),
				})
				hasVisible = true
			case "tool_use":
				highlighted := highlightJSON(b.ToolInput)
				rm.Blocks = append(rm.Blocks, renderedBlock{
					Type:     "tool_use",
					ToolName: b.ToolName,
					HTML:     template.HTML(highlighted),
				})
				hasVisible = true
			case "tool_result":
				rm.Blocks = append(rm.Blocks, renderedBlock{
					Type:    "tool_result",
					HTML:    template.HTML("<pre class=\"tool-output\">" + html.EscapeString(truncate(b.Text, 2000)) + "</pre>"),
					IsError: b.IsError,
				})
				if msg.Role == "assistant" {
					hasVisible = true
				}
			}
		}
		if hasVisible {
			rendered = append(rendered, rm)
		}
	}

	tmpl, err := template.New("page").Parse(htmlTemplate)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	data := struct {
		Meta     SessionMeta
		Messages []renderedMessage
	}{
		Meta:     meta,
		Messages: rendered,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}

var codeBlockRe = regexp.MustCompile(`<pre><code class="language-(\w+)">([\s\S]*?)</code></pre>`)

func renderMarkdown(text string) string {
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)

	htmlFlags := mkhtml.CommonFlags | mkhtml.HrefTargetBlank
	renderer := mkhtml.NewRenderer(mkhtml.RendererOptions{Flags: htmlFlags})

	md := markdown.ToHTML([]byte(text), p, renderer)
	result := string(md)

	result = codeBlockRe.ReplaceAllStringFunc(result, func(match string) string {
		subs := codeBlockRe.FindStringSubmatch(match)
		if len(subs) != 3 {
			return match
		}
		lang := subs[1]
		code := html.UnescapeString(subs[2])
		highlighted, err := highlightCode(code, lang)
		if err != nil {
			return match
		}
		return highlighted
	})

	return result
}

func highlightCode(code, lang string) (string, error) {
	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	style := styles.Get("monokai")
	formatter := chromahtml.New(
		chromahtml.WithClasses(false),
		chromahtml.PreventSurroundingPre(false),
	)

	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, iterator); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func highlightJSON(jsonStr string) string {
	var pretty bytes.Buffer
	if err := jsonIndent(&pretty, []byte(jsonStr)); err == nil {
		jsonStr = pretty.String()
	}
	result, err := highlightCode(jsonStr, "json")
	if err != nil {
		return "<pre>" + html.EscapeString(jsonStr) + "</pre>"
	}
	return result
}

func jsonIndent(dst *bytes.Buffer, src []byte) error {
	var v interface{}
	if err := json.Unmarshal(src, &v); err != nil {
		return err
	}
	enc := json.NewEncoder(dst)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... (truncated)"
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Claude Code Share{{if .Meta.Project}} — {{.Meta.Project}}{{end}}</title>
<link rel="icon" type="image/svg+xml" href="data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 16 16'><path fill='%23D97757' d='m3.127 10.604 3.135-1.76.053-.153-.053-.085H6.11l-.525-.032-1.791-.048-1.554-.065-1.505-.08-.38-.081L0 7.832l.036-.234.32-.214.455.04 1.009.069 1.513.105 1.097.064 1.626.17h.259l.036-.105-.089-.065-.068-.064-1.566-1.062-1.695-1.121-.887-.646-.48-.327-.243-.306-.104-.67.435-.48.585.04.15.04.593.456 1.267.981 1.654 1.218.242.202.097-.068.012-.049-.109-.181-.9-1.626-.96-1.655-.428-.686-.113-.411a2 2 0 0 1-.068-.484l.496-.674L4.446 0l.662.089.279.242.411.94.666 1.48 1.033 2.014.302.597.162.553.06.17h.105v-.097l.085-1.134.157-1.392.154-1.792.052-.504.25-.605.497-.327.387.186.319.456-.045.294-.19 1.23-.37 1.93-.243 1.29h.142l.161-.16.654-.868 1.097-1.372.484-.545.565-.601.363-.287h.686l.505.751-.226.775-.707.895-.585.759-.839 1.13-.524.904.048.072.125-.012 1.897-.403 1.024-.186 1.223-.21.553.258.06.263-.218.536-1.307.323-1.533.307-2.284.54-.028.02.032.04 1.029.098.44.024h1.077l2.005.15.525.346.315.424-.053.323-.807.411-3.631-.863-.872-.218h-.12v.073l.726.71 1.331 1.202 1.667 1.55.084.383-.214.302-.226-.032-1.464-1.101-.565-.497-1.28-1.077h-.084v.113l.295.432 1.557 2.34.08.718-.112.234-.404.141-.444-.08-.911-1.28-.94-1.44-.759-1.291-.093.053-.448 4.821-.21.246-.484.186-.403-.307-.214-.496.214-.98.258-1.28.21-1.016.19-1.263.112-.42-.008-.028-.092.012-.953 1.307-1.448 1.957-1.146 1.227-.274.109-.477-.247.045-.44.266-.39 1.586-2.018.956-1.25.617-.723-.004-.105h-.036l-4.212 2.736-.75.096-.324-.302.04-.496.154-.162 1.267-.871z'/></svg>">
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{
  --bg:#1a1a1a;--surface:#262626;--surface-hover:#303030;
  --border:#333;--text:#e8e8e8;--text-secondary:#999;--text-tertiary:#666;
  --accent:#D97757;--accent-soft:rgba(217,119,87,.12);
  --user-bg:#353535;--code-bg:#1e1e1e;--code-header:#2a2a2a;
  --green:#4ade80;--red:#f87171;--blue:#60a5fa;
  --radius:12px;--radius-sm:8px;--max-w:780px;
}
html{font-size:15px;-webkit-font-smoothing:antialiased;-moz-osx-font-smoothing:grayscale}
body{background:var(--bg);color:var(--text);font-family:'Inter',system-ui,-apple-system,sans-serif;line-height:1.65;min-height:100vh}

.topbar{position:sticky;top:0;z-index:100;background:rgba(26,26,26,.82);backdrop-filter:blur(20px) saturate(1.4);-webkit-backdrop-filter:blur(20px) saturate(1.4);border-bottom:1px solid var(--border)}
.topbar-inner{max-width:var(--max-w);margin:0 auto;padding:14px 24px;display:flex;align-items:center;justify-content:space-between}
.topbar-left{display:flex;align-items:center;gap:12px}
.logo{display:flex;align-items:center;gap:9px;text-decoration:none;color:var(--text)}
.logo svg{width:28px;height:28px}
.logo-text{font-weight:600;font-size:.95rem;letter-spacing:-.01em}
.logo-badge{font-size:.65rem;font-weight:600;letter-spacing:.04em;text-transform:uppercase;color:var(--accent);background:var(--accent-soft);padding:2px 7px;border-radius:5px;margin-left:2px}

.session-meta{max-width:var(--max-w);margin:0 auto;padding:32px 24px 0}
.session-title{font-size:1.35rem;font-weight:600;letter-spacing:-.02em;margin-bottom:6px}
.session-info{display:flex;align-items:center;gap:16px;flex-wrap:wrap;color:var(--text-tertiary);font-size:.78rem}
.session-info-item{display:flex;align-items:center;gap:5px}
.session-info-item svg{width:14px;height:14px;opacity:.7}
.session-divider{max-width:var(--max-w);margin:24px auto 0;padding:0 24px}
.session-divider hr{border:none;border-top:1px solid var(--border)}

.messages{max-width:var(--max-w);margin:0 auto;padding:8px 24px 80px}

.msg{padding:24px 0;position:relative}
.msg+.msg{border-top:1px solid rgba(255,255,255,.04)}
.msg-header{display:flex;align-items:center;gap:10px;margin-bottom:12px}
.avatar{width:28px;height:28px;border-radius:50%;display:flex;align-items:center;justify-content:center;font-size:.7rem;font-weight:600;flex-shrink:0}
.avatar-user{background:#444;color:#ccc}
.avatar-assistant{background:var(--accent-soft);color:var(--accent)}
.avatar-assistant svg{width:16px;height:16px}
.msg-sender{font-weight:600;font-size:.82rem}

.msg-body{padding-left:38px;overflow-wrap:break-word;word-break:break-word}
.msg-body p{margin-bottom:12px;color:var(--text)}
.msg-body p:last-child{margin-bottom:0}
.msg-body strong{font-weight:600;color:#fff}
.msg-body em{color:var(--text-secondary);font-style:italic}
.msg-body a{color:var(--blue);text-decoration:none}
.msg-body a:hover{text-decoration:underline}
.msg-body ul,.msg-body ol{margin:8px 0 12px 20px;color:var(--text-secondary)}
.msg-body li{margin-bottom:4px}
.msg-body code:not(pre code){font-family:'JetBrains Mono',monospace;font-size:.85em;background:rgba(255,255,255,.07);padding:2px 6px;border-radius:4px;color:#e0a370}
.msg-body h1,.msg-body h2,.msg-body h3,.msg-body h4{margin:1em 0 .5em;color:#fff}
.msg-body blockquote{border-left:3px solid var(--accent);padding-left:12px;color:var(--text-secondary);margin:.8em 0}
.msg-body table{border-collapse:collapse;margin:.8em 0;width:100%}
.msg-body th,.msg-body td{border:1px solid var(--border);padding:6px 10px;text-align:left}
.msg-body th{background:var(--surface)}
.msg-body pre{background:var(--code-bg);border-radius:var(--radius);padding:14px 16px;overflow-x:auto;margin:14px 0;border:1px solid var(--border)}
.msg-body pre code{font-family:'JetBrains Mono',monospace;font-size:.8rem;line-height:1.7;color:#d4d4d4;background:none;padding:0}

.msg-user .msg-body{background:var(--user-bg);padding:14px 18px;margin-left:38px;border-radius:var(--radius) var(--radius) var(--radius) 4px;overflow-wrap:break-word;word-break:break-word}
.msg-user .msg-body p{color:var(--text);margin-bottom:0}

.tool-block{margin:14px 0;border-radius:var(--radius);overflow:hidden;border:1px solid var(--border)}
.tool-header{display:flex;align-items:center;gap:8px;padding:10px 14px;background:var(--surface);font-size:.78rem;font-weight:500;color:var(--text-secondary);cursor:pointer;user-select:none;transition:background .15s}
.tool-header:hover{background:var(--surface-hover)}
.tool-header svg{width:15px;height:15px;flex-shrink:0}
.tool-icon{color:var(--accent)}
.tool-name{font-family:'JetBrains Mono',monospace;font-weight:500;font-size:.75rem}
.tool-chevron{margin-left:auto;transition:transform .2s;color:var(--text-tertiary)}
.tool-chevron.open{transform:rotate(180deg)}
.tool-body{padding:12px 16px;background:var(--code-bg);border-top:1px solid var(--border);font-family:'JetBrains Mono',monospace;font-size:.78rem;line-height:1.65;color:var(--text-secondary);max-height:300px;overflow-y:auto;display:none}
.tool-body.show{display:block}
.tool-status{display:inline-flex;align-items:center;gap:4px;font-size:.68rem;margin-left:auto;margin-right:8px}
.tool-status .dot{width:6px;height:6px;border-radius:50%}
.tool-status .dot.success{background:var(--green)}
.tool-status .dot.error{background:var(--red)}

.thinking-block{margin:14px 0;border-radius:var(--radius);border:1px solid rgba(255,255,255,.06);background:rgba(255,255,255,.02)}
.thinking-header{display:flex;align-items:center;gap:8px;padding:10px 14px;font-size:.78rem;color:var(--text-tertiary);cursor:pointer;user-select:none}
.thinking-header svg{width:14px;height:14px;opacity:.5}
.thinking-body{padding:0 14px 12px;font-size:.82rem;line-height:1.65;color:var(--text-tertiary);font-style:italic;display:none}
.thinking-body.show{display:block}

.tool-output{background:var(--code-bg);padding:10px;border-radius:4px;font-size:.8rem;white-space:pre-wrap;word-break:break-word;max-height:400px;overflow-y:auto;font-family:'JetBrains Mono',monospace;color:var(--text-secondary)}
.chroma{background:var(--code-bg)!important;border-radius:var(--radius);padding:14px 16px;overflow-x:auto;border:1px solid var(--border);margin:14px 0}

.footer{text-align:center;padding:40px 24px 32px;font-size:.72rem;color:var(--text-tertiary)}
.footer a{color:var(--accent);text-decoration:none}
.footer a:hover{text-decoration:underline}

::-webkit-scrollbar{width:6px;height:6px}
::-webkit-scrollbar-track{background:transparent}
::-webkit-scrollbar-thumb{background:#444;border-radius:3px}
::-webkit-scrollbar-thumb:hover{background:#555}

@media(max-width:640px){
  html{font-size:14px}
  .topbar-inner,.session-meta,.messages,.session-divider,.footer{padding-left:16px;padding-right:16px}
  .msg-body{padding-left:0}
  .msg-user .msg-body{margin-left:0}
  .session-title{font-size:1.15rem}
}

@keyframes fadeUp{from{opacity:0;transform:translateY(8px)}to{opacity:1;transform:translateY(0)}}
.msg{animation:fadeUp .3s ease both}
.msg:nth-child(1){animation-delay:.05s}
.msg:nth-child(2){animation-delay:.1s}
.msg:nth-child(3){animation-delay:.15s}
.msg:nth-child(4){animation-delay:.2s}
.msg:nth-child(5){animation-delay:.25s}
</style>
</head>
<body>

<nav class="topbar">
  <div class="topbar-inner">
    <div class="topbar-left">
      <span class="logo">
        <svg viewBox="0 0 24 24" fill="#D97757" width="28" height="28"><path d="M17.3041 3.541h-3.6718l6.696 16.918H24Zm-10.6082 0L0 20.459h3.7442l1.3693-3.5527h7.0052l1.3693 3.5528h3.7442L10.5363 3.5409Zm-.3712 10.2232 2.2914-5.9456 2.2914 5.9456Z"/></svg>
        <span class="logo-text">Claude Code</span>
        <span class="logo-badge">Share</span>
      </span>
    </div>
  </div>
</nav>

<div class="session-meta">
  <h1 class="session-title">{{if .Meta.FirstPrompt}}{{.Meta.FirstPrompt}}{{else}}Claude Conversation{{end}}</h1>
  <div class="session-info">
    {{if .Meta.MessageCount}}<span class="session-info-item">
      <svg fill="none" viewBox="0 0 16 16" stroke="currentColor" stroke-width="1.5"><rect x="2" y="3" width="12" height="10" rx="2"/><path d="M5 7h6M5 10h3"/></svg>
      {{.Meta.MessageCount}} messages
    </span>{{end}}
    {{if .Meta.Project}}<span class="session-info-item">
      <svg fill="none" viewBox="0 0 16 16" stroke="currentColor" stroke-width="1.5"><path d="M3 2h7l3 3v9H3z"/><path d="M10 2v3h3"/></svg>
      {{.Meta.Project}}
    </span>{{end}}
    {{if .Meta.Date}}<span class="session-info-item">
      <svg fill="none" viewBox="0 0 16 16" stroke="currentColor" stroke-width="1.5"><circle cx="8" cy="8" r="6"/><path d="M6 8h4"/></svg>
      {{.Meta.Date}}
    </span>{{end}}
  </div>
</div>
<div class="session-divider"><hr></div>

<div class="messages">
{{range .Messages}}
  {{if eq .Role "user"}}
  <div class="msg msg-user">
    <div class="msg-header">
      <div class="avatar avatar-user">U</div>
      <span class="msg-sender">You</span>
    </div>
    <div class="msg-body">
      {{range .Blocks}}
        {{if eq .Type "text"}}{{.HTML}}{{end}}
      {{end}}
    </div>
  </div>
  {{else}}
  <div class="msg msg-assistant">
    <div class="msg-header">
      <div class="avatar avatar-assistant">
        <svg viewBox="0 0 16 16" fill="currentColor"><path d="m3.127 10.604 3.135-1.76.053-.153-.053-.085H6.11l-.525-.032-1.791-.048-1.554-.065-1.505-.08-.38-.081L0 7.832l.036-.234.32-.214.455.04 1.009.069 1.513.105 1.097.064 1.626.17h.259l.036-.105-.089-.065-.068-.064-1.566-1.062-1.695-1.121-.887-.646-.48-.327-.243-.306-.104-.67.435-.48.585.04.15.04.593.456 1.267.981 1.654 1.218.242.202.097-.068.012-.049-.109-.181-.9-1.626-.96-1.655-.428-.686-.113-.411a2 2 0 0 1-.068-.484l.496-.674L4.446 0l.662.089.279.242.411.94.666 1.48 1.033 2.014.302.597.162.553.06.17h.105v-.097l.085-1.134.157-1.392.154-1.792.052-.504.25-.605.497-.327.387.186.319.456-.045.294-.19 1.23-.37 1.93-.243 1.29h.142l.161-.16.654-.868 1.097-1.372.484-.545.565-.601.363-.287h.686l.505.751-.226.775-.707.895-.585.759-.839 1.13-.524.904.048.072.125-.012 1.897-.403 1.024-.186 1.223-.21.553.258.06.263-.218.536-1.307.323-1.533.307-2.284.54-.028.02.032.04 1.029.098.44.024h1.077l2.005.15.525.346.315.424-.053.323-.807.411-3.631-.863-.872-.218h-.12v.073l.726.71 1.331 1.202 1.667 1.55.084.383-.214.302-.226-.032-1.464-1.101-.565-.497-1.28-1.077h-.084v.113l.295.432 1.557 2.34.08.718-.112.234-.404.141-.444-.08-.911-1.28-.94-1.44-.759-1.291-.093.053-.448 4.821-.21.246-.484.186-.403-.307-.214-.496.214-.98.258-1.28.21-1.016.19-1.263.112-.42-.008-.028-.092.012-.953 1.307-1.448 1.957-1.146 1.227-.274.109-.477-.247.045-.44.266-.39 1.586-2.018.956-1.25.617-.723-.004-.105h-.036l-4.212 2.736-.75.096-.324-.302.04-.496.154-.162 1.267-.871z"/></svg>
      </div>
      <span class="msg-sender">Claude</span>
    </div>
    <div class="msg-body">
      {{range .Blocks}}
        {{if eq .Type "text"}}
          {{.HTML}}
        {{else if eq .Type "thinking"}}
          <div class="thinking-block">
            <div class="thinking-header" onclick="toggleThinking(this)">
              <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="8" cy="8" r="6"/><path d="M8 5v3"/><circle cx="8" cy="11" r=".5" fill="currentColor"/></svg>
              <span>Thinking…</span>
              <svg class="tool-chevron" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" style="margin-left:auto"><path d="M4 6l4 4 4-4"/></svg>
            </div>
            <div class="thinking-body">{{.HTML}}</div>
          </div>
        {{else if eq .Type "tool_use"}}
          <div class="tool-block">
            <div class="tool-header" onclick="toggleTool(this)">
              <svg class="tool-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M4 4l4 4-4 4"/><path d="M10 12h4"/></svg>
              <span class="tool-name">{{.ToolName}}</span>
              <span class="tool-status"><span class="dot success"></span></span>
              <svg class="tool-chevron" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 6l4 4 4-4"/></svg>
            </div>
            <div class="tool-body">{{.HTML}}</div>
          </div>
        {{else if eq .Type "tool_result"}}
          <div class="tool-block">
            <div class="tool-header" onclick="toggleTool(this)">
              <svg class="tool-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M3 2h7l3 3v9H3z"/><path d="M10 2v3h3"/></svg>
              <span class="tool-name">{{if .IsError}}Error{{else}}Result{{end}}</span>
              <span class="tool-status"><span class="dot {{if .IsError}}error{{else}}success{{end}}"></span></span>
              <svg class="tool-chevron" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 6l4 4 4-4"/></svg>
            </div>
            <div class="tool-body">{{.HTML}}</div>
          </div>
        {{end}}
      {{end}}
    </div>
  </div>
  {{end}}
{{end}}
</div>

<div class="footer">
  Shared from Claude Code · Generated by Claude, an AI assistant by <a href="https://anthropic.com" target="_blank">Anthropic</a>
</div>

<script>
function toggleTool(el){
  var b=el.nextElementSibling;
  var c=el.querySelector('.tool-chevron');
  b.classList.toggle('show');
  c.classList.toggle('open');
}
function toggleThinking(el){
  var b=el.nextElementSibling;
  var c=el.querySelector('.tool-chevron');
  b.classList.toggle('show');
  c.classList.toggle('open');
}
</script>
</body>
</html>`
