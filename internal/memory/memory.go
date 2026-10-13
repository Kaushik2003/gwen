// Package memory is Gwen's memory (gwen-harness/harness.md section 4): what
// she learns about the user from what they say, kept as claims with the
// words they came from, and what she recalls of it for each message.
//
// This package is the contract its parts share: refs, trust, the key
// vocabulary, the read path's query and packet, lanes, and Memory, the
// interface the API uses. The parts are subpackages: learn (the write path,
// the worker, and consolidation), recall (the read path), vec (embeddings
// and the vector index), files (the memory folder), mem0 and supermemory (the
// optional vendor lanes), and service, which puts them together.
package memory

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
)

// ErrDisabled is matched by the errors of a memory that is switched off or
// cannot do what was asked without a model.
var ErrDisabled = errors.New("memory unavailable")

// Memory is what the API asks of Gwen's memory. The daemon has none (nil)
// when memory is switched off.
type Memory interface {
	// Note is the memory note for a user message: what Gwen recalls that
	// bears on it, within the budget, with refs numbered from req.RefStart.
	Note(ctx context.Context, req NoteRequest) (Note, error)
	// Recall runs the read path for a query.
	Recall(ctx context.Context, q Query) (Packet, error)
	// Get returns the claim of key about subject (nil is the user) that held
	// at asOf, or store.ErrNotFound.
	Get(ctx context.Context, key string, subjectID *string, asOf time.Time) (model.Claim, error)
	// Explain says where a ref came from and what became of it.
	Explain(ctx context.Context, ref string) (Explanation, error)
	// Claims lists claims, each with what the dashboard shows beside it.
	Claims(ctx context.Context, f store.ClaimFilter) ([]ClaimView, error)
	// Entities lists the people and things claims are about.
	Entities(ctx context.Context) ([]model.Entity, error)
	// Conflicts lists conflicts in status ("" for every status) with both
	// claims.
	Conflicts(ctx context.Context, status string) ([]ConflictView, error)
	// Profile returns the profile card for day (YYYY-MM-DD), compiling it
	// first when the newest is from an earlier day.
	Profile(ctx context.Context, day string) (model.ProfileCard, error)
	// Observation returns a thread's observation log; ok is false when the
	// thread has none yet.
	Observation(ctx context.Context, threadID string) (o model.ChatObservation, ok bool, err error)
	// Status reports what the memory holds and how its lanes are.
	Status(ctx context.Context) Status

	// ChatTurn tells the memory a turn was stored, so it learns from it in
	// the background. It never blocks on a model.
	ChatTurn(ctx context.Context, t ChatTurn)
	// Remember learns from words the user typed to be remembered: they are
	// read by the model now and committed as the user's own edit.
	Remember(ctx context.Context, req RememberRequest) ([]model.Claim, error)
	// Edit replaces a claim with the user's correction, which supersedes it.
	Edit(ctx context.Context, id string, req EditRequest) (model.Claim, error)
	// Confirm makes a candidate or quarantined claim active, on the user's
	// word.
	Confirm(ctx context.Context, id string) (model.Claim, error)
	// Reject marks a claim not true; it is never learned again from the same
	// words.
	Reject(ctx context.Context, id string) (model.Claim, error)
	// Suppress keeps a claim but stops Gwen from bringing it up (on), or lets
	// her again.
	Suppress(ctx context.Context, id string, on bool) (model.Claim, error)
	// Forget erases a claim everywhere it lives and says where.
	Forget(ctx context.Context, id string) (ForgetReceipt, error)
	// ForgetAll erases everything the memory holds; chats are kept.
	ForgetAll(ctx context.Context) (ForgetReceipt, error)
	// Resolve settles a conflict.
	Resolve(ctx context.Context, conflictID, resolution string) (ConflictView, error)
	// AddResource reads a document the user gave Gwen to remember.
	AddResource(ctx context.Context, req AddResourceRequest) (model.Resource, error)
	// Resources lists the documents.
	Resources(ctx context.Context) ([]model.Resource, error)
	// RemoveResource forgets a document.
	RemoveResource(ctx context.Context, id string) (ForgetReceipt, error)
	// Rebuild rebuilds a derived lane from the claims and episodes.
	Rebuild(ctx context.Context, lane LaneName) error
	// Sleep runs consolidation now instead of waiting for an idle moment.
	Sleep(ctx context.Context) (SleepReport, error)
	// Export returns everything the memory holds, for the user to keep.
	Export(ctx context.Context) (Export, error)
}

// NoteRequest asks for the memory note of a user message.
type NoteRequest struct {
	Text     string // the user's message
	ThreadID string // "" for a new chat
	// Exclude are refs already in the model's context (the messages it sees
	// verbatim), which the note leaves out.
	Exclude []string
	// RefStart numbers the note's refs: m<RefStart>, m<RefStart+1>, ...
	RefStart int
	AsOf     time.Time
	// Profile is the reader: ProfileFrontier or ProfileLocal.
	Profile string
}

// Reader profiles, which set the note's budget (harness 3.3).
const (
	ProfileFrontier = "frontier"
	ProfileLocal    = "local"
)

// Note budgets in tokens, by reader profile.
const (
	NoteTokensFrontier = 1200
	NoteTokensLocal    = 2000
)

// Note is a memory note: Text goes to the model after the message; Refs maps
// each ref in it (m3) to what it names (claim:<id>, msg:<id>, ep:<id>,
// doc:<id>). Packet is what the read path found.
type Note struct {
	Text   string
	Refs   map[string]string
	Packet Packet
}

// ChatTurn is a turn the API stored: the user's message and the reply.
type ChatTurn struct {
	ThreadID string
	User     model.ChatMessage
	Reply    model.ChatMessage
	// Forget are claim ids the reply asked to stop bringing up; they are
	// suppressed at once and offered to the user to forget for good.
	Forget []string
}

// RememberRequest is words the user typed to be remembered.
type RememberRequest struct {
	Text    string
	Channel string // model.ChannelUI or model.ChannelCLI
}

// EditRequest is the user's correction of a claim: Value, for a one-valued
// claim whose value is plain text, or Text, the new statement of any other.
// ValidFrom and ValidTo, when set, are YYYY-MM-DD, "" for an open end.
type EditRequest struct {
	Text      string
	Value     *string
	ValidFrom *string
	ValidTo   *string
}

// AddResourceRequest names a document on this device.
type AddResourceRequest struct {
	Path  string
	Title string // "" takes the file's name
}

// ForgetReceipt says where forgotten memory was erased: each lane and
// whether it confirmed.
type ForgetReceipt struct {
	Claims   int
	Episodes int
	Lanes    []LaneReceipt
}

// LaneReceipt is one lane's part of a ForgetReceipt.
type LaneReceipt struct {
	Lane LaneName
	OK   bool
	Note string // why not, or what remains (such as the chat a fact came from)
}

// ClaimView is a claim with what the dashboard shows beside it.
type ClaimView struct {
	model.Claim
	Subject  string // the subject's name, "" for the user
	Label    string // the key's label, such as "Works at"
	Source   string // where it came from, in words: "chat “Weekend plans”, 3 Oct"
	ThreadID string // the chat it came from, "" when none
	Disputed bool   // it is in an open conflict
}

// ConflictView is a conflict with both claims.
type ConflictView struct {
	model.Conflict
	A, B ClaimView
}

// Explanation is why Gwen holds something: the claim or the words, its
// source, the words around it, and the claims of the same key over time.
type Explanation struct {
	Ref     string
	Claim   *ClaimView
	Quote   string // the verbatim words it rests on
	Context []model.ChatMessage
	Episode *model.Episode
	History []ClaimView
	Links   []model.MemoryLink
}

// Status is what the memory holds and how its lanes are.
type Status struct {
	Enabled   bool
	Learning  bool // a model is set up to learn from chats
	Claims    map[string]int
	Conflicts int
	Jobs      map[string]int
	Lanes     []LaneStatus
	// LastLearned and LastSlept are when extraction and consolidation last
	// finished.
	LastLearned *time.Time
	LastSlept   *time.Time
	Profile     *model.ProfileCard
	Ledger      LedgerStatus
}

// LaneStatus is one lane's health.
type LaneStatus struct {
	Lane    LaneName
	Enabled bool
	OK      bool
	Items   int    // what it holds, when it can tell; -1 when not
	Detail  string // a model id, a URL, or why it is down
}

// LedgerStatus is the ledger's length and whether its chain verifies.
type LedgerStatus struct {
	Rows     int64
	Verified bool
	BadSeq   int64
}

// SleepReport is what one consolidation did.
type SleepReport struct {
	Learned    int // claims committed from extraction
	Merged     int // near-duplicates merged
	Conflicts  int // conflicts found
	Insights   int // inferred claims proposed
	Observed   int // threads whose observation log grew
	Reconciled int // lane rows repaired
	Profile    bool
}

// Export is everything the memory holds.
type Export struct {
	ExportedAt time.Time
	Claims     []model.Claim
	Entities   []model.Entity
	Episodes   []model.Episode
	Conflicts  []model.Conflict
	Resources  []model.Resource
}

// LaneName names a retrieval lane.
type LaneName string

// The lanes.
const (
	LaneClaims      LaneName = "claims"      // claims by key and words
	LaneChats       LaneName = "chats"       // chat messages by words
	LaneEpisodes    LaneName = "episodes"    // episodes by words
	LaneEntity      LaneName = "entity"      // the claims of people named
	LaneTime        LaneName = "time"        // what was said in a time window
	LaneData        LaneName = "data"        // tasks, goals, and time worked, by fixed queries
	LaneVec         LaneName = "vec"         // embeddings in index.db
	LaneMem0        LaneName = "mem0"        // Mem0's recall over verified claims
	LaneSupermemory LaneName = "supermemory" // Supermemory's search over documents
	LaneFiles       LaneName = "files"       // the memory folder (a projection, not searched)
)

// Intents the router reads a query as.
const (
	IntentLookup   = "lookup"   // where do I work, Anna's birthday
	IntentEpisodic = "episodic" // what did we decide on Tuesday
	IntentResource = "resource" // what does the lease say
	IntentPrefer   = "prefer"   // what music do I like
	IntentData     = "data"     // how long did I work last week
	IntentAny      = "any"
)

// TimeRange is a span of time, [From, To).
type TimeRange struct {
	From, To time.Time
}

// RunCtx is who reads: a tainted run, or one that can reach the world, sees
// sensitive items only as refs.
type RunCtx struct {
	Tainted    bool
	WorldReach bool
}

// Budget bounds a recall: the tokens of the packet, the candidates pulled
// before packing, and how long the lanes may take.
type Budget struct {
	Tokens     int
	Candidates int
	Deadline   time.Duration
}

// Query is one recall.
type Query struct {
	Text string
	// AsOf is the instant facts are read at; zero is now.
	AsOf time.Time
	// Window limits episodic lanes to a span; nil reads it from Text.
	Window *TimeRange
	// Intent overrides the router's reading of Text; "" lets it read.
	Intent string
	// Lanes limits the lanes asked; none lets the router pick.
	Lanes []LaneName
	// Exclude are refs to leave out.
	Exclude []string
	Budget  Budget
	Run     RunCtx
}

// Hit is one thing recalled.
type Hit struct {
	Ref     string // claim:<id>, msg:<id>, ep:<id>, doc:<id>, data:<name>
	Kind    string // "claim", "message", "episode", "resource", "data"
	Key     string // a claim's key
	Subject string // a claim's subject's name, "" for the user
	// Text is what to show: a claim's statement, or the words of a message,
	// an episode, or a document, cut to fit.
	Text        string
	Value       json.RawMessage // a claim's value
	At          time.Time       // when it was said, or a claim recorded
	Source      string          // where it came from, in words
	ValidFrom   *time.Time
	ValidTo     *time.Time
	Trust       string
	Sensitivity string
	Disputed    bool
	Score       float64
	Lanes       []LaneName
}

// Packet is what a recall found, packed to its budget. Abstain is set when
// the query asks about the past and nothing was found to answer it; Degraded
// lists the lanes that failed or ran out of time.
type Packet struct {
	Intent   string
	Hits     []Hit
	Tokens   int
	Abstain  bool
	Degraded []LaneName
}

// Lane is one retrieval source.
type Lane interface {
	Name() LaneName
	// Search returns hits for q, best first, at most q.Budget.Candidates.
	Search(ctx context.Context, q Query) ([]Hit, error)
}

// Projection is a lane fed from Gwen's own memory: it holds copies of active
// claims (or resources), can drop them, and can be rebuilt.
type Projection interface {
	Lane
	// Upsert stores records, idempotently on Record.ID.
	Upsert(ctx context.Context, recs []Record) error
	// Delete drops records by Record.ID.
	Delete(ctx context.Context, ids []string) error
	// Health is nil when the lane answers.
	Health(ctx context.Context) error
}

// Record is what a projection holds of a claim or a resource.
type Record struct {
	ID        string // the claim's or resource's id
	Kind      string // "claim" or "resource"
	Key       string
	Text      string
	ValidFrom *time.Time
	At        time.Time
	Meta      map[string]string
}
