package agent

import (
	"path/filepath"
	"slices"
	"strings"
)

// Command classes, strongest first (plan.md 3.16).
const (
	classFloor       = "floor"
	classDestructive = "destructive"
	classExternal    = "external"
	classShell       = "shell.run"
)

// Command is a shell command as the policy sees it: its strongest class, the
// prefix a remembered grant may match, and whether it is simple enough for a
// grant (one part, no redirection).
type Command struct {
	Class  string
	Prefix string
	Simple bool
	// Plain: nothing in it a word list can be fooled by: no expansions,
	// quotes, escapes, globs, redirections, or operators. Only a plain
	// command may run without asking, even in Trusted (plan.md 3.16).
	Plain bool
	// Why says what made it floor, destructive, or external.
	Why string
}

// twoWordPrefix are the commands whose prefix is their first two words.
var twoWordPrefix = []string{"git", "go", "npm", "pnpm", "yarn", "npx", "bun", "deno", "node", "cargo", "make",
	"python", "python3", "uv", "pip", "docker", "gh", "kubectl"}

// splitParts cuts a command at ;, &&, ||, |, &, newlines, $( and backticks.
// It cuts inside quotes too, so quoting cannot hide an operator: a part may
// then hold half a quoted string, which only ever makes the class stronger.
func splitParts(cmd string) []string {
	var parts []string
	var b strings.Builder
	flush := func() {
		if p := strings.TrimSpace(b.String()); p != "" {
			parts = append(parts, p)
		}
		b.Reset()
	}
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case c == ';' || c == '\n' || c == '`' || c == '(' || c == ')':
			flush()
		case c == '&' || c == '|':
			flush()
			if i+1 < len(cmd) && cmd[i+1] == c {
				i++
			}
		case c == '$' && i+1 < len(cmd) && cmd[i+1] == '(':
			flush()
			i++
		default:
			b.WriteByte(c)
		}
	}
	flush()
	return parts
}

// words splits a part into words, dropping quotes and leading NAME=value
// assignments.
func words(part string) []string {
	f := strings.Fields(part)
	out := make([]string, 0, len(f))
	for _, w := range f {
		w = strings.Trim(w, `"'`)
		if w == "" {
			continue
		}
		if len(out) == 0 && strings.Contains(w, "=") && !strings.HasPrefix(w, "-") && !strings.ContainsAny(w, "/") {
			continue // FOO=bar before the command
		}
		out = append(out, w)
	}
	return out
}

// base is a command word without its folder: /usr/bin/rm is rm.
func base(w string) string { return filepath.Base(w) }

// ClassifyCommand classifies a command the agent asks to run. home is the
// user's home folder, runtime Gwen's runtime folder ($XDG_RUNTIME_DIR/gwen),
// and gwenBin the gwen binary.
func ClassifyCommand(cmd, home, runtime, gwenBin string) Command {
	parts := splitParts(cmd)
	out := Command{Class: classShell}
	rank := map[string]int{classShell: 0, classExternal: 1, classDestructive: 2, classFloor: 3}
	var real []string
	for _, p := range parts {
		class, why := classifyPart(p, home, runtime, gwenBin)
		if rank[class] > rank[out.Class] {
			out.Class, out.Why = class, why
		}
		if w := words(p); len(w) > 0 && w[0] != "cd" {
			real = append(real, p)
		}
	}
	inline := false
	if len(real) > 0 {
		out.Prefix, inline = prefixOf(words(real[0]))
	}
	// Inline code (python -c, node -e, bash -c) runs anything, so no grant may cover it.
	out.Simple = len(real) == 1 && !inline && !strings.ContainsAny(cmd, "<>")
	out.Plain = out.Simple && plain(cmd)
	return out
}

// plain reports whether a command has none of the shell's ways to say one
// thing and mean another, which a word list cannot see through.
func plain(cmd string) bool {
	if strings.ContainsAny(cmd, "$`'\"\\*?[]{}~<>|;&()!#\n\t") {
		return false
	}
	for _, w := range strings.Fields(cmd) {
		switch base(w) {
		case "eval", "source", ".", "exec", "env", "xargs", "nohup", "setsid", "timeout", "nice", "base64", "xxd", "openssl":
			return false
		}
	}
	return true
}

// prefixOf is the prefix a remembered grant matches: the first word, or the
// first two for the tools in twoWordPrefix, and an interpreter's module or
// script with it. inline says the command runs code given on its line.
func prefixOf(w []string) (string, bool) {
	b := base(w[0])
	if len(w) > 1 {
		switch b {
		case "python", "python3", "node", "deno", "bun", "ruby", "perl", "bash", "sh", "zsh", "fish":
			switch w[1] {
			case "-c", "-e", "--eval", "-p", "--print", "-":
				return w[0] + " " + w[1], true
			case "-m":
				if len(w) > 2 {
					return w[0] + " -m " + w[2], false
				}
			}
		}
		if slices.Contains(twoWordPrefix, b) && !strings.HasPrefix(w[1], "-") {
			return w[0] + " " + w[1], false
		}
	}
	return w[0], false
}

// classifyPart is one part's class and why.
func classifyPart(part, home, runtime, gwenBin string) (string, string) {
	w := words(part)
	if len(w) == 0 {
		return classShell, ""
	}
	if why := floorWords(w, home, runtime, gwenBin); why != "" {
		return classFloor, why
	}
	if why := destructiveWords(w); why != "" {
		return classDestructive, why
	}
	if why := externalWords(w); why != "" {
		return classExternal, why
	}
	return classShell, ""
}

// floorWords says why a part touches what nothing may: becoming root,
// secrets, Gwen herself, or what runs on its own later.
func floorWords(w []string, home, runtime, gwenBin string) string {
	for _, x := range w {
		switch base(x) {
		case "sudo", "su", "doas", "pkexec", "run0":
			return "it runs as another user"
		}
	}
	if b := base(w[0]); b == "gwen" || b == "gwend" || (gwenBin != "" && w[0] == gwenBin) {
		return "it runs Gwen herself"
	}
	for _, x := range w {
		if why := floorPath(expand(x, home), runtime); why != "" {
			return why
		}
	}
	return ""
}

// expand turns ~ and $HOME into the home folder.
func expand(p, home string) string {
	switch {
	case p == "~" || p == "$HOME" || p == "${HOME}":
		return home
	case strings.HasPrefix(p, "~/"):
		return home + p[1:]
	case strings.HasPrefix(p, "$HOME/"):
		return home + p[5:]
	case strings.HasPrefix(p, "${HOME}/"):
		return home + p[7:]
	}
	return p
}

// floorPath says why a path is off limits, "" when it is not. It matches
// anywhere in a word, so an option like --file=~/.ssh/key counts too.
func floorPath(p, runtime string) string {
	secret := []string{"/.ssh", "/.gnupg", "/.claude.json", "/.claude/.credentials.json", "/.config/gwen",
		"/.local/share/gwen/credentials", "/.local/share/keyrings", "/.password-store", "/.netrc", "/.aws/credentials",
		"/.docker/config.json", "/.kube/config"}
	for _, s := range secret {
		if strings.Contains(p, s) {
			return "it touches your keys or credentials"
		}
	}
	for _, s := range []string{"gwend.sock", "hook.sock", "agent.sock", "remote.sock"} {
		if strings.Contains(p, s) {
			return "it reaches Gwen's sockets"
		}
	}
	if runtime != "" && strings.Contains(p, runtime) {
		return "it reaches Gwen's runtime folder"
	}
	for _, s := range []string{".git/hooks", ".git/config", "/.claude/settings", "/.config/systemd", "/.config/autostart",
		"/.bashrc", "/.bash_profile", "/.profile", "/.zshrc", "/.zprofile", "/.config/fish/config.fish"} {
		if strings.Contains(p, s) {
			return "it changes what runs by itself later"
		}
	}
	return ""
}

// destructiveWords says why a part deletes or discards something.
func destructiveWords(w []string) string {
	has := func(s ...string) bool {
		for _, x := range w[1:] {
			if slices.Contains(s, x) {
				return true
			}
		}
		return false
	}
	for i, x := range w {
		switch b := base(x); b {
		case "rm", "rmdir", "unlink", "shred", "truncate", "dd", "kill", "pkill", "killall", "wipefs":
			return "it deletes or stops something (" + b + ")"
		default:
			if strings.HasPrefix(b, "mkfs") {
				return "it formats a disk"
			}
		}
		if (base(x) == "find") && (has("-delete") || slices.Contains(w[i:], "-exec") && slices.ContainsFunc(w[i:],
			func(s string) bool { return base(s) == "rm" })) {
			return "it deletes the files it finds"
		}
	}
	b := base(w[0])
	sub := ""
	if len(w) > 1 {
		sub = w[1]
	}
	switch b {
	case "git":
		switch {
		case sub == "reset" && has("--hard", "--keep", "--merge"):
			return "it throws away changes (git reset)"
		case sub == "clean":
			return "it deletes untracked files (git clean)"
		case sub == "checkout" && (has("--", ".", "-f", "--force")):
			return "it throws away changes (git checkout)"
		case sub == "restore" && !has("--staged") || sub == "restore" && has("--worktree", "-W"):
			return "it throws away changes (git restore)"
		case sub == "branch" && has("-D", "-d", "--delete"):
			return "it deletes a branch"
		case sub == "stash" && has("drop", "clear"):
			return "it deletes stashed changes"
		case sub == "push" && has("--force", "-f", "--force-with-lease", "--delete", "-d"):
			return "it overwrites or deletes a remote branch"
		case sub == "tag" && has("-d", "--delete"):
			return "it deletes a tag"
		}
	case "chmod", "chown", "chgrp":
		if has("-R", "--recursive") {
			return "it changes permissions recursively"
		}
	case "docker", "podman":
		if has("rm", "rmi", "prune") {
			return "it deletes containers or images"
		}
	}
	return ""
}

// externalWords says why a part leaves this machine in your name.
func externalWords(w []string) string {
	b := base(w[0])
	sub := ""
	if len(w) > 1 {
		sub = w[1]
	}
	has := func(s ...string) bool {
		for _, x := range w[1:] {
			for _, p := range s {
				if x == p || strings.HasPrefix(x, p+"=") {
					return true
				}
			}
		}
		return false
	}
	switch b {
	case "git":
		if sub == "push" || sub == "send-email" {
			return "it sends commits to a remote"
		}
	case "gh":
		if len(w) > 2 {
			read := []string{"view", "list", "status", "diff", "checks", "ls"}
			if sub == "api" {
				if has("-X", "--method", "-f", "-F", "--field", "--raw-field", "--input") {
					return "it writes through GitHub's API"
				}
				return ""
			}
			if !slices.Contains(read, w[2]) {
				return "it changes something on GitHub"
			}
		}
	case "curl":
		for i, x := range w {
			if (x == "-X" || x == "--request") && i+1 < len(w) && !strings.EqualFold(w[i+1], "GET") {
				return "it sends a request that changes something"
			}
			if strings.HasPrefix(x, "-X") && len(x) > 2 && !strings.EqualFold(x[2:], "GET") {
				return "it sends a request that changes something"
			}
		}
		if has("-d", "--data", "--data-binary", "--data-raw", "--data-urlencode", "-F", "--form", "-T", "--upload-file", "--json") {
			return "it uploads data"
		}
	case "wget":
		if has("--post-data", "--post-file", "--method", "--body-data", "--body-file") {
			return "it uploads data"
		}
	case "ssh", "scp", "sftp":
		return "it connects to another machine"
	case "rsync":
		for _, x := range w[1:] {
			if !strings.HasPrefix(x, "-") && strings.Contains(x, ":") && !strings.HasPrefix(x, "/") {
				return "it copies to another machine"
			}
		}
	case "npm", "pnpm", "yarn", "bun", "cargo", "gem", "twine", "poetry":
		if sub == "publish" || sub == "upload" || sub == "push" {
			return "it publishes a package"
		}
	case "docker", "podman":
		if sub == "push" || sub == "login" {
			return "it pushes an image"
		}
	}
	return ""
}
