package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/memory/files"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
)

// The worker's pace.
const (
	tickInterval = 30 * time.Second
	jobsPerTick  = 100
	// maxAttempts is how many times a job is tried before it fails for good.
	maxAttempts = 6
	// observeDelay lets a chat settle before its observation log is folded.
	observeDelay = 10 * time.Second
	// Consolidation runs once a local day, when the user has been away this
	// long or the evening has come.
	sleepIdle = 10 * time.Minute
	sleepHour = 22
	// jobsKept is how long done jobs are kept.
	jobsKept = 7 * 24 * time.Hour
	// vecChars is the most of a message or episode that is embedded.
	vecChars = 6000
	// page is how many rows a full read takes at a time.
	page = 500
)

// Backoffs: a job's retries, and extraction's after a model error.
var (
	jobBackoff     = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour}
	extractBackoff = []time.Duration{5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 6 * time.Hour}
	sleepRetry     = 30 * time.Minute
)

// Run is the worker: it ticks every 30 seconds, and soon after a wake, until
// ctx ends. Jobs left running by a crash are pending again first.
func (s *Service) Run(ctx context.Context) {
	if n, err := s.o.Repos.MemoryState.RequeueRunning(ctx); err != nil {
		slog.Warn("memory: jobs left running were not requeued", "err", err)
	} else if n > 0 {
		slog.Info("memory: jobs left running are pending again", "jobs", n)
	}
	t := s.o.Clock.NewTimer(tickInterval)
	defer t.Stop()
	for {
		s.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C():
		case <-s.wake:
		}
		t.Reset(tickInterval)
	}
}

// Tick does one round of the worker's work now: extraction, due jobs, and
// consolidation when it is time. Run calls it; tests call it to step the
// worker. With memory off it does nothing.
func (s *Service) Tick(ctx context.Context) {
	s.tickMu.Lock()
	defer s.tickMu.Unlock()
	if !s.enabled() || ctx.Err() != nil {
		return
	}
	ls := s.lanesNow()
	s.extractStep(ctx)
	more := s.runJobs(ctx, ls)
	s.sleepStep(ctx)
	if more {
		s.Wake(false)
	}
}

// runJobs runs up to jobsPerTick due jobs; it reports whether more may be
// due. Jobs that need a model wait while there is none.
func (s *Service) runJobs(ctx context.Context, ls laneSet) bool {
	kinds := []string{model.JobProject, model.JobFiles, model.JobProfile}
	if _, err := s.model(); err == nil {
		kinds = append(kinds, model.JobObserve, model.JobExtract)
	}
	for range jobsPerTick {
		if ctx.Err() != nil {
			return false
		}
		j, err := s.o.Repos.MemoryState.NextJob(ctx, s.now(), kinds...)
		if isNotFound(err) {
			return false
		}
		if err != nil {
			slog.Warn("memory: next job", "err", err)
			return false
		}
		s.runJob(ctx, ls, j)
	}
	return true
}

// runJob runs a job and records how it went: done, pending again after a
// backoff, or failed for good after maxAttempts.
func (s *Service) runJob(ctx context.Context, ls laneSet, j model.MemoryJob) {
	err := s.doJob(ctx, ls, j)
	if ctx.Err() != nil {
		return // left running; the next start requeues it
	}
	if err == nil {
		if err := s.o.Repos.MemoryState.FinishJob(ctx, j.ID); err != nil {
			slog.Warn("memory: job not marked done", "job", j.ID, "err", err)
		}
		return
	}
	var retry *time.Time
	if j.Attempts < maxAttempts {
		at := s.now().Add(jobBackoff[min(max(j.Attempts, 1), len(jobBackoff))-1])
		retry = &at
	}
	slog.Warn("memory: job failed", "kind", j.Kind, "subject", j.Subject, "attempt", j.Attempts,
		"retry", retry != nil, "err", err)
	if err := s.o.Repos.MemoryState.FailJob(ctx, j.ID, err.Error(), retry); err != nil {
		slog.Warn("memory: job failure not recorded", "job", j.ID, "err", err)
	}
}

// doJob does a job's work.
func (s *Service) doJob(ctx context.Context, ls laneSet, j model.MemoryJob) error {
	switch j.Kind {
	case model.JobProject:
		return s.project(ctx, ls, j.Subject)
	case model.JobFiles:
		if ls.files == nil {
			return nil
		}
		_, err := s.syncFiles(ctx, ls.files, false)
		return err
	case model.JobObserve:
		_, err := s.learner.Observe(ctx, j.Subject)
		return err
	case model.JobProfile:
		_, err := s.compileProfile(ctx, s.today())
		return err
	case model.JobExtract:
		res, err := s.learner.Extract(ctx, true)
		s.changed(ctx, res.Claims, res.Conflicts)
		return err
	}
	slog.Warn("memory: a job of an unknown kind is dropped", "kind", j.Kind)
	return nil
}

// extractStep learns from what was said since the last run, when it is due,
// or at once after a wake that asked for it. After a model error it waits
// 5 minutes, then 30, 2 hours, and 6 hours.
func (s *Service) extractStep(ctx context.Context) {
	force := s.force.Swap(false)
	s.stateMu.Lock()
	until := s.extract.until
	s.stateMu.Unlock()
	if s.now().Before(until) {
		if force {
			s.force.Store(true)
		}
		return
	}
	res, err := s.learner.Extract(ctx, force)
	s.recordExtract(ctx, err)
	s.changed(ctx, res.Claims, res.Conflicts)
}

// recordExtract keeps how the last extraction went, for the backoff and the
// status. No model is not an error: learning is paused.
func (s *Service) recordExtract(ctx context.Context, err error) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	switch {
	case err == nil, errors.Is(err, memory.ErrDisabled):
		s.extract = backoffState{}
	case ctx.Err() != nil:
	default:
		s.extract.fails++
		s.extract.lastErr = err.Error()
		s.extract.until = s.now().Add(extractBackoff[min(s.extract.fails, len(extractBackoff))-1])
		slog.Warn("memory: learning from chats failed", "fails", s.extract.fails, "retry", s.extract.until, "err", err)
	}
}

// sleepStep consolidates once a local day, when the user has been idle for
// 10 minutes or it is after 22:00.
func (s *Service) sleepStep(ctx context.Context) {
	now := s.now()
	idle := s.o.Idle != nil && s.o.Idle(ctx)
	s.stateMu.Lock()
	switch {
	case !idle:
		s.idleSince = time.Time{}
	case s.idleSince.IsZero():
		s.idleSince = now
	}
	since, retry := s.idleSince, s.sleepAfter
	s.stateMu.Unlock()
	if now.Before(retry) || s.sleptToday(ctx, now) {
		return
	}
	if !(idle && now.Sub(since) >= sleepIdle) && now.In(s.loc).Hour() < sleepHour {
		return
	}
	if _, err := s.sleep(ctx); err != nil && ctx.Err() == nil {
		slog.Warn("memory: consolidation failed", "err", err)
		s.stateMu.Lock()
		s.sleepAfter = now.Add(sleepRetry)
		s.stateMu.Unlock()
	}
}

// sleptToday reports whether consolidation ran today.
func (s *Service) sleptToday(ctx context.Context, now time.Time) bool {
	last, err := store.GetLocalTime(ctx, s.o.DB.SQL(), store.KeyMemoryLastSlept)
	if err != nil {
		if !isNotFound(err) {
			slog.Warn("memory: when consolidation last ran is unknown", "err", err)
		}
		return false
	}
	return s.o.Day(last) == s.o.Day(now)
}

// sleep consolidates: what is pending is learned, duplicates merged,
// contradictions and insights looked for, the lanes reconciled, and today's
// profile card compiled. Model steps that fail are logged and skipped.
func (s *Service) sleep(ctx context.Context) (memory.SleepReport, error) {
	s.sleepMu.Lock()
	defer s.sleepMu.Unlock()
	var rep memory.SleepReport
	var ids []string
	conflicts := false
	modelErr := func(step string, err error) {
		if !errors.Is(err, memory.ErrDisabled) {
			slog.Warn("memory: a consolidation step failed", "step", step, "err", err)
		}
	}
	if _, err := s.model(); err == nil {
		res, err := s.learner.Extract(ctx, true)
		s.recordExtract(ctx, err)
		if err != nil {
			modelErr("extract", err)
		}
		rep.Learned, ids, conflicts = res.Learned, append(ids, res.Claims...), res.Conflicts
	}
	n, err := s.learner.Dedupe(ctx)
	if err != nil {
		return rep, fmt.Errorf("merge duplicates: %w", err)
	}
	rep.Merged = n
	if rep.Conflicts, err = s.learner.Contradict(ctx); err != nil {
		modelErr("contradict", err)
	}
	if rep.Insights, err = s.learner.Insights(ctx); err != nil {
		modelErr("insights", err)
	}
	if rep.Reconciled, err = s.reconcile(ctx, s.lanesNow()); err != nil {
		slog.Warn("memory: reconciling the lanes failed", "err", err)
	}
	if rep.Merged+rep.Conflicts+rep.Insights > 0 && ids == nil {
		ids = []string{}
	}
	s.changed(ctx, ids, conflicts || rep.Conflicts > 0)
	if _, err := s.compileProfile(ctx, s.today()); err != nil {
		return rep, fmt.Errorf("profile card: %w", err)
	}
	rep.Profile = true
	if _, err := s.o.Repos.MemoryState.PruneJobs(ctx, s.now().Add(-jobsKept)); err != nil {
		return rep, fmt.Errorf("prune jobs: %w", err)
	}
	now := s.now()
	if err := store.SetLocalTime(ctx, s.o.DB.SQL(), store.KeyMemoryLastSlept, now, now); err != nil {
		return rep, err
	}
	return rep, nil
}

// compileProfile compiles the profile card for day.
func (s *Service) compileProfile(ctx context.Context, day string) (model.ProfileCard, error) {
	s.profileDirty.Store(false)
	card, err := s.learner.Profile(ctx, day)
	if err != nil {
		s.profileDirty.Store(true)
	}
	return card, err
}

// project brings one item up to date in every lane that holds copies (spec
// 9, 4): a live, active, shown claim is stored, and anything else dropped;
// live messages and episodes are embedded; a document is sent to
// Supermemory. Lanes that are off are skipped.
func (s *Service) project(ctx context.Context, ls laneSet, subject string) error {
	kind, id, ok := memory.ParseRef(subject)
	if !ok {
		slog.Warn("memory: a project job names no item", "subject", subject)
		return nil
	}
	ref := memory.Ref(kind, id)
	var errs []error
	switch kind {
	case memory.RefClaim:
		c, err := s.o.Repos.Memory.Claim(ctx, id)
		if err != nil && !isNotFound(err) {
			return err
		}
		shown := err == nil && c.Status == model.ClaimActive && !c.Suppressed
		if ls.vec != nil {
			if shown {
				errs = append(errs, ls.vec.Upsert(ctx, []memory.Record{claimRecord(c)}))
			} else {
				errs = append(errs, ls.vec.Delete(ctx, []string{ref}))
			}
		}
		if ls.mem0 != nil {
			if shown && c.Sensitivity != model.SensitivitySensitive {
				errs = append(errs, ls.mem0.Upsert(ctx, []memory.Record{claimRecord(c)}))
			} else {
				errs = append(errs, s.mem0Delete(ctx, ls.mem0, []string{id}))
			}
		}
	case memory.RefMessage:
		if ls.vec == nil {
			return nil
		}
		m, err := s.o.Repos.Memory.Message(ctx, id)
		switch {
		case err == nil:
			errs = append(errs, ls.vec.Upsert(ctx, []memory.Record{messageRecord(m.ChatMessage)}))
		case isNotFound(err):
			errs = append(errs, ls.vec.Delete(ctx, []string{ref}))
		default:
			return err
		}
	case memory.RefEpisode:
		if ls.vec == nil {
			return nil
		}
		e, err := s.o.Repos.Memory.Episode(ctx, id)
		switch {
		case err == nil:
			errs = append(errs, ls.vec.Upsert(ctx, []memory.Record{episodeRecord(e)}))
		case isNotFound(err):
			errs = append(errs, ls.vec.Delete(ctx, []string{ref}))
		default:
			return err
		}
	case memory.RefResource:
		if ls.sm == nil {
			return nil
		}
		rec, err := s.resourceRecord(ctx, id)
		switch {
		case err == nil:
			errs = append(errs, ls.sm.Upsert(ctx, []memory.Record{rec}))
		case isNotFound(err):
			errs = append(errs, ls.sm.Delete(ctx, []string{id}))
		default:
			return err
		}
	}
	return errors.Join(errs...)
}

// claimRecord is what a lane holds of a claim.
func claimRecord(c model.Claim) memory.Record {
	return memory.Record{ID: memory.Ref(memory.RefClaim, c.ID), Kind: "claim", Key: c.Key, Text: c.Text,
		ValidFrom: c.ValidFrom, At: c.RecordedAt, Meta: map[string]string{"sensitivity": c.Sensitivity}}
}

// messageRecord is what the vector lane holds of a chat message.
func messageRecord(m model.ChatMessage) memory.Record {
	return memory.Record{ID: memory.Ref(memory.RefMessage, m.ID), Kind: "message", Text: memory.Cut(m.Text, vecChars),
		At: m.At, Meta: map[string]string{"sensitivity": model.SensitivityPersonal}}
}

// episodeRecord is what the vector lane holds of an episode.
func episodeRecord(e model.Episode) memory.Record {
	text := e.Body
	if e.Title != "" {
		text = e.Title + "\n" + text
	}
	return memory.Record{ID: memory.Ref(memory.RefEpisode, e.ID), Kind: "episode", Text: memory.Cut(text, vecChars),
		At: e.At, Meta: map[string]string{"sensitivity": e.Sensitivity}}
}

// resourceRecord is what Supermemory holds of a document: its text, title,
// and date. A document removed, or whose text is erased, is not found.
func (s *Service) resourceRecord(ctx context.Context, id string) (memory.Record, error) {
	res, err := s.o.Repos.MemoryState.Resource(ctx, id)
	if err != nil {
		return memory.Record{}, err
	}
	if res.EpisodeID == nil {
		return memory.Record{}, store.Fail(store.ErrNotFound, "document %s has no text", id)
	}
	e, err := s.o.Repos.Memory.Episode(ctx, *res.EpisodeID)
	if err != nil {
		return memory.Record{}, err
	}
	return memory.Record{ID: res.ID, Kind: "resource", Text: e.Body, ValidFrom: res.DocumentAt, At: res.AddedAt,
		Meta: map[string]string{"sensitivity": e.Sensitivity, "title": res.Title}}, nil
}

// reconcile repairs the lanes: the vector index drops what is gone and
// gets back what is missing; Mem0 loses claims no longer shown and gets
// back active ones. Items to add are queued as project jobs. It returns how
// many rows it repaired.
func (s *Service) reconcile(ctx context.Context, ls laneSet) (int, error) {
	if ls.vec == nil && ls.mem0 == nil {
		return 0, nil
	}
	claims, err := s.o.Repos.Memory.Claims(ctx, store.ClaimFilter{Statuses: []string{model.ClaimActive}})
	if err != nil {
		return 0, err
	}
	n := 0
	var jobs []store.NewJob
	var errs []error
	if ls.vec != nil {
		want, err := s.liveRefs(ctx, claims)
		if err != nil {
			return 0, err
		}
		have, err := ls.vec.Refs(ctx)
		if err != nil {
			return 0, err
		}
		held := make(map[string]bool, len(have))
		var drop []string
		for _, ref := range have {
			held[ref] = true
			if !want[ref] {
				drop = append(drop, ref)
			}
		}
		if err := ls.vec.Delete(ctx, drop); err != nil {
			errs = append(errs, err)
		} else {
			n += len(drop)
		}
		for ref := range want {
			if !held[ref] {
				jobs = append(jobs, store.NewJob{Kind: model.JobProject, Subject: ref})
			}
		}
	}
	if ls.mem0 != nil {
		shown := map[string]bool{}
		for _, c := range claims {
			if c.Sensitivity != model.SensitivitySensitive {
				shown[c.ID] = true
			}
		}
		rows, err := s.o.Repos.MemoryState.Backends(ctx, string(memory.LaneMem0), model.BackendSynced)
		if err != nil {
			return n, err
		}
		stored := map[string]bool{}
		var gone []model.MemoryBackend
		var ext []string
		for _, b := range rows {
			stored[b.ItemID] = true
			if !shown[b.ItemID] && b.ExternalID != "" {
				gone, ext = append(gone, b), append(ext, b.ExternalID)
			}
		}
		if err := s.mem0DeleteRows(ctx, ls.mem0, gone, ext); err != nil {
			errs = append(errs, err)
		} else {
			n += len(gone)
		}
		for id := range shown {
			if !stored[id] {
				jobs = append(jobs, store.NewJob{Kind: model.JobProject, Subject: memory.Ref(memory.RefClaim, id)})
			}
		}
	}
	if len(jobs) > 0 {
		if err := s.o.Repos.MemoryState.Enqueue(ctx, jobs...); err != nil {
			return n, err
		}
		n += len(jobs)
		s.Wake(false)
	}
	return n, errors.Join(errs...)
}

// liveRefs are the refs the vector index should hold: active shown claims,
// and every live message and episode.
func (s *Service) liveRefs(ctx context.Context, claims []model.Claim) (map[string]bool, error) {
	want := map[string]bool{}
	for _, c := range claims {
		want[memory.Ref(memory.RefClaim, c.ID)] = true
	}
	for after := int64(0); ; {
		msgs, err := s.o.Repos.Memory.MessagesAfter(ctx, after, page)
		if err != nil {
			return nil, err
		}
		for _, m := range msgs {
			want[memory.Ref(memory.RefMessage, m.ID)] = true
			after = m.Rowid
		}
		if len(msgs) < page {
			break
		}
	}
	for after := int64(0); ; {
		eps, err := s.o.Repos.Memory.EpisodesAfter(ctx, after, page)
		if err != nil {
			return nil, err
		}
		for _, e := range eps {
			want[memory.Ref(memory.RefEpisode, e.ID)] = true
			after = e.Rowid
		}
		if len(eps) < page {
			break
		}
	}
	return want, nil
}

// syncFiles reads back what the user changed in the memory folder, applies
// it, and writes the folder again from the claims. After a forget,
// forgetHistory drops the folder's git history too. It returns the claims
// the user's changes touched.
func (s *Service) syncFiles(ctx context.Context, f folder, forgetHistory bool) ([]string, error) {
	s.filesMu.Lock()
	defer s.filesMu.Unlock()
	changes, err := f.Scan(ctx)
	if err != nil {
		s.setFilesErr(err)
		return nil, err
	}
	var ids []string
	for _, ch := range changes {
		got, err := s.applyFileChange(ctx, ch)
		if ctx.Err() != nil {
			return ids, ctx.Err()
		}
		if err != nil {
			slog.Warn("memory: a change in the memory folder was not applied", "kind", ch.Kind, "file", ch.Path,
				"err", err)
			continue
		}
		ids = append(ids, got...)
	}
	n, err := s.projectFiles(ctx, f)
	if err == nil && forgetHistory {
		err = f.ForgetHistory(ctx)
	}
	s.setFilesErr(err)
	if err != nil {
		return ids, err
	}
	s.saveFilesState(ctx, f.Dir(), n)
	if len(ids) > 0 {
		s.changed(ctx, ids, false)
	}
	return ids, nil
}

// projectFiles writes the folder from the active claims; it returns how many
// claims it was given.
func (s *Service) projectFiles(ctx context.Context, f folder) (int, error) {
	claims, err := s.o.Repos.Memory.Claims(ctx, store.ClaimFilter{Statuses: []string{model.ClaimActive},
		ExcludeSensitive: true})
	if err != nil {
		return 0, err
	}
	views, err := s.router.Views(ctx, claims)
	if err != nil {
		return 0, err
	}
	entities, err := s.o.Repos.Memory.Entities(ctx, "")
	if err != nil {
		return 0, err
	}
	return len(views), f.Project(ctx, views, entities)
}

// applyFileChange carries out one change the user made in the folder: an
// edited line corrects its claim, a deleted line stops Gwen bringing it up,
// and a new line is remembered as the user's own words.
func (s *Service) applyFileChange(ctx context.Context, ch files.Change) ([]string, error) {
	switch ch.Kind {
	case files.ChangeEdit:
		c, err := s.learner.Edit(ctx, ch.ClaimID, memory.EditRequest{Text: ch.Text})
		if err != nil {
			return nil, err
		}
		return []string{ch.ClaimID, c.ID}, nil
	case files.ChangeRemove:
		if _, err := s.learner.Suppress(ctx, ch.ClaimID, true); err != nil {
			return nil, err
		}
		return []string{ch.ClaimID}, nil
	case files.ChangeAdd:
		claims, err := s.learner.Remember(ctx, addedLine(ch), model.ChannelUI)
		if err != nil {
			return nil, err
		}
		ids := make([]string, len(claims))
		for i, c := range claims {
			ids[i] = c.ID
		}
		return ids, nil
	}
	return nil, nil
}

// addedLine is a line the user added to the folder, as words to remember: a
// line under a label in profile.md reads "<Label> <line>", and one in a
// person's file or under a name in other.md is about them.
func addedLine(ch files.Change) string {
	text := strings.TrimSpace(ch.Text)
	switch {
	case ch.Path == files.ProfileFile && ch.Section != "":
		if !strings.HasPrefix(strings.ToLower(text), strings.ToLower(ch.Section)) {
			text = ch.Section + " " + text
		}
	case ch.Subject != "":
		if !strings.Contains(strings.ToLower(text), strings.ToLower(ch.Subject)) {
			text = "About " + ch.Subject + ": " + text
		}
	}
	return text
}
