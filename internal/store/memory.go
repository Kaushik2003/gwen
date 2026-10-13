package store

import (
	"context"
	"time"

	"github.com/kzark/gwen/internal/model"
)

// memoryVersion is the schema version that added the memory tables.
const memoryVersion = 10

// MemoryRepo keeps Gwen's memory (00010_memory.sql): episodes, claims,
// entities, conflicts, links, and the ledger of every change to them. It
// stores and reads; what to store is decided by internal/memory. Every write
// that changes claims goes through Apply, in one transaction with its ledger
// rows.
type MemoryRepo interface {
	// AddEpisode inserts an episode, setting its id, content_hash, and
	// envelope, and returns it as stored.
	AddEpisode(ctx context.Context, e model.Episode) (model.Episode, error)
	// Episode returns a live episode.
	Episode(ctx context.Context, id string) (model.Episode, error)
	// EpisodesAfter returns live episodes inserted on this device after
	// rowid, oldest first, at most limit.
	EpisodesAfter(ctx context.Context, rowid int64, limit int) ([]EpisodeRow, error)
	// MessagesAfter returns live chat messages of live threads inserted on
	// this device after rowid, oldest first, at most limit.
	MessagesAfter(ctx context.Context, rowid int64, limit int) ([]MessageRow, error)
	// MessagesBefore returns up to n live messages of a thread said before
	// at, oldest first: the context of a batch.
	MessagesBefore(ctx context.Context, threadID string, at time.Time, n int) ([]model.ChatMessage, error)
	// Message returns a live message of a live thread, with its thread's
	// title.
	Message(ctx context.Context, id string) (model.ChatHit, error)
	// SearchEpisodes returns up to limit live episodes that share words with
	// text, the best match first.
	SearchEpisodes(ctx context.Context, text string, limit int) ([]EpisodeHit, error)
	// EpisodesBetween returns up to limit live episodes said in [from, to),
	// newest first.
	EpisodesBetween(ctx context.Context, from, to time.Time, limit int) ([]model.Episode, error)
	// MessagesBetween returns up to limit live messages of live threads said
	// in [from, to), newest first.
	MessagesBetween(ctx context.Context, from, to time.Time, limit int) ([]model.ChatHit, error)

	// Claim returns a claim in any status; a forgotten one is ErrNotFound.
	Claim(ctx context.Context, id string) (model.Claim, error)
	// Claims returns the claims f selects, newest recorded first.
	Claims(ctx context.Context, f ClaimFilter) ([]model.Claim, error)
	// ClaimsByKey returns the live claims of key about subject (nil is the
	// user) in the given statuses (all when none), newest valid_from first.
	ClaimsByKey(ctx context.Context, key string, subjectID *string, statuses ...string) ([]model.Claim, error)
	// ClaimsByHash returns every claim with content_hash, forgotten ones
	// included, so a fact the user rejected or forgot is not learned again.
	ClaimsByHash(ctx context.Context, hash string) ([]model.Claim, error)
	// ClaimsFrom returns the live claims whose source is sourceKind and
	// sourceID.
	ClaimsFrom(ctx context.Context, sourceKind, sourceID string) ([]model.Claim, error)
	// SearchClaims returns up to limit live claims, in any status, that share
	// words with text, the best match first.
	SearchClaims(ctx context.Context, text string, limit int) ([]ClaimHit, error)
	// CountClaims counts live claims by status.
	CountClaims(ctx context.Context) (map[string]int, error)

	// Entity returns a live entity.
	Entity(ctx context.Context, id string) (model.Entity, error)
	// Entities returns live entities of kind ("" for every kind), by name.
	Entities(ctx context.Context, kind string) ([]model.Entity, error)
	// FindEntities returns live entities whose name or an alias equals name,
	// ignoring case.
	FindEntities(ctx context.Context, name string) ([]model.Entity, error)

	// Conflict returns a conflict.
	Conflict(ctx context.Context, id string) (model.Conflict, error)
	// Conflicts returns live conflicts in status ("" for every status),
	// newest first.
	Conflicts(ctx context.Context, status string) ([]model.Conflict, error)
	// OpenConflictsOf returns the open conflicts a claim is in.
	OpenConflictsOf(ctx context.Context, claimID string) ([]model.Conflict, error)

	// LinksFrom returns the live links of a claim or an episode.
	LinksFrom(ctx context.Context, fromID string) ([]model.MemoryLink, error)
	// LinksTo returns the live links to a row.
	LinksTo(ctx context.Context, toKind, toID string) ([]model.MemoryLink, error)

	// Apply makes a change in one transaction: entities, claims, claim
	// updates, conflicts, links, then ledger rows and jobs. A claim update
	// whose precondition fails is ErrConflict and nothing is written.
	Apply(ctx context.Context, ch MemoryChange) error
	// Ledger returns ledger rows after seq, oldest first, at most limit.
	Ledger(ctx context.Context, after int64, limit int) ([]model.LedgerEntry, error)
	// VerifyLedger checks the hash chain and returns the seq of the first row
	// that does not verify, or 0 when all do.
	VerifyLedger(ctx context.Context) (int64, error)

	// EraseEpisode forgets an episode: its words are erased and it stays a
	// tombstone. Claims resting on it are not touched.
	EraseEpisode(ctx context.Context, id string, ledger LedgerItem) error
	// EraseAll forgets everything memory holds: every claim and episode is
	// erased to a tombstone, entities, conflicts, and links are deleted,
	// notes, observation logs, profile cards, jobs, backends, usage, and
	// resources are removed, and one ledger row records it. Chats are kept.
	EraseAll(ctx context.Context, ledger LedgerItem) (Erased, error)
}

// EpisodeRow is an episode with its rowid on this device.
type EpisodeRow struct {
	Rowid int64
	model.Episode
}

// MessageRow is a chat message with its rowid on this device and its
// thread's title.
type MessageRow struct {
	Rowid       int64
	ThreadTitle string
	model.ChatMessage
}

// EpisodeHit is an episode that matched a search; Rank is FTS5's, lower is
// better.
type EpisodeHit struct {
	model.Episode
	Rank float64
}

// ClaimHit is a claim that matched a search; Rank is FTS5's, lower is better.
type ClaimHit struct {
	model.Claim
	Rank float64
}

// ClaimFilter selects claims. Zero values select everything live.
type ClaimFilter struct {
	Statuses  []string // none: every status
	KeyPrefix string   // "user." selects user.employer and user.city
	// Subject selects claims about one subject when SubjectSet: nil is the
	// user.
	Subject           *string
	SubjectSet        bool
	IncludeSuppressed bool
	// ExcludeSensitive leaves out sensitive claims.
	ExcludeSensitive bool
	// ValidAt, when set, keeps claims valid in the world at that instant.
	ValidAt *time.Time
	Limit   int // 0 is no limit
	Offset  int
}

// MemoryChange is what one write of memory changes, applied in order by
// Apply. Inserted rows get their envelope from the store; ids are the
// caller's (model.NewID).
type MemoryChange struct {
	// Entities are inserted, or, when the id exists, renamed and given the
	// aliases.
	Entities  []model.Entity
	Claims    []model.Claim
	Updates   []ClaimUpdate
	Conflicts []model.Conflict
	// ConflictUpdates resolve or dismiss conflicts.
	ConflictUpdates []ConflictUpdate
	// Links are inserted; one that is already live is kept.
	Links  []model.MemoryLink
	Ledger []LedgerItem
	Jobs   []NewJob
}

// ClaimUpdate changes a claim. WasStatus, when not "", must be the claim's
// status now, or the whole change fails with ErrConflict. A nil field is not
// changed.
type ClaimUpdate struct {
	ID        string
	WasStatus string
	Status    *string
	// SupersededBy sets status superseded, superseded_by, and superseded_at
	// (now).
	SupersededBy *string
	ValidTo      Nullable[time.Time]
	Suppressed   *bool
	Confidence   *float64
	// Trust raises or lowers how far the claim is trusted, as when the user
	// confirms it.
	Trust *string
	// Forget makes the claim a tombstone: deleted_at is set and its text,
	// value, and span are erased. Its content_hash stays.
	Forget bool
}

// ConflictUpdate resolves (Resolution set) or dismisses a conflict.
type ConflictUpdate struct {
	ID         string
	Status     string
	Resolution *string
}

// LedgerItem is a ledger row to append: kind (model.LedgerMemory when ""),
// and data, marshalled as a JSON object holding ids and content hashes, never
// the words.
type LedgerItem struct {
	Kind  string
	RunID *string
	Data  any
}

// NewJob is a memory job to enqueue. A pending or running job of the same
// kind and subject is kept instead.
type NewJob struct {
	Kind     string
	Subject  string
	Payload  any // marshalled as a JSON object; nil is {}
	RunAfter time.Time
}

// Erased counts what EraseAll forgot.
type Erased struct {
	Claims, Episodes, Entities, Resources int
	Conflicts, Links                      int
}

// SelfRepo keeps self_versions, how the assistant chose to treat the user.
type SelfRepo interface {
	// Current returns the newest live version, or ErrNotFound before the
	// assistant chose anything.
	Current(ctx context.Context) (model.SelfVersion, error)
	// Add appends a version, setting its id and envelope.
	Add(ctx context.Context, v model.SelfVersion) (model.SelfVersion, error)
	// History returns up to limit live versions, newest first.
	History(ctx context.Context, limit int) ([]model.SelfVersion, error)
}

// MemoryStateRepo keeps memory's device-local tables: observation logs,
// chat notes, profile cards, jobs, lane backends, usage, and resources.
type MemoryStateRepo interface {
	// Observation returns a thread's observation log, or ErrNotFound.
	Observation(ctx context.Context, threadID string) (model.ChatObservation, error)
	// SetObservation inserts or replaces a thread's observation log.
	SetObservation(ctx context.Context, o model.ChatObservation) error

	// AddNote stores the memory note sent with a user message.
	AddNote(ctx context.Context, n model.ChatNote) error
	// Notes returns a thread's notes by message id.
	Notes(ctx context.Context, threadID string) (map[string]model.ChatNote, error)

	// LatestProfile returns the newest profile card, or ErrNotFound.
	LatestProfile(ctx context.Context) (model.ProfileCard, error)
	// AddProfile stores a profile card, keeping the newest 30.
	AddProfile(ctx context.Context, c model.ProfileCard) (model.ProfileCard, error)

	// Enqueue adds jobs as Apply does, outside a change.
	Enqueue(ctx context.Context, jobs ...NewJob) error
	// NextJob marks the oldest pending job of one of kinds (any kind when
	// none) whose run_after has come running, and returns it, or
	// ErrNotFound when none is due.
	NextJob(ctx context.Context, now time.Time, kinds ...string) (model.MemoryJob, error)
	// FinishJob marks a running job done.
	FinishJob(ctx context.Context, id int64) error
	// FailJob records a running job's error: pending again at retryAt, or
	// failed for good when retryAt is nil.
	FailJob(ctx context.Context, id int64, msg string, retryAt *time.Time) error
	// RequeueRunning makes every running job pending again, at startup.
	RequeueRunning(ctx context.Context) (int, error)
	// PruneJobs deletes done jobs updated before before.
	PruneJobs(ctx context.Context, before time.Time) (int, error)
	// JobCounts counts jobs by state.
	JobCounts(ctx context.Context) (map[string]int, error)

	// Backend returns where an item lives in a lane, or ErrNotFound.
	Backend(ctx context.Context, backend, itemID string) (model.MemoryBackend, error)
	// SetBackend inserts or replaces a backend row.
	SetBackend(ctx context.Context, b model.MemoryBackend) error
	// Backends returns a lane's rows in status ("" for every status).
	Backends(ctx context.Context, backend, status string) ([]model.MemoryBackend, error)
	// BackendByExternal returns the row of a lane's external id, or
	// ErrNotFound.
	BackendByExternal(ctx context.Context, backend, externalID string) (model.MemoryBackend, error)

	// TouchClaims counts one use of each claim at at.
	TouchClaims(ctx context.Context, ids []string, at time.Time) error
	// Usage returns the uses of claims by id; a claim never used is absent.
	Usage(ctx context.Context, ids []string) (map[string]ClaimUsage, error)

	// AddResource stores a resource and the episode holding its text in one
	// transaction, setting the episode's id and envelope and the resource's
	// episode_id.
	AddResource(ctx context.Context, r model.Resource, text model.Episode) (model.Resource, error)
	// Resources returns the live resources, newest first.
	Resources(ctx context.Context) ([]model.Resource, error)
	// Resource returns a live resource.
	Resource(ctx context.Context, id string) (model.Resource, error)
	// RemoveResource deletes a resource and erases its episode.
	RemoveResource(ctx context.Context, id string, ledger LedgerItem) error
}

// ClaimUsage is how often and when a claim was last used.
type ClaimUsage struct {
	Uses       int
	LastUsedAt time.Time
}
