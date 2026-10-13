package memory

import (
	"strings"

	"github.com/kzark/gwen/internal/model"
)

// Ref kinds: a ref is "<kind>:<id>", and never goes dead (harness 2.5).
const (
	RefClaim    = "claim"
	RefMessage  = "msg"
	RefEpisode  = "ep"
	RefResource = "doc"
	RefData     = "data"
)

// Ref makes a ref.
func Ref(kind, id string) string { return kind + ":" + id }

// ParseRef splits a ref into its kind and id; ok is false for anything else.
// A document's ref may name a part after a #, as in doc:<id>#c12; id is then
// the document's.
func ParseRef(ref string) (kind, id string, ok bool) {
	kind, id, ok = strings.Cut(ref, ":")
	if !ok || id == "" {
		return "", "", false
	}
	switch kind {
	case RefClaim, RefMessage, RefEpisode:
		return kind, id, len(id) == 36
	case RefResource:
		id, _, _ = strings.Cut(id, "#")
		return kind, id, len(id) == 36
	case RefData:
		return kind, id, true
	}
	return "", "", false
}

// Tier is a trust level's rank, 0 for the user's own edit to 4 for content
// from outside; -1 for an unknown level.
func Tier(trust string) int {
	switch trust {
	case model.TrustOwnerEdit:
		return 0
	case model.TrustOwnerSaid:
		return 1
	case model.TrustObserved:
		return 2
	case model.TrustInferred:
		return 3
	case model.TrustExternal:
		return 4
	}
	return -1
}

// ValidTrust reports whether trust is a trust level.
func ValidTrust(trust string) bool { return Tier(trust) >= 0 }

// SensitivityRank orders sensitivity levels: public 0, personal 1,
// sensitive 2; -1 for anything else.
func SensitivityRank(s string) int {
	switch s {
	case model.SensitivityPublic:
		return 0
	case model.SensitivityPersonal:
		return 1
	case model.SensitivitySensitive:
		return 2
	}
	return -1
}

// MoreSensitive returns the more sensitive of two levels.
func MoreSensitive(a, b string) string {
	if SensitivityRank(b) > SensitivityRank(a) {
		return b
	}
	return a
}
