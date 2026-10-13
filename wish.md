Kzark, I think we should treat Gwen as more than a chatbot with a calendar and a task list. We should build her as a personal assistant runtime with a proactive interface, a task-execution harness, and a permission system.

The important distinction is that Gwen shouldn't merely remind you what to do. She should understand what you're supposed to do, help you begin, prepare the tools and context you need, and eventually carry out parts of the work with your approval.

For example, imagine it's 7 PM and you have DSA practice scheduled.

- Gwen recognizes that the calendar event is an upcoming commitment.
- Five minutes before it starts, she notifies you.
- At the scheduled time, she says something like, "Time for DSA. I've opened today's problem and your practice notes."
- Your timeline shows the task as ready, with its resources attached.
- You solve the problem yourself, or ask Gwen for a hint.
- If you get stuck, you discuss it with her.
- Afterward, she records the outcome and updates the task.

That's a coherent experience, not six disconnected features.

I'd break the work into eight implementation prompts, executed in dependency order. We should audit the existing codebase first, though, rather than asking a coding agent to rebuild everything from scratch.

## 1. What should Gwen become?

A personal timeline

One place for calendar appointments, actionable tasks, recurring routines, deadlines, and ongoing work.

A proactive companion

Timely reminders, contextual voice messages, and gentle nudges that help you start instead of just notifying you.

A task-execution harness

Break requests into steps, select the appropriate tools, track progress, recover from errors, and request approval at the right moments.

A connected workspace

Email, browser, code projects, notes, calendar, and other integrations, all controlled through a consistent task interface.

These should be built as separate capabilities sharing one task model and one execution system. That will make it much easier to add new integrations later without redesigning Gwen every time.

## 2. First, the design file: how much can it help?

I found the newly imported `design(1).md`, which appears to be the design specification intended for `@ui/design.md`. It is a substantial improvement over using a generic design reference, and I'd keep most of its structure.&#x20;

design(1).md



Here's my assessment:

Keep: the architecture of the design system

Design tokens, reusable components, typography, explicit runtime states, accessibility, and performance guidance are all valuable. They let us evolve Gwen without redesigning every screen independently.

Change: the visual personality

The near-black graphite surfaces, restrained red accent, compact panels, and monospace-heavy engineering aesthetic are appropriate for the Lab. They may feel too severe as the default interface for a companion you interact with all day.

Add: a task-first experience

The specification covers Companion, Workspace, and Lab modes, but it needs a stronger definition of the daily timeline, task preparation, notification behavior, approval flows, and background execution.

### The visual direction I'd choose

Keep the believable Spider-Man-style engineering workstation influence, but make Gwen feel more approachable and less like a permanent debugging console.

[I’ve Tried 50+ Productivity Tools — Here are My Top 9](https://images.openai.com/static-rsc-4/pVCahgWS1yJA5Lv2DiUUYwzOAXVj9b_IMsluXQNpObM-ssiBirbNeFpJFFgJZYl-xBAknr5MVvXZz2cU4Ii0cdA_SDyDHayWxtQYdwaOHaxv4k58Ou3uHAhCMcaN3vvQk60IgphY8mV7ki2ez9CRk24_r-OhakTY_0LWGjja0DQ?purpose=inline)

Companion — warm, calm, personal

Soft charcoal or warm graphite, warm-white text, a muted rose or coral accent, more breathing room in conversations, and Gwen's character integrated into the interface.

[OFFLIGHT – AI scheduler for seamless execution](https://images.openai.com/static-rsc-4/fsT5Kb0RPVXhMoVhSg2RcM66bJakdvZOl01TQXApp-c1l-jFGcDr1M9T32vC1JrjlYLAqJSapftl4gqhBrf7E28PgppIS82zr8716kALudWafmCXYrQt9y3Lk2PM_HIq7qkBsQiWfBGpUBs58jWhySVIxwu48yBqKy5bdBSt_qE?purpose=inline)

Timeline — clear, action-oriented

Quiet surfaces, a strong current-time indicator, readable event blocks, compact deadlines, and a prominent next action. Color should distinguish meaning, not decorate every card.

[WebDevCody (@webdevcody) on X](https://images.openai.com/static-rsc-4/UwDY7vM9JJ8Me2AWplXUAj9jLA7qa6Qzfbqr_Fu9azu-gqOEkC62mpPEFKvjIGz9o7xnHeuQVohcU1RFbHSCx9nDkMJynzGhGXZFnT7TgHFu66GrrOzgcTv7aE4_4h4SqKK4Vi9j_tauCvATzqRHvKxJ6rqMgJTEazap5V9ICww?purpose=inline)

Lab and execution — technical

Preserve the dense engineering aesthetic here: logs, file paths, tool calls, runtime metrics, execution steps, and errors.

These are visual direction references, not a claim that we have implemented or validated those exact designs.

My recommendation: don't replace the entire design system because you dislike the colors. Separate the design tokens from the product experience. Define a softer companion palette, retain the technical palette for Lab and execution, and give the timeline its own hierarchy while reusing the same underlying components.

The existing file also explicitly warns against fake telemetry and says the design system doesn't define model behavior, memory policy, tool permissions, or inference scheduling. That's important: a better-looking interface alone won't give Gwen the ability to act reliably.&#x20;

design(1).md



## 3. The architecture we should build toward

Before writing implementation prompts, establish one shared model that connects the calendar, timeline, reminders, and task runner.

\#chatgpt-mermaid-\_r_jg\_{font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI","Helvetica","Apple Color Emoji","Arial",sans-serif,"Segoe UI Emoji","Segoe UI Symbol";font-size:16px;fill:rgb(237, 237, 237);}@keyframes edge-animation-frame{from{stroke-dashoffset:0;}}@keyframes dash{to{stroke-dashoffset:0;}}#chatgpt-mermaid-\_r_jg\_ .edge-animation-slow{stroke-dasharray:9,5!important;stroke-dashoffset:900;animation:dash 50s linear infinite;stroke-linecap:round;}#chatgpt-mermaid-\_r_jg\_ .edge-animation-fast{stroke-dasharray:9,5!important;stroke-dashoffset:900;animation:dash 20s linear infinite;stroke-linecap:round;}#chatgpt-mermaid-\_r_jg\_ .error-icon{fill:rgb(48, 48, 48);}#chatgpt-mermaid-\_r_jg\_ .error-text{fill:rgb(237, 237, 237);stroke:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_jg\_ .edge-thickness-normal{stroke-width:1px;}#chatgpt-mermaid-\_r_jg\_ .edge-thickness-thick{stroke-width:3.5px;}#chatgpt-mermaid-\_r_jg\_ .edge-pattern-solid{stroke-dasharray:0;}#chatgpt-mermaid-\_r_jg\_ .edge-thickness-invisible{stroke-width:0;fill:none;}#chatgpt-mermaid-\_r_jg\_ .edge-pattern-dashed{stroke-dasharray:3;}#chatgpt-mermaid-\_r_jg\_ .edge-pattern-dotted{stroke-dasharray:2;}#chatgpt-mermaid-\_r_jg\_ .marker{fill:rgb(175, 175, 175);stroke:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_jg\_ .marker.cross{stroke:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_jg\_ svg{font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI","Helvetica","Apple Color Emoji","Arial",sans-serif,"Segoe UI Emoji","Segoe UI Symbol";font-size:16px;}#chatgpt-mermaid-\_r_jg\_ p{margin:0;}#chatgpt-mermaid-\_r_jg\_ .label{font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI","Helvetica","Apple Color Emoji","Arial",sans-serif,"Segoe UI Emoji","Segoe UI Symbol";color:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_jg\_ .cluster-label text{fill:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_jg\_ .cluster-label span{color:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_jg\_ .cluster-label span p{background-color:transparent;}#chatgpt-mermaid-\_r_jg\_ .label text,#chatgpt-mermaid-\_r_jg\_ span{fill:rgb(237, 237, 237);color:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_jg\_ .node rect,#chatgpt-mermaid-\_r_jg\_ .node circle,#chatgpt-mermaid-\_r_jg\_ .node ellipse,#chatgpt-mermaid-\_r_jg\_ .node polygon,#chatgpt-mermaid-\_r_jg\_ .node path{fill:rgb(17, 40, 19);stroke:rgb(58, 132, 63);stroke-width:1px;}#chatgpt-mermaid-\_r_jg\_ .rough-node .label text,#chatgpt-mermaid-\_r_jg\_ .node .label text,#chatgpt-mermaid-\_r_jg\_ .image-shape .label,#chatgpt-mermaid-\_r_jg\_ .icon-shape .label{text-anchor:middle;}#chatgpt-mermaid-\_r_jg\_ .node .katex path{fill:#000;stroke:#000;stroke-width:1px;}#chatgpt-mermaid-\_r_jg\_ .rough-node .label,#chatgpt-mermaid-\_r_jg\_ .node .label,#chatgpt-mermaid-\_r_jg\_ .image-shape .label,#chatgpt-mermaid-\_r_jg\_ .icon-shape .label{text-align:center;}#chatgpt-mermaid-\_r_jg\_ .node.clickable{cursor:pointer;}#chatgpt-mermaid-\_r_jg\_ .root .anchor path{fill:rgb(175, 175, 175)!important;stroke-width:0;stroke:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_jg\_ .arrowheadPath{fill:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_jg\_ .edgePath .path{stroke:rgb(175, 175, 175);stroke-width:1px;}#chatgpt-mermaid-\_r_jg\_ .flowchart-link{stroke:rgb(175, 175, 175);fill:none;}#chatgpt-mermaid-\_r_jg\_ .edgeLabel{background-color:rgb(0, 0, 0);text-align:center;}#chatgpt-mermaid-\_r_jg\_ .edgeLabel p{background-color:rgb(0, 0, 0);}#chatgpt-mermaid-\_r_jg\_ .edgeLabel rect{opacity:0.5;background-color:rgb(0, 0, 0);fill:rgb(0, 0, 0);}#chatgpt-mermaid-\_r_jg\_ .labelBkg{background-color:rgba(0, 0, 0, 0.5);}#chatgpt-mermaid-\_r_jg\_ .cluster rect{fill:rgb(48, 48, 48);stroke:rgba(255, 255, 255, 0.15);stroke-width:1px;}#chatgpt-mermaid-\_r_jg\_ .cluster text{fill:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_jg\_ .cluster span{color:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_jg\_ div.mermaidTooltip{position:absolute;text-align:center;max-width:200px;padding:2px;font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI","Helvetica","Apple Color Emoji","Arial",sans-serif,"Segoe UI Emoji","Segoe UI Symbol";font-size:12px;background:rgb(48, 48, 48);border:1px solid rgba(255, 255, 255, 0.15);border-radius:2px;pointer-events:none;z-index:100;}#chatgpt-mermaid-\_r_jg\_ .flowchartTitleText{text-anchor:middle;font-size:18px;fill:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_jg\_ rect.text{fill:none;stroke-width:0;}#chatgpt-mermaid-\_r_jg\_ .icon-shape,#chatgpt-mermaid-\_r_jg\_ .image-shape{background-color:rgb(0, 0, 0);text-align:center;}#chatgpt-mermaid-\_r_jg\_ .icon-shape p,#chatgpt-mermaid-\_r_jg\_ .image-shape p{background-color:rgb(0, 0, 0);padding:2px;}#chatgpt-mermaid-\_r_jg\_ .icon-shape .label rect,#chatgpt-mermaid-\_r_jg\_ .image-shape .label rect{opacity:0.5;background-color:rgb(0, 0, 0);fill:rgb(0, 0, 0);}#chatgpt-mermaid-\_r_jg\_ .label-icon{display:inline-block;height:1em;overflow:visible;vertical-align:-0.125em;}#chatgpt-mermaid-\_r_jg\_ .node .label-icon path{fill:currentColor;stroke:revert;stroke-width:revert;}#chatgpt-mermaid-\_r_jg\_ .node .neo-node{stroke:rgb(58, 132, 63);}#chatgpt-mermaid-\_r_jg\_ [data-look="neo"].node rect,#chatgpt-mermaid-\_r_jg\_ [data-look="neo"].cluster rect,#chatgpt-mermaid-\_r_jg\_ [data-look="neo"].node polygon{stroke:url(#chatgpt-mermaid-\_r_jg\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_jg\_ [data-look="neo"].swimlane.cluster rect{filter:none;}#chatgpt-mermaid-\_r_jg\_ [data-look="neo"].node path{stroke:url(#chatgpt-mermaid-\_r_jg\_-gradient);stroke-width:1px;}#chatgpt-mermaid-\_r_jg\_ [data-look="neo"].node .outer-path{filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_jg\_ [data-look="neo"].node .neo-line path{stroke:rgb(58, 132, 63);filter:none;}#chatgpt-mermaid-\_r_jg\_ [data-look="neo"].node circle{stroke:url(#chatgpt-mermaid-\_r_jg\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_jg\_ [data-look="neo"].node circle .state-start{fill:#000000;}#chatgpt-mermaid-\_r_jg\_ [data-look="neo"].icon-shape .icon{fill:url(#chatgpt-mermaid-\_r_jg\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_jg\_ [data-look="neo"].icon-shape .icon-neo path{stroke:url(#chatgpt-mermaid-\_r_jg\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_jg\_ .node text{font-size:14px;font-weight:600;letter-spacing:normal;fill:rgb(140, 223, 173);}#chatgpt-mermaid-\_r_jg\_ .edgeLabels text{font-size:13px;font-weight:600;letter-spacing:-0.08px;fill:rgb(140, 223, 173);}#chatgpt-mermaid-\_r_jg\_ .node tspan[font-weight="normal"],#chatgpt-mermaid-\_r_jg\_ .edgeLabels tspan[font-weight="normal"]{font-weight:600;}#chatgpt-mermaid-\_r_jg\_ .edgeLabel .label rect{opacity:1;rx:13px;ry:13px;fill:rgb(2, 28, 12);stroke:rgb(26, 75, 45);stroke-width:1px;}#chatgpt-mermaid-\_r_jg\_ .node rect,#chatgpt-mermaid-\_r_jg\_ .node circle,#chatgpt-mermaid-\_r_jg\_ .node ellipse,#chatgpt-mermaid-\_r_jg\_ .node polygon,#chatgpt-mermaid-\_r_jg\_ .node path{fill:rgb(0, 55, 22);stroke:rgba(255, 255, 255, 0.1);stroke-width:1px;}#chatgpt-mermaid-\_r_jg\_ .node rect{rx:16px;ry:16px;}#chatgpt-mermaid-\_r_jg\_ .node.mermaid-decision .label-container{fill:rgb(2, 28, 12);stroke:rgb(26, 75, 45);stroke-dasharray:2px,2px;}#chatgpt-mermaid-\_r_jg\_ .edgePaths .flowchart-link{stroke:rgb(175, 175, 175);stroke-width:1px;stroke-linecap:round;stroke-linejoin:round;}#chatgpt-mermaid-\_r_jg\_ .marker{fill:rgb(175, 175, 175);stroke:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_jg\_ :root{--mermaid-font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI","Helvetica","Apple Color Emoji","Arial",sans-serif,"Segoe UI Emoji","Segoe UI Symbol";}Google CalendarUnified TimelineManual Tasks andRoutinesEmail and Other AppsTask PlannerReminder SchedulerDesktop Notificationsand VoiceExecution PlanPermission and ApprovalGateTool AdaptersBrowser, Email, Code,NotesExecution ResultsTask State and ActivityLogConversation ContextGwen

The crucial design choice is that the timeline and the execution harness must share the same task identity and state, rather than maintaining separate versions of a task.

For example, a Google Calendar event can appear as a scheduled item in the timeline. When you choose to work on it, Gwen can attach preparation steps, open resources, and track the resulting execution without creating a duplicate appointment.

A few distinctions matter:

- A calendar event is a scheduled commitment, not necessarily a checklist task.
- An actionable event can have preparation and completion steps.
- A reminder is a scheduled trigger, not a separate task.
- A task can exist without a calendar event.
- A task can have an execution plan without granting Gwen permission to execute every step.

This structure will support the email, DSA, and coding examples without needing a different system for each.

## 4. The eight prompts I'd give the coding agent

Run these sequentially, not all at once. Each prompt should produce a working, testable increment and leave the codebase in a state where the next phase can build on it.

The first prompt is deliberately an audit. We shouldn't assume which framework, database, desktop notification system, or scheduler Gwen currently uses.

## 01

Audit

## Audit Gwen's existing architecture

Start here

Goal: establish what's already implemented, where the changes belong, and what must be reused.

# Prompt 01 — Gwen Architecture and Implementation Audit

Act as a senior software engineer working on Gwen, an existing local AI companion and personal assistant.

## Objective

Audit the current codebase before implementing new functionality. Gwen should eventually support a unified personal timeline, Google Calendar integration, proactive notifications, task planning, app integrations, and controlled task execution.

## Inspect

1. Repository structure, application entry points, framework, package manager, and runtime.
2. Existing UI routes, components, state management, and `@ui/design.md`.
3. Existing task, calendar, reminder, notification, conversation, and memory implementations.
4. Database schemas, migrations, persistence, background workers, and scheduling mechanisms.
5. Desktop capabilities: native notifications, voice output, startup behavior, URL opening, and window management.
6. Tool integrations, permissions, authentication, and any existing agent loop or execution harness.
7. Tests, development scripts, environment variables, and current build status.

## Deliverables

- Map the existing architecture and relevant file paths.
- Identify which requested capabilities already exist, partially exist, or are missing.
- Trace the current lifecycle of a task from creation through completion.
- Identify the cause of calendar events not appearing in the timeline.
- Identify how background work can continue when the main window is idle or minimized.
- Document integration, security, reliability, and migration risks.
- Recommend a phased implementation plan with dependencies and acceptance criteria.

Do not rewrite the application, replace working components, or make speculative architectural changes during this audit. Verify conclusions against actual code and tests. Distinguish confirmed findings from assumptions.

At the end, provide a file-by-file change plan for the next phase.

## 02

Timeline

## Unify Google Calendar and tasks

Goal: every commitment appears in the same daily view, without duplicate or misleading tasks.

# Prompt 02 — Unified Calendar and Task Timeline

Implement the unified timeline in the existing Gwen codebase, following the architecture audit.

## Requirements

1. Connect to Google Calendar using the appropriate supported OAuth flow and least-privilege permissions. Never hardcode credentials or expose tokens to the UI.
2. Import and synchronize upcoming calendar events, including event updates, cancellations, recurring events, time zones, and all-day events.
3. Display calendar appointments alongside manually created tasks, recurring routines, deadlines, and tasks created by Gwen.
4. Clearly distinguish event types and origins. A calendar appointment must not automatically become a duplicate, independently editable task.
5. Allow an event to have optional preparation steps, checklists, resources, and a completion status where appropriate.
6. Provide Today, Upcoming, and Past views, a clear current-time indicator, and useful empty, loading, disconnected, and error states.
7. Preserve source event IDs and stable task IDs. Synchronization must be idempotent: repeated syncs must not create duplicates.
8. Respect calendar permissions and distinguish read-only imported events from events Gwen is authorized to modify.
9. Handle offline operation, expired authorization, pagination, recurring-event exceptions, and incremental synchronization where supported.
10. Keep calendar data and task data in the existing persistence architecture unless the audit establishes a concrete reason to change it.

## Acceptance criteria

- Existing Google Calendar appointments appear on the correct date and at the correct local time.
- Updating or deleting a source event is reflected in the timeline.
- Repeated synchronization does not duplicate events.
- A task can be completed without accidentally deleting or modifying its source appointment.
- Timeline actions persist across application restarts.
- Tests cover time-zone boundaries, recurring events, sync retries, and source-event identity.

Inspect the existing implementation first. Implement the smallest complete vertical slice, add tests, and document setup and OAuth configuration.

## 03

Proactive

## Build a reliable reminder and notification engine

Goal: notify at the right time even when Gwen's conversation window is idle.

# Prompt 03 — Gwen Reminder Scheduler and Proactive Notifications

Implement a persistent reminder and notification subsystem using the existing application architecture.

## Reminder behavior

Support configurable reminder rules, including:

- 5 minutes before an event or task.
- At the scheduled start time.
- Additional reminders explicitly configured by the user.
- Snooze, dismiss, and mark-complete actions.
- Recurring tasks and changed or cancelled calendar events.

## Reliability

1. Schedule reminders from persisted task/event data, not from temporary UI timers.
2. Support application restart, missed reminders, system sleep/wake, and clock/time-zone changes.
3. Prevent duplicate notifications using stable reminder IDs and idempotent delivery records.
4. Reconcile reminders when events change or are cancelled.
5. Respect quiet hours, notification preferences, and the user's configured lead times.
6. Never silently mark an undelivered notification as delivered.
7. Use the platform's appropriate native notification mechanism. Report unsupported capabilities honestly.

## Proactive companion behavior

Allow Gwen to initiate a contextual message when a relevant task is approaching, starts, or remains unstarted.

Provide user controls for:

- Desktop notification only.
- Notification plus a message in Gwen's interface.
- Optional spoken reminders.
- Quiet hours and notification intensity.
- Snooze, mute, and disable controls.

Avoid repeatedly interrupting the user. Track acknowledgement and snooze state, and suppress stale reminders when the task is completed or no longer relevant.

## Safety and correctness

A reminder must never itself execute an external action. Speaking a reminder, opening a task, sending an email, and performing the task are distinct operations.

## Acceptance criteria

Verify pre-start and start-time notifications, restart recovery, duplicate suppression, snoozing, cancellations, and quiet hours with automated tests and an observable event log. Document limitations imposed by the operating system or application lifecycle.

## 04

UI/UX

## Redesign Gwen around daily work

Goal: make the interface approachable and actionable while preserving the engineering-workstation identity.

# Prompt 04 — Gwen Companion and Task-First UI Redesign

Use the existing `@ui/design.md` as a structural starting point, not an immutable visual specification.

First inspect the actual application screens, current component hierarchy, and design tokens. Then implement a cohesive UI redesign.

## Visual direction

Create a premium, restrained desktop companion inspired by believable engineering workstations. Do not copy proprietary movie interfaces.

The default experience should feel warm, calm, personal, and easy to use. Reserve dense technical layouts for the Lab and execution views.

Replace the current visual direction where necessary:

- Introduce a softer warm-neutral surface palette and a muted rose/coral brand accent.
- Keep semantic success, warning, error, and informational colors distinct from the brand accent.
- Use consistent typography, spacing, contrast, focus states, and keyboard navigation.
- Avoid excessive glow, gradients, oversized headings, decorative telemetry, and unnecessary card-on-card layouts.
- Support reduced motion and lightweight character animation.

## Core experience

Design and implement:

1. Companion: conversation, voice controls, contextual messages, and Gwen's character state.
2. Timeline: today's appointments, tasks, current time, upcoming commitments, and next actions.
3. Task detail: objective, schedule, checklist, resources, execution steps, approval requirements, and results.
4. Execution view: current step, progress, tool activity, errors, and pause/stop/approval actions.
5. Integrations: connected apps, authorization state, available capabilities, and disconnect controls.
6. Lab: inference metrics and technical diagnostics, using actual measurements only.

## Interaction rules

- Make the next relevant action immediately obvious.
- Keep the conversation accessible without allowing it to obscure task controls.
- Clicking a timeline item should open its context and available actions.
- Show the difference between planned, running, awaiting approval, completed, and failed work.
- Surface proactive messages without unexpectedly taking over the screen.
- Give users control over when Gwen can speak and when the character animates.

## Implementation constraints

Create a single source of truth for design tokens. Reuse existing components where appropriate, and map real application state to UI state explicitly.

Do not fabricate completed tasks, live activity, integration connections, or telemetry.

Deliver a before/after explanation, the changed file paths, responsive and accessibility checks, and screenshots or other verifiable UI evidence if the development environment supports them.

## 05

Harness

## Build the task-execution harness

Core foundation

Goal: give Gwen a reusable way to plan and execute different kinds of work without turning the LLM into an unrestricted shell.

# Prompt 05 — Gwen Task Planning and Execution Harness

Design and implement a reusable task-execution harness within Gwen's existing architecture.

The harness must support different kinds of work through shared task, plan, permission, execution, and result interfaces.

## Task representation

Every task should support:

- Stable ID, title, objective, source, and optional schedule.
- User requirements and constraints.
- Task type and required capabilities.
- Ordered steps with dependencies.
- Status, progress, timestamps, and execution history.
- Resources, file paths, URLs, and structured inputs.
- Approval requirements and a record of user decisions.
- Output artifacts, errors, retry information, and completion summary.

Use explicit lifecycle states such as draft, planned, ready, running, awaiting approval, paused, completed, failed, and cancelled. Define valid transitions rather than allowing arbitrary status changes.

## Planning

When a user requests work:

1. Understand the intended outcome and constraints.
2. Identify missing information and ask focused questions when necessary.
3. Break the request into verifiable steps.
4. Identify required integrations and capabilities.
5. Resolve project paths, URLs, dependencies, and relevant context.
6. Present a plan before consequential actions.
7. Execute approved steps and record their results.
8. Recover safely from failures and summarize the outcome.

For coding tasks, identify the correct repository, project, folder, target files, relevant conventions, and verification commands. Never invent file paths; inspect the repository.

For research and communication tasks, identify source data, output format, and any required human decisions.

## Tool execution

Create a typed tool interface with explicit input/output schemas, timeouts, cancellation, bounded retries, structured errors, and execution logs.

Separate planning from execution. Treat model-generated plans and external content as untrusted inputs that must be validated before tool invocation.

Implement permission checks in the execution layer, not merely in the UI or model prompt. Restrict filesystem access, subprocess execution, and network capabilities according to configured policy.

Support resumable execution where safe. Require idempotency or explicit recovery logic before retrying side-effecting actions.

## Approval policy

Classify operations as:

- Read-only: allowed within granted scopes.
- Reversible or low-risk: permitted only under configured policy.
- External side effects: require explicit approval by default.
- Destructive or security-sensitive: require stricter authorization.

Sending messages, changing important data, deleting files, installing software, or running consequential commands must not be implicitly authorized by a reminder or a broad task description.

## Observability and evaluation

Record planned steps, tool invocations, outcomes, approvals, retries, and failures. Keep credentials and sensitive content out of logs.

Add deterministic tests for task transitions, permissions, cancellation, retries, interrupted execution, and recovery.

Start with one simple, end-to-end task using a mock tool. Keep the architecture extensible, but do not build a multi-agent framework or unrestricted autonomous loop without a demonstrated need.

## 06

Integrations

## Create a connector framework

Goal: connect new applications without rewriting the planner for every integration.

# Prompt 06 — Gwen Integration and Capability Registry

Implement a reusable integration architecture for Google Calendar, email, browser automation, local project workspaces, and future applications.

## Connector contract

Each connector should expose:

- Stable integration identity and connection status.
- Authentication and revocation lifecycle.
- Declared capabilities and required permission scopes.
- Typed input/output schemas.
- Read, write, and external-side-effect classifications.
- Execution, timeout, cancellation, and error behavior.
- Audit events and capability-specific tests.

Keep credentials in an appropriate secure store. Do not expose access tokens to frontend components or include them in prompts, logs, or task artifacts.

## Initial connectors

Prioritize the following, subject to existing architecture:

1. Google Calendar: list and synchronize events.
2. Browser: open approved URLs and, later, perform bounded browser workflows.
3. Email: search and read authorized messages; draft replies; send only after explicit approval.
4. Local workspace: inspect permitted repositories and files, prepare code changes, and run allowlisted verification commands.
5. Notes: search, create, and update notes within granted permissions.

Do not claim a connector supports a capability until its implementation and authorization flow exist.

## Planner integration

The task planner should request capabilities by name rather than depending on connector-specific implementation details.

The execution harness must verify permissions on every relevant invocation. Revoking an integration should prevent subsequent calls and invalidate applicable credentials.

## Acceptance criteria

Demonstrate that the same task interface can invoke at least two real or test-backed connectors, validate their inputs, handle failures, enforce permissions, and return structured results.

Document which capabilities are operational, which are simulated in tests, and which remain future work.

## 07

Workflows

## Implement real assistant workflows

Goal: prove the same harness works for email, DSA practice, and coding tasks.

# Prompt 07 — End-to-End Gwen Assistant Workflows

Use the shared timeline, reminder scheduler, task harness, permission system, and connector framework to implement three complete workflows.

## Workflow A: Email review

User request: "Go through my last five emails and tell me which ones need a response."

1. Retrieve the five most recent eligible messages within the authorized mailbox scope.
2. Summarize each message, identify likely response requirements, and explain the reasoning.
3. Suggest a reply for each relevant message.
4. Let the user discuss, revise, or reject each draft.
5. Allow the user either to send the reply manually or explicitly authorize Gwen to send it through the connected email integration.
6. Record successful sends, failures, and unresolved drafts.

Reading and drafting must not automatically imply permission to send. Confirm recipients and final content before sending.

## Workflow B: Scheduled DSA practice

User request: "Remind me to do DSA at 7 PM and open today's problem and notes."

1. Create or associate the task with the requested schedule.
2. Attach the relevant problem URL, notes, and any configured resources.
3. Schedule the configured pre-start reminder and start-time reminder.
4. At the appropriate time, notify the user and offer to start the session.
5. Open the configured URLs using the approved desktop/browser capability.
6. Track whether the session started, was snoozed, or was completed.
7. Offer a brief post-session summary and persist it if requested.

Do not claim that a browser was opened unless the operation succeeded. Handle unavailable browsers and invalid URLs gracefully.

## Workflow C: Coding task

User request: "Fix this issue in my project."

1. Identify the repository and confirm the intended project if ambiguous.
2. Inspect the issue, repository structure, relevant files, and local conventions.
3. Produce a plan with concrete file paths and verification commands.
4. Ask for clarification when requirements or destructive changes are ambiguous.
5. Make bounded changes in the authorized workspace.
6. Run appropriate tests and report actual results.
7. Show changed files, remaining risks, and any required manual steps.

Do not deploy, publish, push, or perform other consequential external actions without the necessary explicit authorization.

## Cross-workflow requirements

All three workflows must use the same task lifecycle, approval model, structured tool interface, and execution history.

Add end-to-end tests for success, failure, cancellation, interrupted execution, permission denial, and retry recovery. Keep real integrations separate from deterministic test fixtures.

## 08

Quality

## Test the entire assistant as a system

Goal: verify that Gwen can be trusted with progressively more responsibility.

# Prompt 08 — Gwen Reliability, Safety, and Evaluation

Create an end-to-end reliability and evaluation suite for Gwen.

Test the interaction between calendar synchronization, timeline rendering, persistent reminders, proactive conversation, task planning, connectors, and approvals.

## Required scenarios

- A calendar event is created, moved, cancelled, or made recurring.
- Gwen restarts or the computer sleeps before a reminder is due.
- A reminder fires once, is snoozed, or becomes stale because its task was completed.
- A task is resumed after an interrupted execution.
- An integration disconnects during a tool call.
- A tool times out or returns malformed output.
- A task requires clarification or user approval.
- An email draft is revised before sending.
- A coding task fails its tests.
- The user cancels work while a tool is running.

## Evaluation metrics

Measure:

- Reminder delivery accuracy and duplicate rate.
- Calendar synchronization correctness.
- Task completion rate by workflow and failure category.
- Invalid tool-call rate.
- Approval-policy violations.
- Recovery success after interruption.
- Time from scheduled start to acknowledged task start.
- User dismissals, snoozes, and repeated interruptions.

Do not use a single aggregate success score to conceal dangerous failures. Report results by category and distinguish automated test coverage from real-world observations.

## Safety checks

Verify that untrusted email content, web pages, and repository files cannot override tool permissions or the task's authorized scope.

Ensure that logs, task artifacts, and diagnostic views do not expose secrets unnecessarily.

## Deliverables

Provide automated test results, reproducible evaluation fixtures, a list of known failure modes, and a release checklist.

Do not enable broader autonomy merely because basic tests pass. Expand permissions only after the relevant workflows demonstrate reliable behavior.

## 5. How the finished experience should behave

The prompts become much easier to implement when we agree on the expected user experience.

7:00 PM — DSA practice

Source: Google Calendar · Scheduled

Upcoming

6:55 PM · Gwen

"DSA starts in five minutes. Want me to open today's problem and your notes?"

Start session

Snooze

Not today

When you choose Start session

Gwen opens the configured resources, changes the task state to running, and records which actions succeeded. She doesn't mark the task complete just because the browser opened.

This interaction should work whether the task originated in Google Calendar, was created manually, or was generated from a recurring routine.

### When should Gwen speak without being asked?

I'd introduce three distinct levels of proactivity:

| Level      | Behavior                              | Example                                          |
| ---------- | ------------------------------------- | ------------------------------------------------ |
| 1. Notify  | Desktop notification                  | "Meeting in 5 minutes."                          |
| 2. Engage  | Contextual message, optionally spoken | "It's time for DSA. Ready to begin?"             |
| 3. Prepare | Perform approved setup actions        | Open the problem, notes, and relevant workspace. |

The user should configure these levels globally and override them for individual tasks. Start with levels 1 and 2, then add level 3 through explicit, testable capabilities.

One architectural detail matters here: the reminder scheduler should decide when a proactive event is due, while Gwen's conversation layer decides what to say. Neither should independently authorize consequential actions.

## 6. How good can the task harness become?

The long-term goal is a harness that can take an outcome-oriented request and turn it into a traceable, resumable execution.

For example:

USER REQUEST

"Review my last five emails and help me respond."

GWEN'S PLAN

1. Read five authorized messages.
2. Identify which messages need a response.
3. Prepare proposed replies.
4. Discuss and revise the drafts with you.
5. Send only the replies you explicitly approve, or leave them for manual sending.

EXECUTION RECORD

Messages retrieved

Complete

Drafts prepared

Complete

Send approved replies

Approval

A strong harness should eventually provide:

- Planning: convert natural-language requests into concrete steps and dependencies.
- Context resolution: locate the right repository, project, files, messages, and URLs.
- Capability selection: determine which connected tool can perform each step.
- Policy enforcement: verify what Gwen is permitted to do before every consequential operation.
- Execution: run steps with timeouts, cancellation, bounded retries, and structured outputs.
- Persistence: save task state so work can resume after a restart.
- Observability: record what happened, which tools were called, and what failed.
- Evaluation: measure whether tasks succeed reliably, rather than assuming the model's response means they did.

You do not need a complicated multi-agent system to achieve this. A single planner, a typed tool registry, a permission layer, a persistent execution engine, and good evaluations are a much stronger starting point.

## 7. What I would implement first

I would not build all eight phases simultaneously. The best order is:

First: audit and fix the calendar timeline

This is your immediate, visible problem. Verify whether the issue is synchronization, filtering, data modeling, or rendering, then make appointments appear correctly.

Second: make reminders reliable

Implement the five-minute warning, start-time notification, snooze, duplicate prevention, and restart recovery. This makes Gwen useful even before advanced autonomy exists.

Third: improve the interface

Establish the companion and timeline visual systems, then connect them to actual task state instead of designing around placeholders.

Fourth: build one end-to-end harness

Use the DSA workflow first because it combines scheduling, notifications, browser actions, task status, and completion without initially requiring sensitive external writes.

Fifth: add email and expand the connector system

Once the harness is reliable, add reading, drafting, review, and approval-controlled sending. Reuse the same execution lifecycle for future integrations.

The key engineering principle: every phase should deliver a complete vertical slice, not just a new screen or a new abstraction. By the end of the first two phases, Gwen should reliably show your appointments and remind you. By the end of the DSA workflow, she should be able to turn one of those appointments into a real, prepared work session. That gives us a solid foundation for the much more capable assistant you want to build.