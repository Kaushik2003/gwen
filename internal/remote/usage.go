package remote

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

// Claude's usage limits (plan.md 3.15): Claude Code tells only a status line
// how much of the five-hour and weekly limits is used. Each session Gwen
// starts gets `gwen statusline` as its status line (statusLineSettings),
// which sends the numbers here as the hook event StatusLine and then runs
// your own status line, so the terminal looks as it did.

// usageKeepEvery is how often an unchanged reading is written to the
// database anyway, so a restart shows roughly how old it is.
const usageKeepEvery = 5 * time.Minute

// loadUsage reads the reading kept by the last run. s.mu is held or the
// service is not shared yet.
func (s *Service) loadUsage(ctx context.Context) {
	if s.db == nil {
		return
	}
	raw, err := store.GetLocal(ctx, s.db.SQL(), store.KeyClaudeUsage)
	if err != nil {
		return
	}
	var u wire.ClaudeUsage
	if json.Unmarshal([]byte(raw), &u) == nil {
		s.usage, s.usageKept = u, u
	}
}

// Usage is the newest reading of Claude's usage limits.
func (s *Service) Usage() wire.ClaudeUsage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usage
}

// takeUsage records a reading from a status line and tells the screens when
// it changed. s.mu is held.
func (s *Service) takeUsage(ctx context.Context, u *wire.ClaudeUsage) {
	if u == nil || (u.FiveHour == nil && u.SevenDay == nil) {
		return
	}
	now := s.clk.Now()
	next := wire.ClaudeUsage{FiveHour: cleanWindow(u.FiveHour), SevenDay: cleanWindow(u.SevenDay), At: now.UnixMilli()}
	changed := !sameWindow(next.FiveHour, s.usage.FiveHour) || !sameWindow(next.SevenDay, s.usage.SevenDay)
	s.usage = next
	if !changed && now.Sub(time.UnixMilli(s.usageKept.At)) < usageKeepEvery {
		return
	}
	s.usageKept = next
	if b, err := json.Marshal(next); err == nil && s.db != nil {
		if err := store.SetLocal(ctx, s.db.SQL(), store.KeyClaudeUsage, string(b), now); err != nil {
			slog.Warn("keep Claude's usage", "err", err)
		}
	}
	s.opts.Publish(wire.EventCodeUsage, next)
}

// cleanWindow keeps a share between 0 and 100.
func cleanWindow(w *wire.UsageWindow) *wire.UsageWindow {
	if w == nil {
		return nil
	}
	c := *w
	c.UsedPercent = min(max(c.UsedPercent, 0), 100)
	c.ResetsAt = max(c.ResetsAt, 0)
	return &c
}

func sameWindow(a, b *wire.UsageWindow) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// statusLineSettings is the --settings value that gives a session Gwen
// starts `gwen statusline` as its status line, keeping the padding and
// refresh of yours. "" when there is no gwen binary to run.
func (s *Service) statusLineSettings(dir string) string {
	if s.opts.GwenBin == "" {
		return ""
	}
	line := map[string]any{"type": "command", "command": shellQuote(s.opts.GwenBin) + " statusline"}
	if own := OwnStatusLine(s.home, dir); own != nil {
		for _, key := range []string{"padding", "refreshInterval"} {
			if v, ok := own[key]; ok {
				line[key] = v
			}
		}
	}
	b, err := json.Marshal(map[string]any{"statusLine": line})
	if err != nil {
		return ""
	}
	return string(b)
}

// OwnStatusLine is the status line you set for Claude Code in dir, as
// Claude Code picks it: the folder's .claude/settings.local.json, then its
// .claude/settings.json, then your settings.json ($CLAUDE_CONFIG_DIR or
// ~/.claude). nil when none sets one.
func OwnStatusLine(home, dir string) map[string]any {
	user := filepath.Join(home, ".claude")
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		user = d
	}
	var files []string
	if dir != "" {
		files = append(files, filepath.Join(dir, ".claude", "settings.local.json"), filepath.Join(dir, ".claude", "settings.json"))
	}
	files = append(files, filepath.Join(user, "settings.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var settings struct {
			StatusLine map[string]any `json:"statusLine"`
		}
		if json.Unmarshal(b, &settings) == nil && settings.StatusLine != nil {
			return settings.StatusLine
		}
	}
	return nil
}

// shellQuote quotes s for sh when it needs it.
func shellQuote(s string) string {
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '/' || c == '.' || c == '-' || c == '_') {
			out := "'"
			for _, c := range s {
				if c == '\'' {
					out += `'\''`
				} else {
					out += string(c)
				}
			}
			return out + "'"
		}
	}
	return s
}
