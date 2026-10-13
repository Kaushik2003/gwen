package memory

import (
	"regexp"
	"slices"
	"strings"

	"github.com/kzark/gwen/internal/model"
)

// Subjects a key can be about.
const (
	SubjectUser   = "user"   // the user; claims.subject_id is NULL
	SubjectEntity = "entity" // a person, organisation, place, or topic
)

// Entity kinds (entities.kind), an open list checked here.
const (
	EntityPerson = "person"
	EntityOrg    = "org"
	EntityPlace  = "place"
	EntityTopic  = "topic"
)

// EntityKinds are the kinds an entity may have.
var EntityKinds = []string{EntityPerson, EntityOrg, EntityPlace, EntityTopic}

// KeySpec is a key of the vocabulary: what its claims are about, whether one
// value holds at a time, its label in the dashboard and the memory files, and
// whether its claims are sensitive by default.
type KeySpec struct {
	Key         string
	Label       string
	Cardinality string
	Subject     string
	Sensitive   bool
}

// Keys is the known vocabulary. Other keys are valid when they fall under a
// namespace (Namespaces) and are many-valued.
var Keys = []KeySpec{
	{"user.name", "Name", model.CardinalityOne, SubjectUser, false},
	{"user.preferred_name", "Likes to be called", model.CardinalityOne, SubjectUser, false},
	{"user.pronouns", "Pronouns", model.CardinalityOne, SubjectUser, false},
	{"user.birthday", "Birthday", model.CardinalityOne, SubjectUser, false},
	{"user.home", "Lives in", model.CardinalityOne, SubjectUser, false},
	{"user.timezone", "Time zone", model.CardinalityOne, SubjectUser, false},
	{"user.employer", "Works at", model.CardinalityOne, SubjectUser, false},
	{"user.role", "Works as", model.CardinalityOne, SubjectUser, false},
	{"user.school", "Studies at", model.CardinalityOne, SubjectUser, false},
	{"user.study", "Studies", model.CardinalityOne, SubjectUser, false},
	{"user.work_hours", "Usual working hours", model.CardinalityOne, SubjectUser, false},
	{"user.wake_time", "Usually wakes up", model.CardinalityOne, SubjectUser, false},
	{"user.sleep_time", "Usually goes to bed", model.CardinalityOne, SubjectUser, false},
	{"user.language", "Speaks", model.CardinalityMany, SubjectUser, false},
	{"user.ambition", "Wants to", model.CardinalityMany, SubjectUser, false},
	{"user.skill", "Is good at", model.CardinalityMany, SubjectUser, false},
	{"user.struggle", "Struggles with", model.CardinalityMany, SubjectUser, false},
	{"user.routine", "Routine", model.CardinalityMany, SubjectUser, false},
	{"user.pet", "Pet", model.CardinalityMany, SubjectUser, false},
	{"user.plan", "Plans", model.CardinalityMany, SubjectUser, false},
	{"user.health", "Health", model.CardinalityMany, SubjectUser, true},
	{"user.fact", "About you", model.CardinalityMany, SubjectUser, false},
	{"pref.likes", "Likes", model.CardinalityMany, SubjectUser, false},
	{"pref.dislikes", "Dislikes", model.CardinalityMany, SubjectUser, false},
	{"pref.work", "Works best", model.CardinalityMany, SubjectUser, false},
	{"pref.music", "Music", model.CardinalityMany, SubjectUser, false},
	{"pref.food", "Food", model.CardinalityMany, SubjectUser, false},
	{"pref.communication", "How to talk to them", model.CardinalityMany, SubjectUser, false},
	{"person.relation", "Is your", model.CardinalityOne, SubjectEntity, false},
	{"person.birthday", "Birthday", model.CardinalityOne, SubjectEntity, false},
	{"person.home", "Lives in", model.CardinalityOne, SubjectEntity, false},
	{"person.employer", "Works at", model.CardinalityOne, SubjectEntity, false},
	{"person.role", "Works as", model.CardinalityOne, SubjectEntity, false},
	{"person.contact", "Contact", model.CardinalityMany, SubjectEntity, true},
	{"person.fact", "Notes", model.CardinalityMany, SubjectEntity, false},
	{"org.fact", "Notes", model.CardinalityMany, SubjectEntity, false},
	{"place.fact", "Notes", model.CardinalityMany, SubjectEntity, false},
	{"topic.fact", "Notes", model.CardinalityMany, SubjectEntity, false},
}

// Namespaces are the first parts a key may have, and what they are about.
var Namespaces = map[string]string{
	"user":   SubjectUser,
	"pref":   SubjectUser,
	"person": SubjectEntity,
	"org":    SubjectEntity,
	"place":  SubjectEntity,
	"topic":  SubjectEntity,
}

// keyPattern is a dotted lowercase key of two to five parts.
var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z0-9_]+){1,4}$`)

// Spec returns key's spec: the known one, or for a valid key under a
// namespace a many-valued one labelled from its last part. ok is false for
// an invalid key.
func Spec(key string) (KeySpec, bool) {
	if i := slices.IndexFunc(Keys, func(k KeySpec) bool { return k.Key == key }); i >= 0 {
		return Keys[i], true
	}
	if !keyPattern.MatchString(key) {
		return KeySpec{}, false
	}
	ns, rest, _ := strings.Cut(key, ".")
	subject, ok := Namespaces[ns]
	if !ok {
		return KeySpec{}, false
	}
	last := rest[strings.LastIndexByte(rest, '.')+1:]
	label := strings.ReplaceAll(last, "_", " ")
	label = strings.ToUpper(label[:1]) + label[1:]
	return KeySpec{Key: key, Label: label, Cardinality: model.CardinalityMany, Subject: subject,
		Sensitive: ns == "user" && strings.HasPrefix(rest, "health")}, true
}

// EntityKindOf is the entity kind a key's subject must have: person for
// person.*, and so on; "" for keys about the user.
func EntityKindOf(key string) string {
	ns, _, _ := strings.Cut(key, ".")
	if Namespaces[ns] != SubjectEntity {
		return ""
	}
	return ns
}

// ValidEntityKind reports whether kind is an entity kind.
func ValidEntityKind(kind string) bool { return slices.Contains(EntityKinds, kind) }
