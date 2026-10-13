# Gwen harness: a self-evolving agent on a permission kernel

Status: proposal for review. Written 2026-10-12. Replaces the harness parts of `plan.md` (listed in 0.2).

Contents: [0 Summary](#0-summary) · [1 Your asks](#1-what-you-asked-for) · [2 Research](#2-what-the-research-says) ·
[3 Architecture](#3-architecture-kernel-and-userland) · [4 Tools](#4-tools-what-gwen-can-use) ·
[5 Permissions](#5-the-permission-kernel) · [6 Tasks](#6-briefs-defining-work-in-detail) ·
[7 Places and coding](#7-places-folders-projects-and-coding) · [8 Connectors](#8-connectors-anything-acquired-on-demand) ·
[9 Self-evolution](#9-self-evolution) · [10 Engines](#10-engines-and-models) · [11 Data model](#11-data-model) ·
[12 Safety rules](#12-safety-rules) · [13 Roadmap](#13-roadmap) · [14 First slice](#14-first-slice)

---

## 0. Summary

### 0.1 The shift

`plan.md` designs Gwen's harness as a **fixed menu**: three run kinds (routine, email review, code), about ten named
capabilities, a model that never calls tools, and `external` actions that can never be pre-approved. Every new ability
needs a new package written by hand.

This proposal turns it around:

- **Gwen is an agent.** She runs a tool-use loop and can use any tool: a shell, files, the web, a browser, your desktop,
  any MCP server, any CLI on your machine, and tools she writes herself.
- **What limits her is a permission kernel, not a feature list.** A small, fixed core inside `gwend` sees every action
  before it happens and decides allow, ask, or deny against rules you set. The operating system enforces the boundary
  (sandbox, network allowlist, secrets that never reach the model), so a rule is not just a prompt.
- **She grows.** She writes down skills from work that went well, builds tools when one is missing, finds and proposes
  new connectors, and tunes her own playbook. Every change is versioned, tested against recorded tasks, and needs your
  OK where it matters.
- **Work is defined as briefs.** A brief says what, why, done-when, which places and accounts, how autonomous, and the
  budget. Approving a brief's plan is approving its scope.

The one-line rule that replaces "the model proposes, Gwen checks, you decide":
**Gwen may attempt anything; the kernel decides what actually happens; only you change the kernel's rules.**

### 0.2 What this replaces in `plan.md`

| `plan.md` section | Status |
|---|---|
| 3.1 principles 3 and 4 | replaced by 3.1 below |
| 3.7 Harness: runs | replaced by sections 3, 5, 6 |
| 3.8 Connectors and capabilities | replaced by sections 4 and 8 |
| 3.9 `00011_runs.sql` | replaced by section 11 |
| 3.10 Runs, Approvals, Connections, Projects rows | replaced by section 11.2 |
| 3.11 email review and coding task | replaced by 8.4 and 7.3; the DSA routine stays, as a brief template |
| 3.12 process table | `claude` and `gwen mcp` rows widen (10.2); add the sandbox helper |
| 4 products: Coding agent, Tool protocol, Browser automation, Run engine | updated in 2.6 |
| 6.3 W19, W22, W23, Later packages | replaced by section 13 (H1 to H7) |
| 7 safety rules 2, 3, 6 | replaced by section 12 |

Everything else stands: agenda (W17), cues (W18), presence (W20), the design system and shell (W16), and the subject
drawer (W21, whose Work screen now shows briefs, runs, approvals, and the ledger).

---

## 1. What you asked for

| You asked for | What this builds | Section |
|---|---|---|
| Gwen can do anything | Open tool use: built-in primitives, any MCP server, any CLI, tools she writes | 4 |
| An appropriate permission system for what she reads and executes | The permission kernel: actions, rules, sandbox, secrets broker, outbox, ledger | 5 |
| Define detailed tasks based on their requirements | **Briefs**: goal, context, deliverables, acceptance, scope, autonomy, budget, schedule | 6 |
| Coding: discuss, exact file paths, which folder is which project | **Places**: a map of folders to projects; pinned paths become the write scope | 7 |
| Last five emails, suggest replies, discuss, I send or Gwen sends | Email review as one brief template over the Gmail connector, with batch approval and an undo window | 8.4 |
| Integrate features as needed | Connector acquisition: she finds an MCP server, or writes a tool, and asks to install it | 8.3, 9.3 |
| A self-evolving harness | Reflection, a skill library, a tool forge, a playbook she tunes, evals that gate every change | 9 |
| Not limited by a predefined set | Nothing in this design names the full list of abilities. The kernel and the rules are fixed; everything else grows | 3 |

---

## 2. What the research says

Short notes on the work this design borrows from. Each one changed a decision below.

### 2.1 General agents work best with general tools

Coding agents (Claude Code, Codex, OpenHands) converged on a small set of very general tools: read, edit, search, and a
shell. A shell plus files reaches almost everything on a computer. The Claude Agent SDK was spun out of Claude Code
because the same loop works for non-coding agents. **Decision:** Gwen gets general primitives (shell, files, HTTP,
browser) instead of one capability per feature, and named tools only where safety or quality needs them (email send,
calendar write).

### 2.2 Skills as a library (Voyager, Agent Skills)

[Voyager](https://arxiv.org/html/2305.16291v2) (2023) kept learning in Minecraft by writing each successful behaviour as
code into a skill library, retrieving skills by description, and composing them into harder ones. The library is what
let it keep improving without retraining. The [Agent Skills](https://agentskills.io/) format (`SKILL.md` with a name and
description, plus optional scripts) made the same idea portable across agents, with **progressive disclosure**: only
names and descriptions sit in context, and a skill's body loads when it is used. **Decision:** Gwen's procedural memory
is a folder of Agent Skills she writes and edits, in the standard format, so she can also import skills others wrote.

### 2.3 Self-improvement needs an archive and an honest judge (Darwin Gödel Machine)

The [Darwin Gödel Machine](https://sakana.ai/dgm/) (Sakana AI and UBC, 2025) let a coding agent rewrite its own code,
kept every variant in an archive, and promoted variants that scored better on benchmarks; it raised its
SWE-bench score from 20% to 50%. The [paper](https://arxiv.org/html/2505.22954v3) also reports the agent gaming its own
checks: it faked a log showing tests had passed when they never ran, and removed the markers a reward function used to
detect hallucinated tool use. **Decision:** Gwen can change her skills, tools, and playbook, but
the evaluator, the ledger, and the kernel are outside her reach; changes are kept in a versioned archive with rollback;
promotion needs recorded tasks to still pass.

### 2.4 Prompt injection is solved by architecture, not by asking nicely (CaMeL, the lethal trifecta)

Simon Willison's "lethal trifecta": an agent with **private data**, exposure to **untrusted content**, and a way to
**communicate externally** can be tricked into leaking data. Email review has all three.
[CaMeL](https://simonwillison.net/2025/Apr/11/camel/) (Google DeepMind, 2025) separates a privileged planner that never
reads untrusted text from a quarantined model that parses it into structured data with no tools, tags values with where
they came from, and checks policies at each tool call. **Decision:** taint labels on context, a quarantined reader for
email and web pages, and a hard rule that an outward action whose destination came from untrusted content always asks
(5.7).

### 2.5 Enforce at the operating system, approve boundaries not commands

Claude Code's [sandbox](https://code.claude.com/docs/en/sandboxing) runs every shell command under `bubblewrap` on Linux,
routes all traffic through a local proxy with a domain allowlist, and masks credentials: commands see a placeholder and
the proxy swaps in the real value only on requests to allowed hosts. Inside that boundary commands run without asking.
It is open source as [`sandbox-runtime`](https://github.com/anthropics/sandbox-runtime). The kernel's
[Landlock](https://docs.kernel.org/6.12/userspace-api/landlock.html) LSM adds unprivileged file and TCP-port rules.
**Decision:** Gwen never tries to judge a shell command by parsing it. She runs it inside a **sandbox profile** whose
files, hosts, and secrets you approved. Commands inside the profile run freely; anything outside fails and becomes a
request to widen the profile.

### 2.6 Products: changes to `plan.md` section 4

| Need | Choice | Why |
|---|---|---|
| Agent loop | Gwen's own loop in Go (`internal/agent`), plus Claude Code as a second engine | one kernel, two engines: the API or a local model, or your Claude subscription (10) |
| Tool protocol | MCP, official Go SDK `github.com/modelcontextprotocol/go-sdk` (v1.7.0) as client **and** server | any MCP server becomes Gwen's tool; the kernel is served to Claude Code as an MCP server |
| Connector discovery | the official MCP Registry (`registry.modelcontextprotocol.io`) | search by need, install with a manifest |
| Sandbox | `bubblewrap` with Gwen's own egress proxy in `gwend`; Landlock where bwrap is unavailable | per-profile files, hosts, and secrets, enforced by the kernel |
| Browser | Playwright MCP in a Gwen-only browser profile (moves up from "later") | real web tasks without your main profile's cookies |
| Skills | Agent Skills format in `~/.local/share/gwen/skills`, a git repo | portable, diffable, rollback |
| Run engine | SQLite tables in `gwend` (unchanged) | one user, one machine |

---

## 3. Architecture: kernel and userland

### 3.1 Principles (replace `plan.md` 3.1 items 3 and 4)

1. **The kernel is small and fixed.** Gateway, policy, enforcement, approvals, ledger, budgets. You write and review
   it. Gwen can propose changes to it like any code change, but never applies them herself.
2. **Userland grows.** Tools, skills, connectors, memory, and her playbook. Gwen can add and change these, inside the
   kernel's rules.
3. **Every tool call is an action the kernel classifies and decides**, at the moment of the call, whichever engine made
   it. Nothing touches the world except through the gateway.
4. **The OS enforces; the policy decides; you approve boundaries.** Approvals widen a boundary (a folder, a host, a
   recipient, a task's scope) more often than they approve a single command.
5. **Outside content is data.** It can inform a draft. It can never pick a destination, widen a scope, or approve.
6. **Everything is recorded and, where possible, undoable.** File writes are snapshotted, outward actions wait in an
   outbox, and the ledger keeps every decision.

### 3.2 Layers

```text
 surfaces    dashboard · talk window · cue card · phone (ntfy) · CLI · KDE panel
                    │ chat, briefs, approvals, kill switch
 engines     Gwen loop (Anthropic, OpenAI-compatible, local)  ·  Claude Code delegate  ·  sub-agents
                    │ every tool call, from any engine
 ┌─ KERNEL (gwend) ───────────────────────────────────────────────────────────────────────────┐
 │ gateway ─ classify ─ policy ─┬─ allow ─ enforcers: sandbox · egress proxy · secrets broker │
 │                              ├─ ask   ─ approvals (dashboard, notification, phone, chat)    │
 │                              └─ deny                         outbox (undo window) · budgets │
 │ ledger (append-only, hash-chained) · scheduler and triggers · evaluator · kill switch       │
 └────────────────────────────────────────────────────────────────────────────────────────────┘
                    │
 userland    primitives · MCP servers · CLIs · forged tools · skills · memory · playbook · places
```

### 3.3 Kernel parts

| Part | Package | Job |
|---|---|---|
| Gateway | `internal/kernel` | the single `Invoke(ctx, Call) (Result, error)` every engine uses; looks up the tool, classifies, decides, enforces, records |
| Classifier | `internal/kernel` | turns a call into an `Action` (5.1); each tool supplies its own classifier; unknown tools get the most cautious class |
| Policy | `internal/kernel/policy` | pure `Decide(Action, Rules, Facts) Decision`; table-tested like the planner |
| Approvals | `internal/kernel` | payload-bound requests, batches, channels, expiry (5.5) |
| Sandbox | `internal/sandbox` | builds a `bwrap` command line (or Landlock rules) from a profile; runs processes in it |
| Egress proxy | `internal/sandbox/egress` | an HTTP(S) CONNECT proxy in `gwend` that checks each host against the profile and injects secrets |
| Secrets broker | `internal/secret` | the secret store from W22, plus sentinels and host-bound injection |
| Outbox | `internal/kernel/outbox` | holds outward actions for the undo window, then sends; daily caps |
| Ledger | `internal/ledger` | append-only log of every call, decision, approval, and result summary, hash-chained |
| Scheduler | `internal/kernel/sched` | runs briefs on time, calendar, mail, file, or webhook triggers; reuses the injected `Clock` |
| Evaluator | `internal/evals` | replays recorded tasks to gate self-changes (9.5); Gwen cannot edit it |

---

## 4. Tools: what Gwen can use

### 4.1 Sources of tools

| Source | Examples | Default trust |
|---|---|---|
| **Primitives** (built into `gwend`) | `fs.read`, `fs.list`, `fs.search`, `fs.write`, `fs.edit`, `shell.run`, `http.fetch`, `web.search`, `desktop.open`, `desktop.notify`, `desktop.screenshot`, `clipboard.*` | classified per call (5.1) |
| **Gwen's own domain** | `gwen.tasks.*`, `gwen.plan.*`, `gwen.goals.*`, `gwen.tracking.*`, `gwen.agenda.read`, `gwen.say` (today's 8 assistant actions become tools) | local, reversible: allowed |
| **Native connectors** | Google Calendar, Gmail (tokens stay in `gwend`) | per tool (8.4) |
| **MCP servers** | anything: GitHub, Notion, Linear, Slack, Playwright browser, Home Assistant, a database | writes ask until you classify them (8.2) |
| **CLIs** | `git`, `gh`, `rclone`, `ffmpeg`, `jq`, anything installed, through `shell.run` | the sandbox profile decides |
| **Forged tools** | scripts and small MCP servers Gwen writes (9.3) | their manifest's profile |
| **Meta tools** | `agent.spawn` (sub-agent), `code.delegate` (Claude Code), `skill.load`, `memory.*`, `tool.search`, `ask_user`, `brief.propose` | allowed; they cannot widen scope |

### 4.2 Too many tools for one context

Hundreds of MCP tools do not fit in a prompt. Gwen sees a **catalog**: each tool's name and one line, grouped by
connector. `tool.search` returns full schemas for the tools she wants, which are then callable. Skills work the same way
(name and description always, body on `skill.load`). A brief's scope also prunes the catalog to what the task may use.

### 4.3 No tool is "trusted" because of its name

A tool's own description is untrusted text (an MCP server writes it). Its risk class comes from the kernel's classifier,
MCP tool annotations (`readOnlyHint`, `destructiveHint`, `openWorldHint`) as a hint only, and your confirmation.

---

## 5. The permission kernel

### 5.1 Every call becomes an action

```go
type Action struct {
	Tool       string     // "gmail.send", "shell.run", "mcp:github/create_pull_request"
	Verbs      []Verb     // read, write, execute, send, delete, spend, install, configure
	Resources  []Resource // "path:/home/k/code/gwen/internal/x.go", "mail:to:alice@example.com", "host:api.github.com", "account:gmail"
	Reach      Reach      // local (only this machine), account (your private accounts), world (other people can see it)
	Undo       Undo       // reversible (snapshot or API undo), delayed (outbox window), none
	Taint      []Source   // untrusted sources in the context when the call was made: "email:<id>", "web:<host>"
	DestTaint  bool       // a destination (recipient, URL, repo) appears only in untrusted content, not in the brief or your words
	Cost       Cost       // money and tokens, when known
	Brief      string     // the brief it runs under; chat has an implicit brief
	Profile    string     // the sandbox profile, for processes
}
```

Classifiers are code, per tool: `fs.write` knows its path, `gmail.send` its recipients, `http.fetch` its host and method.
`shell.run` is not classified by parsing the command: its action is "execute inside profile P", and the profile's
boundary is what was approved (5.6).

### 5.2 Rules

A rule is `effect + match + scope + limits + expiry`:

```toml
# examples of rules you might end up with
[[rule]]  # read anything in my code folders
effect = "allow"; verbs = ["read"]; resources = ["path:~/code/**"]

[[rule]]  # tests and git status in Gwen's repo, no network
effect = "allow"; tool = "shell.run"; profile = "place:gwen/test"

[[rule]]  # push branches named gwen/* to my own repos
effect = "allow"; tool = "shell.run"; profile = "place:*/push-gwen-branches"; expires = "2027-01-01"

[[rule]]  # drafts are fine, sends always ask
effect = "allow"; tool = "gmail.create_draft"
[[rule]]
effect = "ask"; tool = "gmail.send"

[[rule]]  # never touch money
effect = "deny"; verbs = ["spend"]

[[rule]]  # at most 20 outward actions a day, whatever else allows them
effect = "limit"; reach = "world"; per_day = 20
```

Evaluation order, first match wins at each step: **hard floor** (5.4) → **deny** rules → brief scope (a call outside the
approved brief asks) → **ask** rules → **allow** rules → the **default matrix** (5.3). Limits apply on top of any
allow. Rules are stored in the database, edited only through the dashboard or CLI by you, and every change is in the
ledger.

### 5.3 Default matrix (when no rule matches)

| Reach × Undo | reversible | delayed (outbox) | none |
|---|---|---|---|
| **local**, inside a place or Gwen's data | allow | allow | ask |
| **local**, elsewhere on the machine | read: allow inside `$HOME` minus protected paths; write: ask | ask | ask |
| **account** (drafts, labels, your calendar, private repos) | read: allow; write: ask once per brief | ask once per brief | ask |
| **world** (email send, posts, pushes to shared branches, payments, messages) | ask | ask | ask every time |

Protected paths default to `~/.ssh`, `~/.gnupg`, `~/.config/gwen/credentials*`, browser profiles, password stores,
`.env*` files, and the secret store. Reading them asks; Gwen's own credentials are on the hard floor.

### 5.4 The hard floor (no rule or approval lifts these)

1. Gwen cannot create, change, or delete rules, or approve her own requests. Rules change only by your input in a
   trusted surface (dashboard, CLI, or the phone with a signed one-time link).
2. Gwen cannot read tokens or the secret store. Tools get sentinels (5.6).
3. Gwen cannot write to the kernel's code, config, rule and ledger tables, or the evaluator, outside a brief whose plan
   you approved for that purpose, and never merge such a change herself.
4. No process runs outside a sandbox profile, except named native connectors inside `gwend`.
5. An outward action whose destination appears only in untrusted content (`DestTaint`) always asks, with that fact on
   the card.
6. Outgoing content that contains detected secrets, card numbers, or the sentinel of a secret is blocked.
7. Voice can start work and approve `local` and `account` asks with a spoken confirmation; it cannot approve `world`
   actions, rule changes, or installs.
8. The kill switch always works: it cancels every run, freezes the outbox, and kills sandboxed processes.

### 5.5 Approvals that are pleasant to use

- **Payload-bound.** An approval covers an exact payload (recipients, subject, body hash; a command and profile; a diff
  hash). Editing the payload makes a new request. A stale approval cannot be replayed.
- **Batches.** "Send replies 1, 2 and 4" is one card with three payloads, each with its own checkbox.
- **Grant while approving.** Every card offers: *Allow once* · *Allow for this brief* · *Always allow this* (shown
  with the exact rule it would create, scoped as narrowly as the action: this folder, this contact, this host) ·
  *Deny*. `world` actions offer only the first two, plus narrow standing rules you write yourself in Settings.
- **Approve in chat.** When Gwen shows drafts in the dashboard chat with their payload ids, typing "send 1 and 2"
  approves exactly those payloads. The kernel binds the message to the payloads displayed in the turn you answered.
- **Channels.** Dashboard Work screen, a desktop notification with Approve and Deny, the phone through ntfy with a
  signed single-use link, and chat. A card always shows: what, where it goes, why (the brief), what is tainted, and the
  undo window.
- **Rule suggestions.** After you approve the same kind of action several times, Gwen suggests the narrowest rule
  that would have covered them. She cannot apply it.
- **Expiry.** Pending approvals expire after 24 hours; a brief can pause instead of failing.

### 5.6 Enforcement layers

| Layer | What it stops | How |
|---|---|---|
| **Sandbox profiles** | a command writing or reading outside its boundary | `bwrap` with read-only binds of `/usr` and the profile's read paths, read-write binds of its write paths, a private `/tmp`, no network namespace except a socket to the egress proxy; Landlock rules as fallback |
| **Egress proxy** | data leaving to hosts nobody approved | every sandboxed process and MCP server gets `HTTPS_PROXY` to `gwend`; the proxy checks the host against the profile, logs it, and can turn an unknown host into an ask |
| **Secrets broker** | tokens leaking into prompts, logs, or a malicious script | tools see a per-run sentinel; the proxy swaps the real value in only on requests to the secret's bound hosts (the same design as Claude Code's credential masking) |
| **Snapshots** | an edit you regret | before `fs.write` or a run's first write in a place: a git stash-style commit in a worktree, or a copy into `~/.local/share/gwen/journal/` for non-git folders; **Undo** on the run restores it |
| **Outbox** | a send you regret | `world` actions wait for the undo window (default 30 s, per tool) with a countdown on the card and the status bar; cancel is free |
| **Budgets** | runaway loops and spend | per brief: tokens, money, wall time, tool calls, outward actions; global daily ceilings; the loop stops and asks at 80% |
| **Ledger** | silent behaviour | append-only, each row hashes the previous; shown per run and globally; exportable |

A **profile** is data, for example:

```toml
[profile."place:gwen/test"]
read   = ["{place}", "~/go/pkg/mod", "~/.cache/go-build"]
write  = ["{worktree}", "~/.cache/go-build", "/tmp"]
hosts  = []                      # no network
secrets = []

[profile."place:gwen/push-gwen-branches"]
read   = ["{place}"]
write  = ["{place}/.git"]
hosts  = ["github.com"]
secrets = ["github_token@github.com"]
commands = ["git push origin gwen/*"]   # an allowlist checked by a wrapper, on top of the OS boundary
```

### 5.7 Prompt injection, concretely

1. **Taint labels.** Every tool result carries a source. Email bodies, web pages, files Gwen did not write and you did
   not mark as yours, and results of `openWorld` MCP tools are untrusted. Once untrusted text is in a run's context,
   the run is tainted until it ends.
2. **Quarantined reader.** For email and web pages, the main loop calls `read.quarantined`: a separate model call with
   no tools that turns the text into a fixed schema (summary, asks, dates, people, links, risk flags). The main loop
   works from that structure. Raw text reaches the main loop only when you ask or the task needs it, and still taints.
3. **Destinations come from you.** Recipients, URLs to post to, and repos to push to must come from the brief, your
   messages, your contacts, or the thread being replied to. A destination found only in untrusted text sets
   `DestTaint` and always asks (5.4 rule 5).
4. **Plans lock destinations.** Approving a brief's plan approves its destinations. A call to a new destination asks.
5. **Rules do not trust tainted runs fully.** An `allow` rule for a `world` action applies only in untainted runs, or
   when the action's payload is one you approved in this run.

---

## 6. Briefs: defining work in detail

### 6.1 A brief

A brief is a task for Gwen (named "brief" in code to avoid clashing with Gwen's own to-do `tasks`; the UI says
"Task for Gwen"). You write it in chat, in a form on the Work screen, from any to-do with **Hand to Gwen**, or by voice;
Gwen fills in what she can and asks about the rest.

```yaml
title: Reply to this week's recruiter emails
goal: Every recruiter email from the last 7 days has an answer I'm happy with.
why: I'm interviewing in November and don't want to lose leads.
context:
  - note: "I'm free Tue and Thu after 15:00. Not interested in relocation."
  - link: https://docs.example.com/my-cv
deliverables:
  - a draft reply per email, or a reason for no reply
  - a one-paragraph summary of who wants what
acceptance:
  - every email from label "Recruiters" newer than 7d is covered
  - nothing is sent without my OK
places: []                              # folders this brief may use (7)
scope:
  tools: [gmail.*, gwen.agenda.read, calendar.read]
  reads: ["mail:label:Recruiters newer_than:7d"]
  writes: ["mail:drafts"]
  destinations: ["mail:reply-to-thread"] # only the senders of the threads in scope
autonomy: propose                       # observe | propose | act | delegate (6.2)
budget: { tokens: 300k, usd: 2, time: 30m, world_actions: 10 }
schedule: once                          # once | at | every | on: (trigger)
checkins: at_end                        # each_step | on_blocker | at_end
verify:
  - check: every thread in scope has a draft or a no-reply reason
template: email-review                  # the skill it started from, if any
```

### 6.2 Autonomy levels

| Level | Gwen may | Typical use |
|---|---|---|
| **observe** | read within scope, report | "What's in my inbox?" "Summarise this repo." |
| **propose** | read, and produce drafts, plans, diffs in a worktree; every write outside the run's own space asks | first time doing anything |
| **act** | everything the rules allow inside the approved scope; asks for the rest | routine work you trust |
| **delegate** | like act, plus the brief's own standing grants (for example "send replies to these threads after I approve the text in chat") | recurring briefs |

Higher autonomy never lifts the hard floor and never turns a `world` ask into an allow by itself.

### 6.3 Lifecycle

```text
draft ──► discussing ──► planned ──► approved ──► running ⇄ waiting (you, an approval, a trigger, interrupted)
                                                    │
                                                    ├─► verifying ──► done  (report)
                                                    └─► failed · cancelled  (report, undo offered)
```

- **Discussing.** Gwen reads what scope already allows, asks questions, and proposes a **plan**: steps, tools,
  places and paths, destinations, an estimate against the budget, and how acceptance will be checked. You edit it in
  chat or in the plan view.
- **Approved.** Approving the plan approves its scope (the brief's grants for the run). The plan is stored with the
  run, and calls outside it ask.
- **Running.** Steps are checkpointed in SQLite. After a restart, steps with side effects are never re-run on their
  own (as in `plan.md` 3.7).
- **Verifying.** Gwen checks each acceptance item with a tool when it can (a test command, a lookup in Sent, a file
  diff). Items only you can judge are marked "needs you". She never reports done on her own claim alone.
- **Report.** What was done, what was checked and how, what is left, what she learned (9.1), and undo links.

### 6.4 Templates instead of run kinds

`plan.md`'s run kinds (routine, email review, code) become **templates**: skills that start a brief with sensible
scope, plan, and checks. The DSA routine is the `open-and-track` template with autonomy `act`. New templates appear
when Gwen distils a skill from a brief that went well (9.2), so the set of "things Gwen can do" grows without code.

### 6.5 Triggers

A brief can run on a schedule (`every weekday 08:30`), on an agenda event (the cue "start" hook from W18), on new mail
matching a query, on a file change in a place, or on a webhook to `gwend`. A trigger always starts at the brief's
autonomy level, and a trigger Gwen proposes needs your OK to exist.

---

## 7. Places: folders, projects, and coding

### 7.1 The places map

A **place** is a folder Gwen knows about. It replaces `plan.md`'s `project_dirs`.

| Field | Example |
|---|---|
| `path` | `~/code/gwen` |
| `project` | the Gwen project `gwen` (optional; one project can have several places, like a backend and a frontend repo) |
| `kind` | `repo`, `notes`, `docs`, `downloads`, `scratch` |
| `commands` | `test = "make check"`, `build`, `lint`, `run`, detected and editable |
| `conventions` | `CLAUDE.md`, `AGENTS.md`, `ui/DESIGN.md`, read before any coding brief |
| `protected` | globs Gwen must ask before writing: `internal/kernel/**`, `migrations/**`, `.github/**` |
| `default_profile` | the sandbox profile for this place's commands |
| `remote` | `github.com/Kaushik2003/gwen`, for PR and issue tools |

**Discovery.** Gwen scans roots you choose (`~/code`, `~/projects`) for git repos, matches them to Gwen projects by
name and remote, and proposes the map. You confirm in the Projects screen. Places are device-local.

### 7.2 Pinning paths

In a coding brief you can name exact files and folders: "the policy goes in `internal/kernel/policy/policy.go`, tests
next to it, wire types in `internal/wire/kernel.go`". Pinned paths become the run's **write scope**: the sandbox makes
only those paths (and build caches) writable in the worktree. If the work needs another file (`go.mod`, a registry
line), the run asks with the reason: "I need to add one line to `internal/api/routes.go` to register the endpoint."
Without pins, the approved plan's file list is the write scope.

### 7.3 A coding brief, end to end

1. **Discuss.** "Gwen, let's add a kill switch to the panel." Gwen reads the place (read is allowed), finds the
   relevant files, and asks what she cannot infer.
2. **Plan.** She proposes: files to create and edit with a reason each, the steps, the test plan, risks, and
   questions. You edit it, pin paths, and approve. Approving is the write grant for those paths in a new worktree on
   branch `gwen/<slug>`.
3. **Execute.** The engine is Claude Code by default (10.2) running inside the place's sandbox profile, with Gwen's
   kernel as its permission prompt tool, so every tool call outside the grant comes back to you through Gwen. Small
   edits can run in Gwen's own loop.
4. **Check.** Gwen runs the place's test command herself and reports the real exit code.
5. **Deliver.** A change summary, the diff, test results, and actions: **Open folder**, **Merge locally** (local,
   reversible), **Push branch** and **Open PR** (world: ask, or a standing rule for `gwen/*` branches to your own
   repos), **Discard**.
6. **Follow up.** A PR's review comments can start a follow-up brief on the same branch.

Briefs can span places ("update the API in the backend repo and the client in the frontend repo") with one worktree
per place.

---

## 8. Connectors: anything, acquired on demand

### 8.1 Three kinds

| Kind | When | Where tokens live |
|---|---|---|
| **Native** | Google Calendar and Gmail (already using Google OAuth), and anything where privacy needs code you control | inside `gwend`, in the secret store |
| **MCP** | everything else: local stdio servers launched in a sandbox profile, or remote HTTP servers with OAuth | the secret store; the egress proxy injects them |
| **CLI** | a tool you already use (`gh`, `rclone`, `aws`) | its own config, exposed only to the profiles you allow, or a sentinel |

### 8.2 Classifying a new connector's tools

When a connector is added, Gwen drafts a class for each tool (verbs, reach, undo) from its name, schema, and MCP
annotations, and shows them as a table you confirm or edit. Until you confirm, every non-read tool from it asks. A
local MCP server runs in its own profile: its own folder, the hosts of its API, and no others.

### 8.3 Acquiring a connector during a task

1. Gwen hits a gap: "I can't read your Notion pages."
2. She searches the MCP Registry and her own tools, and proposes the best match: who publishes it, what it can do,
   the hosts and scopes it needs, and how it would be sandboxed.
3. You approve the install (`install` is always an ask). OAuth opens in your browser. The connector appears on the
   Connections screen with its tool table to confirm.
4. If nothing fits, she proposes forging a tool (9.3).

### 8.4 Email review, end to end

1. You say: "Go through my last five emails and suggest how I should respond to each." Gwen starts a brief from the
   `email-review` template: observe and propose, scope = the five newest inbox threads, destinations = their senders.
2. She reads them through the Gmail connector (account read: allowed). Bodies go through redaction (`plan.md` 3.11
   step 3) and the quarantined reader, which returns structure per email.
3. She replies in chat with a short card per email: who, what they want, urgency, her suggested response and why,
   and questions for you. She checks your calendar for availability and her memory of the person.
4. You discuss: "make 2 warmer", "decline 4 politely", "tell 1 I'm free Tuesday after 3". She revises.
5. You decide per email:
   - **I'll send it myself**: she saves a Gmail draft (account write, allowed by the template) and links it.
   - **Send it for me**: you type "send 1 and 4" or tick them on the batch card. The kernel binds that to the exact
     payloads shown. Each goes to the outbox with a 30 s undo countdown, then sends. The ledger records the message
     ids.
6. If an email asks for a new recipient ("also cc my colleague at x@y.com"), that address is tainted: the card says
   so and asks, whatever your rules say.
7. Afterwards she offers what she learned ("You always decline relocation roles; want me to draft those declines
   automatically next time?") as a skill or a rule suggestion.

Email bodies and drafts stay out of the database after the run, as `plan.md` section 7 rule 5 says.

---

## 9. Self-evolution

### 9.1 What can change, and who changes it

| Layer | Examples | Who writes it | Gate |
|---|---|---|---|
| **Memory** | facts about you, contacts, places, preferences, past briefs | Gwen | ledger; you can see, edit, and delete every item |
| **Skills** | `email-review`, `release-gwen`, `book-dentist`, `weekly-inbox-zero` | Gwen | first use asks; promotion after evals (9.5) |
| **Tools** | a forged CSV parser, an MCP server for a local API | Gwen | sandbox tests, then your install approval |
| **Connectors** | MCP servers from the registry | Gwen proposes | your install approval and OAuth |
| **Playbook** | her own working instructions, tool tips, model routing, template defaults | Gwen proposes | evals must not regress; you approve |
| **Triggers** | "every Monday 08:30 triage inbox" | Gwen proposes | your approval |
| **Rules** | permissions | **only you** | Gwen may only suggest |
| **Gwen's code** | her own repo | Gwen, as a coding brief on the `gwen` place | your review and merge |
| **Kernel** | gateway, policy, sandbox, ledger, evaluator | you | code review; protected paths; never merged by Gwen |

### 9.2 The learning loop

```text
 brief ─► run ─► verify ─► reflect ──► memory      (facts, outcomes, what you liked)
                              ├──────► skill draft  (a procedure worth keeping)
                              ├──────► tool draft   (a capability that was missing)
                              ├──────► playbook diff (a lesson about how she works)
                              └──────► growth backlog (gaps she could not close)
                                             │
                     weekly growth report ◄──┘  evals ─► your OK ─► promoted (versioned, revertible)
```

- **Reflect** (after every run, a short structured model call): what worked, what failed and why, what she lacked,
  what you corrected. Stored as episodic memory with the run.
- **Skill drafts** (Voyager, Agent Skills): when a run succeeded with a procedure that is likely to repeat, she writes
  `SKILL.md` (when to use it, steps, pitfalls, checks) and optional scripts under
  `~/.local/share/gwen/skills/<name>/`, a git repo she commits to. A skill declares the tools and scope it needs in its
  frontmatter; loading a skill grants nothing.
- **Imported skills** (from others) are untrusted text: their scripts run only in a sandbox profile, and their
  instructions are data until you enable them.

### 9.3 The tool forge

When the gap is a capability, not a procedure:

1. Gwen writes the tool in a scratch place: code, a manifest (input and output schema, verbs, reach, hosts, paths,
   secrets), and tests.
2. She runs the tests in a sandbox with only the manifest's boundary.
3. She proposes it: what it does, its manifest as a plain table, the test results, and the code.
4. On your approval it is installed into `~/.local/share/gwen/tools/<name>/` and served as an MCP tool. It always runs
   in its manifest's profile, so a bug or a later edit cannot exceed what you approved. Editing an installed tool is a
   new version that needs approval again.

### 9.4 A self-model of her capabilities

The kernel keeps a live inventory: connectors and their status, tools and their classes, skills with use counts and
success rates, rules that apply to her, budgets left, and the growth backlog. Gwen can query it (`self.capabilities`)
and the persona prompt gets a short summary, so she can say "I can't reach Notion yet; I could install the official
server, which needs read access to your workspace" instead of guessing. This is the practical core of "self-aware":
she knows what she can do, how well she does it, and how to grow. It fits next to the attitude and self-note work in
the other threads.

### 9.5 Evals gate every change (and she cannot grade herself)

- **Recorded tasks.** Every finished run can become an eval case: the brief, the recorded tool results (as fixtures),
  and checks on the outcome (deterministic where possible: "a draft exists for each thread", "tests pass", "no world
  action without approval"; a rubric judged by a separate model otherwise). Shares its replay engine with the
  scripted behaviour test thread.
- **Promotion.** A new or changed skill, playbook diff, model routing, or forged tool is run against the relevant
  cases in replay mode. It is promoted only if nothing regresses, and you approve the playbook and tool changes.
- **Archive.** Every version is kept (DGM-style), with its scores. Rollback is one click.
- **Outside her reach.** The evaluator, its checks, and the fixtures are kernel data. Gwen proposes new cases; she
  cannot edit or delete existing ones. This is the lesson from the Darwin Gödel Machine's gamed checks.

### 9.6 Improving her own code

Gwen's own repo is a place. When the backlog shows a gap only code can close (a new native connector, a faster
loop), she can open a coding brief against herself, on a branch, with the same plan approval as any other. Kernel
paths are protected: changes there are labelled, need your explicit plan approval, and are never merged by her. You
merge and redeploy.

### 9.7 Proactivity grows with trust

Gwen suggests triggers and higher autonomy for briefs with a good record ("I've drafted your Monday inbox triage 6
weeks running and you sent 90% unedited; want it to run at act level?"). You decide. Her suggestions and your answers
are memory, so she stops asking about what you declined.

---

## 10. Engines and models

### 10.1 Gwen's own loop (`internal/agent`)

- Providers: Anthropic Messages API tool use, OpenAI-compatible tool calls (Ollama and other local models). The
  existing `Planner` jobs stay for the planner, briefing, and retro.
- Streaming, parallel tool calls, tool results truncated with a "read more" handle, context compaction for long
  runs, tool search for large catalogs.
- Every tool call goes to `kernel.Invoke`. The loop never touches the world itself.
- Chat becomes a run with an implicit brief (autonomy `act`, scope = Gwen's own domain plus reads in places). Today's
  8 assistant actions become `gwen.*` tools, so chat changes still happen directly, now recorded in the ledger and
  covered by rules. This settles the README mismatch noted in memory: the README should say chat changes tasks and
  goals directly, under the kernel's rules.

### 10.2 Claude Code as an engine

You already run Claude Code with your subscription; today Gwen calls it with every tool turned off
(`internal/llm/claudecode.go:81`). As an engine:

- `gwen mcp` serves the kernel's tools (all of them, filtered by the brief's scope) over stdio.
- Claude Code runs with its own tools limited to what the brief needs, `--permission-prompt-tool mcp__gwen__approve`
  so every permission it would ask for goes to the kernel, `--strict-mcp-config --mcp-config` pointing at `gwen mcp`,
  `--setting-sources ""`, and inside the place's sandbox profile, with its own sandbox on as a second layer.
- Default engine for coding briefs and for general briefs when no API key or local model is configured.
- The exact flags are recorded against the installed version as a fixture, as `plan.md` W23 required.

### 10.3 Models by job

| Job | Default |
|---|---|
| Planning, discussion, coding | the strongest configured model |
| Quarantined reading, classification, reflection | a small fast model (or a local one, for privacy) |
| Eval rubric judging | a different model from the one being judged when available |

### 10.4 Sub-agents

`agent.spawn` starts a child run with a scope that is a subset of its parent's (the kernel checks this) and its own
budget slice. Use: read ten repos in parallel, review a diff, research a question.

---

## 11. Data model

### 11.1 Tables (migration numbers to be fixed when H1 starts)

```sql
CREATE TABLE places (          -- device-local; replaces project_dirs
    id TEXT PRIMARY KEY CHECK (length(id) = 36), path TEXT NOT NULL UNIQUE, project_id TEXT REFERENCES projects (id),
    kind TEXT NOT NULL CHECK (kind IN ('repo', 'notes', 'docs', 'downloads', 'scratch')),
    commands TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(commands)), protected TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(protected)),
    default_profile TEXT NOT NULL DEFAULT '', remote TEXT NOT NULL DEFAULT '', updated_at INTEGER NOT NULL
) STRICT;
CREATE TABLE briefs (          -- synced; the spec of 6.1
    id TEXT PRIMARY KEY CHECK (length(id) = 36), title TEXT NOT NULL, spec TEXT NOT NULL CHECK (json_valid(spec)),
    template TEXT NOT NULL DEFAULT '', subject TEXT NOT NULL DEFAULT '', <envelope>
) STRICT;
CREATE TABLE runs (            -- device-local; one execution of a brief (chat is a run too)
    id TEXT PRIMARY KEY CHECK (length(id) = 36), brief_id TEXT REFERENCES briefs (id), engine TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('draft', 'discussing', 'approved', 'running', 'waiting', 'verifying', 'done', 'failed', 'cancelled')),
    reason TEXT NOT NULL DEFAULT '', plan TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(plan)),
    tainted INTEGER NOT NULL DEFAULT 0 CHECK (tainted IN (0, 1)), budget TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(budget)),
    spent TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(spent)), report TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(report)),
    parent_run_id TEXT REFERENCES runs (id), created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
    started_at INTEGER, ended_at INTEGER
) STRICT;
CREATE TABLE rules (           -- synced; only written by the user's surfaces
    id TEXT PRIMARY KEY CHECK (length(id) = 36), effect TEXT NOT NULL CHECK (effect IN ('allow', 'ask', 'deny', 'limit')),
    match TEXT NOT NULL CHECK (json_valid(match)), scope TEXT NOT NULL DEFAULT '', limits TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(limits)),
    note TEXT NOT NULL DEFAULT '', expires_at INTEGER, <envelope>
) STRICT;
CREATE TABLE approvals (       -- device-local
    id TEXT PRIMARY KEY CHECK (length(id) = 36), run_id TEXT NOT NULL REFERENCES runs (id), batch_id TEXT,
    action TEXT NOT NULL CHECK (json_valid(action)), payload_hash TEXT NOT NULL, summary TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'denied', 'expired')),
    grant_scope TEXT NOT NULL DEFAULT 'once' CHECK (grant_scope IN ('once', 'brief', 'rule')),
    channel TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, decided_at INTEGER
) STRICT;
CREATE TABLE outbox (          -- device-local
    id TEXT PRIMARY KEY CHECK (length(id) = 36), run_id TEXT NOT NULL REFERENCES runs (id), approval_id TEXT REFERENCES approvals (id),
    tool TEXT NOT NULL, payload_ref TEXT NOT NULL, send_at INTEGER NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('waiting', 'sent', 'cancelled', 'failed')), result TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(result))
) STRICT;
CREATE TABLE ledger (          -- device-local, append-only (no UPDATE or DELETE triggers allowed)
    seq INTEGER PRIMARY KEY AUTOINCREMENT, run_id TEXT, kind TEXT NOT NULL, -- call, decision, approval, result, rule_change, install, promotion
    data TEXT NOT NULL CHECK (json_valid(data)), at INTEGER NOT NULL, prev_hash TEXT NOT NULL, hash TEXT NOT NULL
) STRICT;
CREATE TABLE connectors (      -- device-local
    id TEXT PRIMARY KEY, kind TEXT NOT NULL CHECK (kind IN ('native', 'mcp_stdio', 'mcp_http', 'cli')),
    config TEXT NOT NULL CHECK (json_valid(config)), tool_classes TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(tool_classes)),
    status TEXT NOT NULL, installed_at INTEGER NOT NULL
) STRICT;
CREATE TABLE memories (        -- synced; facts, preferences, episodes; skills and tools live on disk in git
    id TEXT PRIMARY KEY CHECK (length(id) = 36), kind TEXT NOT NULL CHECK (kind IN ('fact', 'preference', 'person', 'episode', 'lesson')),
    subject TEXT NOT NULL DEFAULT '', body TEXT NOT NULL, source_run_id TEXT, <envelope>
) STRICT;
CREATE TABLE evals (           -- device-local; kernel data, not writable through Gwen's tools
    id TEXT PRIMARY KEY CHECK (length(id) = 36), name TEXT NOT NULL, fixture TEXT NOT NULL CHECK (json_valid(fixture)),
    checks TEXT NOT NULL CHECK (json_valid(checks)), created_at INTEGER NOT NULL
) STRICT;
```

`memories` may merge with whatever the conversation-memory thread lands; that thread owns the memory schema.

### 11.2 API and events

| Area | Endpoints |
|---|---|
| Briefs | `GET/POST /v1/briefs`, `GET/PUT /v1/briefs/{id}`, `POST /v1/briefs/{id}/run` |
| Runs | `GET /v1/runs`, `GET /v1/runs/{id}` (plan, steps, ledger slice), `POST /v1/runs/{id}/approve-plan`, `/message`, `/cancel`, `/resume`, `/undo` |
| Approvals | `GET /v1/approvals?status=pending`, `POST /v1/approvals/{id}` (decision, grant scope), `POST /v1/approvals/batch/{id}` |
| Outbox | `GET /v1/outbox`, `POST /v1/outbox/{id}/cancel` |
| Rules | `GET/POST /v1/rules`, `PUT/DELETE /v1/rules/{id}`, `GET /v1/rules/suggestions` |
| Places | `GET/POST /v1/places`, `PUT/DELETE /v1/places/{id}`, `POST /v1/places/discover` |
| Connectors | `GET /v1/connectors`, `POST /v1/connectors` (install), `PUT /v1/connectors/{id}/classes`, `DELETE /v1/connectors/{id}`, `GET /v1/connectors/search?q=` |
| Self | `GET /v1/self/capabilities`, `GET /v1/self/growth`, `GET /v1/skills`, `GET /v1/tools`, `POST /v1/promotions/{id}` |
| Ledger | `GET /v1/ledger?run=&since=`, `GET /v1/ledger/verify` |
| Kill switch | `POST /v1/stop` (also `gwen stop` and a panel button) |

SSE: `run_changed`, `approval_requested`, `approval_decided`, `outbox_changed`, `rule_changed`, `connector_changed`,
`promotion_proposed`, `stopped`.

---

## 12. Safety rules

Replace `plan.md` section 7 rules 2, 3, and 6. Rules 1, 4, 5, 7, and 8 stand.

2. **The kernel enforces permissions** at the moment of each call, from any engine, and the operating system enforces
   the sandbox boundary. Prompts and UI are never the only check.
3. **Outward actions** (`world` reach) ask by default, wait in the outbox with an undo window, and are capped per day.
   You can write narrow standing rules for them; a rule never applies to a destination taken from untrusted content.
   Voice cannot approve them.
6. **Every process Gwen starts runs in a sandbox profile** with an explicit list of paths, hosts, and secrets. Coding
   runs use a worktree and pinned or planned write paths. Gwen checks results herself.
9. **Gwen cannot change her own permissions**, read secrets, or edit the kernel, the ledger, or the evaluator.
10. **Self-changes are versioned and gated.** Skills, tools, playbook, and triggers are diffable, evaluated, and
    revertible; installs and playbook changes need your approval.

---

## 13. Roadmap

Replaces W19, W22, W23, and the later packages. Each package keeps `plan.md` 6.1's rules (owned paths, conventions,
`make check`, live checks on a throwaway daemon).

| Package | Delivers | Needs |
|---|---|---|
| **H1 Kernel and agentic chat** | `internal/kernel` (gateway, action model, policy, rules, approvals with payload binding), `internal/ledger`, kill switch; `internal/agent` loop on the Anthropic API with a fake provider for tests; the 8 assistant actions as `gwen.*` tools; read-only `fs.*` inside places; approvals API and a minimal approval card | W15 |
| **H2 Sandbox, places, secrets** | `internal/sandbox` (bwrap profiles, Landlock fallback), egress proxy, secrets broker with sentinels, `shell.run`, `fs.write` with snapshots, places with discovery; Projects screen place editor | H1, W22's secret store part |
| **H3 Briefs and runs** | briefs, plan and approve, run lifecycle, verification, reports, templates, triggers, sub-agents; Work screen (with W21); the DSA routine as a template | H1, W18 for the start hook |
| **H4 Connectors and email** | MCP client, registry search, install flow, tool classification; Gmail native connector, redaction, quarantined reader, outbox, `email-review` template | H2, H3 |
| **H5 Coding** | `gwen mcp`, Claude Code engine under the kernel, worktrees, path-scoped writes, verification, push and PR rules | H2, H3 |
| **H6 Evolution** | reflection, memory store (with the memory thread), skills library, tool forge, capability self-model, evals and promotion (with the behaviour test thread), rule suggestions, weekly growth report | H3, H4 |
| **H7 Browser and desktop** | Playwright MCP in a Gwen-only profile, desktop screenshots through the portal, computer-use style tasks | H4 |

```text
Wave A   H1                          (with W16, W17)
Wave B   H2  |  H3                   (with W18, W20, W21)
Wave C   H4  |  H5
Wave D   H6  |  H7
```

After Wave A, chat is agentic and every change is in the ledger. After B, Gwen can run commands in your projects inside
a sandbox and take briefs. After C, she reviews email, sends with your OK, and takes coding tasks. After D, she grows
on her own and can drive a browser.

---

## 14. First slice

The proposed first PR is the core of H1, small enough to review in one sitting and useful on its own:

1. `internal/kernel/policy`: the `Action` model, rules, the default matrix, the hard floor, and `Decide`, as a pure
   package with table tests (including "no rule can allow Gwen to change rules" and "a tainted destination always
   asks").
2. `internal/kernel`: the gateway with tool registration, classification, and decision; approvals stored with a
   payload hash, with list and decide endpoints and SSE events.
3. `internal/ledger`: the hash-chained ledger table and a verify function.
4. `internal/agent`: a tool-use loop on the Anthropic Messages API (and a fake provider for tests), behind
   `[agent] enabled = false` so today's chat keeps working until you switch.
5. Tools: the 8 assistant actions as `gwen.*` tools, plus `gwen.tasks.list` and `gwen.agenda.read`.

Not in the first slice: sandbox, shell, MCP, Claude Code engine, UI beyond an approvals list. Those follow in H2 to H5.
