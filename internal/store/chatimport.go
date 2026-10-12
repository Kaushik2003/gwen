package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/kzark/gwen/internal/model"
)

// legacyMessage is a message as the assistant runs before 00009_chats.sql
// stored it, in wire.AssistantMessage's JSON.
type legacyMessage struct {
	Role     string          `json:"role"`
	Text     string          `json:"text"`
	At       int64           `json:"at"`
	Mood     string          `json:"mood"`
	Attitude string          `json:"attitude"`
	Actions  json.RawMessage `json:"actions"`
}

type legacyRun struct {
	id      string
	created time.Time
	raw     []json.RawMessage
	msgs    []legacyMessage
	parent  int // the run this one continued, or -1
	child   bool
}

// importChats turns the assistant conversations kept before
// 00009_chats.sql into threads, once. Each turn's run held the whole
// conversation so far, so a run that no later run continues is a whole
// conversation. It becomes a thread whose id is that run's id, so a client
// that remembers the run finds the thread. Every run of the conversation then
// keeps only its own turn, and its subject is the thread.
func importChats(ctx context.Context, db *DB) error {
	_, err := GetLocal(ctx, db.sql, KeyChatsImported)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrNotFound) {
		return err
	}
	err = db.InTx(ctx, func(tx *sql.Tx) error {
		runs, err := legacyRuns(ctx, tx)
		if err != nil {
			return err
		}
		full := make(map[[32]byte]int, len(runs))
		for i, r := range runs {
			full[fingerprint(r.msgs)] = i
		}
		for i, r := range runs {
			if n := len(r.msgs); n > 2 {
				if p, ok := full[fingerprint(r.msgs[:n-2])]; ok && p != i {
					runs[i].parent, runs[p].child = p, true
				}
			}
		}
		thread := make([]string, len(runs))
		for i, r := range runs {
			if r.child {
				continue
			}
			if err := importConversation(ctx, tx, db, r); err != nil {
				return err
			}
			for j := i; j >= 0 && thread[j] == ""; j = runs[j].parent {
				thread[j] = r.id
			}
		}
		for i, r := range runs {
			turn := r.raw[max(0, len(r.raw)-2):]
			out, err := json.Marshal(struct {
				ThreadID string            `json:"thread_id"`
				Messages []json.RawMessage `json:"messages"`
			}{thread[i], turn})
			if err != nil {
				return err
			}
			const qTurnOnly = `UPDATE llm_runs SET subject_id = ?, output = ? WHERE id = ?`
			if _, err := tx.ExecContext(ctx, qTurnOnly, thread[i], string(out), r.id); err != nil {
				return err
			}
		}
		return SetLocalTime(ctx, tx, KeyChatsImported, db.Now(), db.Now())
	})
	if err != nil {
		return fmt.Errorf("import chats: %w", err)
	}
	return nil
}

// legacyRuns reads the answered assistant runs that still hold a whole
// conversation, oldest first. A run whose output does not read is left as
// it is.
func legacyRuns(ctx context.Context, tx *sql.Tx) ([]legacyRun, error) {
	const qLegacyRuns = `SELECT id, output, created_at FROM llm_runs WHERE kind = 'assistant' AND status = 'ok'
		ORDER BY created_at, id`
	rows, err := tx.QueryContext(ctx, qLegacyRuns)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runs []legacyRun
	for rows.Next() {
		var (
			r       = legacyRun{parent: -1}
			output  string
			created int64
		)
		if err := rows.Scan(&r.id, &output, &created); err != nil {
			return nil, err
		}
		var out struct {
			ThreadID string            `json:"thread_id"`
			Messages []json.RawMessage `json:"messages"`
		}
		if json.Unmarshal([]byte(output), &out) != nil || out.ThreadID != "" || len(out.Messages) == 0 {
			continue
		}
		r.created, r.raw, r.msgs = timeOf(created), out.Messages, make([]legacyMessage, len(out.Messages))
		ok := true
		for i, m := range out.Messages {
			if json.Unmarshal(m, &r.msgs[i]) != nil ||
				r.msgs[i].Role != model.ChatUser && r.msgs[i].Role != model.ChatAssistant {
				ok = false
				break
			}
		}
		if ok {
			runs = append(runs, r)
		}
	}
	return runs, rows.Err()
}

// fingerprint identifies a list of messages by who said what, when.
func fingerprint(msgs []legacyMessage) [32]byte {
	h := sha256.New()
	for _, m := range msgs {
		h.Write([]byte(m.Role + "\x00" + strconv.FormatInt(m.At, 10) + "\x00" + m.Text + "\x00"))
	}
	var sum [32]byte
	h.Sum(sum[:0])
	return sum
}

// importConversation writes the whole conversation of run r as a thread.
func importConversation(ctx context.Context, tx *sql.Tx, db *DB, r legacyRun) error {
	at := func(m legacyMessage) time.Time {
		if m.At <= 0 {
			return r.created
		}
		return timeOf(m.At)
	}
	title := r.msgs[0].Text
	for _, m := range r.msgs {
		if m.Role == model.ChatUser {
			title = m.Text
			break
		}
	}
	first := at(r.msgs[0])
	if err := insertChatThread(ctx, tx, model.ChatThread{ID: r.id, Title: chatTitle(title),
		Envelope: model.Envelope{CreatedAt: first, UpdatedAt: first, DeviceID: db.DeviceID(), Rev: 1}}); err != nil {
		return err
	}
	for _, m := range r.msgs {
		actions := []byte("[]")
		var list []json.RawMessage
		if json.Unmarshal(m.Actions, &list) == nil && list != nil {
			actions = m.Actions
		}
		when := at(m)
		if err := insertChatMessage(ctx, tx, model.ChatMessage{ID: model.NewID(), ThreadID: r.id, Role: m.Role,
			Text: m.Text, At: when, Mood: m.Mood, Attitude: m.Attitude, Actions: actions,
			Envelope: model.Envelope{CreatedAt: when, UpdatedAt: when, DeviceID: db.DeviceID(), Rev: 1}}); err != nil {
			return err
		}
	}
	return nil
}
