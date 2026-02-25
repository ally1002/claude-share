package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func writeSession(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	return writeTempFile(t, dir, "session.jsonl", content)
}

func TestParseHistory_Basic(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "history.jsonl",
		`{"display":"hello world","timestamp":1000,"project":"/home/user/proj","sessionId":"aaa"}
{"display":"second prompt","timestamp":2000,"project":"/home/user/proj","sessionId":"bbb"}
`)

	sessions, err := ParseHistory(dir)
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
	assert.Equal(t, "bbb", sessions[0].ID)
	assert.Equal(t, "aaa", sessions[1].ID)
	assert.Equal(t, "hello world", sessions[1].FirstPrompt)
	assert.Equal(t, int64(2000), sessions[0].Timestamp)
}

func TestParseHistory_DeduplicatesBySessionID(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "history.jsonl",
		`{"display":"first","timestamp":1000,"project":"/proj","sessionId":"aaa"}
{"display":"second","timestamp":2000,"project":"/proj","sessionId":"aaa"}
{"display":"third","timestamp":3000,"project":"/proj","sessionId":"aaa"}
`)

	sessions, err := ParseHistory(dir)
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, "first", sessions[0].FirstPrompt)
}

func TestParseHistory_SkipsEmptySessionID(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "history.jsonl",
		`{"display":"no id","timestamp":1000,"project":"/proj","sessionId":""}
{"display":"has id","timestamp":2000,"project":"/proj","sessionId":"bbb"}
`)

	sessions, err := ParseHistory(dir)
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, "bbb", sessions[0].ID)
}

func TestParseHistory_SkipsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "history.jsonl",
		`not json at all
{"display":"good","timestamp":1000,"project":"/proj","sessionId":"aaa"}
{broken json
`)

	sessions, err := ParseHistory(dir)
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, "aaa", sessions[0].ID)
}

func TestParseHistory_MissingFile(t *testing.T) {
	_, err := ParseHistory(t.TempDir())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "open history")
}

func TestFindSessionPath_FindsJSONL(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "projects", "my-project")
	writeTempFile(t, projDir, "sess-123.jsonl", `{}`)

	path, err := FindSessionPath(dir, "sess-123")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(projDir, "sess-123.jsonl"), path)
}

func TestFindSessionPath_NotFound(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "projects", "my-project"), 0755))

	_, err := FindSessionPath(dir, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestFindSessionPath_NoProjectsDir(t *testing.T) {
	_, err := FindSessionPath(t.TempDir(), "anything")
	assert.Error(t, err)
}

func TestParseSession_PlainTextUserMessage(t *testing.T) {
	path := writeSession(t,
		`{"type":"user","timestamp":"2025-01-01T00:00:00Z","message":{"id":"u1","role":"user","content":"Hello Claude"}}
`)

	msgs, err := ParseSession(path, ParseOpts{})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "user", msgs[0].Role)
	assert.Len(t, msgs[0].Blocks, 1)
	assert.Equal(t, "text", msgs[0].Blocks[0].Type)
	assert.Equal(t, "Hello Claude", msgs[0].Blocks[0].Text)
}

func TestParseSession_AssistantGroupsByMessageID(t *testing.T) {
	path := writeSession(t,
		`{"type":"assistant","timestamp":"2025-01-01T00:00:00Z","message":{"id":"msg1","role":"assistant","content":[{"type":"text","text":"Hello"}]}}
{"type":"assistant","timestamp":"2025-01-01T00:00:01Z","message":{"id":"msg1","role":"assistant","content":[{"type":"text","text":" world"}],"stop_reason":"end_turn"}}
`)

	msgs, err := ParseSession(path, ParseOpts{})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "assistant", msgs[0].Role)
	assert.Len(t, msgs[0].Blocks, 2)
	assert.Equal(t, "Hello", msgs[0].Blocks[0].Text)
	assert.Equal(t, " world", msgs[0].Blocks[1].Text)
}

func TestParseSession_InterleaveOrder(t *testing.T) {
	path := writeSession(t,
		`{"type":"user","timestamp":"T1","message":{"id":"u1","role":"user","content":"Question 1"}}
{"type":"assistant","timestamp":"T2","message":{"id":"a1","role":"assistant","content":[{"type":"text","text":"Answer 1"}],"stop_reason":"end_turn"}}
{"type":"user","timestamp":"T3","message":{"id":"u2","role":"user","content":"Question 2"}}
{"type":"assistant","timestamp":"T4","message":{"id":"a2","role":"assistant","content":[{"type":"text","text":"Answer 2"}],"stop_reason":"end_turn"}}
`)

	msgs, err := ParseSession(path, ParseOpts{})
	require.NoError(t, err)
	require.Len(t, msgs, 4)
	assert.Equal(t, "user", msgs[0].Role)
	assert.Equal(t, "assistant", msgs[1].Role)
	assert.Equal(t, "user", msgs[2].Role)
	assert.Equal(t, "assistant", msgs[3].Role)
}

func TestParseSession_SkipsMetaMessages(t *testing.T) {
	path := writeSession(t,
		`{"type":"user","timestamp":"T1","isMeta":true,"message":{"id":"u1","role":"user","content":"<system>meta</system>"}}
{"type":"user","timestamp":"T2","message":{"id":"u2","role":"user","content":"real message"}}
`)

	msgs, err := ParseSession(path, ParseOpts{})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "real message", msgs[0].Blocks[0].Text)
}

func TestParseSession_SkipsProgressAndFileHistory(t *testing.T) {
	path := writeSession(t,
		`{"type":"file-history-snapshot","uuid":null}
{"type":"progress","uuid":"xxx"}
{"type":"user","timestamp":"T1","message":{"id":"u1","role":"user","content":"hello"}}
`)

	msgs, err := ParseSession(path, ParseOpts{})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
}

func TestParseSession_ThinkingIncluded(t *testing.T) {
	path := writeSession(t,
		`{"type":"assistant","timestamp":"T1","message":{"id":"a1","role":"assistant","content":[{"type":"thinking","thinking":"Let me think..."}]}}
{"type":"assistant","timestamp":"T2","message":{"id":"a1","role":"assistant","content":[{"type":"text","text":"Here is my answer"}],"stop_reason":"end_turn"}}
`)

	msgs, err := ParseSession(path, ParseOpts{IncludeThinking: false})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Len(t, msgs[0].Blocks, 1)
	assert.Equal(t, "text", msgs[0].Blocks[0].Type)

	msgs, err = ParseSession(path, ParseOpts{IncludeThinking: true})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Len(t, msgs[0].Blocks, 2)
	assert.Equal(t, "thinking", msgs[0].Blocks[0].Type)
	assert.Equal(t, "Let me think...", msgs[0].Blocks[0].Text)
}

func TestParseSession_ThinkingFallsBackToText(t *testing.T) {
	path := writeSession(t,
		`{"type":"assistant","timestamp":"T1","message":{"id":"a1","role":"assistant","content":[{"type":"thinking","text":"fallback text","thinking":""}],"stop_reason":"end_turn"}}
`)

	msgs, err := ParseSession(path, ParseOpts{IncludeThinking: true})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "fallback text", msgs[0].Blocks[0].Text)
}

func TestParseSession_ToolUseIncluded(t *testing.T) {
	path := writeSession(t,
		`{"type":"assistant","timestamp":"T1","message":{"id":"a1","role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"Read","input":{"path":"/tmp/x"}}]}}
{"type":"assistant","timestamp":"T2","message":{"id":"a1","role":"assistant","content":[{"type":"text","text":"Done"}],"stop_reason":"end_turn"}}
`)

	msgs, err := ParseSession(path, ParseOpts{IncludeTools: false})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Len(t, msgs[0].Blocks, 1)
	assert.Equal(t, "text", msgs[0].Blocks[0].Type)

	msgs, err = ParseSession(path, ParseOpts{IncludeTools: true})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Len(t, msgs[0].Blocks, 2)
	assert.Equal(t, "tool_use", msgs[0].Blocks[0].Type)
	assert.Equal(t, "Read", msgs[0].Blocks[0].ToolName)
	assert.Equal(t, "toolu_1", msgs[0].Blocks[0].ToolUseID)
	assert.Contains(t, msgs[0].Blocks[0].ToolInput, "/tmp/x")
}

func TestParseSession_ToolResultUserMessage(t *testing.T) {
	path := writeSession(t,
		`{"type":"user","timestamp":"T1","message":{"id":"u1","role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"file contents here"}]}}
`)

	msgs, err := ParseSession(path, ParseOpts{IncludeTools: false})
	require.NoError(t, err)
	assert.Len(t, msgs, 0)

	msgs, err = ParseSession(path, ParseOpts{IncludeTools: true})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "tool_result", msgs[0].Blocks[0].Type)
	assert.Equal(t, "file contents here", msgs[0].Blocks[0].Text)
}

func TestParseSession_ToolResultArrayContent(t *testing.T) {
	path := writeSession(t,
		`{"type":"user","timestamp":"T1","message":{"id":"u1","role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":[{"type":"text","text":"line one"},{"type":"text","text":"line two"}]}]}}
`)

	msgs, err := ParseSession(path, ParseOpts{IncludeTools: true})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "line one\nline two", msgs[0].Blocks[0].Text)
}

func TestParseSession_ToolResultError(t *testing.T) {
	path := writeSession(t,
		`{"type":"user","timestamp":"T1","message":{"id":"u1","role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"boom","is_error":true}]}}
`)

	msgs, err := ParseSession(path, ParseOpts{IncludeTools: true})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.True(t, msgs[0].Blocks[0].IsError)
}

func TestParseSession_EmptyAssistantBlocksSkipped(t *testing.T) {
	path := writeSession(t,
		`{"type":"assistant","timestamp":"T1","message":{"id":"a1","role":"assistant","content":[{"type":"text","text":""}],"stop_reason":"end_turn"}}
`)

	msgs, err := ParseSession(path, ParseOpts{})
	require.NoError(t, err)
	assert.Len(t, msgs, 0)
}

func TestParseSession_MissingFile(t *testing.T) {
	_, err := ParseSession("/nonexistent/path.jsonl", ParseOpts{})
	assert.Error(t, err)
}

func TestExtractToolResultContent_String(t *testing.T) {
	assert.Equal(t, "hello", extractToolResultContent([]byte(`"hello"`)))
}

func TestExtractToolResultContent_Array(t *testing.T) {
	assert.Equal(t, "a\nb", extractToolResultContent([]byte(`[{"type":"text","text":"a"},{"type":"text","text":"b"}]`)))
}

func TestExtractToolResultContent_Null(t *testing.T) {
	assert.Equal(t, "", extractToolResultContent(nil))
}

func TestExtractToolResultContent_RawFallback(t *testing.T) {
	assert.Equal(t, "12345", extractToolResultContent([]byte(`12345`)))
}
