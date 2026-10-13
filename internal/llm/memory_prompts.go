package llm

// The prompts of the memory jobs. None talks to the user, so none opens with
// the persona; each asks for a single JSON object and includes its schema
// verbatim. A change to any of them changes MemoryPromptVersion.

var (
	extractJob    = prompt{name: "memory_extract", system: extractPrompt, schema: ExtractSchema}
	verifyJob     = prompt{name: "memory_verify", system: verifyPrompt, schema: VerifySchema}
	observeJob    = prompt{name: "memory_observe", system: observePrompt, schema: ObserveSchema}
	reflectJob    = prompt{name: "memory_reflect", system: reflectPrompt, schema: ReflectSchema}
	contradictJob = prompt{name: "memory_contradict", system: contradictPrompt, schema: ContradictSchema}
	insightJob    = prompt{name: "memory_insight", system: insightPrompt, schema: InsightSchema}
)

// ExtractSchema is the JSON Schema of an extraction reply. The value is a
// string because a strict schema cannot hold an object of any shape; an
// object written inside it as JSON is read as an object.
const ExtractSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["facts"],
  "properties": {
    "facts": {
      "type": "array",
      "maxItems": 40,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["source", "quote", "subject", "key", "updates", "value", "text", "valid_from", "valid_to", "occurred_at", "sensitivity", "confidence"],
        "properties": {
          "source": {"type": "string", "pattern": "^i[0-9]+$", "description": "the ref of the item the fact comes from"},
          "quote": {"type": "string", "description": "the exact words of the source item that state the fact, at most 200 characters"},
          "subject": {"type": "string", "pattern": "^(user|e[0-9]+|(person|org|place|topic):.+)$", "description": "user, a known entity's ref such as e2, or kind:Name for a new one, such as person:Anna"},
          "key": {"type": "string", "pattern": "^[a-z][a-z0-9_]*(\\.[a-z0-9_]+){1,4}$"},
          "updates": {"type": "string", "pattern": "^(k[0-9]+)?$", "description": "the ref of the known claim this fact replaces, or an empty string"},
          "value": {"type": "string", "description": "the bare value: a name, a place, a date as YYYY-MM-DD, or a few words"},
          "text": {"type": "string", "description": "a short profile line, the user implied and never named"},
          "valid_from": {"type": "string", "pattern": "^([0-9]{4}-[0-9]{2}-[0-9]{2})?$", "description": "YYYY-MM-DD, the first day it holds, or an empty string"},
          "valid_to": {"type": "string", "pattern": "^([0-9]{4}-[0-9]{2}-[0-9]{2})?$", "description": "YYYY-MM-DD, the last day it holds, or an empty string"},
          "occurred_at": {"type": "string", "pattern": "^([0-9]{4}-[0-9]{2}-[0-9]{2})?$", "description": "YYYY-MM-DD, the day the event happens, or an empty string"},
          "sensitivity": {"type": "string", "enum": ["public", "personal", "sensitive"]},
          "confidence": {"type": "number", "description": "from 0 to 1"}
        }
      }
    }
  }
}`

const extractPrompt = `You keep the long-term memory of a personal assistant, Gwen. You read items, messages between the user and Gwen or notes the user wrote, and write down the facts in them that are likely to matter in later conversations.

The user message is a JSON object: today, now, and timezone; keys, the vocabulary of fact keys, each with its label, its cardinality (one: one value at a time; many: any number of values), and whether it is about the user or about an entity; entities, the people, organisations, places, and topics already known, each with a ref such as e2, its kind, its name, and its aliases; known, facts already held, each with a ref such as k3, its key, its subject, its text, and since when it holds; context, earlier messages (c1, c2, ...) that come before the items; items, what you read (i1, i2, ...), each with its speaker (user, gwen for the assistant, or document for text others wrote), the local time it was written (at), and its text; and explicit.

What to keep:
- Facts about the user and their life that will still matter in a week or a year: their name and what they like to be called, who they are, their work and study, where they live, the people in their life and how they relate to them, birthdays and other dates, what they like and dislike, routines and habits, ambitions, struggles, and plans with dates.
- Health, beliefs (such as religion or politics), and money only when the user volunteers them, always with sensitivity sensitive.
- Facts about the people, organisations, places, and topics in their life, such as a friend's job or a sister's birthday.

What not to keep:
- Small talk, greetings, jokes, hypotheticals, and passing moods.
- Questions, and what Gwen suggested or offered.
- Requests that are tasks for the planner ("add a task to call the bank", "move my gym session to 6"): the planner already keeps them. A plan with a date that says something about their life ("I fly to Berlin on 3 May") is a fact.
- Anything about Gwen herself.
- Passwords, card or account numbers, government ids, codes, keys, and every other secret: never output them at all, not even in part.
- Facts that known already holds unchanged.

Read facts only from items; context is there to understand them, such as what a "yes" answers. Each fact comes from one item, its source. Write only what the item states or clearly implies, never a guess.

For each fact:
- source: the ref of the item it comes from.
- quote: the exact contiguous words of the source item that state the fact, at most 200 characters, copied character for character, with the same spelling, case, and punctuation. Prefer quoting the user. When the user only confirms what Gwen said ("yes, Monday"), quote the user's words. A fact that only Gwen stated may be returned with Gwen's words as the quote; it will be checked with the user.
- subject: "user" for the user; a known entity's ref (e2) when the item means that entity, by name or alias; or "kind:Name" for a new person, org, place, or topic, such as "person:Anna" or "org:Acme", never the user.
- key: one from keys when one fits; otherwise user.fact for the user, person.fact for a person (org.fact, place.fact, topic.fact for the others), or a new lowercase dotted key under user, pref, person, org, place, or topic, such as pref.sport. The subject must fit the key: keys under user and pref are about the user, and a key under person, org, place, or topic is about an entity of that kind.
- updates: a one-valued key holds one value at a time, so when the fact replaces a known claim (a new employer, a move to another city), set updates to that claim's ref. Also set it when the fact corrects or replaces a known claim of a many-valued key. Otherwise it is an empty string.
- value: the bare value as a short string: a name, a place, a role, a date as YYYY-MM-DD (or MM-DD for a yearly date without a year, such as a birthday), or a few words. When the value has parts, write it as a JSON object inside the string, such as "{\"start\": \"09:00\", \"end\": \"17:30\"}".
- text: a short profile line that says the fact on its own. The user is implied and never named, and "their" means the user's; other subjects are named. Examples: "Works at Acme as a backend intern", "Anna is their sister", "Anna's birthday is 3 May", "Prefers to study in the morning".
- valid_from, the first day the fact holds; valid_to, the last day it holds; occurred_at, the day an event happened or will happen: each as YYYY-MM-DD when the item states or implies it, else an empty string. Resolve relative dates ("last Monday", "next month", "in two weeks") against the item's at, never against today.
- sensitivity: public for what anyone could know (a public job, a city), personal for most facts about their life, sensitive for health, beliefs, money, sexuality, and anything else they would not want shared.
- confidence: from 0 to 1, how surely the item states this as true: 0.9 or more for a plain statement, lower for hedged words ("I might move"), a joke, or a guess.

Split a statement into one fact per key. For example, the user item i1 at 2026-10-07 18:20, "Started at Acme last Monday as a backend intern. My sister Anna visits on the 12th!", holds four facts:
- user.employer, subject "user", value "Acme", text "Works at Acme", quote "Started at Acme last Monday", valid_from "2026-10-05"
- user.role, subject "user", value "backend intern", text "Works as a backend intern", quote "as a backend intern", valid_from "2026-10-05"
- person.relation, subject "person:Anna", value "sister", text "Anna is their sister", quote "My sister Anna"
- user.plan, subject "user", value "Anna visits", text "Their sister Anna visits on 12 Oct", quote "My sister Anna visits on the 12th", occurred_at "2026-10-12"

When explicit is true, the user typed every item to be remembered: extract every fact they state, even a small one, with user.fact when no other key fits.

Return {"facts": []} when nothing is worth keeping, which is common.

Reply with a single JSON object and nothing else, matching this JSON Schema:
` + ExtractSchema

// VerifySchema is the JSON Schema of a verification reply.
const VerifySchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["verdicts"],
  "properties": {
    "verdicts": {
      "type": "array",
      "maxItems": 40,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["ref", "supported", "quote"],
        "properties": {
          "ref": {"type": "string", "pattern": "^v[0-9]+$"},
          "supported": {"type": "string", "enum": ["yes", "partly", "no"]},
          "quote": {"type": "string", "description": "when the check's quote is empty, the words of its context that state the claim; otherwise an empty string"}
        }
      }
    }
  }
}`

const verifyPrompt = `You check facts before a personal assistant, Gwen, keeps them in her memory.

The user message is a JSON object: checks, each with a ref such as v1; the claim, a fact about the user or someone in their life (a claim about the user leaves them unnamed, so "Works at Acme" means the user works at Acme); the quote it was drawn from; the speaker of the quote (user, gwen for the assistant, or document for text others wrote); and the context, the message before it and the text the quote is in.

For each check, decide whether the quote, read with its context, supports the claim:
- yes: it states the claim or clearly implies it, as something true, not as a joke, a question, a wish, a hypothetical, or something denied.
- partly: it supports only part of the claim, or the claim says more than the words do, such as a date, a name, or a detail that is not there.
- no: it does not support the claim, contradicts it, or is about someone else.

Judge only the words given, not what is likely. Read "I", "my", and "you" by who is speaking: when the speaker is gwen, "you" is the user. A short answer such as "yes" supports what it answers in the message before it.

When a check's quote is an empty string, find the exact words of its context that state the claim and copy them into quote, character for character, at most 200 characters; when there are none, answer no. Otherwise quote is an empty string.

Return one verdict for every check, with its ref.

Reply with a single JSON object and nothing else, matching this JSON Schema:
` + VerifySchema

// ObserveSchema is the JSON Schema of an observer reply.
const ObserveSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["observations"],
  "properties": {
    "observations": {
      "type": "array",
      "maxItems": 30,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["date", "priority", "text"],
        "properties": {
          "date": {"type": "string", "pattern": "^[0-9]{4}-[0-9]{2}-[0-9]{2}$", "description": "YYYY-MM-DD, the day of the message it comes from"},
          "priority": {"type": "string", "enum": ["high", "medium", "low"]},
          "text": {"type": "string", "description": "one sentence, at most 300 characters"}
        }
      }
    }
  }
}`

const observePrompt = `You keep the observation log of one conversation between the user and their personal assistant, Gwen. The oldest messages of the conversation are about to leave her view, so you fold them into short, dated observations that she reads instead.

The user message is a JSON object: today; log, the observations already written, one per line as "- YYYY-MM-DD [priority] text", oldest first, or an empty string; and messages, the messages to fold, oldest first, each with its role (user, or assistant for Gwen), the local time it was written (at), and its text.

Write observations of what will matter for the rest of this conversation and for later ones:
- what the user said, decided, committed to, asked for, and felt, and the facts about them and their life;
- what Gwen did (the changes she says she made) and what she promised.

Each observation is one plain sentence of at most 300 characters that makes sense on its own, dated with the day of the message it comes from. The log is Gwen's own notes, so write "they" for the user and "you" for Gwen, such as "They decided to drop the Rust course; you moved its tasks to someday." Be specific: name the tasks, people, dates, times, and numbers, and write dates as dates, resolved against the message's day.

priority is high for decisions, commitments, deadlines, and facts about them; medium for the rest of what matters, such as requests, what Gwen did, and how they felt; and low for small talk. Skip pleasantries, and anything the log already holds.

Return the new observations oldest first, or {"observations": []} when nothing is new.

Reply with a single JSON object and nothing else, matching this JSON Schema:
` + ObserveSchema

// ReflectSchema is the JSON Schema of a reflector reply.
const ReflectSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["log"],
  "properties": {
    "log": {"type": "string", "description": "the condensed log, one observation per line as - YYYY-MM-DD [priority] text, oldest first, at most 9000 characters"}
  }
}`

const reflectPrompt = `You condense the observation log of a conversation between the user and their personal assistant, Gwen. The log is Gwen's own notes on the part of the conversation she no longer sees, and it has grown too long. Rewrite it to at most 9,000 characters so that it still tells her everything that matters.

The user message is a JSON object: today, and log, one observation per line as "- YYYY-MM-DD [priority] text", oldest first, where priority is high, medium, or low.

Rules:
- Keep the same line format, one observation per line, oldest first.
- Keep the high-priority lines; shorten their words only when you must.
- Merge lines that repeat or continue each other into one line, with the date of the newest of them and the highest priority among them.
- Drop lines that later lines make obsolete, such as a plan that changed, a task that was finished, or a question that was answered; keep the newest state.
- When the log is still too long, drop low-priority lines first, then medium ones.
- Never add anything that is not in the log.

Reply with a single JSON object and nothing else, matching this JSON Schema:
` + ReflectSchema

// ContradictSchema is the JSON Schema of a contradiction scan's reply.
const ContradictSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["conflicts"],
  "properties": {
    "conflicts": {
      "type": "array",
      "maxItems": 20,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["a", "b", "reason"],
        "properties": {
          "a": {"type": "string", "pattern": "^k[0-9]+$"},
          "b": {"type": "string", "pattern": "^k[0-9]+$"},
          "reason": {"type": "string", "description": "one short sentence on why both cannot be true now"}
        }
      }
    }
  }
}`

const contradictPrompt = `You check the memory of a personal assistant, Gwen, for facts that contradict each other.

The user message is a JSON object: today, and claims, the facts she holds, each with a ref such as k4, its key, its subject (who or what it is about), its text (a claim about the user leaves them unnamed), and since and until, the days it holds from and to, or empty strings.

Return the pairs of claims that cannot both be true now, each named once by refs a and b, with a short reason. For example: two current employers when the claims suggest only one job, two different birthdays for one person, or two home cities at the same time.

These are not contradictions: different details of the same thing ("Works at Acme" and "Works as a backend intern"), facts about different people, facts that hold at different times (one ended before the other started), and things that can be true together (two hobbies, two languages).

Return {"conflicts": []} when there are none, which is common.

Reply with a single JSON object and nothing else, matching this JSON Schema:
` + ContradictSchema

// InsightSchema is the JSON Schema of an insight reply.
const InsightSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["insights"],
  "properties": {
    "insights": {
      "type": "array",
      "maxItems": 5,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["key", "value", "text", "basis", "confidence"],
        "properties": {
          "key": {"type": "string", "pattern": "^(pref\\.work|user\\.routine|user\\.struggle)(\\.[a-z0-9_]+)*$"},
          "value": {"type": "string", "description": "the pattern itself, in a few words"},
          "text": {"type": "string", "description": "a short profile line, the user implied and never named"},
          "basis": {"type": "string", "description": "the numbers from data that show it"},
          "confidence": {"type": "number", "description": "from 0 to 1"}
        }
      }
    }
  }
}`

const insightPrompt = `You look for patterns in a person's own tracked data, for the memory of their personal assistant, Gwen.

The user message is a JSON object: today; known, what Gwen already knows about them, one statement each; and data, summaries of their tracked data: the days they worked against their target, the tasks they finished, how their goals are pacing, their energy check-ins, and the plans they rolled over.

Propose at most 5 insights that the data clearly supports and known does not already say: when and how they work best, a routine they keep, or a struggle that keeps coming back. For each insight:
- key: pref.work for how and when they work best, user.routine for a habit they keep, or user.struggle for something that keeps getting in their way, or a key under one of these, such as pref.work.time.
- value: the pattern itself in a few words, such as "mornings".
- text: a short profile line, the user implied and never named, such as "Works best before noon" or "Often rolls Friday plans over to Monday".
- basis: the numbers from data that show it, such as "met the target on 9 of 11 mornings and 2 of 10 afternoons".
- confidence: from 0 to 1, how clearly the data shows it. Below 0.5 is not worth proposing.

The user confirms every insight before Gwen keeps it, so be specific and fair, and never judge them. Do not guess causes that the data cannot show.

Return {"insights": []} when nothing is clear, which is common.

Reply with a single JSON object and nothing else, matching this JSON Schema:
` + InsightSchema
