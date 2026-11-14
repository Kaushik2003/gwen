package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Gwen's summary of a commit (plan.md 3.15, History): what changed and why,
// in two sentences, from the Claude conversation that made it.

// CommitSummaryRequest is a commit's subject and the conversation that made
// it, shortened to its first prompt and the turns that led to the commit.
type CommitSummaryRequest struct {
	Subject      string `json:"subject"`
	Conversation string `json:"conversation"`
}

// CommitSummaryOutput is Gwen's summary of the commit.
type CommitSummaryOutput struct {
	Summary string `json:"summary"`
}

// MaxCommitSummary is the longest summary taken, in characters.
const MaxCommitSummary = 400

// CommitSummarySchema is the JSON Schema of a commit summary reply.
const CommitSummarySchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["summary"],
  "properties": {
    "summary": {"type": "string", "minLength": 1, "maxLength": 400}
  }
}`

const commitSummaryPrompt = `You summarise one git commit for the person who asked for it, from the coding conversation with Claude that made it.

The user message is a JSON object: subject, the commit's first line, and conversation, the person's first prompt and the turns that led to the commit ("You:" is the person, "Claude:" is Claude, "Tool:" is a tool Claude ran).

Write one or two plain sentences, at most 400 characters: what the commit changes for the person or their users, and why, when the conversation says why. Lead with the effect, not the files. Use the person's own words for things where you can. Do not invent anything the conversation does not say, and do not mention Claude, the conversation, or this summary.

Reply with a single JSON object and nothing else, matching this JSON Schema:
` + CommitSummarySchema

var commitSummaryJob = prompt{name: "commit_summary", system: commitSummaryPrompt, schema: CommitSummarySchema}

// CommitSummary asks the model for a commit's summary.
func (a *adapter) CommitSummary(ctx context.Context, req CommitSummaryRequest) (CommitSummaryOutput, error) {
	text, _, err := a.ask(ctx, commitSummaryJob, req)
	if err != nil {
		return CommitSummaryOutput{}, err
	}
	var out CommitSummaryOutput
	if err := strictDecode(text, &out); err != nil {
		return CommitSummaryOutput{}, fmt.Errorf("the reply is not the summary JSON: %w", err)
	}
	out.Summary = strings.TrimSpace(out.Summary)
	switch {
	case out.Summary == "":
		return CommitSummaryOutput{}, errors.New("the summary is empty")
	case len([]rune(out.Summary)) > MaxCommitSummary:
		return CommitSummaryOutput{}, fmt.Errorf("the summary is longer than %d characters", MaxCommitSummary)
	}
	return out, nil
}
