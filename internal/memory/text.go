package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Tokens estimates how many tokens text costs a model: about four ASCII
// characters to a token, and a token for every other character, which errs
// high for accented Latin and is close for CJK.
func Tokens(text string) int {
	ascii, other := 0, 0
	for _, r := range text {
		if r < utf8.RuneSelf {
			ascii++
		} else {
			other++
		}
	}
	return (ascii+3)/4 + other
}

// Normalize folds text for comparing words: lower case, typographic quotes
// and dashes made plain, and every run of spaces one space.
func Normalize(text string) string {
	text = strings.Map(func(r rune) rune {
		switch r {
		case '‘', '’', '‚', '‛', '′':
			return '\''
		case '“', '”', '„', '‟', '″':
			return '"'
		case '–', '—', '−':
			return '-'
		case ' ':
			return ' '
		}
		return unicode.ToLower(r)
	}, text)
	return strings.Join(strings.Fields(text), " ")
}

// ContainsQuote reports whether quote appears in text, word for word, after
// Normalize; an empty quote never does.
func ContainsQuote(text, quote string) bool {
	q := strings.Trim(Normalize(quote), " .,;:!?\"'")
	return q != "" && strings.Contains(Normalize(text), q)
}

// ClaimHash is a claim's content hash: its subject, key, and value, so the
// same fact said twice has the same hash.
func ClaimHash(subjectID *string, key string, value json.RawMessage) string {
	subject := ""
	if subjectID != nil {
		subject = *subjectID
	}
	return Hash(subject + "\x00" + key + "\x00" + normalizeValue(value))
}

// Hash is the hex SHA-256 of s.
func Hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// normalizeValue folds a JSON value so that "Acme" and " acme " are the same
// fact: strings are normalized, and everything else is re-encoded compactly.
func normalizeValue(v json.RawMessage) string {
	var x any
	if err := json.Unmarshal(v, &x); err != nil {
		return Normalize(string(v))
	}
	var fold func(any) any
	fold = func(x any) any {
		switch t := x.(type) {
		case string:
			return Normalize(t)
		case []any:
			for i := range t {
				t[i] = fold(t[i])
			}
			return t
		case map[string]any:
			for k := range t {
				t[k] = fold(t[k])
			}
			return t
		}
		return x
	}
	b, err := json.Marshal(fold(x))
	if err != nil {
		return Normalize(string(v))
	}
	return string(b)
}

// Cut shortens text to at most n characters, at a word when one is near, with
// an ellipsis when it cut.
func Cut(text string, n int) string {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) <= n || n < 2 {
		return text
	}
	r := []rune(text)[:n-1]
	s := string(r)
	if i := strings.LastIndexByte(s, ' '); i > len(s)*2/3 {
		s = s[:i]
	}
	return strings.TrimRight(s, " ,.;:-") + "…"
}
