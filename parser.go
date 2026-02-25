package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type SessionSummary struct {
	ID          string
	Project     string
	FirstPrompt string
	Timestamp   int64 // Unix ms
}

type Message struct {
	Role      string // "user" or "assistant"
	Blocks    []ContentBlock
	Timestamp string // ISO 8601
}

type ContentBlock struct {
	Type      string // "text", "thinking", "tool_use", "tool_result"
	Text      string
	ToolName  string
	ToolInput string // JSON
	ToolUseID string
	IsError   bool
}

type ParseOpts struct {
	IncludeTools    bool
	IncludeThinking bool
}

type historyEntry struct {
	Display   string `json:"display"`
	Timestamp int64  `json:"timestamp"`
	Project   string `json:"project"`
	SessionID string `json:"sessionId"`
}

type sessionRow struct {
	Type      string          `json:"type"`
	UUID      string          `json:"uuid"`
	Timestamp string          `json:"timestamp"`
	SessionID string          `json:"sessionId"`
	IsMeta    bool            `json:"isMeta"`
	Message   json.RawMessage `json:"message"`
	Content   json.RawMessage `json:"content"`
	Subtype   string          `json:"subtype"`
}

type apiMessage struct {
	ID         string          `json:"id"`
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	StopReason *string         `json:"stop_reason"`
}

type contentBlockRaw struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

func ParseHistory(claudeDir string) ([]SessionSummary, error) {
	path := filepath.Join(claudeDir, "history.jsonl")
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open history: %w", err)
	}
	defer f.Close()

	seen := make(map[string]*SessionSummary)
	var order []string

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		var e historyEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		if e.SessionID == "" {
			continue
		}
		if _, ok := seen[e.SessionID]; !ok {
			seen[e.SessionID] = &SessionSummary{
				ID:          e.SessionID,
				Project:     e.Project,
				FirstPrompt: e.Display,
				Timestamp:   e.Timestamp,
			}
			order = append(order, e.SessionID)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan history: %w", err)
	}

	results := make([]SessionSummary, 0, len(order))
	for _, id := range order {
		results = append(results, *seen[id])
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp > results[j].Timestamp
	})
	return results, nil
}

func FindSessionPath(claudeDir, sessionID string) (string, error) {
	projectsDir := filepath.Join(claudeDir, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return "", fmt.Errorf("read projects dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		jsonlPath := filepath.Join(projectsDir, entry.Name(), sessionID+".jsonl")
		if info, err := os.Stat(jsonlPath); err == nil && !info.IsDir() {
			return jsonlPath, nil
		}
	}
	return "", fmt.Errorf("session %s not found", sessionID)
}

func ParseSession(path string, opts ParseOpts) ([]Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open session: %w", err)
	}
	defer f.Close()

	type userEntry struct {
		msg Message
		seq int
	}
	type assistantGroup struct {
		blocks   []ContentBlock
		ts       string
		firstSeq int
	}

	var userMsgs []userEntry
	assistantGroups := make(map[string]*assistantGroup)
	var assistantIDs []string

	seq := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		var row sessionRow
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			seq++
			continue
		}

		switch row.Type {
		case "user":
			if row.IsMeta {
				seq++
				continue
			}
			msg := parseUserRow(row, opts)
			if msg != nil {
				userMsgs = append(userMsgs, userEntry{msg: *msg, seq: seq})
			}

		case "assistant":
			if row.Message == nil {
				seq++
				continue
			}
			var api apiMessage
			if err := json.Unmarshal(row.Message, &api); err != nil {
				seq++
				continue
			}
			blocks := extractAssistantBlocks(api.Content, opts)
			if len(blocks) == 0 {
				seq++
				continue
			}
			grp, exists := assistantGroups[api.ID]
			if !exists {
				grp = &assistantGroup{ts: row.Timestamp, firstSeq: seq}
				assistantGroups[api.ID] = grp
				assistantIDs = append(assistantIDs, api.ID)
			}
			grp.blocks = append(grp.blocks, blocks...)
		}
		seq++
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	type seqMsg struct {
		msg Message
		seq int
	}
	var all []seqMsg
	for _, u := range userMsgs {
		all = append(all, seqMsg{msg: u.msg, seq: u.seq})
	}
	for _, id := range assistantIDs {
		grp := assistantGroups[id]
		if len(grp.blocks) > 0 {
			all = append(all, seqMsg{
				msg: Message{Role: "assistant", Blocks: grp.blocks, Timestamp: grp.ts},
				seq: grp.firstSeq,
			})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].seq < all[j].seq })

	msgs := make([]Message, len(all))
	for i, a := range all {
		msgs[i] = a.msg
	}
	return msgs, nil
}

func parseUserRow(row sessionRow, opts ParseOpts) *Message {
	if row.IsMeta || row.Message == nil {
		return nil
	}

	var api apiMessage
	if err := json.Unmarshal(row.Message, &api); err != nil {
		return nil
	}

	msg := &Message{Role: "user", Timestamp: row.Timestamp}

	var rawStr string
	if err := json.Unmarshal(api.Content, &rawStr); err == nil {
		if strings.Contains(rawStr, "<command-name>") || strings.Contains(rawStr, "<local-command") || strings.Contains(rawStr, "<system-reminder>") {
			return nil
		}
		msg.Blocks = []ContentBlock{{Type: "text", Text: rawStr}}
		return msg
	}

	var blocks []contentBlockRaw
	if err := json.Unmarshal(api.Content, &blocks); err != nil {
		return nil
	}

	for _, b := range blocks {
		switch b.Type {
		case "tool_result":
			if !opts.IncludeTools {
				continue
			}
			text := extractToolResultContent(b.Content)
			msg.Blocks = append(msg.Blocks, ContentBlock{
				Type:      "tool_result",
				Text:      text,
				ToolUseID: b.ToolUseID,
				IsError:   b.IsError,
			})
		case "text":
			if strings.Contains(b.Text, "<command-name>") || strings.Contains(b.Text, "<local-command") || strings.Contains(b.Text, "<system-reminder>") {
				continue
			}
			msg.Blocks = append(msg.Blocks, ContentBlock{Type: "text", Text: b.Text})
		}
	}

	if len(msg.Blocks) == 0 {
		return nil
	}
	return msg
}

func extractAssistantBlocks(raw json.RawMessage, opts ParseOpts) []ContentBlock {
	var blocks []contentBlockRaw
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}

	var result []ContentBlock
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				result = append(result, ContentBlock{Type: "text", Text: b.Text})
			}
		case "thinking":
			if !opts.IncludeThinking {
				continue
			}
			text := b.Thinking
			if text == "" {
				text = b.Text
			}
			if text != "" {
				result = append(result, ContentBlock{Type: "thinking", Text: text})
			}
		case "tool_use":
			if !opts.IncludeTools {
				continue
			}
			inputStr := "{}"
			if b.Input != nil {
				inputStr = string(b.Input)
			}
			result = append(result, ContentBlock{
				Type:      "tool_use",
				ToolName:  b.Name,
				ToolInput: inputStr,
				ToolUseID: b.ID,
			})
		}
	}
	return result
}

func extractToolResultContent(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var arr []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &arr); err == nil {
		var parts []string
		for _, a := range arr {
			if a.Text != "" {
				parts = append(parts, a.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return string(raw)
}
