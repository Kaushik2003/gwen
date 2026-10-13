package recall

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kzark/gwen/internal/memory"
)

// Intent rules, read without a model. Each is tried in this order; the first
// that matches wins. A time window alone makes a query episodic only after
// the other rules, so "how long did I work last week" is about data.
var (
	lookupRE = regexp.MustCompile(`(?i)\bwhere do i (work|live)\b|\bwhat('s| is) my\b|\bwhen is .*birthday\b|` +
		`\bwho is\b|\bdo you (know|remember) (my|what|where|when|who)\b`)
	episodicRE = regexp.MustCompile(`(?i)\bwhat did (we|i)\b|\blast time\b|\bremember when\b|` +
		`\bwe (talked|discussed|decided)\b`)
	resourceRE = regexp.MustCompile(`(?i)\bthe (document|pdf|file|notes|lease|contract)\b`)
	preferRE   = regexp.MustCompile(`(?i)\bdo i like\b|\bmy favou?rite\b|\bprefer`)
	dataRE     = regexp.MustCompile(`(?i)\bhow (much|long|many)\b.*\b(work|worked|study|studied|tasks|hours)\b|` +
		`\bhours\b|\btracked\b`)
)

// intentOf reads what a query asks for; window is the time it names, if any.
func intentOf(text string, window *memory.TimeRange) string {
	switch {
	case lookupRE.MatchString(text):
		return memory.IntentLookup
	case episodicRE.MatchString(text):
		return memory.IntentEpisodic
	case resourceRE.MatchString(text):
		return memory.IntentResource
	case preferRE.MatchString(text):
		return memory.IntentPrefer
	case dataRE.MatchString(text):
		return memory.IntentData
	case window != nil:
		return memory.IntentEpisodic
	}
	return memory.IntentAny
}

// weights are the lanes' weights in fusion, by intent. A lane missing from an
// intent's row, or of weight 0, is not asked.
var weights = map[string]map[memory.LaneName]float64{
	memory.IntentLookup: {memory.LaneClaims: 2.0, memory.LaneEntity: 1.5, memory.LaneChats: 0.6,
		memory.LaneEpisodes: 0.6, memory.LaneTime: 0.5, memory.LaneData: 0.3, memory.LaneVec: 1.0,
		memory.LaneMem0: 0.8, memory.LaneSupermemory: 0.5},
	memory.IntentEpisodic: {memory.LaneClaims: 0.8, memory.LaneEntity: 0.8, memory.LaneChats: 1.5,
		memory.LaneEpisodes: 1.2, memory.LaneTime: 2.0, memory.LaneData: 0.3, memory.LaneVec: 1.0,
		memory.LaneMem0: 0.5, memory.LaneSupermemory: 0.5},
	memory.IntentResource: {memory.LaneClaims: 0.5, memory.LaneEntity: 0.5, memory.LaneChats: 0.5,
		memory.LaneEpisodes: 1.5, memory.LaneTime: 0.5, memory.LaneData: 0, memory.LaneVec: 1.0,
		memory.LaneMem0: 0.3, memory.LaneSupermemory: 2.0},
	memory.IntentPrefer: {memory.LaneClaims: 1.5, memory.LaneEntity: 1.0, memory.LaneChats: 0.6,
		memory.LaneEpisodes: 0.6, memory.LaneTime: 0.3, memory.LaneData: 0, memory.LaneVec: 1.5,
		memory.LaneMem0: 1.2, memory.LaneSupermemory: 0.3},
	memory.IntentData: {memory.LaneClaims: 0.5, memory.LaneEntity: 0.3, memory.LaneChats: 0.3,
		memory.LaneEpisodes: 0.3, memory.LaneTime: 0.5, memory.LaneData: 3.0, memory.LaneVec: 0.3,
		memory.LaneMem0: 0.2, memory.LaneSupermemory: 0},
	memory.IntentAny: {memory.LaneClaims: 1.2, memory.LaneEntity: 1.0, memory.LaneChats: 1.0,
		memory.LaneEpisodes: 1.0, memory.LaneTime: 0.8, memory.LaneData: 0.5, memory.LaneVec: 1.0,
		memory.LaneMem0: 0.8, memory.LaneSupermemory: 0.8},
}

// weight is a lane's weight for an intent; a lane the table does not name
// (another extra lane) weighs 1.
func weight(intent string, lane memory.LaneName) float64 {
	row, ok := weights[intent]
	if !ok {
		row = weights[memory.IntentAny]
	}
	if w, ok := row[lane]; ok {
		return w
	}
	return 1
}

// keyRule maps words in a query to the keys of the claims it asks about.
type keyRule struct {
	re   *regexp.Regexp
	keys []string
}

// keyRules are the phrase-to-key rules of the claims lane. Birthday is
// special: with a person named it is their birthday, otherwise the user's.
var keyRules = []keyRule{
	{regexp.MustCompile(`(?i)\b(work(s|ed|ing)?|jobs?|employer)\b`), []string{"user.employer", "user.role"}},
	{regexp.MustCompile(`(?i)\b(live[sd]?|living|home|city)\b`), []string{"user.home"}},
	{regexp.MustCompile(`(?i)\bnames?\b`), []string{"user.name"}},
	{regexp.MustCompile(`(?i)\b(school|stud(y|ies|ied|ying))\b`), []string{"user.school", "user.study"}},
}

var birthdayRE = regexp.MustCompile(`(?i)\bbirthdays?\b`)

// userKeys are the keys about the user that text asks about.
func userKeys(text string) []string {
	var keys []string
	for _, r := range keyRules {
		if r.re.MatchString(text) {
			keys = append(keys, r.keys...)
		}
	}
	return keys
}

// stopWords are words too common, or too much about time, to tell what a
// query is about.
var stopWords = func() map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(`about above after again all also and any are because been before being
		below between both but can cannot could did does doing don down during each few for from further get got
		had has have having her here hers herself him himself his how into its itself just let like made make
		many may more most much must myself not now off once only other our ours ourselves out over own really
		same she should some such than that the their theirs them themselves then there these they this those
		through too under until very was were what when where which while who whom why will with would you your
		yours yourself yourselves tell told remember know talk talked said say ago day days week weeks month
		months year today yesterday last this time monday tuesday wednesday thursday friday saturday sunday
		january february march april june july august september october november december`) {
		m[w] = true
	}
	return m
}()

// words are the distinct content words of text: lower case, three letters or
// more, not stop words, a trailing s dropped.
func words(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}) {
		if utf8.RuneCountInString(w) < 3 || stopWords[w] {
			continue
		}
		w = stem(w)
		if !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}

func stem(w string) string {
	if len(w) > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") {
		return w[:len(w)-1]
	}
	return w
}

// overlap counts the words of query found in text.
func overlap(query []string, text string) int {
	if len(query) == 0 {
		return 0
	}
	have := map[string]bool{}
	for _, w := range words(text) {
		have[w] = true
	}
	n := 0
	for _, w := range query {
		if have[w] {
			n++
		}
	}
	return n
}

// mentions reports whether name appears in text as whole words, ignoring
// case.
func mentions(text, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	re, err := regexp.Compile(`(?i)(^|[^\pL\pN])` + regexp.QuoteMeta(name) + `($|[^\pL\pN])`)
	return err == nil && re.MatchString(text)
}
