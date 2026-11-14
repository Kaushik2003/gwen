package remote

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

// Which Claude conversation made a commit (plan.md 3.15, History). Claude
// Code keeps each conversation as a transcript, and `git commit` prints the
// new commit's short id: "[main 20dd8fb] subject". Gwen reads the transcripts
// of a project's folder and the folders inside it for the result of each Bash
// call that ran git commit. It remembers how far it read each file, so a
// second look reads only what was added since.

// commitPrinted is the line git commit prints for the commit it made.
var commitPrinted = regexp.MustCompile(`(?m)^\[[^\]\s]+(?: \(root-commit\))? ([0-9a-f]{7,40})\] `)

// commitCalls finds a Bash call that may commit: the line names git and
// commit. Only these lines are decoded.
var commitCalls = [][]byte{[]byte(`"tool_use"`), []byte(`git`), []byte(`commit`)}

// commitScan is what Gwen read from one transcript so far.
type commitScan struct {
	size    int64             // bytes read, up to the end of the last whole line
	pending map[string]string // the commands of calls waiting for their result, by tool_use id
	commits []foundCommit
	// head holds the conversation's folder, title and first prompt, fed
	// the lines that name them.
	head *transcript
}

// foundCommit is a commit a call made: the short id git printed, or, when
// it printed none (git commit -q, or its output cut off), the command, until
// git itself names the commit (resolveQuiet).
type foundCommit struct {
	short   string
	at      int64
	command string
	tried   bool // resolveQuiet looked and found nothing
}

// commitScans are the transcripts read, by path.
type commitScans struct {
	mu    sync.Mutex
	files map[string]*commitScan
}

// scan reads what was added to path since the last time and returns the
// result, nil when the file cannot be read. The caller must not change it.
func (c *commitScans) scan(path string) *commitScan {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.files == nil {
		c.files = map[string]*commitScan{}
	}
	sc := c.files[path]
	st, err := os.Stat(path)
	if err != nil {
		delete(c.files, path)
		return nil
	}
	if sc == nil || st.Size() < sc.size {
		sc = &commitScan{pending: map[string]string{}, head: &transcript{path: path, tools: map[string]toolCall{}}}
		c.files[path] = sc
	}
	if st.Size() == sc.size {
		return sc
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	if _, err := f.Seek(sc.size, io.SeekStart); err != nil {
		return nil
	}
	r := bufio.NewReaderSize(f, 256<<10)
	for {
		line, err := readLongLine(r)
		if errors.Is(err, io.EOF) {
			break // a line still being written is read next time
		}
		if err != nil {
			return sc
		}
		sc.size += int64(len(line)) + 1
		sc.read(line)
	}
	return sc
}

// read takes one whole line of a transcript.
func (sc *commitScan) read(line []byte) {
	switch {
	case bytes.Contains(line, []byte(`"type":"custom-title"`)), bytes.Contains(line, []byte(`"type":"ai-title"`)):
		sc.head.parse(0, line)
	case bytes.Contains(line, []byte(`"type":"user"`)):
		if sc.head.first == "" || sc.head.cwd == "" {
			sc.head.parse(0, line)
			sc.head.events = nil
		}
		if len(sc.pending) > 0 && bytes.Contains(line, []byte(`"tool_result"`)) {
			sc.results(line)
		}
	case bytes.Contains(line, []byte(`"type":"assistant"`)):
		for _, mark := range commitCalls {
			if !bytes.Contains(line, mark) {
				return
			}
		}
		sc.calls(line)
	}
}

// calls notes the Bash calls on an assistant line that run git commit.
func (sc *commitScan) calls(line []byte) {
	var l transcriptLine
	if json.Unmarshal(line, &l) != nil || l.IsSidechain {
		return
	}
	var blocks []contentBlock
	if json.Unmarshal(l.Message.Content, &blocks) != nil {
		return
	}
	for _, b := range blocks {
		if b.Type != "tool_use" || b.Name != "Bash" {
			continue
		}
		var in struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(b.Input, &in) == nil && strings.Contains(in.Command, "git") && strings.Contains(in.Command, "commit") {
			sc.pending[b.ID] = truncate(in.Command, 4000)
		}
	}
}

// results finds the commits printed in the results of the noted calls.
func (sc *commitScan) results(line []byte) {
	var l transcriptLine
	if json.Unmarshal(line, &l) != nil {
		return
	}
	var blocks []contentBlock
	if json.Unmarshal(l.Message.Content, &blocks) != nil {
		return
	}
	at := int64(0)
	if ts, err := time.Parse(time.RFC3339Nano, l.Timestamp); err == nil {
		at = ts.UnixMilli()
	}
	for _, b := range blocks {
		command, ok := sc.pending[b.ToolUseID]
		if b.Type != "tool_result" || !ok {
			continue
		}
		delete(sc.pending, b.ToolUseID)
		if b.IsError {
			continue
		}
		printed := commitPrinted.FindAllStringSubmatch(resultText(b.Content), -1)
		for _, m := range printed {
			sc.commits = append(sc.commits, foundCommit{short: m[1], at: at})
		}
		if len(printed) == 0 && at > 0 && strings.Contains(command, "commit") {
			sc.commits = append(sc.commits, foundCommit{at: at, command: command})
		}
	}
}

// resolveQuiet names the commit a quiet git commit made: the commit in the
// repository of cwd made from a minute before the call's result to a few
// seconds after, whose subject is in the command. "" when there is none or
// more than one.
func resolveQuiet(ctx context.Context, cwd string, c foundCommit) string {
	at := c.at / 1000
	out, err := git(ctx, cwd, "log", "--all", "--no-merges", "--format=%h%x00%s",
		"--since=@"+strconv.FormatInt(at-60, 10), "--until=@"+strconv.FormatInt(at+5, 10))
	if err != nil {
		return ""
	}
	found := ""
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		short, subject, ok := strings.Cut(line, "\x00")
		if !ok || len(strings.TrimSpace(subject)) < 4 || !strings.Contains(c.command, subject) {
			continue
		}
		if found != "" {
			return "" // two commits fit: say nothing rather than guess
		}
		found = short
	}
	return found
}

// folderTranscripts are the transcripts kept for dir and the folders inside
// it: Claude Code names each folder's directory after its path, so these
// are the directories named after dir, or after dir and more.
func (s *Service) folderTranscripts(dir string) []string {
	base := claudeProjectsDir(s.home)
	name := projectDirName(dir)
	dirs, _ := os.ReadDir(base)
	var out []string
	for _, d := range dirs {
		if !d.IsDir() || (d.Name() != name && !strings.HasPrefix(d.Name(), name+"-")) {
			continue
		}
		files, _ := os.ReadDir(filepath.Join(base, d.Name()))
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".jsonl") {
				out = append(out, filepath.Join(base, d.Name(), f.Name()))
			}
		}
	}
	return out
}

// CommitSessions lists the commits made by Claude conversations in dir or a
// folder inside it, newest first, each with its conversation.
func (s *Service) CommitSessions(ctx context.Context, dir string) (wire.CommitSessions, error) {
	out := wire.CommitSessions{Commits: []wire.CommitSession{}}
	real, err := s.allowedDir(dir)
	if err != nil {
		return out, fail(wire.CodeInvalidRequest, "%s", ErrOutsideRoots.Error())
	}
	var ids []string
	for _, file := range s.folderTranscripts(real) {
		sc := s.commits.scan(file)
		if sc == nil {
			continue
		}
		s.commits.mu.Lock()
		some := len(sc.commits) > 0
		cwd, title, first := sc.head.cwd, sc.head.title, sc.head.first
		s.commits.mu.Unlock()
		if !some || cwd == "" || !within(filepath.Clean(cwd), real) {
			continue // a folder whose name only looks like it
		}
		found := s.commits.named(ctx, sc, cwd)
		if len(found) == 0 {
			continue
		}
		if title == filepath.Base(cwd) {
			title = "" // Gwen names a session after its folder: no name of its own
		}
		id := strings.TrimSuffix(filepath.Base(file), ".jsonl")
		ids = append(ids, id)
		conv := wire.CommitSession{ClaudeSessionID: id, Title: title, FirstPrompt: truncate(oneLine(first, 400), 200), Cwd: cwd}
		if last, err := s.repos.CodeSessions.LastOfClaude(ctx, id); err == nil {
			conv.Sandbox = last.Sandbox
		}
		s.mu.Lock()
		if live := s.running(id); live != nil {
			conv.SessionID = live.ID
		}
		s.mu.Unlock()
		for _, c := range found {
			conv.Commit, conv.At = c.short, c.at
			out.Commits = append(out.Commits, conv)
		}
	}
	if summaries, err := s.repos.CodeSessions.CommitSummaries(ctx, ids); err == nil {
		for i, c := range out.Commits {
			out.Commits[i].Summary = summaries[store.CommitKey(c.ClaudeSessionID, c.Commit)]
		}
	}
	sort.SliceStable(out.Commits, func(i, j int) bool { return out.Commits[i].At > out.Commits[j].At })
	return out, nil
}

// named is the commits of a scan that have a short id, asking git for the
// ones a quiet commit made the first time they are seen.
func (c *commitScans) named(ctx context.Context, sc *commitScan, cwd string) []foundCommit {
	c.mu.Lock()
	var quiet []int
	for i, f := range sc.commits {
		if f.short == "" && !f.tried {
			quiet = append(quiet, i)
		}
	}
	ask := make([]foundCommit, len(quiet))
	for n, i := range quiet {
		ask[n] = sc.commits[i]
	}
	c.mu.Unlock()
	names := make([]string, len(ask))
	for n, f := range ask {
		names[n] = resolveQuiet(ctx, cwd, f)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for n, i := range quiet {
		if i < len(sc.commits) && sc.commits[i].at == ask[n].at && sc.commits[i].short == "" {
			sc.commits[i].short, sc.commits[i].tried = names[n], true
		}
	}
	var out []foundCommit
	for _, f := range sc.commits {
		if f.short != "" {
			out = append(out, f)
		}
	}
	return out
}

// summaryText is how much of a conversation goes to the model for a
// commit's summary: its first prompt and the end of what led to the commit.
const summaryText = 24 << 10

// CommitConversation is what led to a commit in a conversation of dir: the
// first prompt and the turns up to the commit, as text for the model.
func (s *Service) CommitConversation(dir, claudeID, commit string) (text string, err error) {
	real, err := s.allowedDir(dir)
	if err != nil {
		return "", fail(wire.CodeInvalidRequest, "%s", ErrOutsideRoots.Error())
	}
	var path string
	for _, file := range s.folderTranscripts(real) {
		if strings.TrimSuffix(filepath.Base(file), ".jsonl") == claudeID {
			path = file
			break
		}
	}
	sc := s.commits.scan(path)
	if path == "" || sc == nil {
		return "", fail(wire.CodeNotFound, "that conversation isn't in this folder")
	}
	s.commits.mu.Lock()
	at := int64(-1)
	for _, c := range sc.commits {
		if c.short == commit {
			at = c.at
		}
	}
	s.commits.mu.Unlock()
	if at < 0 {
		return "", fail(wire.CodeNotFound, "that conversation didn't make this commit")
	}
	t := s.files.get(path)
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.refresh(); err != nil {
		return "", fail(wire.CodeUnavailable, "couldn't read the conversation: %v", err)
	}
	var lines []string
	for _, ev := range t.events {
		if at > 0 && ev.At > at {
			break
		}
		switch ev.Kind {
		case wire.TranscriptYou:
			lines = append(lines, "You: "+ev.Text)
		case wire.TranscriptClaude:
			lines = append(lines, "Claude: "+ev.Text)
		case wire.TranscriptTool:
			lines = append(lines, "Tool: "+ev.Tool+" "+ev.Text)
		}
	}
	body := strings.Join(lines, "\n")
	if len(body) > summaryText {
		body = "…\n" + body[len(body)-summaryText:]
	}
	if t.first != "" && !strings.HasPrefix(body, "You: "+t.first) {
		body = "First prompt: " + truncate(t.first, 2000) + "\n\n" + body
	}
	return body, nil
}

// KeepCommitSummary stores Gwen's summary of a commit.
func (s *Service) KeepCommitSummary(ctx context.Context, claudeID, commit, summary string) error {
	return s.repos.CodeSessions.KeepCommitSummary(ctx, claudeID, commit, summary, s.clk.Now())
}
