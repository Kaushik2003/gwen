package wire

import "encoding/json"

// Claim statuses, trust levels, and sensitivity levels, as model has them.
const (
	ClaimCandidate   = "candidate"
	ClaimActive      = "active"
	ClaimQuarantined = "quarantined"
	ClaimRejected    = "rejected"
	ClaimSuperseded  = "superseded"

	TrustOwnerEdit = "owner_edit"
	TrustOwnerSaid = "owner_said"
	TrustObserved  = "observed"
	TrustInferred  = "inferred"
	TrustExternal  = "external"

	SensitivityPublic    = "public"
	SensitivityPersonal  = "personal"
	SensitivitySensitive = "sensitive"
)

// Conflict resolutions.
const (
	ResolveKeepA   = "keep_a"
	ResolveKeepB   = "keep_b"
	ResolveBoth    = "both"
	ResolveNeither = "neither"
)

// MemoryClaim is something Gwen holds about the user or someone in their
// life. Subject is the person or thing it is about, "" for the user; Label is
// its key in words ("Works at"). ValidFrom and ValidTo bound when it was true,
// nil for open. Source says where it came from in words; SourceRef is a ref
// (msg:<id>, ep:<id>) and ThreadID the chat it came from, when it did. Quote
// is the words it rests on. Disputed is set while it is in an open conflict.
type MemoryClaim struct {
	ID           string          `json:"id"`
	Key          string          `json:"key"`
	Label        string          `json:"label"`
	Subject      string          `json:"subject"`
	SubjectID    *string         `json:"subject_id"`
	Text         string          `json:"text"`
	Value        json.RawMessage `json:"value" ts_type:"any"`
	Status       string          `json:"status"`
	Trust        string          `json:"trust"`
	Sensitivity  string          `json:"sensitivity"`
	Confidence   float64         `json:"confidence"`
	ValidFrom    *int64          `json:"valid_from"`
	ValidTo      *int64          `json:"valid_to"`
	MentionedAt  int64           `json:"mentioned_at"`
	RecordedAt   int64           `json:"recorded_at"`
	SupersededBy *string         `json:"superseded_by"`
	Hidden       bool            `json:"hidden"`
	Source       string          `json:"source"`
	SourceRef    string          `json:"source_ref"`
	ThreadID     string          `json:"thread_id"`
	Quote        string          `json:"quote"`
	Disputed     bool            `json:"disputed"`
}

// MemoryClaimList is GET /v1/memory/claims.
type MemoryClaimList struct {
	Claims []MemoryClaim `json:"claims"`
}

// MemoryClaimQuery is the query of GET /v1/memory/claims: Status is a status
// or "all" (default active), Key a key prefix, Subject an entity id or "user",
// Q words to search for. Hidden includes the claims she was asked not to
// bring up.
type MemoryClaimQuery struct {
	Status  string `json:"status"`
	Key     string `json:"key"`
	Subject string `json:"subject"`
	Q       string `json:"q"`
	Hidden  bool   `json:"hidden"`
	Limit   int    `json:"limit"`
	Offset  int    `json:"offset"`
}

// MemoryRef is a ref resolved for showing. Gone is set when what it named was
// forgotten or erased since; Text is then "". Status is a claim's status
// (a candidate is shown as "to confirm"), "" for anything else.
type MemoryRef struct {
	Ref      string `json:"ref"`
	Kind     string `json:"kind"`
	Text     string `json:"text"`
	Source   string `json:"source"`
	ThreadID string `json:"thread_id"`
	Gone     bool   `json:"gone"`
	Status   string `json:"status,omitempty"`
}

// MemoryEpisode is evidence that is not chat: a note, a file edit, a
// document's text.
type MemoryEpisode struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Channel     string `json:"channel"`
	Speaker     string `json:"speaker"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	Source      string `json:"source"`
	At          int64  `json:"at"`
	Sensitivity string `json:"sensitivity"`
}

// MemoryLink is what a claim is about or rests on besides its source.
type MemoryLink struct {
	ToKind   string `json:"to_kind"`
	ToID     string `json:"to_id"`
	Relation string `json:"relation"`
}

// MemoryExplanation is GET /v1/memory/explain?ref=: why Gwen holds
// something. Context is the chat messages around the source; History is the
// claims of the same key about the same subject, newest first.
type MemoryExplanation struct {
	Ref     string             `json:"ref"`
	Claim   *MemoryClaim       `json:"claim"`
	Quote   string             `json:"quote"`
	Context []AssistantMessage `json:"context"`
	Episode *MemoryEpisode     `json:"episode"`
	History []MemoryClaim      `json:"history"`
	Links   []MemoryLink       `json:"links"`
}

// MemoryConflict is two claims that cannot both be true.
type MemoryConflict struct {
	ID         string      `json:"id"`
	Reason     string      `json:"reason"`
	Status     string      `json:"status"`
	Resolution *string     `json:"resolution"`
	A          MemoryClaim `json:"a"`
	B          MemoryClaim `json:"b"`
	CreatedAt  int64       `json:"created_at"`
}

// MemoryConflictList is GET /v1/memory/conflicts.
type MemoryConflictList struct {
	Conflicts []MemoryConflict `json:"conflicts"`
}

// ResolveConflictRequest is the POST /v1/memory/conflicts/{id}/resolve body.
type ResolveConflictRequest struct {
	Resolution string `json:"resolution"`
}

// RememberRequest is the POST /v1/memory/remember body: words to remember.
type RememberRequest struct {
	Text string `json:"text"`
}

// MemoryClaimsResult is what remembering or editing made.
type MemoryClaimsResult struct {
	Claims []MemoryClaim `json:"claims"`
}

// EditClaimRequest is the PATCH /v1/memory/claims/{id} body: the corrected
// value of a one-valued claim whose value is plain text (Value), or the
// corrected statement of any other (Text); and when it became and stopped
// being true (YYYY-MM-DD, "" to leave open), nil to keep.
type EditClaimRequest struct {
	Text      string  `json:"text"`
	Value     *string `json:"value"`
	ValidFrom *string `json:"valid_from"`
	ValidTo   *string `json:"valid_to"`
}

// HideClaimRequest is the POST /v1/memory/claims/{id}/hide body.
type HideClaimRequest struct {
	Hidden bool `json:"hidden"`
}

// ForgetReceipt says where forgotten memory was erased.
type ForgetReceipt struct {
	Claims   int           `json:"claims"`
	Episodes int           `json:"episodes"`
	Lanes    []LaneReceipt `json:"lanes"`
}

// LaneReceipt is one lane's part of a ForgetReceipt.
type LaneReceipt struct {
	Lane string `json:"lane"`
	OK   bool   `json:"ok"`
	Note string `json:"note"`
}

// RecallRequest is the POST /v1/memory/recall body. AsOf is YYYY-MM-DD, ""
// for now; Lanes limits the lanes asked.
type RecallRequest struct {
	Query string   `json:"query"`
	AsOf  string   `json:"as_of"`
	Lanes []string `json:"lanes"`
}

// RecallResult is what a recall found.
type RecallResult struct {
	Intent   string      `json:"intent"`
	Hits     []MemoryHit `json:"hits"`
	Tokens   int         `json:"tokens"`
	Abstain  bool        `json:"abstain"`
	Degraded []string    `json:"degraded"`
}

// MemoryHit is one thing recalled.
type MemoryHit struct {
	Ref         string   `json:"ref"`
	Kind        string   `json:"kind"`
	Key         string   `json:"key"`
	Subject     string   `json:"subject"`
	Text        string   `json:"text"`
	Source      string   `json:"source"`
	At          int64    `json:"at"`
	ValidFrom   *int64   `json:"valid_from"`
	ValidTo     *int64   `json:"valid_to"`
	Trust       string   `json:"trust"`
	Sensitivity string   `json:"sensitivity"`
	Disputed    bool     `json:"disputed"`
	Score       float64  `json:"score"`
	Lanes       []string `json:"lanes"`
}

// MemoryStatus is GET /v1/memory/status.
type MemoryStatus struct {
	Enabled       bool           `json:"enabled"`
	Learning      bool           `json:"learning"`
	Claims        map[string]int `json:"claims"`
	Conflicts     int            `json:"conflicts"`
	Jobs          map[string]int `json:"jobs"`
	Lanes         []MemoryLane   `json:"lanes"`
	LastLearnedAt *int64         `json:"last_learned_at"`
	LastSleptAt   *int64         `json:"last_slept_at"`
	Profile       *ProfileCard   `json:"profile"`
	LedgerRows    int64          `json:"ledger_rows"`
	LedgerOK      bool           `json:"ledger_ok"`
}

// MemoryLane is one lane's health. Items is what it holds, -1 when it cannot
// tell.
type MemoryLane struct {
	Lane    string `json:"lane"`
	Enabled bool   `json:"enabled"`
	OK      bool   `json:"ok"`
	Items   int    `json:"items"`
	Detail  string `json:"detail"`
}

// ProfileCard is what Gwen keeps in mind about the user every day.
type ProfileCard struct {
	Day       string `json:"day"`
	Text      string `json:"text"`
	CreatedAt int64  `json:"created_at"`
}

// MemoryEntity is a person, organisation, place, or topic claims are about.
type MemoryEntity struct {
	ID      string   `json:"id"`
	Kind    string   `json:"kind"`
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
}

// MemoryEntityList is GET /v1/memory/entities.
type MemoryEntityList struct {
	Entities []MemoryEntity `json:"entities"`
}

// MemoryResource is a document the user gave Gwen to remember.
type MemoryResource struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Path       string `json:"path"`
	Mime       string `json:"mime"`
	Size       int64  `json:"size"`
	AddedAt    int64  `json:"added_at"`
	DocumentAt *int64 `json:"document_at"`
}

// MemoryResourceList is GET /v1/memory/resources.
type MemoryResourceList struct {
	Resources []MemoryResource `json:"resources"`
}

// AddResourceRequest is the POST /v1/memory/resources body: a file on the
// daemon's device.
type AddResourceRequest struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}

// RebuildRequest is the POST /v1/memory/rebuild body: the lane to rebuild.
type RebuildRequest struct {
	Lane string `json:"lane"`
}

// SleepReport is what one consolidation did.
type SleepReport struct {
	Learned    int  `json:"learned"`
	Merged     int  `json:"merged"`
	Conflicts  int  `json:"conflicts"`
	Insights   int  `json:"insights"`
	Observed   int  `json:"observed"`
	Reconciled int  `json:"reconciled"`
	Profile    bool `json:"profile"`
}

// MemoryExport is GET /v1/memory/export: everything memory holds.
type MemoryExport struct {
	ExportedAt int64            `json:"exported_at"`
	Claims     []MemoryClaim    `json:"claims"`
	Entities   []MemoryEntity   `json:"entities"`
	Episodes   []MemoryEpisode  `json:"episodes"`
	Conflicts  []MemoryConflict `json:"conflicts"`
	Resources  []MemoryResource `json:"resources"`
}

// SelfVersion is one version of how the assistant chose to treat the user.
type SelfVersion struct {
	ID       string  `json:"id"`
	Attitude string  `json:"attitude"`
	Note     string  `json:"note"`
	Since    *int64  `json:"since"`
	Reason   string  `json:"reason"`
	ThreadID *string `json:"thread_id"`
	At       int64   `json:"at"`
}

// SelfHistory is GET /v1/assistant/self/history, newest first.
type SelfHistory struct {
	Versions []SelfVersion `json:"versions"`
}
