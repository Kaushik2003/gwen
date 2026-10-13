package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/memory/service"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// fakeClaude is a fake Messages API that answers the assistant and each
// memory job by the job's system prompt, and keeps what the assistant was
// last sent.
type fakeClaude struct {
	mu    sync.Mutex
	reply string         // the assistant's next reply
	sent  string         // what the assistant was last sent, system and turns
	jobs  map[string]int // memory jobs asked, by name
	// facts are what extraction finds: each is returned when its quote is
	// in an item.
	facts []map[string]any
	// contradict names, by their words, the claims that cannot both hold.
	contradict [2]string
}

func (f *fakeClaude) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		System []struct {
			Text string `json:"text"`
		} `json:"system"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	system := ""
	for _, b := range body.System {
		system += b.Text + "\n"
	}
	turns, last := "", ""
	for _, m := range body.Messages {
		var text string
		if json.Unmarshal(m.Content, &text) != nil {
			var blocks []struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(m.Content, &blocks)
			for _, b := range blocks {
				text += b.Text
			}
		}
		turns += "\n<" + m.Role + ">" + text
		last = text
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.jobs == nil {
		f.jobs = map[string]int{}
	}
	var out any
	switch {
	case strings.Contains(system, "You keep the long-term memory"):
		f.jobs["extract"]++
		var req struct {
			Items []struct {
				Ref, Text string
			} `json:"items"`
		}
		_ = json.Unmarshal([]byte(last), &req)
		facts := []map[string]any{}
		for _, fact := range f.facts {
			for _, it := range req.Items {
				if strings.Contains(it.Text, fact["quote"].(string)) {
					got := map[string]any{"source": it.Ref, "subject": "user", "valid_from": "", "valid_to": "",
						"occurred_at": "", "sensitivity": "personal", "confidence": 0.9, "updates": ""}
					for k, v := range fact {
						got[k] = v
					}
					facts = append(facts, got)
					break
				}
			}
		}
		out = map[string]any{"facts": facts}
	case strings.Contains(system, "You check facts before"):
		f.jobs["verify"]++
		var req struct {
			Checks []struct {
				Ref string `json:"ref"`
			} `json:"checks"`
		}
		_ = json.Unmarshal([]byte(last), &req)
		verdicts := []map[string]any{}
		for _, c := range req.Checks {
			verdicts = append(verdicts, map[string]any{"ref": c.Ref, "supported": "yes", "quote": ""})
		}
		out = map[string]any{"verdicts": verdicts}
	case strings.Contains(system, "You check the memory of a personal assistant"):
		f.jobs["contradict"]++
		var req struct {
			Claims []struct {
				Ref, Text string
			} `json:"claims"`
		}
		_ = json.Unmarshal([]byte(last), &req)
		refs := map[string]string{}
		for _, c := range req.Claims {
			refs[c.Text] = c.Ref
		}
		conflicts := []map[string]any{}
		if a, b := refs[f.contradict[0]], refs[f.contradict[1]]; a != "" && b != "" {
			conflicts = append(conflicts, map[string]any{"a": a, "b": b, "reason": "only one can be true now"})
		}
		out = map[string]any{"conflicts": conflicts}
	case strings.Contains(system, "You look for patterns"):
		f.jobs["insight"]++
		out = map[string]any{"insights": []any{}}
	case strings.Contains(system, "observation log"):
		f.jobs["observe"]++
		out = map[string]any{"observations": []any{}, "log": ""}
	default:
		f.sent = system + turns
		text, _ := json.Marshal(f.reply)
		fmt.Fprintf(w, `{"content": [{"type": "text", "text": %s}], "stop_reason": "end_turn"}`, text)
		return
	}
	b, _ := json.Marshal(out)
	text, _ := json.Marshal(string(b))
	fmt.Fprintf(w, `{"content": [{"type": "text", "text": %s}], "stop_reason": "end_turn"}`, text)
}

func (f *fakeClaude) set(fn func(f *fakeClaude)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeClaude) lastSent() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sent
}

func (f *fakeClaude) asked(job string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.jobs[job]
}

// TestMemoryEndToEnd runs memory through the daemon and the client: a chat
// turn is learned from, the claim is listed, the next turn's request carries
// it in the memory note and the profile card, and forgetting removes it
// everywhere; remember, resolve, status, and forget-all.
func TestMemoryEndToEnd(t *testing.T) {
	t.Parallel()
	fake := &fakeClaude{facts: []map[string]any{{"key": "user.employer", "value": "Acme",
		"text": "Works at Acme as a backend intern", "quote": "I work at Acme as a backend intern"}}}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	var svc *service.Service
	d := startDaemonWith(t, setup{llm: true}, func(o *options) {
		o.anthropicURL, o.memory, o.recallScale = srv.URL, true, 100
		o.onMemory = func(s *service.Service) { svc = s }
	})
	require.NotNil(t, svc)
	_, err := d.c.PatchConfig(d.ctx, wire.ConfigPatch{"llm": {"provider": "anthropic"}})
	require.NoError(t, err)
	require.NoError(t, config.WriteCredential(config.CredentialsDir(d.dataDir), config.CredLLMAPIKey, []byte("sk\n")))

	st, err := d.c.MemoryStatus(d.ctx)
	require.NoError(t, err)
	require.True(t, st.Enabled)
	require.True(t, st.Learning)
	require.Equal(t, "gwen", st.Lanes[0].Lane)

	// A turn the user asks to be remembered is learned from at once.
	next := stream(t, d)
	fake.set(func(f *fakeClaude) { f.reply = `{"reply": "Noted, Acme.", "mood": "happy", "actions": []}` })
	run, err := d.c.AssistantChat(d.ctx, wire.AssistantChatRequest{
		Message: "Please remember that I work at Acme as a backend intern."})
	require.NoError(t, err)
	var out wire.AssistantOutput
	require.NoError(t, json.Unmarshal(run.Output, &out))
	thread := out.ThreadID
	svc.Tick(d.ctx)
	require.Positive(t, fake.asked("extract"))
	require.Positive(t, fake.asked("verify"), "the fact is checked against its words")
	ev := next(wire.EventMemoryChanged)
	var changed wire.MemoryChanged
	require.NoError(t, json.Unmarshal(ev.Data, &changed))
	require.Len(t, changed.ClaimIDs, 1)
	acme := changed.ClaimIDs[0]

	claims, err := d.c.ListClaims(d.ctx, wire.MemoryClaimQuery{})
	require.NoError(t, err)
	require.Len(t, claims.Claims, 1)
	require.Equal(t, acme, claims.Claims[0].ID)
	require.Equal(t, "Works at Acme as a backend intern", claims.Claims[0].Text)
	require.Equal(t, "Works at", claims.Claims[0].Label)
	require.Equal(t, thread, claims.Claims[0].ThreadID)

	// The next turn carries it in the note and the profile card, and the
	// reply's cite resolves to it.
	fake.set(func(f *fakeClaude) {
		f.reply = `{"reply": "At Acme.", "mood": "smug", "actions": [], "cites": ["m1", "p1"], "forget": []}`
	})
	_, err = d.c.AssistantChat(d.ctx, wire.AssistantChatRequest{Message: "Where do I work again?", ThreadID: &thread})
	require.NoError(t, err)
	sent := fake.lastSent()
	require.Contains(t, sent, "## What you keep in mind about them")
	require.Contains(t, sent, "[p1] Works at Acme as a backend intern")
	require.Contains(t, sent, "[m1] Works at Acme as a backend intern")
	chat, err := d.c.GetThread(d.ctx, thread)
	require.NoError(t, err)
	require.Len(t, chat.Messages, 4)
	require.Equal(t, "Works at Acme as a backend intern", chat.Messages[0].Learned[0].Text, "what the message taught")
	require.Equal(t, "active", chat.Messages[0].Learned[0].Status)
	cites := chat.Messages[3].Cites
	require.Len(t, cites, 1, "the note's ref and the card's name the same claim")
	require.Equal(t, "claim:"+acme, cites[0].Ref)
	require.Equal(t, "Works at Acme as a backend intern", cites[0].Text)
	require.Equal(t, "active", cites[0].Status)

	// Remember: the words become a claim of the user's own.
	next = stream(t, d)
	remembered, err := d.c.Remember(d.ctx, wire.RememberRequest{Text: "I work at Globex on weekends"}, "")
	require.NoError(t, err)
	require.Len(t, remembered.Claims, 1)
	globex := remembered.Claims[0]
	require.Equal(t, "owner_edit", globex.Trust)
	require.Equal(t, "active", globex.Status)
	next(wire.EventMemoryChanged)

	// Resolve: consolidation finds the two cannot both hold; the user says
	// both are true.
	fake.set(func(f *fakeClaude) { f.contradict = [2]string{"Works at Acme as a backend intern", globex.Text} })
	rep, err := d.c.SleepMemory(d.ctx)
	require.NoError(t, err)
	require.Equal(t, 1, rep.Conflicts)
	require.True(t, rep.Profile)
	conflicts, err := d.c.ListConflicts(d.ctx, "")
	require.NoError(t, err)
	require.Len(t, conflicts.Conflicts, 1)
	require.True(t, conflicts.Conflicts[0].A.Disputed)
	resolved, err := d.c.ResolveConflict(d.ctx, conflicts.Conflicts[0].ID, wire.ResolveConflictRequest{
		Resolution: wire.ResolveBoth})
	require.NoError(t, err)
	require.Equal(t, "resolved", resolved.Status)
	require.Equal(t, "active", resolved.A.Status)
	require.Equal(t, "active", resolved.B.Status)
	require.False(t, resolved.A.Disputed)

	// Forget: the claim is gone from the store, the lanes, the note, and the
	// card; the receipt says the chat still holds its words.
	receipt, err := d.c.ForgetClaim(d.ctx, acme)
	require.NoError(t, err)
	require.Equal(t, 1, receipt.Claims)
	require.Equal(t, "gwen", receipt.Lanes[0].Lane)
	require.True(t, receipt.Lanes[0].OK)
	require.Contains(t, receipt.Lanes[0].Note, "still in Past chats")
	_, err = d.c.GetClaim(d.ctx, acme)
	apiErr(t, err, wire.CodeNotFound)
	claims, err = d.c.ListClaims(d.ctx, wire.MemoryClaimQuery{Status: "all"})
	require.NoError(t, err)
	for _, c := range claims.Claims {
		require.NotEqual(t, acme, c.ID)
	}
	fake.set(func(f *fakeClaude) { f.reply = `{"reply": "Globex.", "mood": "happy", "actions": []}` })
	_, err = d.c.AssistantChat(d.ctx, wire.AssistantChatRequest{Message: "Where do I work again?", ThreadID: &thread})
	require.NoError(t, err)
	sent = fake.lastSent()
	require.NotContains(t, sent, "Works at Acme as a backend intern", "forgotten everywhere: note, card, past notes")
	require.Contains(t, sent, globex.Text)
	chat, err = d.c.GetThread(d.ctx, thread)
	require.NoError(t, err)
	require.True(t, chat.Messages[3].Cites[0].Gone, "the old cite shows something since forgotten")
	require.Empty(t, chat.Messages[0].Learned)

	// Status, then forget everything.
	st, err = d.c.MemoryStatus(d.ctx)
	require.NoError(t, err)
	require.Equal(t, 1, st.Claims["active"])
	require.True(t, st.LedgerOK)
	require.NotNil(t, st.LastSleptAt)
	_, err = d.c.ForgetAllMemory(d.ctx, "everything")
	apiErr(t, err, wire.CodeInvalidRequest)
	all, err := d.c.ForgetAllMemory(d.ctx, "forget everything")
	require.NoError(t, err)
	require.Equal(t, 1, all.Claims)
	claims, err = d.c.ListClaims(d.ctx, wire.MemoryClaimQuery{Status: "all"})
	require.NoError(t, err)
	require.Empty(t, claims.Claims)
	chat, err = d.c.GetThread(d.ctx, thread)
	require.NoError(t, err)
	require.Len(t, chat.Messages, 6, "chats are kept")

	// Memory off: the routes say so, chats go on.
	_, err = d.c.PatchConfig(d.ctx, wire.ConfigPatch{"memory": {"enabled": false}})
	require.NoError(t, err)
	_, err = d.c.ListClaims(d.ctx, wire.MemoryClaimQuery{})
	apiErr(t, err, wire.CodeUnavailable)
	_, err = d.c.AssistantChat(d.ctx, wire.AssistantChatRequest{Message: "Hi again", ThreadID: &thread})
	require.NoError(t, err)
}

// TestMemoryNotRunning: a daemon that runs no memory answers 503 on
// the memory routes.
func TestMemoryNotRunning(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	_, err := d.c.MemoryStatus(d.ctx)
	apiErr(t, err, wire.CodeUnavailable)
}
