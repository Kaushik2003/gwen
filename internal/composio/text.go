package composio

import (
	"html"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	htmlHint     = regexp.MustCompile(`(?i)<(html|body|div|p|br|table|span|a\s|img|td|tr|li|ul|ol|h[1-6]|style|font|b|i|strong|em)[\s>/]`)
	htmlDrop     = regexp.MustCompile(`(?is)<(script|style|head|title)[^>]*>.*?</(script|style|head|title)>|<!--.*?-->`)
	htmlBreak    = regexp.MustCompile(`(?i)<br\s*/?>|</(p|div|tr|li|h[1-6]|table|blockquote|section|article|header|footer)>`)
	htmlBullet   = regexp.MustCompile(`(?i)<li[^>]*>`)
	htmlCell     = regexp.MustCompile(`(?i)</t[dh]>`)
	htmlTag      = regexp.MustCompile(`(?s)<[^>]+>`)
	spaces       = regexp.MustCompile(`[ \t\f\v\x{00a0}\x{200b}\x{200c}\x{200d}\x{feff}]+`)
	blankLines   = regexp.MustCompile(`\n{3,}`)
	invisibleRun = regexp.MustCompile(`[\x{034f}\x{00ad}\x{2007}\x{200b}-\x{200f}]+`)
)

// looksLikeHTML reports whether text is HTML markup rather than plain text
// that mentions an address in angle brackets.
func looksLikeHTML(s string) bool { return htmlHint.MatchString(s) }

// htmlToText takes the tags out of HTML and keeps its line breaks, list
// bullets and entities as plain text.
func htmlToText(s string) string {
	s = htmlDrop.ReplaceAllString(s, "")
	s = htmlBreak.ReplaceAllString(s, "\n")
	s = htmlBullet.ReplaceAllString(s, "\n- ")
	s = htmlCell.ReplaceAllString(s, " ")
	s = htmlTag.ReplaceAllString(s, "")
	return tidy(html.UnescapeString(s))
}

// tidy trims each line, folds runs of spaces and of blank lines, and drops
// the invisible characters mail templates pad previews with.
func tidy(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = invisibleRun.ReplaceAllString(s, "")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(spaces.ReplaceAllString(l, " "))
	}
	return strings.TrimSpace(blankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}

// oneLine is text on a single line.
func oneLine(s string) string {
	return strings.TrimSpace(spaces.ReplaceAllString(strings.ReplaceAll(tidy(s), "\n", " "), " "))
}

// cut shortens s to n runes, with an ellipsis when it was longer.
func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// capRunes keeps a snapshot's text within n runes and says so when it cut.
func capRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	const note = "\n\n[Cut here: the rest is in the app.]"
	r := []rune(s)
	return strings.TrimSpace(string(r[:n-utf8.RuneCountInString(note)])) + note
}

func joinNonEmpty(sep string, parts ...string) string {
	out := parts[:0:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}

// cap1 capitalises the first letter.
func cap1(s string) string {
	if s == "" {
		return s
	}
	r, n := utf8.DecodeRuneInString(s)
	return strings.ToUpper(string(r)) + s[n:]
}

// parseTime reads an RFC 3339 time, with or without fractions of a second.
func parseTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}
