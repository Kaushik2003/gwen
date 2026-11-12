package remote

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kzark/gwen/internal/wire"
)

// A Claude Code transcript is a JSONL file Claude Code owns. Gwen reads the
// few line types it shows and skips the rest, so a new Claude Code version
// that adds lines changes nothing here (plan.md 3.14.9).

const (
	maxTranscriptLine = 64 << 20
	maxEventText      = 24 << 10
	failedOutputLines = 40
)

// transcript is the parsed events of one file, extended as the file grows.
type transcript struct {
	mu     sync.Mutex
	path   string
	offset int64 // bytes parsed so far
	events []wire.CodeTranscriptEvent
	// tools are the tool calls seen and not yet answered, by tool_use id.
	tools map[string]toolCall
	title string // newest custom title, else the AI's title
	first string // first prompt
	cwd   string
}

type toolCall struct {
	name, summary  string
	added, removed int
}

// transcripts caches the files being read.
type transcripts struct {
	mu    sync.Mutex
	files map[string]*transcript
}

func (c *transcripts) get(path string) *transcript {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.files == nil {
		c.files = map[string]*transcript{}
	}
	t := c.files[path]
	if t == nil {
		t = &transcript{path: path, tools: map[string]toolCall{}}
		c.files[path] = t
	}
	return t
}

func (c *transcripts) drop(path string) {
	c.mu.Lock()
	delete(c.files, path)
	c.mu.Unlock()
}

// refresh parses what was appended since the last call.
func (t *transcript) refresh() error {
	f, err := os.Open(t.path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if st.Size() < t.offset { // rewritten: start again
		t.offset, t.events, t.tools, t.title, t.first = 0, nil, map[string]toolCall{}, "", ""
	}
	if st.Size() == t.offset {
		return nil
	}
	if _, err := f.Seek(t.offset, io.SeekStart); err != nil {
		return err
	}
	r := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := readLongLine(r)
		if err != nil {
			// A last line without its newline is still being written.
			return nil
		}
		at := t.offset
		t.offset += int64(len(line)) + 1
		t.parse(at, line)
	}
}

// readLongLine reads one line without its newline; a line still being
// written (no newline yet) is io.EOF.
func readLongLine(r *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		part, err := r.ReadSlice('\n')
		line = append(line, part...)
		if err == nil {
			return line[:len(line)-1], nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return nil, io.EOF
		}
		if len(line) > maxTranscriptLine {
			return nil, io.EOF
		}
	}
}

type transcriptLine struct {
	Type             string `json:"type"`
	Subtype          string `json:"subtype"`
	Timestamp        string `json:"timestamp"`
	IsMeta           bool   `json:"isMeta"`
	IsSidechain      bool   `json:"isSidechain"`
	IsCompactSummary bool   `json:"isCompactSummary"`
	Cwd              string `json:"cwd"`
	CustomTitle      string `json:"customTitle"`
	AITitle          string `json:"aiTitle"`
	Content          string `json:"content"`
	Message          struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
	Content   json.RawMessage `json:"content"`
}

// shownTypes are the line types worth decoding; most of a transcript is
// bookkeeping lines far larger than these.
var shownTypes = [][]byte{[]byte(`"type":"user"`), []byte(`"type":"assistant"`), []byte(`"type":"system"`),
	[]byte(`"type":"custom-title"`), []byte(`"type":"ai-title"`)}

func (t *transcript) parse(at int64, line []byte) {
	shown := false
	for _, mark := range shownTypes {
		if bytes.Contains(line, mark) {
			shown = true
			break
		}
	}
	if !shown {
		return
	}
	var l transcriptLine
	if json.Unmarshal(line, &l) != nil || l.IsSidechain {
		return
	}
	when := int64(0)
	if ts, err := time.Parse(time.RFC3339Nano, l.Timestamp); err == nil {
		when = ts.UnixMilli()
	}
	if t.cwd == "" && l.Cwd != "" {
		t.cwd = l.Cwd
	}
	add := func(ev wire.CodeTranscriptEvent) {
		ev.Cursor, ev.At = at, when
		if len(ev.Text) > maxEventText {
			ev.Text = ev.Text[:maxEventText] + "\n…"
		}
		t.events = append(t.events, ev)
	}
	switch l.Type {
	case "custom-title":
		if l.CustomTitle != "" {
			t.title = l.CustomTitle
		}
	case "ai-title":
		if t.title == "" && l.AITitle != "" {
			t.title = l.AITitle
		}
	case "system":
		switch l.Subtype {
		case "compact_boundary":
			add(wire.CodeTranscriptEvent{Kind: wire.TranscriptNotice, Text: "Conversation compacted", OK: true})
		case "api_error":
			add(wire.CodeTranscriptEvent{Kind: wire.TranscriptNotice, Text: "Claude hit an error and is retrying", OK: false})
		}
	case "assistant":
		var blocks []contentBlock
		if json.Unmarshal(l.Message.Content, &blocks) != nil {
			return
		}
		for _, b := range blocks {
			switch b.Type {
			case "text":
				if s := strings.TrimSpace(b.Text); s != "" {
					add(wire.CodeTranscriptEvent{Kind: wire.TranscriptClaude, Text: s, OK: true})
				}
			case "tool_use":
				call := toolCall{name: b.Name}
				call.summary, call.added, call.removed = summarizeTool(b.Name, b.Input)
				t.tools[b.ID] = call
			}
		}
	case "user":
		if l.IsMeta {
			return
		}
		if l.IsCompactSummary {
			return
		}
		var text string
		if json.Unmarshal(l.Message.Content, &text) == nil {
			t.prompt(add, text)
			return
		}
		var blocks []contentBlock
		if json.Unmarshal(l.Message.Content, &blocks) != nil {
			return
		}
		var parts []string
		for _, b := range blocks {
			switch b.Type {
			case "text":
				parts = append(parts, b.Text)
			case "image":
				parts = append(parts, "[image]")
			case "tool_result":
				call, ok := t.tools[b.ToolUseID]
				if !ok {
					continue
				}
				delete(t.tools, b.ToolUseID)
				ev := wire.CodeTranscriptEvent{Kind: wire.TranscriptTool, Tool: call.name, Text: call.summary, OK: !b.IsError}
				if ev.OK {
					ev.Added, ev.Removed = call.added, call.removed
				} else if call.name == "Bash" {
					ev.Output = lastLines(resultText(b.Content), failedOutputLines)
				}
				add(ev)
			}
		}
		if len(parts) > 0 {
			t.prompt(add, strings.Join(parts, "\n"))
		}
	}
}

// prompt adds what the person typed, leaving out the wrappers Claude Code
// records around commands and reminders.
func (t *transcript) prompt(add func(wire.CodeTranscriptEvent), text string) {
	text = strings.TrimSpace(text)
	switch {
	case text == "":
		return
	case strings.HasPrefix(text, "[Request interrupted by user"):
		add(wire.CodeTranscriptEvent{Kind: wire.TranscriptNotice, Text: "Interrupted", OK: true})
		return
	case strings.HasPrefix(text, "<command-name>"):
		name := between(text, "<command-name>", "</command-name>")
		args := between(text, "<command-args>", "</command-args>")
		text = strings.TrimSpace(name + " " + args)
		if text == "" {
			return
		}
	case strings.HasPrefix(text, "<"):
		// <local-command-stdout>, <system-reminder>, <task-notification>, ...
		return
	}
	if t.first == "" {
		t.first = text
	}
	add(wire.CodeTranscriptEvent{Kind: wire.TranscriptYou, Text: text, OK: true})
}

func between(s, open, end string) string {
	_, rest, ok := strings.Cut(s, open)
	if !ok {
		return ""
	}
	inner, _, _ := strings.Cut(rest, end)
	return strings.TrimSpace(inner)
}

// summarizeTool is a tool call in one line, and for an edit how many lines
// it adds and removes.
func summarizeTool(name string, input json.RawMessage) (summary string, added, removed int) {
	var in struct {
		Command      string `json:"command"`
		Description  string `json:"description"`
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
		Pattern      string `json:"pattern"`
		Query        string `json:"query"`
		URL          string `json:"url"`
		Prompt       string `json:"prompt"`
		OldString    string `json:"old_string"`
		NewString    string `json:"new_string"`
		Content      string `json:"content"`
		NewSource    string `json:"new_source"`
		Edits        []struct {
			OldString string `json:"old_string"`
			NewString string `json:"new_string"`
		} `json:"edits"`
	}
	_ = json.Unmarshal(input, &in)
	path := in.FilePath
	if path == "" {
		path = in.NotebookPath
	}
	switch name {
	case "Bash":
		return oneLine(in.Command, 200), 0, 0
	case "Edit":
		return path, countLines(in.NewString), countLines(in.OldString)
	case "MultiEdit":
		for _, e := range in.Edits {
			added += countLines(e.NewString)
			removed += countLines(e.OldString)
		}
		return path, added, removed
	case "Write":
		return path, countLines(in.Content), 0
	case "NotebookEdit":
		return path, countLines(in.NewSource), 0
	case "Read":
		return path, 0, 0
	case "Grep", "Glob":
		return oneLine(in.Pattern, 120), 0, 0
	case "WebSearch":
		return oneLine(in.Query, 120), 0, 0
	case "WebFetch":
		return oneLine(in.URL, 120), 0, 0
	}
	for _, s := range []string{in.Description, in.Prompt, path, in.Command, in.Pattern} {
		if s != "" {
			return oneLine(s, 120), 0, 0
		}
	}
	return "", 0, 0
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(strings.TrimSuffix(s, "\n"), "\n") + 1
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	return truncate(s, max)
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// resultText is a tool result's text: a string, or the text blocks of a list.
func resultText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []contentBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return truncate(strings.Join(lines, "\n"), 8000)
}

// page returns up to limit events: the newest before cursor `before`
// (negative: the newest of all), or with after >= 0 the events past it.
func (t *transcript) page(before, after int64, limit int) wire.CodeTranscript {
	t.mu.Lock()
	defer t.mu.Unlock()
	_ = t.refresh()
	out := wire.CodeTranscript{Events: []wire.CodeTranscriptEvent{}, After: t.offset}
	evs := t.events
	if after >= 0 {
		i := sort.Search(len(evs), func(i int) bool { return evs[i].Cursor >= after })
		// Several events can share a line's cursor; `after` is the offset the
		// client was given, which is always past whole lines.
		out.Events = append(out.Events, evs[i:]...)
		return out
	}
	end := len(evs)
	if before >= 0 {
		end = sort.Search(len(evs), func(i int) bool { return evs[i].Cursor >= before })
	}
	start := max(0, end-limit)
	// Never split the events of one line across pages.
	for start > 0 && evs[start-1].Cursor == evs[start].Cursor {
		start--
	}
	out.Events = append(out.Events, evs[start:end]...)
	if start > 0 {
		c := evs[start].Cursor
		out.Before = &c
	}
	return out
}

// head reads what names a conversation without keeping it cached: its
// folder, title, first prompt, and when it was last written.
func transcriptHead(path string) (cwd, title string, active time.Time, ok bool) {
	st, err := os.Stat(path)
	if err != nil {
		return "", "", time.Time{}, false
	}
	t := &transcript{path: path, tools: map[string]toolCall{}}
	if t.refresh() != nil {
		return "", "", time.Time{}, false
	}
	title = t.title
	if title == "" {
		title = truncate(oneLine(t.first, 400), 120)
	}
	return t.cwd, title, st.ModTime(), t.cwd != ""
}

// transcriptCwd reads a transcript's working folder from its first lines.
func transcriptCwd(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 256<<10)
	for range 40 {
		line, err := readLongLine(r)
		if err != nil {
			return ""
		}
		if !bytes.Contains(line, []byte(`"cwd"`)) {
			continue
		}
		var l struct {
			Cwd string `json:"cwd"`
		}
		if json.Unmarshal(line, &l) == nil && l.Cwd != "" {
			return l.Cwd
		}
	}
	return ""
}

// claudeProjectsDir is where Claude Code keeps its transcripts.
func claudeProjectsDir(home string) string { return filepath.Join(home, ".claude", "projects") }
