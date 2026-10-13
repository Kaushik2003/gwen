// Package vec is memory's vector lane: embeddings of claims, messages, and
// episodes from an embedding server (Ollama or OpenAI-compatible), kept in
// index.db with sqlite-vec and searched by cosine distance.
package vec

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"

	_ "modernc.org/sqlite"     // the database/sql driver
	_ "modernc.org/sqlite/vec" // registers vec0 on every connection
)

// defaultCandidates is how many hits Search returns when the query does not
// say.
const defaultCandidates = 40

// probeText is what Health embeds to learn that the server answers and the
// vectors' dimension.
const probeText = "Gwen checks that the embedding model answers."

// Meta keys.
const (
	metaModel = "model"
	metaDim   = "dim"
)

// Options configures an Index. Path is the index.db file; Endpoint and Model
// name the embedding server and model.
type Options struct {
	Path       string
	Endpoint   string
	Model      string
	HTTPClient *http.Client
	Clock      clock.Clock
}

var _ memory.Projection = (*Index)(nil)

// Index is the vector lane. It implements memory.Projection. Records use
// Record.ID as the ref they are found by (claim:<id>, msg:<id>, ep:<id>).
type Index struct {
	db       *sql.DB
	clock    clock.Clock
	embedder embedder
	// local is set when the embedding server is on this machine, the only
	// case where sensitive records are embedded.
	local bool

	mu  sync.RWMutex // guards dim and the vec_items table's existence
	dim int          // 0 until the first vector is known
}

// New opens (creating it when missing) the index at o.Path. When the model
// differs from the one the index was built with, the vectors are dropped so
// a rebuild can repopulate them.
func New(o Options) (*Index, error) {
	if o.Path == "" {
		return nil, errors.New("vec: no index path")
	}
	if o.Endpoint == "" || o.Model == "" {
		return nil, errors.New("vec: an embedding endpoint and model are needed")
	}
	if o.Clock == nil {
		o.Clock = clock.Real()
	}
	if err := os.MkdirAll(filepath.Dir(o.Path), 0o700); err != nil {
		return nil, fmt.Errorf("vec: create the index directory: %w", err)
	}
	dsn := "file:" + o.Path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)" +
		"&_pragma=secure_delete(1)&_pragma=synchronous(NORMAL)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("vec: open %s: %w", o.Path, err)
	}
	db.SetMaxOpenConns(4)
	ix := &Index{db: db, clock: o.Clock, embedder: newEmbedder(o.Endpoint, o.Model, o.HTTPClient),
		local: loopback(o.Endpoint)}
	if err := ix.init(context.Background(), o.Model); err != nil {
		db.Close()
		return nil, fmt.Errorf("vec: open %s: %w", o.Path, err)
	}
	return ix, nil
}

// init creates the tables and drops the vectors of another model.
func (ix *Index) init(ctx context.Context, model string) error {
	const schema = `CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS items (item TEXT PRIMARY KEY, hash TEXT NOT NULL, at INTEGER NOT NULL);`
	if _, err := ix.db.ExecContext(ctx, schema); err != nil {
		return err
	}
	meta, err := ix.meta(ctx)
	if err != nil {
		return err
	}
	if meta[metaModel] != model {
		if err := ix.reset(ctx, 0); err != nil {
			return err
		}
		_, err := ix.db.ExecContext(ctx, `INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)`, metaModel, model)
		return err
	}
	dim, _ := strconv.Atoi(meta[metaDim])
	if dim <= 0 {
		return nil
	}
	if err := ix.createTable(ctx, dim); err != nil {
		return err
	}
	ix.dim = dim
	return nil
}

func (ix *Index) meta(ctx context.Context) (map[string]string, error) {
	rows, err := ix.db.QueryContext(ctx, `SELECT key, value FROM meta`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (ix *Index) createTable(ctx context.Context, dim int) error {
	q := fmt.Sprintf(`CREATE VIRTUAL TABLE IF NOT EXISTS vec_items USING vec0(item TEXT PRIMARY KEY,
		embedding float[%d] distance_metric=cosine)`, dim)
	_, err := ix.db.ExecContext(ctx, q)
	return err
}

// reset drops every vector and item and makes the index dim wide, or of an
// unknown width when dim is 0. The caller holds mu, or is init.
func (ix *Index) reset(ctx context.Context, dim int) error {
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	drop := []string{`DROP TABLE IF EXISTS vec_items`, `DELETE FROM items`, `DELETE FROM meta WHERE key = 'dim'`}
	for _, q := range drop {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	if dim > 0 {
		q := fmt.Sprintf(`CREATE VIRTUAL TABLE vec_items USING vec0(item TEXT PRIMARY KEY,
			embedding float[%d] distance_metric=cosine)`, dim)
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO meta (key, value) VALUES ('dim', ?)`, strconv.Itoa(dim)); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	ix.dim = dim
	return nil
}

// fit makes the index as wide as vectors of dim: it creates the table on the
// first vector and starts over when the model's dimension changed.
func (ix *Index) fit(ctx context.Context, dim int) error {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if ix.dim == dim {
		return nil
	}
	if err := ix.reset(ctx, dim); err != nil {
		return fmt.Errorf("vec: make the index %d wide: %w", dim, err)
	}
	return nil
}

// Name is memory.LaneVec.
func (ix *Index) Name() memory.LaneName { return memory.LaneVec }

// Search embeds q.Text and returns the nearest items, best first, with
// Score 1 - cosine distance.
func (ix *Index) Search(ctx context.Context, q memory.Query) ([]memory.Hit, error) {
	if q.Text == "" {
		return nil, nil
	}
	ix.mu.RLock()
	dim := ix.dim
	ix.mu.RUnlock()
	if dim == 0 {
		return nil, nil // nothing indexed yet
	}
	vecs, err := ix.embedder.embed(ctx, []string{q.Text})
	if err != nil {
		return nil, err
	}
	if len(vecs[0]) != dim {
		return nil, fmt.Errorf("vec: the model's vectors are %d wide, the index's %d; rebuild the index",
			len(vecs[0]), dim)
	}
	n := q.Budget.Candidates
	if n <= 0 {
		n = defaultCandidates
	}
	const qSearch = `SELECT item, distance FROM vec_items WHERE embedding MATCH ? AND k = ? ORDER BY distance`
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	if ix.dim != dim {
		return nil, nil // cleared or resized while embedding
	}
	rows, err := ix.db.QueryContext(ctx, qSearch, blob(vecs[0]), n+len(q.Exclude))
	if err != nil {
		return nil, fmt.Errorf("vec: search: %w", err)
	}
	defer rows.Close()
	var hits []memory.Hit
	for rows.Next() {
		var item string
		var distance float64
		if err := rows.Scan(&item, &distance); err != nil {
			return nil, fmt.Errorf("vec: search: %w", err)
		}
		if slices.Contains(q.Exclude, item) || len(hits) == n {
			continue
		}
		hits = append(hits, memory.Hit{Ref: item, Kind: kindOf(item), Score: 1 - distance,
			Lanes: []memory.LaneName{memory.LaneVec}})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("vec: search: %w", err)
	}
	return hits, nil
}

// kindOf is a hit's kind by its ref.
func kindOf(ref string) string {
	kind, _, _ := memory.ParseRef(ref)
	switch kind {
	case memory.RefClaim:
		return "claim"
	case memory.RefMessage:
		return "message"
	case memory.RefEpisode:
		return "episode"
	case memory.RefResource:
		return "resource"
	}
	return ""
}

// Upsert embeds and stores records whose text changed since they were last
// stored. A sensitive record is embedded only by a server on this machine;
// otherwise it is skipped, and dropped if it was stored before.
func (ix *Index) Upsert(ctx context.Context, recs []memory.Record) error {
	var todo []memory.Record
	var drop []string
	for _, r := range recs {
		if r.ID == "" {
			continue
		}
		if r.Text == "" || r.Meta["sensitivity"] == model.SensitivitySensitive && !ix.local {
			drop = append(drop, r.ID)
			continue
		}
		hash, err := ix.storedHash(ctx, r.ID)
		if err != nil {
			return err
		}
		if hash != memory.Hash(r.Text) {
			todo = append(todo, r)
		}
	}
	if len(drop) > 0 {
		if err := ix.Delete(ctx, drop); err != nil {
			return err
		}
	}
	if len(todo) == 0 {
		return nil
	}
	texts := make([]string, len(todo))
	for i, r := range todo {
		texts[i] = r.Text
	}
	vecs, err := ix.embedder.embed(ctx, texts)
	if err != nil {
		return err
	}
	if err := ix.fit(ctx, len(vecs[0])); err != nil {
		return err
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	for _, v := range vecs {
		if len(v) != ix.dim {
			return fmt.Errorf("vec: the model returned vectors %d and %d wide", ix.dim, len(v))
		}
	}
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("vec: upsert: %w", err)
	}
	defer tx.Rollback()
	now := ix.clock.Now().UnixMilli()
	for i, r := range todo {
		if _, err := tx.ExecContext(ctx, `DELETE FROM vec_items WHERE item = ?`, r.ID); err != nil {
			return fmt.Errorf("vec: upsert: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO vec_items (item, embedding) VALUES (?, ?)`, r.ID,
			blob(vecs[i])); err != nil {
			return fmt.Errorf("vec: upsert: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO items (item, hash, at) VALUES (?, ?, ?)`, r.ID,
			memory.Hash(r.Text), now); err != nil {
			return fmt.Errorf("vec: upsert: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("vec: upsert: %w", err)
	}
	return nil
}

// storedHash is the hash of an item's text as stored, "" when it is not.
func (ix *Index) storedHash(ctx context.Context, item string) (string, error) {
	var hash string
	err := ix.db.QueryRowContext(ctx, `SELECT hash FROM items WHERE item = ?`, item).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("vec: read item: %w", err)
	}
	return hash, nil
}

// Delete drops items by ref.
func (ix *Index) Delete(ctx context.Context, refs []string) error {
	if len(refs) == 0 {
		return nil
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("vec: delete: %w", err)
	}
	defer tx.Rollback()
	for _, ref := range refs {
		if ix.dim > 0 {
			if _, err := tx.ExecContext(ctx, `DELETE FROM vec_items WHERE item = ?`, ref); err != nil {
				return fmt.Errorf("vec: delete: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM items WHERE item = ?`, ref); err != nil {
			return fmt.Errorf("vec: delete: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("vec: delete: %w", err)
	}
	return nil
}

// Health embeds a probe: it is nil when the server answers. The first probe
// also sizes the index.
func (ix *Index) Health(ctx context.Context) error {
	vecs, err := ix.embedder.embed(ctx, []string{probeText})
	if err != nil {
		return err
	}
	ix.mu.RLock()
	dim := ix.dim
	ix.mu.RUnlock()
	if dim == 0 {
		return ix.fit(ctx, len(vecs[0]))
	}
	if len(vecs[0]) != dim {
		return fmt.Errorf("vec: the model's vectors are %d wide, the index's %d; rebuild the index", len(vecs[0]), dim)
	}
	return nil
}

// Count is how many items the index holds.
func (ix *Index) Count(ctx context.Context) (int, error) {
	var n int
	if err := ix.db.QueryRowContext(ctx, `SELECT count(*) FROM items`).Scan(&n); err != nil {
		return 0, fmt.Errorf("vec: count: %w", err)
	}
	return n, nil
}

// Refs lists the refs of every item the index holds, in order, so a
// reconcile can drop those whose row is gone.
func (ix *Index) Refs(ctx context.Context) ([]string, error) {
	rows, err := ix.db.QueryContext(ctx, `SELECT item FROM items ORDER BY item`)
	if err != nil {
		return nil, fmt.Errorf("vec: refs: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return nil, fmt.Errorf("vec: refs: %w", err)
		}
		out = append(out, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("vec: refs: %w", err)
	}
	return out, nil
}

// Clear drops every item, keeping the index's width.
func (ix *Index) Clear(ctx context.Context) error {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if err := ix.reset(ctx, ix.dim); err != nil {
		return fmt.Errorf("vec: clear: %w", err)
	}
	return nil
}

// Close closes the index.
func (ix *Index) Close() error { return ix.db.Close() }

// blob is a vector as sqlite-vec reads it: float32s, little-endian.
func blob(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(x))
	}
	return b
}
