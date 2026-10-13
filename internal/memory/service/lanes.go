package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/memory/files"
	"github.com/kzark/gwen/internal/memory/mem0"
	"github.com/kzark/gwen/internal/memory/supermemory"
	"github.com/kzark/gwen/internal/memory/vec"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
)

// indexFile is the vector index, in the data directory.
const indexFile = "index.db"

// Lane health is cached this long, and asked for with this timeout.
const (
	healthTTL     = time.Minute
	healthTimeout = time.Second
)

// Supermemory's free server holds at most smCap documents; the status warns
// from smWarn.
const (
	smCap  = 10_000
	smWarn = 8_000
)

// vecLane is what the service needs of the vector index.
type vecLane interface {
	memory.Projection
	Count(ctx context.Context) (int, error)
	Refs(ctx context.Context) ([]string, error)
	Clear(ctx context.Context) error
	Close() error
}

// docLane is what the service needs of Supermemory.
type docLane interface {
	memory.Projection
	Count(ctx context.Context) (int, error)
}

// folder is what the service needs of the memory folder.
type folder interface {
	Dir() string
	Scan(ctx context.Context) ([]files.Change, error)
	Project(ctx context.Context, claims []memory.ClaimView, entities []model.Entity) error
	ForgetHistory(ctx context.Context) error
}

// factories make the lanes; tests replace them with fakes.
type factories struct {
	vec   func(path, endpoint, model string) (vecLane, error)
	mem0  func(url, key string, onStored func(claimID, externalID string)) memory.Projection
	sm    func(url, key string) docLane
	files func(dir string) folder
}

func defaultFactories(o Options) factories {
	return factories{
		vec: func(path, endpoint, model string) (vecLane, error) {
			return vec.New(vec.Options{Path: path, Endpoint: endpoint, Model: model, HTTPClient: o.HTTPClient,
				Clock: o.Clock})
		},
		mem0: func(url, key string, onStored func(claimID, externalID string)) memory.Projection {
			return mem0.New(mem0.Options{URL: url, APIKey: key, HTTPClient: o.HTTPClient, OnStored: onStored})
		},
		sm: func(url, key string) docLane {
			return supermemory.New(supermemory.Options{URL: url, APIKey: key, HTTPClient: o.HTTPClient})
		},
		files: func(dir string) folder { return files.New(files.Options{Dir: dir, Clock: o.Clock, Loc: o.Loc}) },
	}
}

// laneSet is the lanes the configuration turns on, each with the settings it
// was built from; a nil lane is off.
type laneSet struct {
	vecKey   string
	vec      vecLane
	mem0Key  string
	mem0     memory.Projection
	smKey    string
	sm       docLane
	filesDir string
	files    folder
	// bad says why a lane the configuration asks for could not be built.
	bad map[memory.LaneName]string
}

// close closes what holds resources.
func (ls *laneSet) close() error {
	if ls.vec == nil {
		return nil
	}
	err := ls.vec.Close()
	ls.vec = nil
	return err
}

// lanesNow brings the lanes up to date with the configuration and returns
// them. A lane whose settings changed is built again; the router is given
// the extra lanes that are on.
func (s *Service) lanesNow() laneSet {
	m := s.cfg()
	s.lanesMu.Lock()
	defer s.lanesMu.Unlock()
	if s.closed.Load() {
		return laneSet{}
	}
	ls := &s.lanes
	changed := false
	if ls.bad == nil {
		ls.bad = map[memory.LaneName]string{}
	}

	// vec: the embedding server and model.
	vecKey := ""
	if m.Enabled && m.EmbedEndpoint != "" && m.EmbedModel != "" {
		vecKey = m.EmbedEndpoint + "\x00" + m.EmbedModel
	}
	if vecKey != ls.vecKey {
		if err := ls.close(); err != nil {
			slog.Warn("memory: closing the vector index", "err", err)
		}
		ls.vecKey, changed = vecKey, true
		delete(ls.bad, memory.LaneVec)
		if vecKey != "" {
			ix, err := s.make.vec(filepath.Join(s.o.DataDir, indexFile), m.EmbedEndpoint, m.EmbedModel)
			if err != nil {
				slog.Warn("memory: the vector index cannot open", "err", err)
				ls.bad[memory.LaneVec] = err.Error()
			} else {
				ls.vec = ix
			}
		}
	}

	// mem0: the URL and its key.
	mem0Key := ""
	key := ""
	if m.Enabled && m.Mem0URL != "" && s.mayReach(m, m.Mem0URL) {
		key = s.credential(config.CredMem0APIKey)
		mem0Key = m.Mem0URL + "\x00" + key
	}
	if mem0Key != ls.mem0Key {
		ls.mem0Key, ls.mem0, changed = mem0Key, nil, true
		if mem0Key != "" {
			ls.mem0 = s.make.mem0(m.Mem0URL, key, s.mem0Stored)
		}
	}

	// supermemory: the URL and its key.
	smKey := ""
	key = ""
	if m.Enabled && m.SupermemoryURL != "" && s.mayReach(m, m.SupermemoryURL) {
		key = s.credential(config.CredSupermemoryAPIKey)
		smKey = m.SupermemoryURL + "\x00" + key
	}
	if smKey != ls.smKey {
		ls.smKey, ls.sm, changed = smKey, nil, true
		if smKey != "" {
			ls.sm = s.make.sm(m.SupermemoryURL, key)
		}
	}

	// files: the folder.
	dir := ""
	if m.Enabled && m.FilesDir != "" {
		var err error
		if dir, err = expandHome(m.FilesDir); err != nil {
			ls.bad[memory.LaneFiles] = err.Error()
			dir = ""
		}
	}
	if dir != ls.filesDir {
		ls.filesDir, ls.files = dir, nil
		if dir != "" {
			delete(ls.bad, memory.LaneFiles)
			ls.files = s.make.files(dir)
		}
	}

	if changed {
		var extra []memory.Lane
		if ls.vec != nil {
			extra = append(extra, ls.vec)
		}
		if ls.mem0 != nil {
			extra = append(extra, ls.mem0)
		}
		if ls.sm != nil {
			extra = append(extra, ls.sm)
		}
		s.router.SetLanes(extra)
	}
	out := *ls
	out.bad = make(map[memory.LaneName]string, len(ls.bad))
	for k, v := range ls.bad {
		out.bad[k] = v
	}
	return out
}

// mayReach reports whether the privacy setting lets memory talk to url:
// with privacy local, only to this machine.
func (s *Service) mayReach(m config.Memory, url string) bool {
	return m.Privacy != config.PrivacyLocal || config.IsLoopbackURL(url)
}

// credential reads an optional API key; a missing one is "".
func (s *Service) credential(name string) string {
	if s.o.CredDir == "" {
		return ""
	}
	key, err := config.ReadCredentialLine(s.o.CredDir, name)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("memory: credential unreadable", "name", name, "err", err)
	}
	return key
}

// expandHome makes a folder setting absolute: "~/" is the user's home.
func expandHome(dir string) (string, error) {
	rest, ok := strings.CutPrefix(dir, "~/")
	if !ok {
		return filepath.Clean(dir), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("the home folder is unknown: %w", err)
	}
	return filepath.Join(home, rest), nil
}

// mem0Stored records where Mem0 keeps a claim.
func (s *Service) mem0Stored(claimID, externalID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	hash := ""
	if c, err := s.o.Repos.Memory.Claim(ctx, claimID); err == nil {
		hash = c.ContentHash
	}
	now := s.now()
	err := s.o.Repos.MemoryState.SetBackend(ctx, model.MemoryBackend{Backend: string(memory.LaneMem0), ItemID: claimID,
		ExternalID: externalID, Status: model.BackendSynced, ContentHash: hash, SyncedAt: &now, UpdatedAt: now})
	if err != nil {
		slog.Warn("memory: where Mem0 keeps a claim was not recorded", "claim", claimID, "err", err)
	}
}

// mem0Delete drops claims from Mem0 by the ids it keeps them under, and
// marks their rows deleted.
func (s *Service) mem0Delete(ctx context.Context, lane memory.Projection, claimIDs []string) error {
	var rows []model.MemoryBackend
	var ext []string
	for _, id := range claimIDs {
		b, err := s.o.Repos.MemoryState.Backend(ctx, string(memory.LaneMem0), id)
		if isNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
		if b.Status == model.BackendDeleted || b.ExternalID == "" {
			continue
		}
		rows, ext = append(rows, b), append(ext, b.ExternalID)
	}
	return s.mem0DeleteRows(ctx, lane, rows, ext)
}

// mem0DeleteRows deletes Mem0 memories and marks their backend rows deleted.
func (s *Service) mem0DeleteRows(ctx context.Context, lane memory.Projection, rows []model.MemoryBackend,
	ext []string) error {
	if len(ext) == 0 {
		return nil
	}
	if err := lane.Delete(ctx, ext); err != nil {
		return err
	}
	now := s.now()
	for _, b := range rows {
		b.Status, b.Error, b.UpdatedAt = model.BackendDeleted, "", now
		if err := s.o.Repos.MemoryState.SetBackend(ctx, b); err != nil {
			return err
		}
	}
	return nil
}

// healthEntry is a lane's cached health.
type healthEntry struct {
	key string
	at  time.Time
	err error
}

// laneHealth asks a lane whether it answers, at most once a minute.
func (s *Service) laneHealth(ctx context.Context, name memory.LaneName, key string,
	check func(context.Context) error) error {
	now := s.now()
	s.stateMu.Lock()
	h, ok := s.health[name]
	s.stateMu.Unlock()
	if ok && h.key == key && now.Sub(h.at) < healthTTL {
		return h.err
	}
	cctx, cancel := context.WithTimeout(ctx, healthTimeout)
	err := check(cctx)
	cancel()
	s.stateMu.Lock()
	s.health[name] = healthEntry{key: key, at: now, err: err}
	s.stateMu.Unlock()
	return err
}

// laneStatuses reports each lane: Gwen's own store first, then the derived
// lanes, on or off.
func (s *Service) laneStatuses(ctx context.Context, ls laneSet, claims int, ledgerOK bool) []memory.LaneStatus {
	m := s.cfg()
	_, detail := s.learning()
	out := []memory.LaneStatus{{Lane: laneGwen, Enabled: true, OK: ledgerOK, Items: claims, Detail: detail}}

	vs := memory.LaneStatus{Lane: memory.LaneVec, Items: -1}
	switch {
	case ls.vec != nil:
		vs.Enabled, vs.Detail = true, m.EmbedModel+" at "+m.EmbedEndpoint
		if err := s.laneHealth(ctx, memory.LaneVec, ls.vecKey, ls.vec.Health); err != nil {
			vs.Detail = err.Error()
		} else {
			vs.OK = true
		}
		if n, err := ls.vec.Count(ctx); err == nil {
			vs.Items = n
		}
	case ls.bad[memory.LaneVec] != "":
		vs.Enabled, vs.Detail = true, ls.bad[memory.LaneVec]
	}
	out = append(out, vs)

	ms := memory.LaneStatus{Lane: memory.LaneMem0, Items: -1}
	switch {
	case ls.mem0 != nil:
		ms.Enabled, ms.Detail = true, m.Mem0URL
		if err := s.laneHealth(ctx, memory.LaneMem0, ls.mem0Key, ls.mem0.Health); err != nil {
			ms.Detail = err.Error()
		} else {
			ms.OK = true
		}
		if rows, err := s.o.Repos.MemoryState.Backends(ctx, string(memory.LaneMem0), model.BackendSynced); err == nil {
			ms.Items = len(rows)
		}
	case m.Enabled && m.Mem0URL != "":
		ms.Enabled, ms.Detail = true, "privacy keeps memory on this device; allow hosted connectors to use "+m.Mem0URL
	}
	out = append(out, ms)

	ss := memory.LaneStatus{Lane: memory.LaneSupermemory, Items: -1}
	switch {
	case ls.sm != nil:
		ss.Enabled, ss.Detail = true, m.SupermemoryURL
		if err := s.laneHealth(ctx, memory.LaneSupermemory, ls.smKey, ls.sm.Health); err != nil {
			ss.Detail = err.Error()
		} else {
			ss.OK = true
		}
		if n, err := ls.sm.Count(ctx); err == nil {
			ss.Items = n
			if n >= smWarn {
				ss.Detail = fmt.Sprintf("%s; it holds %d of the %d documents its free server takes", ss.Detail, n, smCap)
			}
		}
	case m.Enabled && m.SupermemoryURL != "":
		ss.Enabled = true
		ss.Detail = "privacy keeps memory on this device; allow hosted connectors to use " + m.SupermemoryURL
	}
	out = append(out, ss)

	fs := memory.LaneStatus{Lane: memory.LaneFiles, Items: -1}
	switch {
	case ls.files != nil:
		s.stateMu.Lock()
		ferr := s.filesErr
		s.stateMu.Unlock()
		fs.Enabled, fs.OK, fs.Detail = true, ferr == "", ls.files.Dir()
		if ferr != "" {
			fs.Detail = ferr
		}
	case ls.bad[memory.LaneFiles] != "":
		fs.Enabled, fs.Detail = true, ls.bad[memory.LaneFiles]
	}
	return append(out, fs)
}

// setFilesErr records how the last projection of the folder went.
func (s *Service) setFilesErr(err error) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.filesErr = ""
	if err != nil {
		s.filesErr = err.Error()
	}
}

// filesState is what local_state keeps of the folder: where it was last
// written and when.
type filesState struct {
	Dir         string `json:"dir"`
	ProjectedAt int64  `json:"projected_at"`
	Claims      int    `json:"claims"`
}

// saveFilesState records a projection.
func (s *Service) saveFilesState(ctx context.Context, dir string, claims int) {
	b, err := json.Marshal(filesState{Dir: dir, ProjectedAt: s.now().UnixMilli(), Claims: claims})
	if err == nil {
		err = store.SetLocal(ctx, s.o.DB.SQL(), store.KeyMemoryFilesState, string(b), s.now())
	}
	if err != nil {
		slog.Warn("memory: the folder's state was not recorded", "err", err)
	}
}
