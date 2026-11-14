package connect

import "slices"

// Decision is what the policy says about one capability call.
type Decision string

// Decisions.
const (
	Allow  Decision = "allow"  // go ahead
	Ask    Decision = "ask"    // wait for your approval
	Refuse Decision = "refuse" // never, in this phase
)

// Grant relaxes or tightens a capability for one scope: a subject ref, a
// folder, project:<id>, or chat:<id> (W32). Pattern, when not "", narrows it
// to calls with exactly that pattern (a command's prefix). Decision is
// "allow" or "ask".
type Grant struct {
	Capability string
	Pattern    string
	Scope      string
	Decision   Decision
}

// Call is one capability call as the policy sees it.
type Call struct {
	Capability string
	// Pattern is the call's pattern, matched against a grant's by exact
	// equality; "" for a run's capability call.
	Pattern string
	Risk    Risk
	// Scopes are what the call is about: the run's subject and its parents,
	// and the folder it works in. A grant for any of them applies.
	Scopes []string
	Grants []Grant
	// Approved: this run already has your approval for a write, which
	// covers its other writes.
	Approved bool
}

// Decide is the permission policy (plan.md 3.7): read and local are
// allowed, write asks once per run, external asks every time and can never
// be granted, and destructive is not offered. A grant can allow read,
// local, and write for its scope, or make them ask once per run.
//
// Every grant matching the call (its capability, one of its scopes, and its
// pattern exactly) counts, in no particular order, and the strictest wins:
// any ask makes it ask, so an allow for a folder never overrides an ask for
// the subject.
func Decide(c Call) Decision {
	switch c.Risk {
	case External:
		return Ask
	case Read, Local, Write:
	default:
		return Refuse
	}
	decision := Allow
	if c.Risk == Write {
		decision = Ask
	}
	asks, allows := false, false
	for _, g := range c.Grants {
		if g.Capability != c.Capability || g.Pattern != c.Pattern || !slices.Contains(c.Scopes, g.Scope) {
			continue
		}
		switch g.Decision {
		case Ask:
			asks = true
		case Allow:
			allows = true
		}
	}
	switch {
	case asks:
		decision = Ask
	case allows:
		decision = Allow
	}
	// Ask once per run: an approved write of this run covers its other calls.
	if decision == Ask && c.Approved {
		return Allow
	}
	return decision
}

// Grantable reports whether a grant may relax a risk: never for external
// or destructive.
func Grantable(r Risk) bool { return r == Read || r == Local || r == Write }
