package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/memory/docs"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
)

// Words of receipts.
const (
	chatStillThere = "The chat it came from is still in Past chats; delete that chat to erase its words."
	mem0History    = "Mem0 keeps a history of deleted memories; reset Mem0 to clear it"
	// maxTitle is the longest document title the store keeps.
	maxTitle = 200
)

// ChatTurn learns from a stored turn in the background: both messages are
// queued for the lanes, the chat for its observation log, and claims the
// reply was asked to stop bringing up are hidden at once. The worker wakes,
// and extracts at once when the user asked to be remembered.
func (s *Service) ChatTurn(ctx context.Context, t memory.ChatTurn) {
	if !s.enabled() {
		return
	}
	now := s.now()
	var jobs []store.NewJob
	for _, m := range []model.ChatMessage{t.User, t.Reply} {
		if m.ID != "" {
			jobs = append(jobs, store.NewJob{Kind: model.JobProject, Subject: memory.Ref(memory.RefMessage, m.ID)})
		}
	}
	if t.ThreadID != "" {
		jobs = append(jobs, store.NewJob{Kind: model.JobObserve, Subject: t.ThreadID, RunAfter: now.Add(observeDelay)})
	}
	if err := s.o.Repos.MemoryState.Enqueue(ctx, jobs...); err != nil {
		slog.Warn("memory: a chat turn was not queued", "thread", t.ThreadID, "err", err)
	}
	var hidden []string
	for _, ref := range t.Forget {
		id := bareClaimID(ref)
		if _, err := s.learner.Suppress(ctx, id, true); err != nil {
			slog.Warn("memory: a claim the chat asked to stop bringing up was not hidden", "claim", id, "err", err)
			continue
		}
		hidden = append(hidden, id)
	}
	if hidden != nil {
		s.changed(ctx, hidden, false)
	}
	s.Wake(hasCue(t.User.Text))
}

// Remember learns from words the user typed to be remembered.
func (s *Service) Remember(ctx context.Context, req memory.RememberRequest) ([]model.Claim, error) {
	if err := s.on(); err != nil {
		return nil, err
	}
	claims, err := s.learner.Remember(ctx, req.Text, req.Channel)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(claims))
	for i, c := range claims {
		ids[i] = c.ID
	}
	s.changed(ctx, ids, false)
	s.Wake(false)
	return claims, nil
}

// Edit replaces a claim with the user's correction.
func (s *Service) Edit(ctx context.Context, id string, req memory.EditRequest) (model.Claim, error) {
	if err := s.on(); err != nil {
		return model.Claim{}, err
	}
	c, err := s.learner.Edit(ctx, id, req)
	if err != nil {
		return model.Claim{}, err
	}
	s.changed(ctx, []string{id, c.ID}, false)
	s.Wake(false)
	return c, nil
}

// Confirm makes a candidate or quarantined claim active.
func (s *Service) Confirm(ctx context.Context, id string) (model.Claim, error) {
	return s.claimWrite(ctx, id, s.learner.Confirm)
}

// Reject marks a claim not true.
func (s *Service) Reject(ctx context.Context, id string) (model.Claim, error) {
	return s.claimWrite(ctx, id, s.learner.Reject)
}

// Suppress hides a claim from recall, or shows it again.
func (s *Service) Suppress(ctx context.Context, id string, on bool) (model.Claim, error) {
	return s.claimWrite(ctx, id, func(ctx context.Context, id string) (model.Claim, error) {
		return s.learner.Suppress(ctx, id, on)
	})
}

// claimWrite runs a change of one claim through the learner, then tells the
// dashboard and wakes the worker. Conflicts are announced when the claim was
// in an open one, since the change may close it.
func (s *Service) claimWrite(ctx context.Context, id string, write func(context.Context, string) (model.Claim,
	error)) (model.Claim, error) {
	if err := s.on(); err != nil {
		return model.Claim{}, err
	}
	open, err := s.o.Repos.Memory.OpenConflictsOf(ctx, id)
	if err != nil {
		return model.Claim{}, err
	}
	c, err := write(ctx, id)
	if err != nil {
		return model.Claim{}, err
	}
	s.changed(ctx, []string{id}, len(open) > 0)
	s.Wake(false)
	return c, nil
}

// Resolve settles a conflict.
func (s *Service) Resolve(ctx context.Context, conflictID, resolution string) (memory.ConflictView, error) {
	if err := s.on(); err != nil {
		return memory.ConflictView{}, err
	}
	c, err := s.learner.Resolve(ctx, conflictID, resolution)
	if err != nil {
		return memory.ConflictView{}, err
	}
	s.changed(ctx, []string{c.ClaimA, c.ClaimB}, true)
	s.Wake(false)
	views, err := s.conflictViews(ctx, []model.Conflict{c})
	if err != nil {
		return memory.ConflictView{Conflict: c}, nil
	}
	return views[0], nil
}

// Forget erases a claim everywhere it lives: Gwen's store first, then each
// lane that holds a copy, and the receipt says how each went. The chat the
// claim came from keeps its words until the user deletes it.
func (s *Service) Forget(ctx context.Context, id string) (memory.ForgetReceipt, error) {
	if err := s.on(); err != nil {
		return memory.ForgetReceipt{}, err
	}
	open, err := s.o.Repos.Memory.OpenConflictsOf(ctx, id)
	if err != nil {
		return memory.ForgetReceipt{}, err
	}
	c, err := s.learner.Forget(ctx, id)
	if err != nil {
		return memory.ForgetReceipt{}, err
	}
	s.forgetLedgerCache()
	gwen := memory.LaneReceipt{Lane: laneGwen, OK: true}
	if c.SourceKind == model.SourceMessage || c.SourceKind == model.SourceMem0 {
		if _, err := s.o.Repos.Memory.Message(ctx, c.SourceID); err == nil {
			gwen.Note = chatStillThere
		}
	}
	rc := memory.ForgetReceipt{Claims: 1, Lanes: []memory.LaneReceipt{gwen}}
	ls := s.lanesNow()
	if ls.vec != nil {
		err := ls.vec.Delete(ctx, []string{memory.Ref(memory.RefClaim, id)})
		rc.Lanes = append(rc.Lanes, receipt(memory.LaneVec, err, ""))
	}
	if ls.mem0 != nil {
		rc.Lanes = append(rc.Lanes, receipt(memory.LaneMem0, s.mem0Delete(ctx, ls.mem0, []string{id}), mem0History))
	}
	if ls.files != nil {
		_, err := s.syncFiles(ctx, ls.files, true)
		rc.Lanes = append(rc.Lanes, receipt(memory.LaneFiles, err, ""))
	}
	s.changed(ctx, []string{id}, len(open) > 0)
	return rc, nil
}

// receipt is a lane's part of a receipt: OK with note, or the error.
func receipt(lane memory.LaneName, err error, note string) memory.LaneReceipt {
	if err != nil {
		return memory.LaneReceipt{Lane: lane, Note: err.Error()}
	}
	return memory.LaneReceipt{Lane: lane, OK: true, Note: note}
}

// ForgetAll erases everything the memory holds, in Gwen's store and in every
// lane; chats are kept.
func (s *Service) ForgetAll(ctx context.Context) (memory.ForgetReceipt, error) {
	if err := s.on(); err != nil {
		return memory.ForgetReceipt{}, err
	}
	// What the lanes hold is read before the store forgets where it is.
	resources, err := s.o.Repos.MemoryState.Resources(ctx)
	if err != nil {
		return memory.ForgetReceipt{}, err
	}
	backends, err := s.o.Repos.MemoryState.Backends(ctx, string(memory.LaneMem0), "")
	if err != nil {
		return memory.ForgetReceipt{}, err
	}
	erased, err := s.o.Repos.Memory.EraseAll(ctx, store.LedgerItem{})
	if err != nil {
		return memory.ForgetReceipt{}, err
	}
	s.forgetLedgerCache()
	rc := memory.ForgetReceipt{Claims: erased.Claims, Episodes: erased.Episodes,
		Lanes: []memory.LaneReceipt{{Lane: laneGwen, OK: true, Note: "Chats are kept; delete them in Past chats."}}}
	ls := s.lanesNow()
	if ls.vec != nil {
		rc.Lanes = append(rc.Lanes, receipt(memory.LaneVec, ls.vec.Clear(ctx), ""))
	}
	if ls.mem0 != nil {
		var ext []string
		for _, b := range backends {
			if b.Status != model.BackendDeleted && b.ExternalID != "" {
				ext = append(ext, b.ExternalID)
			}
		}
		var err error
		if len(ext) > 0 {
			err = ls.mem0.Delete(ctx, ext)
		}
		rc.Lanes = append(rc.Lanes, receipt(memory.LaneMem0, err, mem0History))
	}
	if ls.sm != nil {
		ids := make([]string, len(resources))
		for i, r := range resources {
			ids[i] = r.ID
		}
		var err error
		if len(ids) > 0 {
			err = ls.sm.Delete(ctx, ids)
		}
		rc.Lanes = append(rc.Lanes, receipt(memory.LaneSupermemory, err, ""))
	}
	if ls.files != nil {
		_, err := s.syncFiles(ctx, ls.files, true)
		rc.Lanes = append(rc.Lanes, receipt(memory.LaneFiles, err, ""))
	}
	s.changed(ctx, []string{}, true)
	return rc, nil
}

// AddResource reads a document on this device and keeps its text as an
// episode, for Supermemory and the vector index to take up.
func (s *Service) AddResource(ctx context.Context, req memory.AddResourceRequest) (model.Resource, error) {
	if err := s.on(); err != nil {
		return model.Resource{}, err
	}
	path := filepath.Clean(req.Path)
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return model.Resource{}, store.FailField(store.ErrInvalid, "path", "there is no file at %s", path)
	case err != nil:
		return model.Resource{}, store.FailField(store.ErrInvalid, "path", "cannot read %s: %v", path, err)
	case !info.Mode().IsRegular():
		return model.Resource{}, store.FailField(store.ErrInvalid, "path", "%s is not a file", path)
	}
	text, mime, err := docs.Read(ctx, path)
	if err != nil {
		if ctx.Err() != nil {
			return model.Resource{}, ctx.Err()
		}
		return model.Resource{}, store.FailField(store.ErrInvalid, "path", "%v", err)
	}
	if strings.TrimSpace(text) == "" {
		return model.Resource{}, store.FailField(store.ErrInvalid, "path", "%s has no text Gwen can read", path)
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = filepath.Base(path)
	}
	title = memory.Cut(title, maxTitle)
	now := s.now()
	res, err := s.o.Repos.MemoryState.AddResource(ctx, model.Resource{Title: title, Path: path, Mime: mime,
		Size: info.Size(), AddedAt: now},
		model.Episode{Kind: model.EpisodeResource, Channel: model.ChannelDoc, Speaker: model.SpeakerExternal,
			Title: title, Body: text, Source: path, At: now, Sensitivity: model.SensitivityPersonal})
	if err != nil {
		return model.Resource{}, err
	}
	jobs := []store.NewJob{{Kind: model.JobProject, Subject: memory.Ref(memory.RefResource, res.ID)}}
	if res.EpisodeID != nil {
		jobs = append(jobs, store.NewJob{Kind: model.JobProject, Subject: memory.Ref(memory.RefEpisode, *res.EpisodeID)})
	}
	if err := s.o.Repos.MemoryState.Enqueue(ctx, jobs...); err != nil {
		slog.Warn("memory: a document was not queued for the lanes", "resource", res.ID, "err", err)
	}
	s.Wake(false)
	return res, nil
}

// RemoveResource forgets a document: its text is erased in Gwen's store and
// dropped from the lanes.
func (s *Service) RemoveResource(ctx context.Context, id string) (memory.ForgetReceipt, error) {
	if err := s.on(); err != nil {
		return memory.ForgetReceipt{}, err
	}
	res, err := s.o.Repos.MemoryState.Resource(ctx, id)
	if err != nil {
		return memory.ForgetReceipt{}, err
	}
	if err := s.o.Repos.MemoryState.RemoveResource(ctx, id, store.LedgerItem{}); err != nil {
		return memory.ForgetReceipt{}, err
	}
	s.forgetLedgerCache()
	rc := memory.ForgetReceipt{Episodes: 1, Lanes: []memory.LaneReceipt{{Lane: laneGwen, OK: true}}}
	ls := s.lanesNow()
	if ls.sm != nil {
		rc.Lanes = append(rc.Lanes, receipt(memory.LaneSupermemory, ls.sm.Delete(ctx, []string{id}), ""))
	}
	if ls.vec != nil && res.EpisodeID != nil {
		err := ls.vec.Delete(ctx, []string{memory.Ref(memory.RefEpisode, *res.EpisodeID)})
		rc.Lanes = append(rc.Lanes, receipt(memory.LaneVec, err, ""))
	}
	if ls.files != nil {
		_, err := s.syncFiles(ctx, ls.files, true)
		rc.Lanes = append(rc.Lanes, receipt(memory.LaneFiles, err, ""))
	}
	return rc, nil
}

// Rebuild rebuilds a derived lane from Gwen's store: the vector index is
// cleared and every active claim, message, and episode queued for it; the
// folder is written again; Mem0 is emptied and given the active claims;
// Supermemory is given every document again.
func (s *Service) Rebuild(ctx context.Context, lane memory.LaneName) error {
	if err := s.on(); err != nil {
		return err
	}
	ls := s.lanesNow()
	off := func(what string) error {
		return store.FailField(store.ErrInvalid, "lane", "%s is off; turn it on in Settings, Memory first", what)
	}
	switch lane {
	case memory.LaneVec:
		if ls.vec == nil {
			return off("semantic search")
		}
		if err := ls.vec.Clear(ctx); err != nil {
			return err
		}
		claims, err := s.o.Repos.Memory.Claims(ctx, store.ClaimFilter{Statuses: []string{model.ClaimActive}})
		if err != nil {
			return err
		}
		refs, err := s.liveRefs(ctx, claims)
		if err != nil {
			return err
		}
		return s.enqueueProject(ctx, refs)
	case memory.LaneFiles:
		if ls.files == nil {
			return off("the memory folder")
		}
		_, err := s.syncFiles(ctx, ls.files, false)
		return err
	case memory.LaneMem0:
		if ls.mem0 == nil {
			return off("Mem0")
		}
		rows, err := s.o.Repos.MemoryState.Backends(ctx, string(memory.LaneMem0), "")
		if err != nil {
			return err
		}
		var live []model.MemoryBackend
		var ext []string
		for _, b := range rows {
			if b.Status != model.BackendDeleted && b.ExternalID != "" {
				live, ext = append(live, b), append(ext, b.ExternalID)
			}
		}
		if err := s.mem0DeleteRows(ctx, ls.mem0, live, ext); err != nil {
			return err
		}
		claims, err := s.o.Repos.Memory.Claims(ctx, store.ClaimFilter{Statuses: []string{model.ClaimActive},
			ExcludeSensitive: true})
		if err != nil {
			return err
		}
		recs := make([]memory.Record, len(claims))
		for i, c := range claims {
			recs[i] = claimRecord(c)
		}
		if len(recs) == 0 {
			return nil
		}
		return ls.mem0.Upsert(ctx, recs)
	case memory.LaneSupermemory:
		if ls.sm == nil {
			return off("Supermemory")
		}
		resources, err := s.o.Repos.MemoryState.Resources(ctx)
		if err != nil {
			return err
		}
		var recs []memory.Record
		for _, r := range resources {
			rec, err := s.resourceRecord(ctx, r.ID)
			if isNotFound(err) {
				continue
			}
			if err != nil {
				return err
			}
			recs = append(recs, rec)
		}
		if len(recs) == 0 {
			return nil
		}
		return ls.sm.Upsert(ctx, recs)
	}
	return store.FailField(store.ErrInvalid, "lane", "%q cannot be rebuilt; lanes are vec, files, mem0, and supermemory",
		lane)
}

// enqueueProject queues a project job for each ref, in order, and wakes the
// worker.
func (s *Service) enqueueProject(ctx context.Context, refs map[string]bool) error {
	sorted := make([]string, 0, len(refs))
	for ref := range refs {
		sorted = append(sorted, ref)
	}
	slices.Sort(sorted)
	for len(sorted) > 0 {
		n := min(len(sorted), page)
		jobs := make([]store.NewJob, n)
		for i, ref := range sorted[:n] {
			jobs[i] = store.NewJob{Kind: model.JobProject, Subject: ref}
		}
		if err := s.o.Repos.MemoryState.Enqueue(ctx, jobs...); err != nil {
			return fmt.Errorf("queue the rebuild: %w", err)
		}
		sorted = sorted[n:]
	}
	s.Wake(false)
	return nil
}

// Sleep consolidates now instead of waiting for an idle moment.
func (s *Service) Sleep(ctx context.Context) (memory.SleepReport, error) {
	if err := s.on(); err != nil {
		return memory.SleepReport{}, err
	}
	return s.sleep(ctx)
}

// Pulled catches the memory up with rows another device wrote, which sync
// applied: pulled claims, messages, and episodes are queued for the lanes,
// and the folder and the profile card are brought up to date.
func (s *Service) Pulled(ctx context.Context, a store.Applied) {
	if !s.enabled() {
		return
	}
	var jobs []store.NewJob
	for table, kind := range map[string]string{"claims": memory.RefClaim, "chat_messages": memory.RefMessage,
		"episodes": memory.RefEpisode} {
		for _, id := range a.IDs[table] {
			jobs = append(jobs, store.NewJob{Kind: model.JobProject, Subject: memory.Ref(kind, id)})
		}
	}
	if len(a.IDs["claims"])+len(a.IDs["entities"]) > 0 {
		jobs = append(jobs, store.NewJob{Kind: model.JobFiles})
		s.profileDirty.Store(true)
		jobs = append(jobs, store.NewJob{Kind: model.JobProfile, Subject: s.today()})
	}
	if len(jobs) == 0 {
		return
	}
	if err := s.o.Repos.MemoryState.Enqueue(ctx, jobs...); err != nil {
		slog.Warn("memory: pulled rows were not queued for the lanes", "err", err)
		return
	}
	s.Wake(false)
}
