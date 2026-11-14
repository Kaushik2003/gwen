# Give Gwen an interface she can change herself

Status: execution plan. The feature is not implemented.

## Deliver the intended experience

Let Gwen express herself through the entire interface. She can replace navigation, rearrange the workspace, change typography and motion, create screens, and invent interactions. She can do this when the user asks or when she chooses to express herself during their conversation. Routine visual changes do not require a separate approval on every save.

Use editable code rather than a catalog of approved widgets, layout slots, or theme settings. Ship the default interface through the same loader that runs Gwen's own versions. Make the current design a starting point she can change.

Complete this loop inside Gwen:

1. Gwen reads her current interface and the app capabilities available to it.
2. She edits a working copy of her interface directly.
3. She builds and previews the candidate against real app data.
4. She inspects the rendered result and corrects problems.
5. She activates the candidate in the running window.
6. The user can ask for another change or restore any previous version.

Preserve access to tasks, plans, memory, chats, approvals, files, sessions, and terminals throughout this work. Visual freedom does not grant additional authority to send messages, delete data, or run commands. Keep the existing authorization rules for those operations. Do not add a visual-content policy or a design approval queue.

## Start from the current implementation

These are existing implementation points, not proposed files:

| Existing code | Work required |
| --- | --- |
| `ui/embed.go`, `cmd/gwen-ui/main.go` | Replace the embedded-only desktop interface with a loader for versioned local builds. Retain embedded assets for recovery. |
| `ui/src/App.tsx`, `ui/src/shell/Shell.tsx` | Separate app connections and durable state from the replaceable rendered interface. |
| `ui/src/shell/tabs.tsx` | Remove fixed screen names and mandatory Today placement as requirements imposed on custom interfaces. Preserve existing navigation addresses. |
| `ui/src/api.ts`, `cmd/gwen-ui/app.go` | Provide a documented interface to the existing app operations and events. Reuse the existing bindings and daemon client. |
| `cmd/gwen-ui/term.go` | Retain terminal ownership in the host. Reattach terminal views after a visual replacement without starting duplicate shells. |
| `internal/agent/process.go` | Gwen's chat currently has read-only native tools. Provide dedicated interface-authoring tools without enabling unrestricted shell access. |
| `internal/agent/rules.md`, `internal/agent/prompt.go` | Permit Gwen to author her own interface directly. Keep delegation for general engineering work. |
| `internal/api/agenttools.go`, `internal/api/agent.go` | Register authoring tools on the existing local agent connection and report active interface context. |
| `internal/config/paths.go`, `Makefile` | Add local interface locations and ship the runtime, compiler, defaults, and recovery assets. |
| `ui/vite.config.ts`, `ui/package.json` | Preserve the development workflow and define how live TSX, CSS, assets, and imports compile in an installed app. |

Herdr provides a useful precedent in `~/.config/herdr-app/AGENTS.md`: editable interface files, live reload, a last working version, diagnostics, and recovery. Gwen uses Wails and React, so implement the behavior using her existing stack.

## Choose the runtime before migrating screens

Use a small installed host with a replaceable React interface. Keep Wails, the daemon connection, terminal processes, and interface version management outside the editable render tree. Allow the editable entry point to render the whole application area, with its own navigation and styling.

Compare these implementations during the first milestone:

| Candidate | Assess |
| --- | --- |
| Rebuild and reinstall the entire app for each change | Measure interruption and latency. Keep this route for native backend changes; it does not satisfy everyday live expression. |
| Load versioned local interface bundles into the existing Wails window | Preferred direction. Prove asset loading, React dependency sharing, state preservation, diagnostics, and recovery before committing to the loader design. |
| Use the existing Vite development server as the installed runtime | Compare iteration speed and state handling. Account for Node, process supervision, offline operation, and packaging before adopting it. |

Prototype the preferred local-bundle loader and the Vite route using the same small interface. Choose using observed behavior in the real Wails window. Record the decision in this document before expanding the migration.

For the local-bundle route, evaluate an embedded Go compiler for TSX and CSS. Verify its behavior against the current frontend before selecting a dependency. The installed product must include a working compiler and its dependencies. Do not assume the user has a checkout, Node, npm, or a development server running.

Resolve module imports in this prototype. All interface modules must share one React runtime and the same app context objects. Generate a package manifest for the bundled dependencies. Support local modules, fonts, images, SVG, CSS, and dynamic imports. Give unresolved imports actionable diagnostics. Do not fetch dependencies or execute package install scripts implicitly when loading an interface.

Support arbitrary CSS rather than requiring a list of allowed utility classes. Preserve the current Tailwind-generated styles for the initial default interface. Verify how newly authored styles compile; do not claim that new Tailwind utilities work unless the live compiler actually generates them.

## Define ownership and local files

Use the XDG directories already resolved by `config.Paths`. The following paths are proposed:

```text
~/.config/gwen/interface/
  workspaces/<workspace-id>/
    manifest.json
    src/main.tsx
    src/styles.css
    assets/
  AUTHORING.md
  gwen.d.ts

~/.local/share/gwen/interface/
  revisions/<revision-id>/
    source/
    build/
    manifest.json
  selection.json

~/.local/state/gwen/interface/
  diagnostics.json
```

Keep each authoring workspace separate. Multiple chats must not write the same working copy. A build reads an immutable source snapshot and produces an immutable revision. Keep original source with each revision so restoring a design also restores editable code.

Use one version manager as the owner of selection changes. Atomically replace `selection.json`. Require the expected active revision when activating or restoring a revision, so competing chats receive a conflict instead of overwriting each other.

Track the active revision, the candidate under observation, the last healthy revision, and the bundled default. Never overwrite a healthy revision in place. Clean up abandoned builds and previews without deleting active revisions, recovery revisions, or versions the user has saved.

Use these lifecycle states in the implementation sketch:

```text
workspace -> building -> build_failed
                    -> built -> previewing -> preview_failed
                                          -> preview_passed
preview_passed -> activating -> healthy
                            -> failed -> restore last healthy
```

Build success alone does not make a revision healthy. A preview must mount, receive data, and report its render status. Activation must produce a separate health acknowledgment. A successful preview does not prove every interaction works.

## Keep authoring out of everyday context

Treat context use as an acceptance requirement. The application renders and runs locally. Rendering, tracking, asset changes, compilation, and health checks must not invoke the model. An active interface costs no recurring model calls merely because it exists.

Keep only a short, stable instruction about interface authoring in the main system prompt. Put the detailed authoring guide, source code, API declarations, revision history, and screenshots on disk. Load them on demand. Do not append the execution plan or the complete frontend to Gwen's prompt. Preserve the stable prompt construction in `internal/agent/prompt.go`; send changing revision identifiers through the existing per-turn context only when relevant.

Use a separate authoring conversation managed by the existing agent manager. Keep Gwen's persona and the user's visual intent, but pass only the requested change, current revision, current view, and relevant preferences. Do not copy the user's project chat, terminal transcript, unrelated memories, or this execution plan into it. Present its progress in the same Gwen UI. This is context separation within the product, not a new engineering persona or a requirement to hand visual expression to another assistant.

Reuse the authoring conversation for corrections to the same change. When the change is complete, retain source and diagnostics as local artifacts and return a compact result to the main conversation: what changed, active revision, and how to undo it. A later unrelated change starts from current source and relevant design notes rather than replaying the previous authoring transcript. Do not claim source caching removes tokens already present in an ongoing conversation.

Generate a small source index from the actual revision. Include module paths, exported symbols, imports, and the files associated with views. Let Gwen search the index and request selected source ranges and API declarations. Keep the entire source available through paginated reads. The index helps locate code; it does not limit what she can edit. Do not implement a repository-wide semantic indexing service for this first release.

Apply patches against file hashes. Return changed paths, hashes, and a short diff summary, not every complete file. Reuse immutable source snapshots and compiled artifacts by content hash. Compiler caches save machine work; they do not by themselves save model input tokens.

Expose one compact `interface` tool with typed operations for the authoring workflow below, rather than adding eight verbose always-advertised tools. Generate the operation schema and handler dispatch from one authoritative definition. Provide focused documentation on request. Do not depend on dynamic tool discovery until the installed agent client has demonstrated that it supports it. Measure the extra tool-schema tokens even when the user does not author anything.

Batch independent source reads and file edits. Make preview compilation, mount observation, and diagnostic collection one host job. Return actionable diagnostics first, with a cursor for complete logs. Request a fresh screenshot for visual verification, then request another only when a subsequent change needs inspection. Do not attach an image to every status response.

Subscribe to existing events for job completion instead of asking the model to poll. Ordinary renders, timers, focus changes, and progress events do not start authoring turns. Reuse the existing agent event path for intentional work. Coalesce obsolete build events and never start another model turn for a superseded preview.

Measure these workflows before declaring the design efficient:

| Workflow | Context and call requirement |
| --- | --- |
| Normal chat with no interface edit | No authoring source, history, images, or full guide in context; no extra inference request. Count the stable instruction and tool-schema overhead. |
| Run the authored interface for an hour | Zero model calls from the interface runtime itself. Existing chat and app actions retain their own accounting. |
| Change one component's spacing or color | Read the relevant source and styles. Do not load unrelated screens or the full API declaration file. |
| Correct the same change | Reuse the workspace and relevant source already in the authoring conversation. Fetch changed ranges rather than rereading the tree. |
| Redesign the whole workspace | Permit broader reads because the request needs them. Record the cost rather than enforcing a creative scope limit. |
| Preview fails | Run local checks without an extra inference call; deliver diagnostics once to the active authoring workflow for any model-driven repair. |
| Finish and continue normal chat | Transfer a compact result, not the authoring transcript. |

Report model requests, tool invocations, input tokens, cache-read tokens, output tokens, source bytes, image count, and elapsed time separately. Use provider-reported usage when available. Label local token estimates as estimates and report unavailable measurements explicitly. Compare a repeated full-source baseline with targeted reads for the same change and output quality. Do not describe cached input as free, infer billed savings from CLI calls, or claim a percentage before measurement.

## Execute in six milestones

### 1. Prove a whole-interface change in the installed window

Build the two runtime prototypes described above. Use an isolated app profile and throwaway data. Keep the currently installed daily-use app untouched.

Change the complete navigation and workspace layout, add a view absent from the current screen union, and load a new local asset and stylesheet. Show that these changes need no Go rebuild and are not limited to a panel slot.

Start tracking, a chat, and a terminal before activation. Confirm that tracking continues, the agent process continues, and the same terminal process remains available afterward. Measure build-to-visible latency; target five seconds for a small warm edit, then record the actual result and machine.

Exit when the selected runtime works offline, its compiler ships with the app, dependency sharing works, and the runtime choice has a written rationale. Stop expansion and revise the design if whole-root replacement breaks the required state guarantees.

Include a context-cost prototype in this milestone. Demonstrate a source index, targeted reads, and a separate authoring conversation using the existing agent manager. Record prompt and tool-schema overhead and the cost of one small edit before committing to the tool contract.

### 2. Implement revision storage, preview, activation, and recovery

Add a proposed `internal/interfaceui` package for source snapshots, compilation, revision metadata, diagnostics, and atomic selection. Keep rendering in the UI host. Add host integration in a proposed `cmd/gwen-ui/interface.go`.

Load assets from a specific immutable revision. Never mix HTML from one revision with scripts or styles from another. Coalesce file saves, discard superseded builds, and activate only the candidate explicitly selected by the version manager. Successful activation may be automatic within Gwen's authoring workflow.

Provide a separate preview window with the candidate's own CSS and lifecycle. Treat it as a live view with real consequences, not as a sandbox. Run automated destructive interaction checks against a disposable profile. Cancel subscriptions, observers, terminal attachments, and timers when the preview closes or a revision is replaced.

Capture compile errors, initial render errors, later render errors, rejected asynchronous operations, and asset failures with revision and source locations. Show diagnostics to Gwen and retain the working interface after failures.

Implement recovery outside editable JavaScript. Add proposed commands `gwen interface status`, `gwen interface restore <revision>`, and `gwen-ui --safe-ui`. Provide a host-owned menu action or supported native shortcut to restore the default. Prove the command-line route works even when editable JavaScript loops forever. A React error boundary or a JavaScript heartbeat cannot recover a blocked webview by itself.

On a crash during activation, start the next window with the last healthy revision. If that revision cannot load, use the bundled default. Never mark an unobserved candidate healthy on startup.

Exit when syntax errors, rendering failures, a stalled webview, a killed activation, and a missing revision all have a demonstrated recovery path.

### 3. Move the default interface onto the same authoring system

Create the proposed `ui/src/interface/` runtime and authoring API. Make the existing application the first editable interface. Include all of its source modules and assets in the shipped source package. Do not distribute an editable entry file that merely calls an inaccessible fixed shell.

Expose existing app methods, events, data hooks, navigation, terminal attachments, chat connections, and native dialogs through documented imports. Generate types from authoritative bindings where possible. Preserve arbitrary layout, CSS, SVG, canvas, animation, and custom interaction code.

Keep reusable default components available for convenience. A custom interface can replace them or omit them. Do not require Today, the current sidebar, the status bar, or the existing tab layout to remain visible.

Move user work that must survive replacement out of component-local state: unsent chat text, task forms, selected project, terminal addresses, and file positions. Specify a versioned state handover for interface-specific state. Preserve stable app data independently of custom UI state. Handle removed custom screens with a recoverable fallback rather than crashing navigation.

Preserve existing deep links from notifications, the CLI, and phone actions. An interface can choose their presentation, but the host must still deliver the target. Keep voice, the tray, and terminal bridges connected during desktop visual changes.

Exit when the shipped source can recreate the default app, a visibly different interface uses the same app capabilities, and task, chat, approval, terminal, file, planning, and tracking workflows still work.

### 4. Give Gwen direct interface-authoring tools

Add a proposed `interface` tool in `internal/api/agentinterface.go` and register it with the existing agent tools. Keep it on the local authenticated agent connection. Use typed operations with the names below; these rows describe operations, not eight additional advertised tools.

| Proposed operation | Result |
| --- | --- |
| `interface_context` | Active revision, current workspace, current view, diagnostic summary, and references to the source index and focused authoring documentation. |
| `interface_read` | Searchable index, paginated source ranges, or selected API declarations for a workspace or revision. Batch independent reads. |
| `interface_patch` | Apply a batch of file changes to this chat's workspace with an expected workspace revision. |
| `interface_preview` | Snapshot, compile, and open a candidate preview. Return its revision and build or mount diagnostics. |
| `interface_inspect` | Render status, current UI state, and a screenshot when a desktop host is attached. |
| `interface_activate` | Activate a previewed candidate against the expected active revision. |
| `interface_history` | List previous revisions with source changes and Gwen's description. |
| `interface_restore` | Restore a previous revision without discarding its successors. |

Return durable job identifiers for long builds and expose status through existing event patterns. Report when no UI host is attached. Do not report a successful visual inspection from a compiler result or stale screenshot.

Update `internal/agent/rules.md` and the prompt context. Explicitly allow Gwen to edit her own interface using these tools. Keep ordinary project engineering delegated to Claude sessions. Do not enable general shell execution to make interface authoring work.

Let Gwen make routine visual changes without repeated approval prompts. Make source history and undo available through natural language and the host recovery controls. Explain effects that interrupt an active interaction, and preserve its draft before replacement. Prevent activation while composition or a native dialog cannot be handed over safely; keep the candidate ready and activate at the next stable point.

Supply examples and generated API declarations in `AUTHORING.md` and `gwen.d.ts`. Show how to read tasks, subscribe to updates, attach a terminal, preserve a draft, and dispose of resources. Include unsupported imports and runtime errors in the feedback loop.

Keep these materials available on disk rather than automatically injecting them. Route source reads and preview images into the authoring conversation. Send the compact completion result back to the user's main chat.

Exit when Gwen herself completes read, edit, preview, inspect, activate, and restore from a normal chat without asking the user to run terminal commands.

### 5. Carry the model to the phone and preserve it across updates

Treat desktop support as the first delivery, with phone support as a required subsequent milestone. The current phone frontend is separate and has its own authentication and asset policy. Do not claim desktop code can automatically replace it.

Add a phone entry point to the interface manifest. Reuse the same source, revision manager, and authoring model with a target-specific app adapter. Preserve paired-device authorization, connection recovery, touch and keyboard behavior, and existing phone deep links. Do not expose desktop-only native bindings to the phone.

Serve immutable phone revision assets through the existing authenticated phone host. Define when a connected phone adopts a new revision and preserve its unsent text. Require an observed phone preview before marking a phone entry healthy. Retain the bundled phone interface as recovery.

Give revisions an explicit runtime API version. Ship new defaults beside user-authored source. Never overwrite that source during install or startup. Compile and preview saved source against a new runtime before adopting it. When incompatible, show diagnostics and retain recoverable source and the bundled interface that works with the installed runtime.

Exit when a desktop and phone can use authored interfaces, app updates preserve their source and history, and an incompatible revision recovers without data loss.

### 6. Verify expression, usefulness, and failure handling

Create a proposed rerunnable `scripts/verify-interface.sh` that uses an isolated profile. Drive the real host and existing app operations. Use a GUI session or a supported virtual display for rendered checks; a browser-only check does not prove the Wails bridge works.

Cover these scenarios:

| Scenario | Required observation |
| --- | --- |
| Replace the whole layout and add an original screen | The new interface fills the window and has no dependency on fixed slots. |
| Render a new interactive task view | A task change reaches the daemon, persists, and appears in the default interface after restore. |
| Continue work through activation | A draft survives, tracked time advances, the chat continues, and the terminal PID is unchanged. |
| Use a custom font, asset, stylesheet, and animation | The installed offline app renders them without a development server. |
| Save malformed TSX or import a missing module | Gwen receives a source diagnostic and the working interface stays active. |
| Fail after initial mount | A later error is attributed to the revision and recovery remains available. |
| Block the editable JavaScript thread | Host or CLI recovery restores a usable window. |
| Build two revisions and finish the older one last | The older build does not replace the selected revision. |
| Activate from two chats | One succeeds and the stale request gets a clear conflict. |
| Kill the app during activation | Restart opens a healthy revision or the bundled default. |
| Repeatedly preview and restore | Watchers, event listeners, terminal viewers, and timers do not accumulate. |
| Install a new app build | Authored source survives and incompatible runtime imports produce recoverable diagnostics. |
| Update the paired phone interface | The paired session and unsent text survive and recovery works. |
| Run without authoring | No extra inference calls or authoring payloads enter the normal conversation. |
| Make a small edit and a correction | Source reads stay relevant, preview jobs do not require model polling, and the main chat receives only a compact result. |

Record revision IDs, command results, measured latency, fresh screenshots, and the context and call measurements defined above as verification artifacts. Include visible checks for keyboard focus, text entry, scaling, scrolling, and reduced-motion preferences. Give Gwen access to these capabilities without enforcing a particular aesthetic.

Exit when the scripted checks pass and Gwen demonstrates three distinct changes herself: a complete workspace redesign, an original useful view, and a visual expression she chooses during a conversation. Restore each through chat and through the host recovery path.

## Hand off the work in verifiable units

Implement milestone 1 first and attach its working prototypes and runtime decision to the review. Then implement milestones 2 through 6 in order. Split milestone 3 by workflow if needed, with a working interface at each review point. Do not build a separate extension marketplace or a new backend permission system as part of this feature.

Keep the existing daily-use installation unchanged until the isolated end-to-end checks pass. Building the feature and activating it in the daily-use app are separate delivery steps. Make no commits, installation changes, or launches of external coding sessions as part of writing this plan.

The complete result is a Gwen whose entire interface she can author, inspect, use, and revise, with your work preserved and a recovery path independent of the design she creates.
