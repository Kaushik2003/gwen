package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kzark/gwen/internal/model"
)

// chatsVersion is the schema version that added the chat tables.
const chatsVersion = 9

// MaxChatTitle is the most characters of a thread's title, taken from its
// first message.
const MaxChatTitle = 80

// ChatRepo keeps the assistant's conversations (00009_chats.sql): threads of
// messages, searchable by their words.
type ChatRepo interface {
	// Get returns a live thread.
	Get(ctx context.Context, id string) (model.ChatThread, error)
	// List returns up to limit live threads, the most recently active first,
	// after skipping offset of them.
	List(ctx context.Context, limit, offset int) ([]model.ChatThread, error)
	// Messages returns a live thread's messages, oldest first.
	Messages(ctx context.Context, threadID string) ([]model.ChatMessage, error)
	// Append adds msgs in order to a live thread, or, when threadID is "", to
	// a new thread titled by the first of them. It returns the thread and
	// the messages as stored.
	Append(ctx context.Context, threadID string, msgs []model.ChatMessage) (model.ChatThread, []model.ChatMessage, error)
	// AppendTurn appends like Append and, in the same transaction, records
	// the turn's ok assistant run, whose subject is the thread and whose
	// output is output(the thread's id, the messages as stored).
	AppendTurn(ctx context.Context, threadID string, msgs []model.ChatMessage,
		output func(threadID string, msgs []model.ChatMessage) any) (model.ChatThread, model.LLMRun, error)
	// Delete deletes a thread. It and its messages stay as tombstones with
	// their words erased, and the assistant runs of its turns keep no words
	// either.
	Delete(ctx context.Context, id string) error
	// Search returns up to limit messages of live threads that share words
	// with text, the best match first, leaving out the messages in skip.
	Search(ctx context.Context, text string, limit int, skip []string) ([]model.ChatHit, error)
}

type chatRepo struct{ db *DB }

// NewChatRepo returns the SQLite ChatRepo.
func NewChatRepo(db *DB) ChatRepo { return chatRepo{db} }

// chatThreadCols reads a thread as t, with when its newest message was said
// (its creation when it has none) and how many it has.
const chatThreadCols = `t.id, t.title,
	COALESCE((SELECT MAX(m.at) FROM chat_messages m WHERE m.thread_id = t.id AND m.deleted_at IS NULL), t.created_at)
		AS last_at,
	(SELECT COUNT(*) FROM chat_messages m WHERE m.thread_id = t.id AND m.deleted_at IS NULL),
	t.created_at, t.updated_at, t.deleted_at, t.device_id, t.rev`

func scanChatThread(s scanner) (model.ChatThread, error) {
	var (
		t    model.ChatThread
		last int64
		env  envelopeScan
	)
	if err := s.Scan(append([]any{&t.ID, &t.Title, &last, &t.Messages}, env.dest()...)...); err != nil {
		return model.ChatThread{}, err
	}
	t.LastAt, t.Envelope = timeOf(last), env.envelope()
	return t, nil
}

const chatMessageCols = `m.id, m.thread_id, m.role, m.text, m.at, m.mood, m.attitude, m.actions,
	m.created_at, m.updated_at, m.deleted_at, m.device_id, m.rev`

func scanChatMessage(s scanner, extra ...any) (model.ChatMessage, error) {
	var (
		m       model.ChatMessage
		at      int64
		actions string
		env     envelopeScan
	)
	dest := append([]any{&m.ID, &m.ThreadID, &m.Role, &m.Text, &at, &m.Mood, &m.Attitude, &actions}, env.dest()...)
	if err := s.Scan(append(dest, extra...)...); err != nil {
		return model.ChatMessage{}, err
	}
	m.At, m.Actions, m.Envelope = timeOf(at), []byte(actions), env.envelope()
	return m, nil
}

func getChatThread(ctx context.Context, q Querier, id string) (model.ChatThread, error) {
	const qGetChatThread = `SELECT ` + chatThreadCols + ` FROM chat_threads t WHERE t.id = ? AND t.deleted_at IS NULL`
	t, err := scanChatThread(q.QueryRowContext(ctx, qGetChatThread, id))
	if err != nil {
		return model.ChatThread{}, fmt.Errorf("get chat: %w", notFound(err, "chat", id))
	}
	return t, nil
}

func (r chatRepo) Get(ctx context.Context, id string) (model.ChatThread, error) {
	return getChatThread(ctx, r.db.sql, id)
}

func (r chatRepo) List(ctx context.Context, limit, offset int) ([]model.ChatThread, error) {
	const qListChatThreads = `SELECT ` + chatThreadCols + ` FROM chat_threads t WHERE t.deleted_at IS NULL
		ORDER BY last_at DESC, t.id DESC LIMIT ? OFFSET ?`
	rows, err := r.db.sql.QueryContext(ctx, qListChatThreads, limit, max(offset, 0))
	if err != nil {
		return nil, fmt.Errorf("list chats: %w", err)
	}
	defer rows.Close()
	out := []model.ChatThread{}
	for rows.Next() {
		t, err := scanChatThread(rows)
		if err != nil {
			return nil, fmt.Errorf("list chats: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r chatRepo) Messages(ctx context.Context, threadID string) ([]model.ChatMessage, error) {
	if _, err := r.Get(ctx, threadID); err != nil {
		return nil, err
	}
	const qChatMessages = `SELECT ` + chatMessageCols + ` FROM chat_messages m
		WHERE m.thread_id = ? AND m.deleted_at IS NULL ORDER BY m.at, m.rowid`
	rows, err := r.db.sql.QueryContext(ctx, qChatMessages, threadID)
	if err != nil {
		return nil, fmt.Errorf("chat messages: %w", err)
	}
	defer rows.Close()
	out := []model.ChatMessage{}
	for rows.Next() {
		m, err := scanChatMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("chat messages: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r chatRepo) Append(ctx context.Context, threadID string, msgs []model.ChatMessage) (model.ChatThread, []model.ChatMessage, error) {
	var (
		thread model.ChatThread
		out    []model.ChatMessage
	)
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		thread, out, err = r.append(ctx, tx, threadID, msgs)
		return err
	})
	if err != nil {
		return model.ChatThread{}, nil, fmt.Errorf("add to chat: %w", err)
	}
	return thread, out, nil
}

func (r chatRepo) AppendTurn(ctx context.Context, threadID string, msgs []model.ChatMessage,
	output func(threadID string, msgs []model.ChatMessage) any) (model.ChatThread, model.LLMRun, error) {
	var (
		thread model.ChatThread
		run    model.LLMRun
	)
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		var (
			stored []model.ChatMessage
			err    error
		)
		if thread, stored, err = r.append(ctx, tx, threadID, msgs); err != nil {
			return err
		}
		run, err = insertLLMRun(ctx, tx, r.db.Now(), model.RunAssistant, thread.ID, model.RunOK,
			output(thread.ID, stored))
		return err
	})
	if err != nil {
		return model.ChatThread{}, model.LLMRun{}, fmt.Errorf("add to chat: %w", err)
	}
	return thread, run, nil
}

// append adds msgs in tx, as Append describes.
func (r chatRepo) append(ctx context.Context, tx *sql.Tx, threadID string, msgs []model.ChatMessage) (model.ChatThread, []model.ChatMessage, error) {
	if len(msgs) == 0 {
		return model.ChatThread{}, nil, Fail(ErrInvalid, "nothing to add to the chat")
	}
	now := r.db.Now()
	env := model.Envelope{CreatedAt: now, UpdatedAt: now, DeviceID: r.db.DeviceID(), Rev: 1}
	if threadID == "" {
		threadID = model.NewID()
		if err := insertChatThread(ctx, tx, model.ChatThread{ID: threadID, Title: chatTitle(msgs[0].Text),
			Envelope: env}); err != nil {
			return model.ChatThread{}, nil, err
		}
	} else if _, err := getChatThread(ctx, tx, threadID); err != nil {
		return model.ChatThread{}, nil, err
	}
	out := make([]model.ChatMessage, len(msgs))
	for i, m := range msgs {
		m.ID, m.ThreadID, m.Envelope = model.NewID(), threadID, env
		if m.At.IsZero() {
			m.At = now
		}
		if len(m.Actions) == 0 {
			m.Actions = []byte("[]")
		}
		if err := insertChatMessage(ctx, tx, m); err != nil {
			return model.ChatThread{}, nil, err
		}
		out[i] = m
	}
	thread, err := getChatThread(ctx, tx, threadID)
	return thread, out, err
}

func insertChatThread(ctx context.Context, q Querier, t model.ChatThread) error {
	const qInsertChatThread = `INSERT INTO chat_threads (id, title, ` + envelopeCols + `) VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := q.ExecContext(ctx, qInsertChatThread, t.ID, t.Title, t.CreatedAt.UnixMilli(), t.UpdatedAt.UnixMilli(),
		nullMillis(t.DeletedAt), t.DeviceID, t.Rev)
	if err != nil {
		return fmt.Errorf("insert chat: %w", classify(err))
	}
	return nil
}

func insertChatMessage(ctx context.Context, q Querier, m model.ChatMessage) error {
	if m.Role != model.ChatUser && m.Role != model.ChatAssistant {
		return FailField(ErrInvalid, "role", "a message is the user's or the assistant's, not %q", m.Role)
	}
	const qInsertChatMessage = `INSERT INTO chat_messages (id, thread_id, role, text, at, mood, attitude, actions, ` +
		envelopeCols + `) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := q.ExecContext(ctx, qInsertChatMessage, m.ID, m.ThreadID, m.Role, m.Text, m.At.UnixMilli(), m.Mood,
		m.Attitude, string(m.Actions), m.CreatedAt.UnixMilli(), m.UpdatedAt.UnixMilli(), nullMillis(m.DeletedAt),
		m.DeviceID, m.Rev)
	if err != nil {
		return fmt.Errorf("insert chat message: %w", classify(err))
	}
	return nil
}

func (r chatRepo) Delete(ctx context.Context, id string) error {
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := getChatThread(ctx, tx, id); err != nil {
			return err
		}
		now, device := r.db.Now().UnixMilli(), r.db.DeviceID()
		const qEraseChatMessages = `UPDATE chat_messages SET text = '', mood = '', attitude = '', actions = '[]',
			deleted_at = ?, updated_at = ?, device_id = ?, rev = rev + 1 WHERE thread_id = ? AND deleted_at IS NULL`
		if _, err := tx.ExecContext(ctx, qEraseChatMessages, now, now, device, id); err != nil {
			return err
		}
		const qEraseChatThread = `UPDATE chat_threads SET title = '', deleted_at = ?, updated_at = ?, device_id = ?,
			rev = rev + 1 WHERE id = ?`
		if _, err := tx.ExecContext(ctx, qEraseChatThread, now, now, device, id); err != nil {
			return err
		}
		const qEraseChatRuns = `UPDATE llm_runs SET output = json_object('thread_id', ?, 'messages', json('[]')),
			updated_at = ? WHERE kind = 'assistant' AND subject_id = ?`
		if _, err := tx.ExecContext(ctx, qEraseChatRuns, id, now, id); err != nil {
			return err
		}
		// The search index drops a deleted word only when its segments merge.
		const qMergeChatIndex = `INSERT INTO chat_messages_fts (chat_messages_fts) VALUES ('optimize')`
		_, err := tx.ExecContext(ctx, qMergeChatIndex)
		return err
	})
	if err != nil {
		return fmt.Errorf("delete chat %s: %w", id, err)
	}
	// The write-ahead log still holds the pages as they were until they are
	// copied into the database; copy them now, when no reader is in the way.
	if _, err := r.db.sql.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		slog.Warn("chat deleted, but the write-ahead log was not checkpointed", "err", err)
	}
	return nil
}

func (r chatRepo) Search(ctx context.Context, text string, limit int, skip []string) ([]model.ChatHit, error) {
	out := []model.ChatHit{}
	match := searchQuery(text)
	if match == "" || limit <= 0 {
		return out, nil
	}
	q := `SELECT ` + chatMessageCols + `, t.title FROM chat_messages_fts f
		JOIN chat_messages m ON m.rowid = f.rowid
		JOIN chat_threads t ON t.id = m.thread_id
		WHERE chat_messages_fts MATCH ? AND m.deleted_at IS NULL AND t.deleted_at IS NULL`
	args := []any{match}
	if len(skip) > 0 {
		q += ` AND m.id NOT IN (` + placeholders(len(skip)) + `)`
		for _, id := range skip {
			args = append(args, id)
		}
	}
	q += ` ORDER BY f.rank, m.at DESC LIMIT ?`
	rows, err := r.db.sql.QueryContext(ctx, q, append(args, limit)...)
	if err != nil {
		return nil, fmt.Errorf("search chats: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var h model.ChatHit
		if h.ChatMessage, err = scanChatMessage(rows, &h.ThreadTitle); err != nil {
			return nil, fmt.Errorf("search chats: %w", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// chatTitle names a thread by its first message: the first line, cut at a
// word to at most MaxChatTitle characters.
func chatTitle(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	line = strings.Join(strings.Fields(line), " ")
	if utf8.RuneCountInString(line) <= MaxChatTitle {
		return line
	}
	cut := string([]rune(line)[:MaxChatTitle-1])
	if i := strings.LastIndexByte(cut, ' '); i > MaxChatTitle/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,.;:-") + "…"
}

// maxSearchTerms bounds the words one search looks for.
const maxSearchTerms = 16

// searchQuery is the FTS5 query matching any word of text worth searching
// for, or "" when there is none. Words are lowercased letters and digits of
// three characters or more, or numbers of two, less common English words.
// Each is quoted, so nothing in text is read as query syntax.
func searchQuery(text string) string {
	seen := map[string]bool{}
	var terms []string
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}) {
		n := utf8.RuneCountInString(w)
		if seen[w] || stopWords[w] || n < 2 || n == 2 && !isDigits(w) {
			continue
		}
		seen[w] = true
		terms = append(terms, `"`+w+`"`)
		if len(terms) == maxSearchTerms {
			break
		}
	}
	return strings.Join(terms, " OR ")
}

func isDigits(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return !unicode.IsDigit(r) }) < 0
}

// stopWords are words too common to say what a message is about.
var stopWords = func() map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(`about above after again all also and any are aren because been before being
		below between both but can cannot could couldn did didn does doesn doing don down during each few for from
		further get got had hadn has hasn have haven having her here hers herself him himself his how into isn its
		itself just let like made make many may more most much must mustn myself not now off once only other ought
		our ours ourselves out over own really same shan she should shouldn some such than that the their theirs
		them themselves then there these they this those through too under until very was wasn were weren what
		when where which while who whom why will with won would wouldn you your yours yourself yourselves
		hey hello thanks thank please okay yeah yes sure gonna wanna want need today tomorrow yesterday`) {
		m[w] = true
	}
	return m
}()
