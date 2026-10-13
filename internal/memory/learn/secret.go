package learn

import (
	"math/big"
	"regexp"
	"strings"
	"unicode"
)

// Secrets are never stored (harness 4.5). The screen looks for what a
// password, a card, an account, or a key looks like in words.
var (
	// digitRun is a run of digits, each pair at most one space or dash apart.
	digitRun = regexp.MustCompile(`\d(?:[ -]?\d)*`)
	// ibanLike is an IBAN as written: a country, two check digits, and up to
	// 30 letters and digits, in groups of four or not.
	ibanLike = regexp.MustCompile(`\b[A-Z]{2}\d{2}(?: ?[A-Z0-9]{4}){2,7}(?: ?[A-Z0-9]{1,4})?\b`)
	// afterPassword is the word that follows a password, passcode, or PIN:
	// after "is", ":", or "=", any word of three or more characters; right
	// after the keyword, only one with a digit or a symbol, so "pin the tab",
	// "password manager", and "password isn't" are not secrets.
	afterPassword = regexp.MustCompile(`(?i)\b(?:password|passwort|passcode|passwd|pin(?:\s*code)?)\b` +
		`(?:(?:\s+(?:is|was|ist)\s+|\s*[:=]\s*)(\S{3,})|\s+(\S+))`)
	// keyPrefix is a token with the prefix of a well-known kind of API key.
	keyPrefix = regexp.MustCompile(`\b(?:sk-[A-Za-z0-9_-]{8,}|ghp_[A-Za-z0-9]{16,}|xox[a-z]-[A-Za-z0-9-]{8,}|` +
		`AKIA[0-9A-Z]{16})`)
	// keyLike is a long run of hex or base64 characters.
	keyLike = regexp.MustCompile(`[A-Za-z0-9+/_=-]{32,}`)
)

// hasSecret reports whether any of texts holds what looks like a secret.
func hasSecret(texts ...string) bool {
	for _, t := range texts {
		if cardNumber(t) || iban(t) || password(t) || apiKey(t) {
			return true
		}
	}
	return false
}

// cardNumber reports whether s holds a run of 13 to 19 digits that passes
// the Luhn check.
func cardNumber(s string) bool {
	for _, run := range digitRun.FindAllString(s, -1) {
		digits := strings.NewReplacer(" ", "", "-", "").Replace(run)
		if len(digits) >= 13 && len(digits) <= 19 && luhn(digits) {
			return true
		}
	}
	return false
}

func luhn(digits string) bool {
	sum := 0
	for i := len(digits) - 1; i >= 0; i-- {
		d := int(digits[i] - '0')
		if (len(digits)-1-i)%2 == 1 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	return sum%10 == 0
}

// iban reports whether s holds an IBAN whose check digits are right.
func iban(s string) bool {
	for _, m := range ibanLike.FindAllString(s, -1) {
		code := strings.ReplaceAll(m, " ", "")
		if len(code) >= 15 && len(code) <= 34 && ibanValid(code) {
			return true
		}
	}
	return false
}

// ibanValid is the ISO 13616 check: the first four characters moved to the
// end, letters made numbers (A is 10), and the whole mod 97 is 1.
func ibanValid(code string) bool {
	var b strings.Builder
	for _, r := range code[4:] + code[:4] {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteString(big.NewInt(int64(r - 'A' + 10)).String())
		default:
			return false
		}
	}
	n, ok := new(big.Int).SetString(b.String(), 10)
	return ok && new(big.Int).Mod(n, big.NewInt(97)).Int64() == 1
}

// password reports whether s gives a password, passcode, or PIN.
func password(s string) bool {
	for _, m := range afterPassword.FindAllStringSubmatch(s, -1) {
		if m[1] != "" {
			return true
		}
		word := strings.TrimRight(m[2], ".,;:!?)\"'")
		if strings.IndexFunc(word, func(r rune) bool {
			return !unicode.IsLetter(r) && r != '\'' && r != '’' && r != '-'
		}) >= 0 {
			return true
		}
	}
	return false
}

// apiKey reports whether s holds a token shaped like an API key: a known
// prefix, or 32 or more hex or base64 characters with letters and digits.
func apiKey(s string) bool {
	if keyPrefix.MatchString(s) {
		return true
	}
	for _, m := range keyLike.FindAllString(s, -1) {
		if strings.IndexFunc(m, unicode.IsDigit) >= 0 && strings.IndexFunc(m, unicode.IsLetter) >= 0 {
			return true
		}
	}
	return false
}
