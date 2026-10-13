package model

import "time"

// Claim statuses (claims.status). Candidate waits for the user's word (a fact
// only Gwen said, or an inference); quarantined waits for corroboration (it
// came from outside the user's control); superseded was true until a newer
// claim replaced it.
const (
	ClaimCandidate   = "candidate"
	ClaimActive      = "active"
	ClaimQuarantined = "quarantined"
	ClaimRejected    = "rejected"
	ClaimSuperseded  = "superseded"
)

// Trust levels (claims.trust), most trusted first: the user edited it, the
// user said it, Gwen observed it in typed data, Gwen inferred it, or it came
// from outside (email, the web, a document, a vendor's extractor).
const (
	TrustOwnerEdit = "owner_edit"
	TrustOwnerSaid = "owner_said"
	TrustObserved  = "observed"
	TrustInferred  = "inferred"
	TrustExternal  = "external"
)

// Sensitivity levels (claims.sensitivity, episodes.sensitivity). Sensitive
// rows never leave the device: they are not synced or sent to a vendor lane.
// Secrets are never stored at all.
const (
	SensitivityPublic    = "public"
	SensitivityPersonal  = "personal"
	SensitivitySensitive = "sensitive"
)

// Claim cardinalities (claims.cardinality): one value at a time, which a
// newer value supersedes, or many that add up.
const (
	CardinalityOne  = "one"
	CardinalityMany = "many"
)

// Speakers of an episode (episodes.speaker).
const (
	SpeakerUser     = "user"
	SpeakerGwen     = "gwen"
	SpeakerExternal = "external"
)

// Episode kinds (episodes.kind), an open list checked in Go.
const (
	EpisodeNote     = "note"     // typed in the dashboard or the CLI to remember
	EpisodeEdit     = "edit"     // an edit of a memory file or of a claim
	EpisodeResource = "resource" // the readable text of a document
)

// Channels (episodes.channel, claims.channel): where the words came in.
const (
	ChannelChat = "chat"
	ChannelUI   = "ui"
	ChannelCLI  = "cli"
	ChannelFile = "file"
	ChannelDoc  = "document"
	ChannelSync = "sync"
)

// Claim source kinds (claims.source_kind): what source_id names.
const (
	SourceMessage = "msg"     // a chat message
	SourceEpisode = "ep"      // an episode
	SourceClaim   = "claim"   // other claims, for a reflection
	SourceUserUI  = "user"    // an edit in the dashboard, with no episode
	SourceMem0    = "mem0"    // Mem0's shadow extractor, verified against a message
	SourceSuperM  = "smemory" // Supermemory's profile of the user's documents
	SourceData    = "data"    // the user's tracked data, for an insight
)

// Conflict statuses (conflicts.status) and resolutions (conflicts.resolution).
const (
	ConflictOpen      = "open"
	ConflictResolved  = "resolved"
	ConflictDismissed = "dismissed"
	ResolveKeepA      = "keep_a"
	ResolveKeepB      = "keep_b"
	ResolveBoth       = "both"
	ResolveNeither    = "neither"
)

// Self version reasons (self_versions.reason).
const (
	SelfChat   = "chat"
	SelfReset  = "reset"
	SelfImport = "import"
)

// Memory job states (memory_jobs.state).
const (
	JobPending = "pending"
	JobRunning = "running"
	JobDone    = "done"
	JobFailed  = "failed"
)

// Memory job kinds (memory_jobs.kind). Project brings one item (subject
// claim:<id>, msg:<id>, ep:<id>, or doc:<id>) up to date in every lane that
// holds copies: added while live, dropped once gone. The others' subjects are
// a thread id (observe), a day (profile), or "".
const (
	JobExtract = "extract"
	JobProject = "project"
	JobFiles   = "files"
	JobObserve = "observe"
	JobProfile = "profile"
)

// Memory backend statuses (memory_backends.status).
const (
	BackendPending = "pending"
	BackendSynced  = "synced"
	BackendDeleted = "deleted"
	BackendFailed  = "failed"
)

// LedgerMemory is the ledger kind of memory's rows.
const LedgerMemory = "memory"

// Entity is a row of entities: a person, organisation, place, or topic that
// claims are about.
type Entity struct {
	ID      string
	Kind    string
	Name    string
	Aliases []string
	Envelope
}

// Episode is a row of episodes: verbatim evidence that is not chat.
type Episode struct {
	ID          string
	Kind        string
	Channel     string
	Speaker     string
	Title       string
	Body        string
	Source      string // a file path, a URL, or a resource id; "" for none
	At          time.Time
	Sensitivity string
	ContentHash string
	Envelope
}

// Claim is a row of claims. SubjectID nil means the user. Value is JSON.
// ValidFrom and ValidTo nil are open ends; Span is the verbatim words of the
// source that support it.
type Claim struct {
	ID             string
	Key            string
	SubjectID      *string
	Value          []byte
	Text           string
	Cardinality    string
	Status         string
	Trust          string
	Sensitivity    string
	Confidence     float64
	ValidFrom      *time.Time
	ValidTo        *time.Time
	OccurredAt     *time.Time
	MentionedAt    time.Time
	RecordedAt     time.Time
	SupersededAt   *time.Time
	SupersededBy   *string
	Suppressed     bool
	SourceKind     string
	SourceID       string
	Span           string
	SourceDeviceID string
	Channel        string
	Extractor      string
	ContentHash    string
	Envelope
}

// ValidAt reports whether the claim held in the world at t.
func (c Claim) ValidAt(t time.Time) bool {
	if c.ValidFrom != nil && t.Before(*c.ValidFrom) {
		return false
	}
	return c.ValidTo == nil || t.Before(*c.ValidTo)
}

// Conflict is a row of conflicts: two claims that cannot both be true.
type Conflict struct {
	ID          string
	ClaimA      string
	ClaimB      string
	Reason      string
	Sensitivity string
	Status      string
	Resolution  *string
	ResolvedAt  *time.Time
	Envelope
}

// MemoryLink is a row of memory_links.
type MemoryLink struct {
	ID       string
	FromKind string // "claim" or "episode"
	FromID   string
	ToKind   string // "task", "goal", "project", "entity", "thread", "message", "episode", "claim"
	ToID     string
	Relation string // "evidence", "about", "mentions"
	Envelope
}

// SelfVersion is a row of self_versions: how the assistant chose to treat the
// user from then on. Since is when it chose the attitude.
type SelfVersion struct {
	ID       string
	Attitude string
	Note     string
	Since    *time.Time
	Reason   string
	ThreadID *string
	Envelope
}

// ChatObservation is a row of chat_observations: the observation log of a
// thread up to and including the message ThroughID.
type ChatObservation struct {
	ThreadID    string
	ThroughID   string
	ThroughAt   time.Time
	Log         string
	Reflections int
	UpdatedAt   time.Time
}

// ChatNote is a row of chat_notes: the memory note sent with a user message,
// and what its refs (m1, m2, ...) name (claim:<id>, msg:<id>, ep:<id>).
type ChatNote struct {
	MessageID string
	ThreadID  string
	Note      string
	Refs      map[string]string
	CreatedAt time.Time
}

// ProfileCard is a row of profile_cards; Refs maps its refs (p1, p2, ...) to
// claim:<id>.
type ProfileCard struct {
	ID        string
	Day       string
	Text      string
	Refs      map[string]string
	CreatedAt time.Time
}

// MemoryJob is a row of memory_jobs.
type MemoryJob struct {
	ID        int64
	Kind      string
	Subject   string
	Payload   []byte // a JSON object
	State     string
	Attempts  int
	RunAfter  time.Time
	LastError string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// MemoryBackend is a row of memory_backends.
type MemoryBackend struct {
	Backend     string
	ItemID      string
	ExternalID  string
	Status      string
	ContentHash string
	Error       string
	SyncedAt    *time.Time
	UpdatedAt   time.Time
}

// Resource is a row of resources: a document the user gave Gwen.
type Resource struct {
	ID          string
	Title       string
	Path        string
	Mime        string
	Size        int64
	ContentHash string
	EpisodeID   *string
	DocumentAt  *time.Time
	AddedAt     time.Time
	DeletedAt   *time.Time
}

// LedgerEntry is a row of ledger.
type LedgerEntry struct {
	Seq      int64
	RunID    *string
	Kind     string
	Data     []byte // a JSON object
	At       time.Time
	PrevHash string
	Hash     string
}
