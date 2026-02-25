package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var stubMeta = SessionMeta{SessionID: "t"}

func textBlock(text string) ContentBlock {
	return ContentBlock{Type: "text", Text: text}
}

func userMsg(text string) Message {
	return Message{Role: "user", Blocks: []ContentBlock{textBlock(text)}}
}

func assistantMsg(text string) Message {
	return Message{Role: "assistant", Blocks: []ContentBlock{textBlock(text)}}
}

func countClass(html, class string) int {
	return strings.Count(html, "class=\"msg "+class+"\"")
}

func TestRenderHTML_BasicConversation(t *testing.T) {
	messages := []Message{userMsg("Hello"), assistantMsg("Hi there")}
	meta := SessionMeta{SessionID: "test-123", Project: "myproject", Date: "Jan 1, 2025"}

	html, err := RenderHTML(messages, meta, RenderOpts{})
	require.NoError(t, err)
	assert.Contains(t, html, "<!DOCTYPE html>")
	assert.Equal(t, 1, countClass(html, "msg-user"))
	assert.Equal(t, 1, countClass(html, "msg-assistant"))
	assert.Contains(t, html, "myproject")
	assert.Contains(t, html, "Jan 1, 2025")
}

func TestRenderHTML_SkipsUserToolResultMessages(t *testing.T) {
	messages := []Message{
		userMsg("Do something"),
		assistantMsg("Done"),
		{Role: "user", Blocks: []ContentBlock{{Type: "tool_result", Text: "output"}}},
	}

	html, err := RenderHTML(messages, stubMeta, RenderOpts{})
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(html, "class=\"msg "))
}

func TestRenderHTML_IncludesToolUse(t *testing.T) {
	messages := []Message{
		{Role: "assistant", Blocks: []ContentBlock{
			textBlock("Let me check"),
			{Type: "tool_use", ToolName: "Read", ToolInput: `{"path":"/tmp"}`},
		}},
	}

	html, err := RenderHTML(messages, stubMeta, RenderOpts{IncludeTools: true})
	require.NoError(t, err)
	assert.Contains(t, html, "tool-block")
	assert.Contains(t, html, "Read")
}

func TestRenderHTML_IncludesThinking(t *testing.T) {
	messages := []Message{
		{Role: "assistant", Blocks: []ContentBlock{
			{Type: "thinking", Text: "Let me think about this"},
			textBlock("Here is my answer"),
		}},
	}

	html, err := RenderHTML(messages, stubMeta, RenderOpts{IncludeThinking: true})
	require.NoError(t, err)
	assert.Contains(t, html, "thinking-block")
	assert.Contains(t, html, "Thinking")
}

func TestRenderHTML_MetaFields(t *testing.T) {
	meta := SessionMeta{
		SessionID:    "abc-123",
		Project:      "testproj",
		Date:         "Feb 25, 2026",
		MessageCount: 42,
		FirstPrompt:  "Fix the bug",
	}

	html, err := RenderHTML([]Message{userMsg("hi")}, meta, RenderOpts{})
	require.NoError(t, err)
	assert.Contains(t, html, "testproj")
	assert.Contains(t, html, "Feb 25, 2026")
	assert.Contains(t, html, "42 messages")
	assert.Contains(t, html, "Fix the bug")
}

func TestRenderHTML_FallbackTitle(t *testing.T) {
	html, err := RenderHTML([]Message{userMsg("hi")}, SessionMeta{SessionID: "t", FirstPrompt: ""}, RenderOpts{})
	require.NoError(t, err)
	assert.Contains(t, html, "Claude Conversation")
}

func TestRenderHTML_EmptyMessages(t *testing.T) {
	html, err := RenderHTML(nil, stubMeta, RenderOpts{})
	require.NoError(t, err)
	assert.Contains(t, html, "<!DOCTYPE html>")
	assert.Equal(t, 0, countClass(html, "msg-user"))
	assert.Equal(t, 0, countClass(html, "msg-assistant"))
}

func TestRenderHTML_SkipsMessagesWithNoVisibleBlocks(t *testing.T) {
	messages := []Message{
		{Role: "user", Blocks: []ContentBlock{
			{Type: "tool_result", Text: "result1"},
			{Type: "tool_result", Text: "result2"},
			{Type: "tool_result", Text: "result3"},
		}},
		assistantMsg("response"),
	}

	html, err := RenderHTML(messages, stubMeta, RenderOpts{})
	require.NoError(t, err)
	assert.Equal(t, 1, countClass(html, "msg-assistant"))
	assert.Equal(t, 0, countClass(html, "msg-user"))
}

func TestRenderMarkdown_BasicText(t *testing.T) {
	assert.Contains(t, renderMarkdown("Hello **world**"), "<strong>world</strong>")
}

func TestRenderMarkdown_CodeBlock(t *testing.T) {
	assert.Contains(t, renderMarkdown("```go\nfmt.Println(\"hi\")\n```"), "style=")
}

func TestRenderMarkdown_InlineCode(t *testing.T) {
	assert.Contains(t, renderMarkdown("Use `fmt.Println`"), "<code>")
}

func TestRenderMarkdown_Links(t *testing.T) {
	result := renderMarkdown("[click](https://example.com)")
	assert.Contains(t, result, "https://example.com")
	assert.Contains(t, result, "target=\"_blank\"")
}

func TestRenderMarkdown_List(t *testing.T) {
	assert.Contains(t, renderMarkdown("- one\n- two\n- three"), "<li>")
}

func TestTruncate_Short(t *testing.T) {
	assert.Equal(t, "hello", truncate("hello", 100))
}

func TestTruncate_ExactLength(t *testing.T) {
	assert.Equal(t, "hello", truncate("hello", 5))
}

func TestTruncate_Long(t *testing.T) {
	assert.Equal(t, "hello\n... (truncated)", truncate("hello world", 5))
}

func TestHighlightCode_ValidLanguage(t *testing.T) {
	result, err := highlightCode("x := 1", "go")
	require.NoError(t, err)
	assert.Contains(t, result, "<pre")
	assert.Contains(t, result, "style=")
}

func TestHighlightCode_UnknownLanguage(t *testing.T) {
	result, err := highlightCode("some text", "nonexistentlang")
	require.NoError(t, err)
	assert.Contains(t, result, "some text")
}

func TestHighlightJSON_ValidJSON(t *testing.T) {
	result := highlightJSON(`{"key":"value"}`)
	assert.Contains(t, result, "key")
	assert.Contains(t, result, "value")
}

func TestHighlightJSON_InvalidJSON(t *testing.T) {
	result := highlightJSON(`not json`)
	assert.Contains(t, result, "not")
	assert.Contains(t, result, "json")
}
