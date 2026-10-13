package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/wire"
)

// Chat list and search sizes.
const (
	DefaultChats      = 50
	MaxChats          = 200
	DefaultChatHits   = 20
	MaxChatHits       = 50
	maxChatQueryRunes = 500
)

// listThreads is GET /v1/assistant/threads?limit=&offset=: the chats, the
// most recently active first.
func (s *Server) listThreads(w http.ResponseWriter, r *http.Request) error {
	limit, err := queryLimit(r, DefaultChats, MaxChats)
	if err != nil {
		return err
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if offset, err = strconv.Atoi(v); err != nil || offset < 0 {
			return badRequest("offset", "offset is a number of chats, 0 or more")
		}
	}
	threads, err := s.Repos.Chats.List(r.Context(), limit, offset)
	if err != nil {
		return err
	}
	out := wire.ChatThreadList{Threads: make([]wire.ChatThread, len(threads))}
	for i, t := range threads {
		out.Threads[i] = threadWire(t)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// getThread is GET /v1/assistant/threads/{id}: a chat and its messages.
func (s *Server) getThread(w http.ResponseWriter, r *http.Request) error {
	ctx, id := r.Context(), r.PathValue("id")
	t, err := s.Repos.Chats.Get(ctx, id)
	if err != nil {
		return err
	}
	msgs, err := s.chatMessages(ctx, id)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, wire.ChatThreadDetail{Thread: threadWire(t), Messages: msgs})
	return nil
}

// deleteThread is DELETE /v1/assistant/threads/{id}: the chat's words are
// erased, and the assistant no longer recalls them; its memory notes and
// observation log go, and so do the claims learned from it that the user did
// not edit (store.ChatRepo.Delete).
func (s *Server) deleteThread(w http.ResponseWriter, r *http.Request) error {
	if err := s.Repos.Chats.Delete(r.Context(), r.PathValue("id")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// searchThreads is GET /v1/assistant/search?q=: the messages of every chat
// that share words with q, the best match first.
func (s *Server) searchThreads(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query().Get("q")
	if len([]rune(q)) > maxChatQueryRunes {
		return badRequest("q", "a search is at most %d characters", maxChatQueryRunes)
	}
	limit, err := queryLimit(r, DefaultChatHits, MaxChatHits)
	if err != nil {
		return err
	}
	hits, err := s.Repos.Chats.Search(r.Context(), q, limit, nil)
	if err != nil {
		return err
	}
	msgs := make([]model.ChatMessage, len(hits))
	for i, h := range hits {
		msgs[i] = h.ChatMessage
	}
	shown := s.messagesWire(r.Context(), msgs)
	out := wire.ChatSearchResult{Hits: make([]wire.ChatHit, len(hits))}
	for i, h := range hits {
		out.Hits[i] = wire.ChatHit{ThreadID: h.ThreadID, ThreadTitle: h.ThreadTitle, Message: shown[i]}
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// queryLimit reads ?limit=, 1 to most, or def when absent.
func queryLimit(r *http.Request, def, most int) (int, error) {
	v := r.URL.Query().Get("limit")
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > most {
		return 0, badRequest("limit", "limit must be 1 to %d", most)
	}
	return n, nil
}

func threadWire(t model.ChatThread) wire.ChatThread {
	return wire.ChatThread{ID: t.ID, Title: t.Title, Messages: t.Messages, CreatedAt: wire.Millis(t.CreatedAt),
		LastAt: wire.Millis(t.LastAt)}
}

func messageWire(m model.ChatMessage) wire.AssistantMessage {
	out := wire.AssistantMessage{ID: m.ID, Role: m.Role, Text: m.Text, At: wire.Millis(m.At), Mood: m.Mood,
		Attitude: m.Attitude}
	if err := json.Unmarshal(m.Actions, &out.Actions); err != nil {
		slog.Warn("chat message actions unreadable", "id", m.ID, "err", err)
	}
	if len(out.Actions) == 0 {
		out.Actions = nil
	}
	return out
}

// chatMessage is a turn as the chat keeps it. Its memory refs are set by the
// caller: the wire form holds them resolved.
func chatMessage(m wire.AssistantMessage) model.ChatMessage {
	actions := []byte("[]")
	if len(m.Actions) > 0 {
		if b, err := json.Marshal(m.Actions); err == nil {
			actions = b
		}
	}
	return model.ChatMessage{Role: m.Role, Text: m.Text, At: time.UnixMilli(m.At), Mood: m.Mood,
		Attitude: m.Attitude, Actions: actions}
}

// usageWire is what a request used, or nil when the provider did not say.
func usageWire(u llm.Usage) *wire.LlmUsage {
	if u == (llm.Usage{}) {
		return nil
	}
	return &wire.LlmUsage{Input: u.Input, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite, Output: u.Output}
}
