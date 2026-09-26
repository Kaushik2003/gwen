# Gwen PRD — index and change protocol

This folder is the complete specification for Gwen. It is written so that a coding agent can
implement any work package **without making architecture decisions and without asking questions**.
If you are an agent and you find yourself about to decide something architectural, that is a bug in
these docs — see [Change protocol](#change-protocol) below.

Gwen is a personal time-tracking and study-planning desktop app for Linux. See
[01-product.md](01-product.md) for what it is and why.

## The manifest is closed

These 13 files are the entire specification. **Do not create a fourteenth.**

| File | Authoritative for |
|---|---|
| [README.md](README.md) | this index, reading order, change protocol |
| [CONVENTIONS.md](CONVENTIONS.md) | Go style, errors, logging, naming, commits, agent rules |
| [01-product.md](01-product.md) | vision, user stories, glossary, non-goals, phase definitions |
| [02-architecture.md](02-architecture.md) | processes, repo layout, ports & adapters, frozen decisions |
| [03-data-model.md](03-data-model.md) | schema DDL, migrations, sync envelope |
| [04-api-contract.md](04-api-contract.md) | every endpoint, payload, error code, SSE event |
| [05-time-engine.md](05-time-engine.md) | state machine, idle/break rules, reclaim, recovery |
| [06-planner.md](06-planner.md) | goals→tasks, urgency formula, capacity fill, rollover |
| [07-integrations.md](07-integrations.md) | notifications, Google Calendar, LLM adapter, sync hub |
| [08-clients.md](08-clients.md) | GUI screens, CLI commands, tray menu |
| [09-packaging.md](09-packaging.md) | build, packaging, systemd, setup wizard, Pi deploy |
| [10-build-plan.md](10-build-plan.md) | work packages, dependency graph, definition of done |
| [11-testing.md](11-testing.md) | test strategy, golden scenarios, acceptance criteria |

The repository root `README.md` is **not** part of this set. It is code-facing (how to build and
run) and is not a specification.

## Reading order

First time through, read [01-product.md](01-product.md) then [02-architecture.md](02-architecture.md).
Those two give you the whole shape. Everything else is reference.

## Read only what your work package needs

Reading more than your package needs wastes context and tempts you to edit files you do not own.
Every package reads the docs in its row, **plus its own section of
[10-build-plan.md](10-build-plan.md) and its acceptance criteria in [11-testing.md](11-testing.md)**.
Follow a link into another doc only for the section it points at.

| Work package | Read |
|---|---|
| W0 skeleton | CONVENTIONS, 02, 03, 04 |
| W1 time engine | CONVENTIONS, 03, 05 |
| W2 activity monitor | CONVENTIONS, 02, 05 |
| W3 store | CONVENTIONS, 03, 04 |
| W4 notifications | CONVENTIONS, 03, 07 |
| W5 daemon + API | CONVENTIONS, 02, 04, 05 |
| W6 CLI | CONVENTIONS, 04, 08 |
| W7 tray | CONVENTIONS, 04, 08 |
| W8 GUI | CONVENTIONS, 04, 08 |
| W9 packaging + setup | CONVENTIONS, 02, 04, 09 |
| W10 planner backend | CONVENTIONS, 03, 04, 06 |
| W11 planner clients | CONVENTIONS, 04, 08 |
| W12 Google Calendar | CONVENTIONS, 03, 04, 07 |
| W13 LLM adapter | CONVENTIONS, 04, 07, 08 |
| W14 sync hub | CONVENTIONS, 03, 07, 09 |

## Change protocol

These rules exist because a spec that grows without bound stops being a spec. They are enforced
mechanically by `make docs-check`, which is part of CI and must pass before any work package is
considered done.

**1. The manifest is closed.** No new files in `docs/`. If something has no home, it belongs inside
an existing file — find the owner in the table above.

**2. One topic, one owner. Other docs link, they never restate.** If the idle threshold is written
in two places, one of them will eventually be wrong. Cross-reference as
`[05-time-engine.md](05-time-engine.md#thresholds)` instead of repeating the value. Duplicated
normative statements are a defect even when the copies currently agree.

**3. Edit in place; delete the superseded text in the same edit.** Docs are not append-only. A
change that adds a new rule without removing the old one leaves a contradiction for the next agent.

**4. No changelog, history, or "deprecated" sections.** Git is the changelog. A doc describes the
system as it is *now*, in the present tense.

**5. Doc and code disagree → fix the doc first, then the code.** Never write code that contradicts
a doc and leave the doc stale. If you believe a doc is wrong, stop, correct the doc in the same
change, and say so in the commit body.

**6. Hard budget: 500 lines per file.** Hitting the ceiling means content is redundant or filed
under the wrong owner. It is never a reason to create a new file.

### What `make docs-check` verifies

- No file in `docs/` outside the manifest, and none missing from it.
- No file over 500 lines.
- Every relative markdown link resolves to a real file, and every `#anchor` matches a real heading
  in the target file. **A stale cross-reference fails the build**, which is what makes rule 2 safe
  to rely on.
- No banned headings (`## Changelog`, `## History`, `## Deprecated`).
- No unresolved-decision markers (`TBD`, `TODO`, `FIXME`, `???`) — one of these in a spec means an
  architecture decision has leaked to the implementer.
