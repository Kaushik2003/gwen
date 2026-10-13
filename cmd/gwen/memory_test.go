package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/client/clienttest"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// Memory fixtures: two claims learned in the same second share their first
// characters, as time-ordered ids do.
var (
	claimAID = "01926d2e-7a4b-7c3d-8e9f-0000000a1b2c"
	claimBID = "01926d2e-7a4b-7c3d-8e9f-0000000d3e4f"
	annaID   = "01926d2e-7a4b-7c3d-8e9f-00000000a4a4"
	confID   = "01926d2e-7a4b-7c3d-8e9f-0000000c0f1c"
	docID    = "01926d2e-7a4b-7c3d-8e9f-0000000d0c0d"
	claimA   = wire.MemoryClaim{ID: claimAID, Key: "user.employer", Label: "Works at", Text: "Works at Acme",
		Value: json.RawMessage(`"Acme"`), Status: wire.ClaimActive, Trust: wire.TrustOwnerSaid,
		Sensitivity: wire.SensitivityPersonal, Source: "chat “Work”, 15 Sep", Quote: "I work at Acme"}
	claimB = wire.MemoryClaim{ID: claimBID, Key: "user.employer", Label: "Works at", Text: "Works at Globex",
		Value: json.RawMessage(`"Globex"`), Status: wire.ClaimCandidate, Trust: wire.TrustInferred,
		Sensitivity: wire.SensitivityPersonal, Source: "your edit", Disputed: true, Hidden: true}
	receipt = wire.ForgetReceipt{Claims: 1, Lanes: []wire.LaneReceipt{{Lane: "claims", OK: true},
		{Lane: "mem0", OK: false, Note: "it will be retried"}}}
)

// memoryFake answers the lookups memory commands make.
func memoryFake() *clienttest.Fake {
	return fake().
		Returns("ListClaims", &wire.MemoryClaimList{Claims: []wire.MemoryClaim{claimA, claimB}}, nil).
		Returns("ListEntities", &wire.MemoryEntityList{Entities: []wire.MemoryEntity{
			{ID: annaID, Kind: "person", Name: "Anna", Aliases: []string{"Annie"}}}}, nil).
		Returns("ListConflicts", &wire.MemoryConflictList{Conflicts: []wire.MemoryConflict{
			{ID: confID, Reason: "two employers", Status: "open", A: claimA, B: claimB}}}, nil).
		Returns("ListResources", &wire.MemoryResourceList{Resources: []wire.MemoryResource{
			{ID: docID, Title: "Lease", Path: "/home/u/lease.pdf", AddedAt: wire.Millis(now)}}}, nil)
}

// runMemoryCLI runs the CLI with stdin.
func runMemoryCLI(t *testing.T, f *clienttest.Fake, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	c := &cli{newAPI: func(string) client.API { return f }, stdin: strings.NewReader(stdin), stdout: &out,
		stderr: &errb, clk: clock.NewFake(now), loc: time.UTC, open: func(string) error { return nil }}
	code = c.execute(context.Background(), args)
	return out.String(), errb.String(), code
}

func TestMemoryStatusCLI(t *testing.T) {
	t.Parallel()
	slept := wire.Millis(testutil.At("09:00"))
	f := memoryFake().Returns("MemoryStatus", &wire.MemoryStatus{Enabled: true, Learning: true,
		Claims: map[string]int{"active": 12, "candidate": 2}, Conflicts: 1, LastSleptAt: &slept,
		Lanes: []wire.MemoryLane{{Lane: "claims", Enabled: true, OK: true, Items: 14},
			{Lane: "vec", Enabled: true, OK: false, Items: -1, Detail: "connection refused"}},
		LedgerRows: 40, LedgerOK: true}, nil)
	out, _, code := runCLI(t, f, "memory")
	require.Equal(t, 0, code)
	require.Contains(t, out, "Memory is on and learning from your chats.")
	require.Contains(t, out, "Remembers: 12 in use, 2 to confirm")
	require.Contains(t, out, "Conflicts: 1 (gwen memory conflicts)")
	require.Contains(t, out, "Last learned: never; last tidied: 15 Sep 09:00")
	require.Regexp(t, `vec\s+down\s+connection refused`, out)
	require.Contains(t, out, "History: 40 changes, verified")

	out, _, code = runCLI(t, f, "memory", "--json")
	require.Equal(t, 0, code)
	var st wire.MemoryStatus
	require.NoError(t, json.Unmarshal([]byte(out), &st))
	require.Equal(t, 12, st.Claims["active"])

	off := memoryFake().Returns("MemoryStatus", nil, &client.APIError{Status: 503, Code: wire.CodeUnavailable,
		Message: "memory is off; turn it on in Settings, Memory"})
	_, errOut, code := runCLI(t, off, "memory")
	require.Equal(t, exitAPI, code)
	require.Contains(t, errOut, "memory is off; turn it on in Settings, Memory")
}

func TestMemoryListCLI(t *testing.T) {
	t.Parallel()
	f := memoryFake()
	out, _, code := runCLI(t, f, "memory", "list")
	require.Equal(t, 0, code)
	require.Regexp(t, `0a1b2c\s+active\s+Works at Acme\s+chat “Work”, 15 Sep`, out)
	require.Regexp(t, `0d3e4f\s+to confirm, hidden, disputed\s+Works at Globex`, out)
	require.Equal(t, wire.MemoryClaimQuery{}, lastCall(t, f, "ListClaims")[0])

	_, _, code = runCLI(t, f, "memory", "list", "--all", "--key", "user.", "--about", "user", "--hidden")
	require.Equal(t, 0, code)
	require.Equal(t, wire.MemoryClaimQuery{Status: "all", Key: "user.", Subject: "user", Hidden: true},
		lastCall(t, f, "ListClaims")[0])
	_, _, code = runCLI(t, f, "memory", "list", "--status", "candidate", "--about", "annie")
	require.Equal(t, 0, code)
	require.Equal(t, wire.MemoryClaimQuery{Status: "candidate", Subject: annaID}, lastCall(t, f, "ListClaims")[0])

	_, errOut, code := runCLI(t, f, "memory", "list", "--about", "Bob")
	require.Equal(t, exitAPI, code)
	require.Contains(t, errOut, `memory knows no one called "Bob"`)
	_, _, code = runCLI(t, f, "memory", "list", "--all", "--status", "active")
	require.Equal(t, exitUsage, code)
}

func TestMemoryIDs(t *testing.T) {
	t.Parallel()
	ids := []string{claimAID, claimBID}
	for in, want := range map[string]string{claimAID: claimAID, "0A1B2C": claimAID, "000d3e4f": claimBID,
		strings.ToUpper(claimBID): claimBID} {
		got, err := resolveMemoryID(in, ids, "memory")
		require.NoError(t, err, in)
		require.Equal(t, want, got, in)
	}
	_, err := resolveMemoryID("01926d2e", ids, "memory")
	require.ErrorIs(t, err, client.ErrAmbiguousID, "ids made together share a prefix")
	_, err = resolveMemoryID("0a1b", ids, "memory")
	require.ErrorIs(t, err, client.ErrUnknownID)
	_, err = resolveMemoryID("ffffff", ids, "memory")
	require.EqualError(t, err, `no memory matches "ffffff"`)

	// A short id is resolved among every claim, hidden ones too; a full one is used as it is.
	f := memoryFake().Returns("ExplainMemory", &wire.MemoryExplanation{Ref: "claim:" + claimBID, Claim: &claimB}, nil)
	_, _, code := runCLI(t, f, "memory", "show", "0d3e4f")
	require.Equal(t, 0, code)
	require.Equal(t, wire.MemoryClaimQuery{Status: "all", Hidden: true, Limit: 500}, lastCall(t, f, "ListClaims")[0])
	require.Equal(t, "claim:"+claimBID, lastCall(t, f, "ExplainMemory")[0])
	f.Reset()
	_, _, code = runCLI(t, f, "memory", "show", claimAID)
	require.Equal(t, 0, code)
	require.Empty(t, f.CallsTo("ListClaims"))
	_, errOut, code := runCLI(t, f, "memory", "show", "01926d2e")
	require.Equal(t, exitUsage, code)
	require.Contains(t, errOut, "ambiguous id")
}

func TestMemoryShowCLI(t *testing.T) {
	t.Parallel()
	from := wire.Millis(testutil.AtOn("2026-08-01", "00:00"))
	a := claimA
	a.ValidFrom = &from
	old := claimB
	old.ID, old.Text, old.Status, old.Hidden, old.Disputed = claimBID, "Works at Initech", "superseded", false, false
	f := memoryFake().Returns("ExplainMemory", &wire.MemoryExplanation{Ref: "claim:" + claimAID, Claim: &a,
		Quote: "I work at Acme", Context: []wire.AssistantMessage{{Role: "user", Text: "I work at Acme now"},
			{Role: "assistant", Text: "Nice!"}}, History: []wire.MemoryClaim{a, old}}, nil)
	out, _, code := runCLI(t, f, "memory", "show", "0a1b2c")
	require.Equal(t, 0, code)
	for _, want := range []string{`^Works at Acme\n`, `key\s+user\.employer \(Works at\)`, `about\s+you`,
		`value\s+"Acme"`, `status\s+active, you said it, personal`, `true\s+since 2026-08-01`,
		`from\s+chat “Work”, 15 Sep`, `words\s+“I work at Acme”`, `You: I work at Acme now`, `Gwen: Nice!`,
		`Before:\n\s+000d3e4f\s+Works at Initech\s+superseded`} {
		require.Regexp(t, want, out)
	}
}

func TestMemorySearchCLI(t *testing.T) {
	t.Parallel()
	f := memoryFake().Returns("RecallMemory", &wire.RecallResult{Intent: "lookup", Abstain: true,
		Degraded: []string{"vec"}}, nil)
	out, _, code := runCLI(t, f, "memory", "search", "where", "do", "I", "work", "--as-of", "2026-09-01")
	require.Equal(t, 0, code)
	require.Equal(t, wire.RecallRequest{Query: "where do I work", AsOf: "2026-09-01"}, lastCall(t, f, "RecallMemory")[0])
	require.Contains(t, out, "Nothing in memory bears on that.")
	require.Contains(t, out, "she would say she doesn't remember")
	require.Contains(t, out, "Not searched (down or too slow): vec")

	f.Returns("RecallMemory", &wire.RecallResult{Hits: []wire.MemoryHit{{Kind: "claim", Text: "Works at Acme",
		Source: "chat “Work”, 15 Sep", Disputed: true}}}, nil)
	out, _, code = runCLI(t, f, "memory", "search", "work")
	require.Equal(t, 0, code)
	require.Regexp(t, `claim\s+Works at Acme \(disputed\)\s+chat “Work”`, out)
	_, _, code = runCLI(t, f, "memory", "search", "work", "--as-of", "Monday")
	require.Equal(t, exitUsage, code)
}

func TestMemoryRememberAndEditCLI(t *testing.T) {
	t.Parallel()
	f := memoryFake().
		Returns("Remember", &wire.MemoryClaimsResult{Claims: []wire.MemoryClaim{claimA}}, nil).
		Returns("EditClaim", &wire.MemoryClaimsResult{Claims: []wire.MemoryClaim{claimB}}, nil)
	out, _, code := runCLI(t, f, "memory", "remember", "I", "work", "at", "Acme")
	require.Equal(t, 0, code)
	require.Equal(t, []any{wire.RememberRequest{Text: "I work at Acme"}, client.RememberCLI}, lastCall(t, f, "Remember"))
	require.Contains(t, out, "Remembered:\n000a1b2c  Works at Acme  (active)")

	_, _, code = runCLI(t, f, "memory", "edit", "0a1b2c", "--value", "Globex")
	require.Equal(t, 0, code)
	args := lastCall(t, f, "EditClaim")
	require.Equal(t, claimAID, args[0])
	require.Equal(t, "Globex", *args[1].(wire.EditClaimRequest).Value)
	_, _, code = runCLI(t, f, "memory", "edit", "0a1b2c", "Works", "at", "Globex", "since", "October")
	require.Equal(t, 0, code)
	require.Equal(t, wire.EditClaimRequest{Text: "Works at Globex since October"}, lastCall(t, f, "EditClaim")[1])
	for _, bad := range [][]string{{"0a1b2c"}, {"0a1b2c", "text", "--value", "v"}} {
		_, _, code = runCLI(t, f, append([]string{"memory", "edit"}, bad...)...)
		require.Equal(t, exitUsage, code, bad)
	}
}

func TestMemoryClaimChangesCLI(t *testing.T) {
	t.Parallel()
	f := memoryFake().
		Returns("ConfirmClaim", &claimA, nil).
		Returns("RejectClaim", &claimB, nil).
		Returns("HideClaim", &claimB, nil)
	out, _, code := runCLI(t, f, "memory", "confirm", "0a1b2c")
	require.Equal(t, 0, code)
	require.Equal(t, claimAID, lastCall(t, f, "ConfirmClaim")[0])
	require.Contains(t, out, "Works at Acme  (active)")
	_, _, code = runCLI(t, f, "memory", "reject", "0d3e4f")
	require.Equal(t, 0, code)
	require.Equal(t, claimBID, lastCall(t, f, "RejectClaim")[0])
	_, _, code = runCLI(t, f, "memory", "hide", "0d3e4f")
	require.Equal(t, 0, code)
	require.Equal(t, []any{claimBID, wire.HideClaimRequest{Hidden: true}}, lastCall(t, f, "HideClaim"))
	_, _, code = runCLI(t, f, "memory", "unhide", "0d3e4f")
	require.Equal(t, 0, code)
	require.Equal(t, []any{claimBID, wire.HideClaimRequest{Hidden: false}}, lastCall(t, f, "HideClaim"))
}

func TestMemoryForgetCLI(t *testing.T) {
	t.Parallel()
	f := memoryFake().Returns("GetClaim", &claimA, nil).Returns("ForgetClaim", &receipt, nil)

	out, _, code := runMemoryCLI(t, f, "n\n", "memory", "forget", "0a1b2c")
	require.Equal(t, 0, code)
	require.Contains(t, out, "Erase “Works at Acme” everywhere? This cannot be undone. [y/N] ")
	require.Contains(t, out, "Nothing was erased.")
	require.Empty(t, f.CallsTo("ForgetClaim"))
	_, _, code = runMemoryCLI(t, f, "", "memory", "forget", "0a1b2c")
	require.Equal(t, 0, code)
	require.Empty(t, f.CallsTo("ForgetClaim"), "no answer is no")

	out, _, code = runMemoryCLI(t, f, "y\n", "memory", "forget", "0a1b2c")
	require.Equal(t, 0, code)
	require.Equal(t, claimAID, lastCall(t, f, "ForgetClaim")[0])
	require.Contains(t, out, "Erased 1 memory.")
	require.Regexp(t, `claims\s+erased`, out)
	require.Regexp(t, `mem0\s+not yet\s+it will be retried`, out)

	f.Reset()
	out, _, code = runMemoryCLI(t, f, "", "memory", "forget", "0a1b2c", "--yes")
	require.Equal(t, 0, code)
	require.Len(t, f.CallsTo("ForgetClaim"), 1)
	require.Empty(t, f.CallsTo("GetClaim"), "--yes does not ask")
	require.NotContains(t, out, "[y/N]")
}

func TestMemoryForgetAllCLI(t *testing.T) {
	t.Parallel()
	f := memoryFake().Returns("ForgetAllMemory", &wire.ForgetReceipt{Claims: 12, Episodes: 3}, nil)
	out, _, code := runMemoryCLI(t, f, "yes\n", "memory", "forget-all")
	require.Equal(t, 0, code)
	require.Contains(t, out, `Type "forget everything" to go on: `)
	require.Contains(t, out, "Nothing was erased.")
	require.Empty(t, f.CallsTo("ForgetAllMemory"))

	out, _, code = runMemoryCLI(t, f, "forget everything\n", "memory", "forget-all")
	require.Equal(t, 0, code)
	require.Equal(t, client.ForgetAllConfirm, lastCall(t, f, "ForgetAllMemory")[0])
	require.Contains(t, out, "Erased 12 memories and 3 notes and documents.")
	_, _, code = runMemoryCLI(t, f, "", "memory", "forget-all", "--yes")
	require.Equal(t, 0, code)
	require.Len(t, f.CallsTo("ForgetAllMemory"), 2)
}

func TestMemoryConflictsCLI(t *testing.T) {
	t.Parallel()
	resolved := wire.MemoryConflict{ID: confID, Reason: "two employers", Status: "resolved"}
	f := memoryFake().Returns("ResolveConflict", &resolved, nil)
	out, _, code := runCLI(t, f, "memory", "conflicts")
	require.Equal(t, 0, code)
	require.Equal(t, "", lastCall(t, f, "ListConflicts")[0])
	require.Contains(t, out, "0c0f1c  two employers\n  a  Works at Acme  (chat “Work”, 15 Sep)\n  b  Works at Globex  (your edit)")
	_, _, code = runCLI(t, f, "memory", "conflicts", "--all")
	require.Equal(t, 0, code)
	require.Equal(t, "all", lastCall(t, f, "ListConflicts")[0])

	out, _, code = runCLI(t, f, "memory", "resolve", "0c0f1c", "B")
	require.Equal(t, 0, code)
	require.Equal(t, []any{confID, wire.ResolveConflictRequest{Resolution: wire.ResolveKeepB}},
		lastCall(t, f, "ResolveConflict"))
	require.Contains(t, out, "Settled: two employers.")
	_, _, code = runCLI(t, f, "memory", "resolve", "0c0f1c", "maybe")
	require.Equal(t, exitUsage, code)
}

func TestMemoryDocsCLI(t *testing.T) {
	t.Parallel()
	f := memoryFake().
		Returns("AddResource", &wire.MemoryResource{ID: docID, Title: "lease.pdf"}, nil).
		Returns("RemoveResource", &wire.ForgetReceipt{Episodes: 1}, nil)
	out, _, code := runCLI(t, f, "memory", "docs")
	require.Equal(t, 0, code)
	require.Regexp(t, `0d0c0d\s+Lease\s+/home/u/lease.pdf\s+added 15 Sep 10:30`, out)

	_, _, code = runCLI(t, f, "memory", "docs", "add", "lease.pdf", "--title", "Lease")
	require.Equal(t, 0, code)
	req := lastCall(t, f, "AddResource")[0].(wire.AddResourceRequest)
	require.True(t, filepath.IsAbs(req.Path), "the daemon gets an absolute path")
	require.Equal(t, "lease.pdf", filepath.Base(req.Path))
	require.Equal(t, "Lease", req.Title)

	out, _, code = runCLI(t, f, "memory", "docs", "remove", "0d0c0d")
	require.Equal(t, 0, code)
	require.Equal(t, docID, lastCall(t, f, "RemoveResource")[0])
	require.Contains(t, out, "Erased 1 note or document.")
}

func TestMemoryMaintenanceCLI(t *testing.T) {
	t.Parallel()
	f := memoryFake().
		Returns("RebuildMemory", nil, nil).
		Returns("SleepMemory", &wire.SleepReport{Learned: 3, Merged: 1, Profile: true}, nil).
		Returns("ExportMemory", &wire.MemoryExport{ExportedAt: wire.Millis(now), Claims: []wire.MemoryClaim{claimA}}, nil).
		Returns("AssistantSelfHistory", &wire.SelfHistory{Versions: []wire.SelfVersion{
			{Attitude: "", Reason: "reset", At: wire.Millis(now)},
			{Attitude: "angry", Note: "Late again.", Reason: "chat", At: wire.Millis(testutil.At("09:00"))}}}, nil)

	out, _, code := runCLI(t, f, "memory", "rebuild", "vec")
	require.Equal(t, 0, code)
	require.Equal(t, wire.RebuildRequest{Lane: "vec"}, lastCall(t, f, "RebuildMemory")[0])
	require.Contains(t, out, "Rebuilt vec")
	_, _, code = runCLI(t, f, "memory", "rebuild", "claims")
	require.Equal(t, exitUsage, code)

	out, _, code = runCLI(t, f, "memory", "sleep")
	require.Equal(t, 0, code)
	require.Contains(t, out, "Learned 3, merged 1")
	require.Contains(t, out, "Refreshed what she keeps in mind about you.")

	out, _, code = runCLI(t, f, "memory", "export")
	require.Equal(t, 0, code)
	var ex wire.MemoryExport
	require.NoError(t, json.Unmarshal([]byte(out), &ex))
	require.Equal(t, claimAID, ex.Claims[0].ID)
	file := filepath.Join(t.TempDir(), "memory.json")
	out, _, code = runCLI(t, f, "memory", "export", "--out", file)
	require.Equal(t, 0, code)
	require.Contains(t, out, "Saved 1 memories, 0 notes, and 0 documents to "+file)
	info, err := os.Stat(file)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "what she remembers is private")

	out, _, code = runCLI(t, f, "memory", "self")
	require.Equal(t, 0, code)
	require.Regexp(t, `15 Sep 10:30\s+reset\s+\(none\)`, out)
	require.Regexp(t, `15 Sep 09:00\s+chat\s+angry\s+Late again.`, out)
	require.Equal(t, 0, lastCall(t, f, "AssistantSelfHistory")[0])
}
