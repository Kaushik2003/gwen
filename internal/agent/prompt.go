package agent

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed rules.md
var rules string

// The most of one CLAUDE.md file, and of all of them, the prompt carries
// (plan.md 3.16).
const (
	maxClaudeMD      = 12000
	maxClaudeMDTotal = 30000
)

// claudeMD is the CLAUDE.md files Claude Code would have read, which it
// skips with --setting-sources "": the user's own, then the folder's.
func claudeMD(home, folder string) string {
	var paths []string
	if home != "" {
		paths = append(paths, filepath.Join(home, ".claude", "CLAUDE.md"))
	}
	if folder != "" {
		paths = append(paths, filepath.Join(folder, "CLAUDE.md"), filepath.Join(folder, ".claude", "CLAUDE.md"),
			filepath.Join(folder, "CLAUDE.local.md"))
	}
	var b strings.Builder
	total := 0
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil || len(strings.TrimSpace(string(data))) == 0 {
			continue
		}
		text := strings.TrimSpace(string(data))
		if len(text) > maxClaudeMD {
			text = cutRunes(text, maxClaudeMD) + "\n\n[cut here: the file goes on]"
		}
		if total+len(text) > maxClaudeMDTotal {
			b.WriteString("\n\n[the remaining CLAUDE.md files were left out]")
			break
		}
		total += len(text)
		fmt.Fprintf(&b, "\n\n### %s\n\n%s", shortHome(p, home), text)
	}
	if b.Len() == 0 {
		return ""
	}
	return "## Instructions from CLAUDE.md files" + b.String()
}

// SystemPrompt is the prompt a chat's process starts with, in place of
// Claude Code's own: who she is, how she works, what she keeps in mind about
// the user, where the chat works, and the CLAUDE.md files (plan.md 3.16). It
// stays the same while the process runs, for the prompt cache.
func SystemPrompt(persona, card, place, home, folder, cwd string, today time.Time) string {
	env := fmt.Sprintf("## Environment\n\nYou run in %s on Linux. Today is %s.", cwd, today.Format("Monday 2 January 2006"))
	parts := []string{strings.TrimSpace(persona), strings.TrimSpace(rules), env}
	if card = strings.TrimSpace(card); card != "" {
		parts = append(parts, "## What you keep in mind about them\n\n"+card)
	}
	if place = strings.TrimSpace(place); place != "" {
		parts = append(parts, "## Where this chat works\n\n"+place)
	}
	if md := claudeMD(home, folder); md != "" {
		parts = append(parts, md)
	}
	return strings.Join(parts, "\n\n")
}

func shortHome(p, home string) string {
	if home != "" && strings.HasPrefix(p, home+"/") {
		return "~" + p[len(home):]
	}
	return p
}

func cutRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && (s[n]&0xC0) == 0x80 {
		n--
	}
	return s[:n]
}
