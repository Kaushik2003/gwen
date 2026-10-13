package llm

import (
	"context"
	"encoding/json"
)

// MemoryPromptVersion names the memory prompts, recorded with every claim
// they produce as part of its extractor.
const MemoryPromptVersion = "m1"

// Memory limits.
const (
	MaxExtractItems   = 60   // items one extraction reads
	MaxExtractFacts   = 40   // facts one extraction returns
	MaxVerifyChecks   = 40   // checks one verification makes
	MaxObservations   = 30   // observations one Observer call adds
	MaxObservationLen = 300  // characters of one observation
	MaxObservationLog = 9000 // characters a Reflector condenses the log to
	MaxContradictions = 20   // conflicts one scan returns
	MaxInsights       = 5    // insights one call proposes
)

// Memory is the jobs Gwen's memory gives a model. None talks to the user, so
// none opens with the persona.
type Memory interface {
	// Extract reads items and returns the facts they state about the user
	// and the people and things in their life.
	Extract(ctx context.Context, req ExtractRequest) (ExtractOutput, error)
	// Verify judges whether quoted words support each claim.
	Verify(ctx context.Context, req VerifyRequest) (VerifyOutput, error)
	// Observe writes dated observations of messages a chat will no longer
	// show verbatim.
	Observe(ctx context.Context, req ObserveRequest) (ObserveOutput, error)
	// Reflect condenses an observation log.
	Reflect(ctx context.Context, req ReflectRequest) (ReflectOutput, error)
	// Contradict finds claims that cannot both be true.
	Contradict(ctx context.Context, req ContradictRequest) (ContradictOutput, error)
	// Insight proposes what the user's own data suggests about them.
	Insight(ctx context.Context, req InsightRequest) (InsightOutput, error)
	// Name is the provider and model, such as "claude_code/claude-sonnet-5-5".
	Name() string
}

// ExtractRequest is what an extraction reads. Items are numbered i1, i2, ...;
// Context items (c1, c2, ...) come before them and are only for
// understanding. Known claims (k1, ...) may be updated by a fact.
type ExtractRequest struct {
	Today    string          `json:"today"`    // YYYY-MM-DD
	Now      string          `json:"now"`      // RFC 3339, with the zone
	Timezone string          `json:"timezone"` // IANA name
	Keys     []ExtractKey    `json:"keys"`
	Entities []ExtractEntity `json:"entities"`
	Known    []ExtractKnown  `json:"known"`
	Context  []ExtractItem   `json:"context"`
	Items    []ExtractItem   `json:"items"`
	// Explicit is set when the user typed the items to be remembered: every
	// fact in them is meant.
	Explicit bool `json:"explicit"`
}

// ExtractKey is a key of the vocabulary.
type ExtractKey struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Cardinality string `json:"cardinality"` // "one" or "many"
	About       string `json:"about"`       // "user" or "entity"
}

// ExtractEntity is a known person, organisation, place, or topic, named by
// its ref (e1, e2, ...).
type ExtractEntity struct {
	Ref     string   `json:"ref"`
	Kind    string   `json:"kind"`
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
}

// ExtractKnown is an active claim a new fact may update.
type ExtractKnown struct {
	Ref     string `json:"ref"`
	Key     string `json:"key"`
	Subject string `json:"subject"` // "user" or an entity ref
	Text    string `json:"text"`
	Since   string `json:"since"` // YYYY-MM-DD or ""
}

// ExtractItem is one message or note. Speaker is "user", "gwen" (the
// assistant), or "document".
type ExtractItem struct {
	Ref     string `json:"ref"`
	Speaker string `json:"speaker"`
	At      string `json:"at"` // YYYY-MM-DD HH:MM, local
	Text    string `json:"text"`
}

// ExtractOutput is an extraction's facts.
type ExtractOutput struct {
	Facts []ExtractedFact `json:"facts"`
}

// ExtractedFact is one fact. Subject is "user", a known entity's ref, or a
// new entity as "<kind>:<name>" (person:Anna). Quote is the item's words that
// state it, copied exactly; Source is that item's ref. Dates are YYYY-MM-DD,
// resolved against the item's day ("last Monday" becomes a date), or "".
// Sensitivity is "public", "personal", "sensitive", or "secret" (passwords,
// card and account numbers, government ids: never kept). Updates is the ref
// of a known claim it replaces, or "".
type ExtractedFact struct {
	Key         string          `json:"key"`
	Subject     string          `json:"subject"`
	Value       json.RawMessage `json:"value"`
	Text        string          `json:"text"`
	Quote       string          `json:"quote"`
	Source      string          `json:"source"`
	ValidFrom   string          `json:"valid_from"`
	ValidTo     string          `json:"valid_to"`
	OccurredAt  string          `json:"occurred_at"`
	Sensitivity string          `json:"sensitivity"`
	Confidence  float64         `json:"confidence"`
	Updates     string          `json:"updates"`
}

// VerifyRequest is claims to check against their quoted words, each with the
// words around the quote.
type VerifyRequest struct {
	Checks []VerifyCheck `json:"checks"`
}

// VerifyCheck is one claim to check, named by its ref (v1, v2, ...). When
// Quote is "", the verifier must find the words in Context that state the
// claim, or answer no.
type VerifyCheck struct {
	Ref     string `json:"ref"`
	Claim   string `json:"claim"`
	Quote   string `json:"quote"`
	Speaker string `json:"speaker"`
	Context string `json:"context"`
}

// VerifyOutput is one verdict per check.
type VerifyOutput struct {
	Verdicts []Verdict `json:"verdicts"`
}

// Verdict says whether the words support a claim: "yes", "partly", or "no".
// Quote is the supporting words when the check had none.
type Verdict struct {
	Ref       string `json:"ref"`
	Supported string `json:"supported"`
	Quote     string `json:"quote"`
}

// ObserveRequest is a chat's observation log so far and the messages to fold
// into it.
type ObserveRequest struct {
	Today    string           `json:"today"`
	Log      string           `json:"log"`
	Messages []ObserveMessage `json:"messages"`
}

// ObserveMessage is a message to observe; Role is "user" or "assistant".
type ObserveMessage struct {
	Role string `json:"role"`
	At   string `json:"at"` // YYYY-MM-DD HH:MM, local
	Text string `json:"text"`
}

// ObserveOutput is the new observations, oldest first.
type ObserveOutput struct {
	Observations []Observation `json:"observations"`
}

// Observation is a dated note with a priority: "high" (decisions,
// commitments, facts about the user), "medium", or "low".
type Observation struct {
	Date     string `json:"date"`
	Priority string `json:"priority"`
	Text     string `json:"text"`
}

// ReflectRequest is an observation log to condense.
type ReflectRequest struct {
	Today string `json:"today"`
	Log   string `json:"log"`
}

// ReflectOutput is the condensed log, in the same line format.
type ReflectOutput struct {
	Log string `json:"log"`
}

// ContradictRequest is claims to compare, named by refs (k1, k2, ...).
type ContradictRequest struct {
	Today  string           `json:"today"`
	Claims []ContradictItem `json:"claims"`
}

// ContradictItem is one claim to compare.
type ContradictItem struct {
	Ref     string `json:"ref"`
	Key     string `json:"key"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	Since   string `json:"since"`
	Until   string `json:"until"`
}

// ContradictOutput is the pairs that cannot both be true now.
type ContradictOutput struct {
	Conflicts []Contradiction `json:"conflicts"`
}

// Contradiction is two claims that cannot both hold, and why.
type Contradiction struct {
	A      string `json:"a"`
	B      string `json:"b"`
	Reason string `json:"reason"`
}

// InsightRequest is what Gwen knows and the user's recent data.
type InsightRequest struct {
	Today string   `json:"today"`
	Known []string `json:"known"` // what she already holds, one statement each
	// Data is summaries of the user's own tracked data: days worked against
	// target, tasks done, goals' pace, energy check-ins, plans rolled over.
	Data []string `json:"data"`
}

// InsightOutput is the insights proposed; the user confirms each.
type InsightOutput struct {
	Insights []Insight `json:"insights"`
}

// Insight is one inferred fact, the data it rests on, and how sure the model
// is (0 to 1).
type Insight struct {
	Key        string          `json:"key"`
	Value      json.RawMessage `json:"value"`
	Text       string          `json:"text"`
	Basis      string          `json:"basis"`
	Confidence float64         `json:"confidence"`
}
