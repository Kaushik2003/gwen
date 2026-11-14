package remote

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// A transcript belongs to Claude Code, so Gwen reads it against recorded
// fixtures (plan.md 3.14.9). The files in testdata/claude-2.1.322 have the
// line types and keys Claude Code 2.1.322 writes, with made-up content.
const w27cClaudeFixtures = "testdata/claude-2.1.322"

// w27cEvent is a transcript event without its cursor and time, which are
// checked against the file itself.
type w27cEvent struct {
	Kind, Text, Tool string
	OK               bool
	Added, Removed   int
}

func w27cEvents(evs []wire.CodeTranscriptEvent) []w27cEvent {
	out := make([]w27cEvent, 0, len(evs))
	for _, e := range evs {
		out = append(out, w27cEvent{Kind: e.Kind, Text: e.Text, Tool: e.Tool, OK: e.OK, Added: e.Added, Removed: e.Removed})
	}
	return out
}

// w27cFixture reads a fixture, with __DIR__ standing for dir in the cwd keys.
func w27cFixture(t *testing.T, name, dir string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(w27cClaudeFixtures, name))
	require.NoError(t, err)
	if dir != "" {
		quoted, err := json.Marshal(dir)
		require.NoError(t, err)
		data = bytes.ReplaceAll(data, []byte(`"__DIR__"`), quoted)
	}
	return data
}

// w27cTranscript copies session.jsonl into a temporary folder, so tests that
// append to it never touch testdata.
func w27cTranscript(t *testing.T) (*transcript, []byte) {
	t.Helper()
	data := w27cFixture(t, "session.jsonl", "")
	path := filepath.Join(t.TempDir(), "5f0c2a1e-7b3d-4c9a-8e21-0a1b2c3d4e5f.jsonl")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return &transcript{path: path, tools: map[string]toolCall{}}, data
}

const w27cCLI = "/home/user/code/cli"

// w27cSessionEvents are the events of session.jsonl. Everything else in it
// is bookkeeping, thinking, wrappers, a subagent's line, line types this
// version of Gwen has never seen, and one line cut short by a crash.
var w27cSessionEvents = []w27cEvent{
	{Kind: wire.TranscriptYou, Text: "Add a --verbose flag to the CLI and make the tests pass.", OK: true},
	{Kind: wire.TranscriptClaude, Text: "I'll read how the flags are parsed first.", OK: true},
	{Kind: wire.TranscriptTool, Tool: "Read", Text: w27cCLI + "/main.go", OK: true},
	{Kind: wire.TranscriptTool, Tool: "Edit", Text: w27cCLI + "/main.go", OK: true, Added: 3, Removed: 1},
	{Kind: wire.TranscriptTool, Tool: "Write", Text: w27cCLI + "/verbose_test.go", OK: true, Added: 5},
	{Kind: wire.TranscriptTool, Tool: "Bash", Text: "go test ./...", OK: false},
	{Kind: wire.TranscriptYou, Text: "Here is the error I see:\n[image]", OK: true},
	{Kind: wire.TranscriptClaude, Text: "The test expects `-v` too; I'll add the short form.", OK: true},
	{Kind: wire.TranscriptTool, Tool: "MultiEdit", Text: w27cCLI + "/main.go", OK: true, Added: 3, Removed: 3},
	{Kind: wire.TranscriptTool, Tool: "Grep", Text: "verbose", OK: false},
	{Kind: wire.TranscriptNotice, Text: "Interrupted", OK: true},
	{Kind: wire.TranscriptYou, Text: "/compact", OK: true},
	{Kind: wire.TranscriptNotice, Text: "Conversation compacted", OK: true},
	{Kind: wire.TranscriptYou, Text: "Now add it to the README.", OK: true},
	{Kind: wire.TranscriptNotice, Text: "Claude hit an error and is retrying", OK: false},
	{Kind: wire.TranscriptClaude, Text: "Added a Flags section to the README.", OK: true},
}

func TestTranscriptParsesARecorded2_1_322Session(t *testing.T) {
	t.Parallel()
	tr, data := w27cTranscript(t)
	require.NoError(t, tr.refresh())

	require.Equal(t, w27cSessionEvents, w27cEvents(tr.events))
	require.Equal(t, int64(len(data)), tr.offset, "every whole line is read, the broken one too")
	require.Equal(t, "Verbose flag", tr.title, "a custom title beats the AI's title written after it")
	require.Equal(t, "Add a --verbose flag to the CLI and make the tests pass.", tr.first,
		"the caveat and the command output before it are not the first message")
	require.Equal(t, w27cCLI, tr.cwd)
	require.Empty(t, tr.tools, "every call was answered; a result for an unknown call is dropped")

	// A cursor is the byte offset of the event's line, and its time is that
	// line's timestamp, so the phone can page by cursor.
	for _, ev := range tr.events {
		require.True(t, ev.Cursor == 0 || data[ev.Cursor-1] == '\n', "cursor %d starts a line", ev.Cursor)
		line, _, _ := bytes.Cut(data[ev.Cursor:], []byte("\n"))
		var l struct {
			Timestamp string `json:"timestamp"`
		}
		require.NoError(t, json.Unmarshal(line, &l))
		at, err := time.Parse(time.RFC3339Nano, l.Timestamp)
		require.NoError(t, err)
		require.Equal(t, at.UnixMilli(), ev.At)
	}

	// A failed Bash call keeps the last 40 lines of what it printed; other
	// tools keep none.
	bash := tr.events[5]
	lines := strings.Split(bash.Output, "\n")
	require.Len(t, lines, failedOutputLines)
	require.Equal(t, "=== RUN   TestCase06", lines[0])
	require.Equal(t, "FAIL", lines[len(lines)-1])
	require.Empty(t, tr.events[9].Output, "a refused Grep shows no output")
	for _, ev := range tr.events {
		if ev.Kind == wire.TranscriptTool && !ev.OK {
			require.Zero(t, ev.Added+ev.Removed, "a failed edit counts no lines")
		}
	}
}

// Unknown line types are skipped, never fatal: a file of nothing else
// parses to no events and reads to its end.
func TestTranscriptSkipsUnknownLineTypes(t *testing.T) {
	t.Parallel()
	lines := []string{
		`{"type":"workflow-checkpoint","state":{"type":"user","step":1},"sessionId":"s"}`,
		`{"type":"api-request-blob","blob":"AAAA","sessionId":"s"}`,
		`{"type":"atis-latch","atis":{"type":"assistant"},"sessionId":"s"}`,
		`{"type":"user","message":{"role":"user","content":42},"cwd":"/x"}`,
		`{"type":"assistant","message":{"content":"not a list"}}`,
		`{"type":"user","message":`,
		`not json at all "type":"system"`,
		``,
		`["type","user"]`,
	}
	data := strings.Join(lines, "\n") + "\n"
	path := filepath.Join(t.TempDir(), "t.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
	tr := &transcript{path: path, tools: map[string]toolCall{}}
	require.NoError(t, tr.refresh())
	require.Empty(t, tr.events)
	require.Equal(t, int64(len(data)), tr.offset)
}

// Claude Code appends one line at a time; a last line without its newline
// is still being written and is read once it is whole.
func TestTranscriptWaitsForALineStillBeingWritten(t *testing.T) {
	t.Parallel()
	tr, data := w27cTranscript(t)
	whole := bytes.Index(data, []byte(`"content":"Now add it to the README."`))
	require.Positive(t, whole)
	start := int64(bytes.LastIndexByte(data[:whole], '\n') + 1)
	cut := start + 40
	require.NoError(t, os.WriteFile(tr.path, data[:cut], 0o600))

	require.NoError(t, tr.refresh())
	require.Equal(t, start, tr.offset, "the half line is not consumed")
	n := len(tr.events)
	require.Equal(t, "Conversation compacted", tr.events[n-1].Text)

	f, err := os.OpenFile(tr.path, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = f.Write(data[cut:])
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.NoError(t, tr.refresh())
	require.Equal(t, w27cSessionEvents, w27cEvents(tr.events))
	require.Equal(t, start, tr.events[n].Cursor, "the finished line keeps the cursor of where it began")
}

// A file that got shorter was rewritten: it is read again from the start.
func TestTranscriptStartsAgainWhenRewritten(t *testing.T) {
	t.Parallel()
	tr, _ := w27cTranscript(t)
	require.NoError(t, tr.refresh())
	require.NotEmpty(t, tr.title)

	short := `{"type":"user","message":{"role":"user","content":"Start over"},"timestamp":"2026-11-12T10:00:00.000Z","cwd":"/home/user/code/cli"}` + "\n"
	require.NoError(t, os.WriteFile(tr.path, []byte(short), 0o600))
	require.NoError(t, tr.refresh())
	require.Equal(t, []w27cEvent{{Kind: wire.TranscriptYou, Text: "Start over", OK: true}}, w27cEvents(tr.events))
	require.Equal(t, int64(0), tr.events[0].Cursor)
	require.Empty(t, tr.title, "the old title went with the old file")
	require.Equal(t, "Start over", tr.first)
}

// The phone pages back from the newest events and polls for newer ones by
// cursor (plan.md 3.14.9).
func TestTranscriptPages(t *testing.T) {
	t.Parallel()
	tr, data := w27cTranscript(t)

	var got []wire.CodeTranscriptEvent
	page := tr.page(-1, -1, 5)
	require.Equal(t, int64(len(data)), page.After)
	for {
		require.LessOrEqual(t, len(page.Events), 5)
		got = append(page.Events, got...)
		if page.Before == nil {
			break
		}
		require.Equal(t, page.Events[0].Cursor, *page.Before)
		page = tr.page(*page.Before, -1, 5)
	}
	require.Equal(t, w27cSessionEvents, w27cEvents(got), "paging back from the newest returns every event once")

	newer := tr.page(-1, int64(len(data)), 50)
	require.Empty(t, newer.Events, "nothing is newer than the end")

	more := `{"type":"user","message":{"role":"user","content":"One more thing"},"timestamp":"2026-11-12T10:00:00.000Z"}` + "\n"
	f, err := os.OpenFile(tr.path, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = f.WriteString(more)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	newer = tr.page(-1, int64(len(data)), 50)
	require.Equal(t, []w27cEvent{{Kind: wire.TranscriptYou, Text: "One more thing", OK: true}}, w27cEvents(newer.Events))
	require.Equal(t, int64(len(data)), newer.Events[0].Cursor)
	require.Equal(t, int64(len(data)+len(more)), newer.After)
}

// Older Claude Code versions put several tool results in one line. Those
// events share a cursor, and a page never splits them, or paging back from
// that cursor would lose one.
func TestTranscriptPageKeepsALinesEventsTogether(t *testing.T) {
	t.Parallel()
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"Read both"},"timestamp":"2026-11-12T09:00:00.000Z"}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"a","name":"Read","input":{"file_path":"/a"}},{"type":"tool_use","id":"b","name":"Read","input":{"file_path":"/b"}}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":"x"},{"type":"tool_result","tool_use_id":"b","content":"y"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Both read."}]}}`,
	}
	path := filepath.Join(t.TempDir(), "t.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	tr := &transcript{path: path, tools: map[string]toolCall{}}

	page := tr.page(-1, -1, 2)
	require.Equal(t, []w27cEvent{
		{Kind: wire.TranscriptTool, Tool: "Read", Text: "/a", OK: true},
		{Kind: wire.TranscriptTool, Tool: "Read", Text: "/b", OK: true},
		{Kind: wire.TranscriptClaude, Text: "Both read.", OK: true},
	}, w27cEvents(page.Events), "the limit gives way rather than split a line")
	require.NotNil(t, page.Before)
	rest := tr.page(*page.Before, -1, 2)
	require.Equal(t, []w27cEvent{{Kind: wire.TranscriptYou, Text: "Read both", OK: true}}, w27cEvents(rest.Events))
	require.Nil(t, rest.Before)
}

// One huge message is cut, so a page stays small enough for a phone.
func TestTranscriptCutsAHugeMessage(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("word ", 10_000)
	line, err := json.Marshal(map[string]any{"type": "assistant", "message": map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}}}})
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "t.jsonl")
	require.NoError(t, os.WriteFile(path, append(line, '\n'), 0o600))
	tr := &transcript{path: path, tools: map[string]toolCall{}}
	require.NoError(t, tr.refresh())
	require.Len(t, tr.events, 1)
	require.Equal(t, maxEventText+len("\n…"), len(tr.events[0].Text))
	require.True(t, strings.HasSuffix(tr.events[0].Text, "\n…"))
}

// The cut falls on a character, not inside one, or the phone would show a
// replacement mark where the text stops.
func TestTranscriptCutsOnACharacter(t *testing.T) {
	t.Parallel()
	line, err := json.Marshal(map[string]any{"type": "assistant", "message": map[string]any{
		"content": []map[string]any{{"type": "text", "text": "a" + strings.Repeat("€", 10_000)}}}})
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "t.jsonl")
	require.NoError(t, os.WriteFile(path, append(line, '\n'), 0o600))
	tr := &transcript{path: path, tools: map[string]toolCall{}}
	require.NoError(t, tr.refresh())
	require.Len(t, tr.events, 1)
	require.True(t, utf8.ValidString(tr.events[0].Text), "the cut never splits a character")
}

// Each tool shows in one line: the field that says what it did.
func TestTranscriptToolSummaries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, input, summary string
		added, removed       int
	}{
		{"Bash", `{"command":"go  vet\n./...","description":"Vet"}`, "go vet ./...", 0, 0},
		{"Read", `{"file_path":"/a/b.go","limit":20}`, "/a/b.go", 0, 0},
		{"NotebookEdit", `{"notebook_path":"/n.ipynb","new_source":"x = 1\ny = 2"}`, "/n.ipynb", 2, 0},
		{"Glob", `{"pattern":"**/*.go"}`, "**/*.go", 0, 0},
		{"WebSearch", `{"query":"bubblewrap overlay"}`, "bubblewrap overlay", 0, 0},
		{"WebFetch", `{"url":"https://example.com/a","prompt":"summarize"}`, "https://example.com/a", 0, 0},
		{"Agent", `{"description":"Find the parser","prompt":"Look for..."}`, "Find the parser", 0, 0},
		{"mcp__notes__search", `{"query":"q"}`, "", 0, 0},
		{"TodoWrite", `{"todos":[]}`, "", 0, 0},
		{"Edit", `not json`, "", 0, 0},
	}
	for _, tt := range tests {
		summary, added, removed := summarizeTool(tt.name, json.RawMessage(tt.input))
		require.Equal(t, tt.summary, summary, tt.name)
		require.Equal(t, [2]int{tt.added, tt.removed}, [2]int{added, removed}, tt.name)
	}
	long, _, _ := summarizeTool("Bash", json.RawMessage(fmt.Sprintf(`{"command":%q}`, strings.Repeat("é", 300))))
	require.Equal(t, strings.Repeat("é", 200)+"…", long, "cut by characters, not bytes")
}

// w27cService is a Service with a database and a temporary home, and no
// core: enough for the conversations and folder lists.
func w27cService(t *testing.T) (*Service, string) {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	clk := clock.NewFake(testutil.At("09:00:00"))
	db := testutil.NewDBWithClock(t, clk)
	svc, err := New(Options{DB: db, Repos: store.NewRepos(db), Clock: clk, Config: config.Defaults, Home: home})
	require.NoError(t, err)
	return svc, home
}

// w27cPlace writes a fixture as Claude Code would keep the transcript of
// conversation id run in dir, last written at mtime.
func w27cPlace(t *testing.T, home, fixture, dir, id string, mtime time.Time) string {
	t.Helper()
	folder := filepath.Join(claudeProjectsDir(home), projectDirName(dir))
	require.NoError(t, os.MkdirAll(folder, 0o700))
	path := filepath.Join(folder, id+".jsonl")
	require.NoError(t, os.WriteFile(path, w27cFixture(t, fixture, dir), 0o600))
	require.NoError(t, os.Chtimes(path, mtime, mtime))
	return path
}

// Continue on the phone lists a folder's conversations (plan.md 3.14.7):
// named by the newest custom title, else the first message; with whether
// it last ran sandboxed and the session it runs in now.
func TestConversationsListsAFoldersTranscripts(t *testing.T) {
	t.Parallel()
	svc, home := w27cService(t)
	dir := filepath.Join(home, "code", "my-app")
	// my.app keeps its transcripts in the same Claude folder as my-app.
	twin := filepath.Join(home, "code", "my.app")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.MkdirAll(twin, 0o755))

	const (
		titled   = "c0000001-0000-4000-8000-000000000001"
		first    = "c0000002-0000-4000-8000-000000000002"
		aiTitled = "c0000003-0000-4000-8000-000000000003"
		noFolder = "c0000004-0000-4000-8000-000000000004"
		untyped  = "c0000005-0000-4000-8000-000000000005"
		ofTwin   = "c0000006-0000-4000-8000-000000000006"
	)
	t0 := testutil.At("08:00:00")
	w27cPlace(t, home, "conversations/titled.jsonl", dir, titled, t0.Add(5*time.Minute))
	w27cPlace(t, home, "conversations/ai-title.jsonl", dir, aiTitled, t0.Add(4*time.Minute))
	w27cPlace(t, home, "conversations/first-message.jsonl", dir, first, t0.Add(3*time.Minute))
	w27cPlace(t, home, "conversations/bookkeeping-only.jsonl", dir, noFolder, t0.Add(2*time.Minute))
	w27cPlace(t, home, "conversations/nothing-typed.jsonl", dir, untyped, t0.Add(time.Minute))
	w27cPlace(t, home, "conversations/titled.jsonl", twin, ofTwin, t0.Add(6*time.Minute))
	// Claude Code keeps subagents in a folder per conversation; they are no
	// conversations of their own.
	sub := filepath.Join(claudeProjectsDir(home), projectDirName(dir), titled, "subagents")
	require.NoError(t, os.MkdirAll(sub, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "agent-a1.jsonl"), w27cFixture(t, "conversations/titled.jsonl", dir), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(sub), "..", "notes.txt"), []byte("x"), 0o600))

	ctx := t.Context()
	started := testutil.At("07:00:00")
	ended := started.Add(time.Hour)
	save := func(id, claude string, sandbox bool, at time.Time) {
		require.NoError(t, svc.repos.CodeSessions.Save(ctx, store.CodeSession{ID: id, ClaudeSessionID: claude, Cwd: dir,
			Sandbox: sandbox, State: store.CodeEnded, StartedAt: at, UpdatedAt: at, EndedAt: &ended}))
	}
	save("cs_rowone0001", titled, true, started)
	// first last ran in Normal, after an earlier run in a sandbox.
	save("cs_rowtwo0002", first, true, started)
	save("cs_rowtri0003", first, false, started.Add(time.Minute))

	svc.mu.Lock()
	svc.sessions["live"] = &session{CodeSession: store.CodeSession{ID: "live", ClaudeSessionID: titled, Cwd: dir, State: store.CodeWorking}}
	svc.sessions["gone"] = &session{CodeSession: store.CodeSession{ID: "gone", ClaudeSessionID: aiTitled, Cwd: dir, State: store.CodeEnded}}
	svc.mu.Unlock()

	got, err := svc.Conversations(ctx, dir)
	require.NoError(t, err)
	require.Equal(t, []wire.CodeConversation{
		{ClaudeSessionID: titled, Title: "Login loop, round two", ActiveAt: t0.Add(5 * time.Minute).UnixMilli(), Sandbox: true, SessionID: "live"},
		{ClaudeSessionID: aiTitled, Title: "Arm64 build failure", ActiveAt: t0.Add(4 * time.Minute).UnixMilli()},
		{ClaudeSessionID: first, ActiveAt: t0.Add(3 * time.Minute).UnixMilli(),
			Title: "Refactor the payment retry loop so that every attempt waits twice as long as the one before, gives up after five tries, …"},
		{ClaudeSessionID: untyped, Title: "Untitled conversation", ActiveAt: t0.Add(time.Minute).UnixMilli()},
	}, got.Conversations)

	// The twin folder sees only its own.
	got, err = svc.Conversations(ctx, twin)
	require.NoError(t, err)
	require.Len(t, got.Conversations, 1)
	require.Equal(t, ofTwin, got.Conversations[0].ClaudeSessionID)
}

func TestConversationsListsTheNewest15(t *testing.T) {
	t.Parallel()
	svc, home := w27cService(t)
	dir := filepath.Join(home, "code", "busy")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	t0 := testutil.At("08:00:00")
	for i := range 17 {
		w27cPlace(t, home, "conversations/titled.jsonl", dir, fmt.Sprintf("c0000000-0000-4000-8000-%012d", i), t0.Add(time.Duration(i)*time.Minute))
	}
	got, err := svc.Conversations(t.Context(), dir)
	require.NoError(t, err)
	require.Len(t, got.Conversations, 15)
	require.Equal(t, "c0000000-0000-4000-8000-000000000016", got.Conversations[0].ClaudeSessionID)
	require.Equal(t, "c0000000-0000-4000-8000-000000000002", got.Conversations[14].ClaudeSessionID)
}

func TestConversationsRefusesAFolderOutsideTheRoots(t *testing.T) {
	t.Parallel()
	svc, home := w27cService(t)
	hidden := filepath.Join(home, ".secret")
	require.NoError(t, os.MkdirAll(hidden, 0o755))
	for _, dir := range []string{t.TempDir(), hidden, "relative/path"} {
		got, err := svc.Conversations(t.Context(), dir)
		var e *Error
		require.ErrorAs(t, err, &e, dir)
		require.Equal(t, wire.CodeInvalidRequest, e.Code)
		require.Empty(t, got.Conversations)
	}

	// A folder Claude Code never ran in has none, and that is no error.
	empty := filepath.Join(home, "code", "fresh")
	require.NoError(t, os.MkdirAll(empty, 0o755))
	got, err := svc.Conversations(t.Context(), empty)
	require.NoError(t, err)
	require.Empty(t, got.Conversations)
}

// Recent folders come from the same transcripts: the folder of the newest
// one in each Claude project, newest first, once each, only inside a root.
func TestRecentFoldersFromTranscripts(t *testing.T) {
	t.Parallel()
	svc, home := w27cService(t)
	a := filepath.Join(home, "code", "a")
	b := filepath.Join(home, "code", "b")
	gone := filepath.Join(home, "code", "gone")
	require.NoError(t, os.MkdirAll(a, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(b, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(b, ".git", "HEAD"), []byte("ref: refs/heads/feature\n"), 0o644))
	t0 := testutil.At("08:00:00")
	w27cPlace(t, home, "conversations/titled.jsonl", a, "c0000000-0000-4000-8000-00000000000a", t0)
	w27cPlace(t, home, "conversations/first-message.jsonl", b, "c0000000-0000-4000-8000-00000000000b", t0.Add(time.Minute))
	// gone was never made, or was deleted since.
	w27cPlace(t, home, "conversations/titled.jsonl", gone, "c0000000-0000-4000-8000-00000000000c", t0.Add(2*time.Minute))
	outside := t.TempDir()
	w27cPlace(t, home, "conversations/titled.jsonl", outside, "c0000000-0000-4000-8000-00000000000d", t0.Add(3*time.Minute))
	// The newest transcript of a project names its folder, even when an
	// older one has none.
	w27cPlace(t, home, "conversations/bookkeeping-only.jsonl", a, "c0000000-0000-4000-8000-00000000000e", t0.Add(-time.Minute))

	got := svc.RecentFolders()
	require.Equal(t, []wire.CodeFolder{
		{Name: "b", Path: b, Git: true, Branch: "feature"},
		{Name: "a", Path: a},
	}, got.Folders)
}
