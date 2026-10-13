package service

import (
	"context"
	"time"

	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

// ledgerTTL is how long the ledger's length and verification are cached.
const ledgerTTL = 10 * time.Minute

var _ memory.Memory = (*Service)(nil)

// Note is the memory note for a user message. With memory off it is empty,
// and the turn goes on without one.
func (s *Service) Note(ctx context.Context, req memory.NoteRequest) (memory.Note, error) {
	if !s.enabled() {
		return memory.Note{}, nil
	}
	s.lanesNow()
	return s.router.Note(ctx, req)
}

// Recall runs the read path for a query.
func (s *Service) Recall(ctx context.Context, q memory.Query) (memory.Packet, error) {
	if err := s.on(); err != nil {
		return memory.Packet{}, err
	}
	s.lanesNow()
	return s.router.Recall(ctx, q)
}

// Get returns the claim of key about subject that held at asOf.
func (s *Service) Get(ctx context.Context, key string, subjectID *string, asOf time.Time) (model.Claim, error) {
	if err := s.on(); err != nil {
		return model.Claim{}, err
	}
	return s.router.Get(ctx, key, subjectID, asOf)
}

// Explain says where a ref came from and what became of it.
func (s *Service) Explain(ctx context.Context, ref string) (memory.Explanation, error) {
	if err := s.on(); err != nil {
		return memory.Explanation{}, err
	}
	return s.router.Explain(ctx, ref)
}

// Claims lists claims with what the dashboard shows beside them.
func (s *Service) Claims(ctx context.Context, f store.ClaimFilter) ([]memory.ClaimView, error) {
	if err := s.on(); err != nil {
		return nil, err
	}
	claims, err := s.o.Repos.Memory.Claims(ctx, f)
	if err != nil {
		return nil, err
	}
	return s.router.Views(ctx, claims)
}

// Entities lists the people and things claims are about.
func (s *Service) Entities(ctx context.Context) ([]model.Entity, error) {
	if err := s.on(); err != nil {
		return nil, err
	}
	return s.o.Repos.Memory.Entities(ctx, "")
}

// Conflicts lists conflicts in status ("" for every status) with both
// claims. A claim since forgotten is shown by its id alone.
func (s *Service) Conflicts(ctx context.Context, status string) ([]memory.ConflictView, error) {
	if err := s.on(); err != nil {
		return nil, err
	}
	cs, err := s.o.Repos.Memory.Conflicts(ctx, status)
	if err != nil {
		return nil, err
	}
	return s.conflictViews(ctx, cs)
}

// conflictViews attaches both claims to each conflict.
func (s *Service) conflictViews(ctx context.Context, cs []model.Conflict) ([]memory.ConflictView, error) {
	var claims []model.Claim
	seen := map[string]bool{}
	for _, c := range cs {
		for _, id := range []string{c.ClaimA, c.ClaimB} {
			if seen[id] {
				continue
			}
			seen[id] = true
			cl, err := s.o.Repos.Memory.Claim(ctx, id)
			if isNotFound(err) {
				continue
			}
			if err != nil {
				return nil, err
			}
			claims = append(claims, cl)
		}
	}
	views, err := s.router.Views(ctx, claims)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]memory.ClaimView, len(views))
	for _, v := range views {
		byID[v.ID] = v
	}
	view := func(id string) memory.ClaimView {
		if v, ok := byID[id]; ok {
			return v
		}
		return memory.ClaimView{Claim: model.Claim{ID: id}}
	}
	out := make([]memory.ConflictView, len(cs))
	for i, c := range cs {
		out[i] = memory.ConflictView{Conflict: c, A: view(c.ClaimA), B: view(c.ClaimB)}
	}
	return out, nil
}

// Profile returns the profile card for day: the newest when it is that day's
// and no claim changed since, else one compiled now.
func (s *Service) Profile(ctx context.Context, day string) (model.ProfileCard, error) {
	if err := s.on(); err != nil {
		return model.ProfileCard{}, err
	}
	if !s.profileDirty.Load() {
		card, err := s.o.Repos.MemoryState.LatestProfile(ctx)
		if err == nil && card.Day == day {
			return card, nil
		}
		if err != nil && !isNotFound(err) {
			return model.ProfileCard{}, err
		}
	}
	return s.compileProfile(ctx, day)
}

// Observation returns a thread's observation log. With memory off there is
// none, so the chat shows its newest messages instead.
func (s *Service) Observation(ctx context.Context, threadID string) (model.ChatObservation, bool, error) {
	if !s.enabled() {
		return model.ChatObservation{}, false, nil
	}
	o, err := s.o.Repos.MemoryState.Observation(ctx, threadID)
	if isNotFound(err) {
		return model.ChatObservation{}, false, nil
	}
	if err != nil {
		return model.ChatObservation{}, false, err
	}
	return o, true, nil
}

// Resources lists the documents.
func (s *Service) Resources(ctx context.Context) ([]model.Resource, error) {
	if err := s.on(); err != nil {
		return nil, err
	}
	return s.o.Repos.MemoryState.Resources(ctx)
}

// Status reports what the memory holds and how its lanes are. With memory
// off it says so, with the counts of what is kept.
func (s *Service) Status(ctx context.Context) memory.Status {
	st := memory.Status{Enabled: s.enabled(), Claims: map[string]int{}, Jobs: map[string]int{}}
	if counts, err := s.o.Repos.Memory.CountClaims(ctx); err == nil {
		st.Claims = counts
	}
	if cs, err := s.o.Repos.Memory.Conflicts(ctx, model.ConflictOpen); err == nil {
		st.Conflicts = len(cs)
	}
	if jobs, err := s.o.Repos.MemoryState.JobCounts(ctx); err == nil {
		st.Jobs = jobs
	}
	for key, dst := range map[string]**time.Time{store.KeyMemoryLastLearned: &st.LastLearned,
		store.KeyMemoryLastSlept: &st.LastSlept} {
		if t, err := store.GetLocalTime(ctx, s.o.DB.SQL(), key); err == nil {
			*dst = &t
		}
	}
	if card, err := s.o.Repos.MemoryState.LatestProfile(ctx); err == nil {
		st.Profile = &card
	}
	st.Ledger = s.ledgerStatus(ctx)
	if !st.Enabled {
		st.Lanes = []memory.LaneStatus{}
		return st
	}
	st.Learning, _ = s.learning()
	total := 0
	for _, n := range st.Claims {
		total += n
	}
	st.Lanes = s.laneStatuses(ctx, s.lanesNow(), total, st.Ledger.Verified)
	return st
}

// ledgerEntry is the cached ledger status.
type ledgerEntry struct {
	at time.Time
	st memory.LedgerStatus
}

// ledgerStatus counts the ledger and verifies its chain, at most every 10
// minutes.
func (s *Service) ledgerStatus(ctx context.Context) memory.LedgerStatus {
	now := s.now()
	s.stateMu.Lock()
	cached := s.ledger
	s.stateMu.Unlock()
	if !cached.at.IsZero() && now.Sub(cached.at) < ledgerTTL {
		return cached.st
	}
	var st memory.LedgerStatus
	for after := int64(0); ; {
		rows, err := s.o.Repos.Memory.Ledger(ctx, after, 1000)
		if err != nil {
			return cached.st
		}
		st.Rows += int64(len(rows))
		if len(rows) < 1000 {
			break
		}
		after = rows[len(rows)-1].Seq
	}
	bad, err := s.o.Repos.Memory.VerifyLedger(ctx)
	if err != nil {
		return cached.st
	}
	st.Verified, st.BadSeq = bad == 0, bad
	s.stateMu.Lock()
	s.ledger = ledgerEntry{at: now, st: st}
	s.stateMu.Unlock()
	return st
}

// forgetLedgerCache makes the next status count the ledger again.
func (s *Service) forgetLedgerCache() {
	s.stateMu.Lock()
	s.ledger = ledgerEntry{}
	s.stateMu.Unlock()
}

// ResolveRef resolves a ref for showing beside a chat (api.MemoryResolver):
// what it names, its source, and for a claim its status. A ref that names
// nothing, or something gone, is Gone. It answers with memory off too, so
// past chats still show what they cited.
func (s *Service) ResolveRef(ctx context.Context, ref string) (wire.MemoryRef, error) {
	kind, id, ok := memory.ParseRef(ref)
	if !ok {
		return wire.MemoryRef{Ref: ref, Gone: true}, nil
	}
	out, err := s.router.Resolve(ctx, ref)
	if err != nil || out.Gone || kind != memory.RefClaim {
		return out, err
	}
	c, err := s.o.Repos.Memory.Claim(ctx, id)
	if isNotFound(err) {
		return wire.MemoryRef{Ref: ref, Kind: out.Kind, Gone: true}, nil
	}
	if err != nil {
		return wire.MemoryRef{}, err
	}
	out.Status = c.Status
	return out, nil
}

// Learned returns, by message id, the live claims learned from each message
// in any status but rejected (api.MemoryResolver).
func (s *Service) Learned(ctx context.Context, msgIDs []string) (map[string][]wire.MemoryRef, error) {
	out := map[string][]wire.MemoryRef{}
	for _, id := range msgIDs {
		claims, err := s.o.Repos.Memory.ClaimsFrom(ctx, model.SourceMessage, id)
		if err != nil {
			return nil, err
		}
		kept := claims[:0]
		for _, c := range claims {
			if c.Status != model.ClaimRejected {
				kept = append(kept, c)
			}
		}
		if len(kept) == 0 {
			continue
		}
		views, err := s.router.Views(ctx, kept)
		if err != nil {
			return nil, err
		}
		for _, v := range views {
			out[id] = append(out[id], wire.MemoryRef{Ref: memory.Ref(memory.RefClaim, v.ID), Kind: memory.RefClaim,
				Text: v.Text, Source: v.Source, ThreadID: v.ThreadID, Status: v.Status})
		}
	}
	return out, nil
}

// Export returns everything the memory holds: claims in every status,
// entities, episodes, conflicts, and documents.
func (s *Service) Export(ctx context.Context) (memory.Export, error) {
	if err := s.on(); err != nil {
		return memory.Export{}, err
	}
	ex := memory.Export{ExportedAt: s.now()}
	var err error
	if ex.Claims, err = s.o.Repos.Memory.Claims(ctx, store.ClaimFilter{IncludeSuppressed: true}); err != nil {
		return ex, err
	}
	if ex.Entities, err = s.o.Repos.Memory.Entities(ctx, ""); err != nil {
		return ex, err
	}
	for after := int64(0); ; {
		eps, err := s.o.Repos.Memory.EpisodesAfter(ctx, after, page)
		if err != nil {
			return ex, err
		}
		for _, e := range eps {
			ex.Episodes = append(ex.Episodes, e.Episode)
			after = e.Rowid
		}
		if len(eps) < page {
			break
		}
	}
	if ex.Conflicts, err = s.o.Repos.Memory.Conflicts(ctx, ""); err != nil {
		return ex, err
	}
	if ex.Resources, err = s.o.Repos.MemoryState.Resources(ctx); err != nil {
		return ex, err
	}
	return ex, nil
}
