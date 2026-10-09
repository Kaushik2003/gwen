<design-context>

---

version: 1.0
name: Gwen-Workshop-Desktop
product: Gwen local AI companion and inference workspace
description: "A restrained, cinematic engineering-workstation UI for a personal AI companion. The visual reference is a believable maker's workstation: near-black graphite surfaces, compact technical panels, warm neutral text, disciplined red accents, practical diagnostic colors, code and terminal typography, and a persistent but unobtrusive character presence. It should feel like software built by a resourceful engineer, not a generic cyberpunk HUD or a marketing website. Every panel must serve a real task. Avoid decorative holograms, atmospheric gradients, excessive glow, oversized headings, and fake telemetry."

colors:
  canvas: "#0A0C0F"
  surface-1: "#11151A"
  surface-2: "#171C22"
  surface-3: "#1D242C"
  surface-4: "#252D36"
  hairline: "#29313B"
  hairline-strong: "#3A4653"
  hairline-subtle: "#20262D"
  ink: "#E8EDF2"
  ink-muted: "#B2BCC7"
  ink-subtle: "#84909D"
  ink-tertiary: "#606C78"
  accent: "#D94A57"
  accent-hover: "#EC606B"
  accent-pressed: "#B93643"
  accent-soft: "#351C22"
  accent-focus: "#F07A83"
  on-accent: "#FFFFFF"
  success: "#55C58A"
  success-soft: "#172D23"
  warning: "#E5B45B"
  warning-soft: "#302819"
  danger: "#F06A6A"
  danger-soft: "#351C1C"
  info: "#76B4E8"
  info-soft: "#192A38"
  telemetry-red: "#F06464"
  telemetry-amber: "#E5B45B"
  telemetry-blue: "#76B4E8"
  telemetry-green: "#55C58A"
  overlay: "#05070ACC"
  scrim: "#05070A99"
  inverse-canvas: "#F1F3F5"
  inverse-ink: "#11151A"

typography:
  display-xl:
    fontFamily: "Inter, Geist Sans, system-ui, sans-serif"
    fontSize: 36px
    fontWeight: 650
    lineHeight: 1.12
    letterSpacing: -1.1px
  display-lg:
    fontFamily: "Inter, Geist Sans, system-ui, sans-serif"
    fontSize: 28px
    fontWeight: 620
    lineHeight: 1.18
    letterSpacing: -0.6px
  display-md:
    fontFamily: "Inter, Geist Sans, system-ui, sans-serif"
    fontSize: 22px
    fontWeight: 600
    lineHeight: 1.25
    letterSpacing: -0.35px
  headline:
    fontFamily: "Inter, Geist Sans, system-ui, sans-serif"
    fontSize: 18px
    fontWeight: 600
    lineHeight: 1.30
    letterSpacing: -0.2px
  panel-title:
    fontFamily: "Inter, Geist Sans, system-ui, sans-serif"
    fontSize: 13px
    fontWeight: 600
    lineHeight: 1.35
    letterSpacing: 0
  body:
    fontFamily: "Inter, Geist Sans, system-ui, sans-serif"
    fontSize: 14px
    fontWeight: 400
    lineHeight: 1.50
    letterSpacing: 0
  body-sm:
    fontFamily: "Inter, Geist Sans, system-ui, sans-serif"
    fontSize: 12px
    fontWeight: 400
    lineHeight: 1.45
    letterSpacing: 0
  caption:
    fontFamily: "Inter, Geist Sans, system-ui, sans-serif"
    fontSize: 11px
    fontWeight: 450
    lineHeight: 1.35
    letterSpacing: 0.1px
  label:
    fontFamily: "Inter, Geist Sans, system-ui, sans-serif"
    fontSize: 11px
    fontWeight: 600
    lineHeight: 1.3
    letterSpacing: 0.45px
  button:
    fontFamily: "Inter, Geist Sans, system-ui, sans-serif"
    fontSize: 12px
    fontWeight: 600
    lineHeight: 1.2
    letterSpacing: 0
  mono:
    fontFamily: "JetBrains Mono, Geist Mono, ui-monospace, SFMono-Regular, monospace"
    fontSize: 12px
    fontWeight: 400
    lineHeight: 1.55
    letterSpacing: 0
  mono-sm:
    fontFamily: "JetBrains Mono, Geist Mono, ui-monospace, SFMono-Regular, monospace"
    fontSize: 11px
    fontWeight: 400
    lineHeight: 1.45
    letterSpacing: 0

rounded:
  xs: 3px
  sm: 4px
  md: 6px
  lg: 8px
  xl: 10px
  pill: 9999px
  full: 9999px

spacing:
  xxs: 4px
  xs: 8px
  sm: 10px
  md: 12px
  lg: 16px
  xl: 20px
  xxl: 24px
  section: 32px

layout:
  titlebar-height: 36px
  toolbar-height: 40px
  statusbar-height: 24px
  sidebar-width: 220px
  inspector-width: 280px
  panel-min-width: 220px
  compact-panel-gap: 1px
  content-padding: 12px

components:
  button-primary:
    backgroundColor: "{colors.accent}"
    textColor: "{colors.on-accent}"
    typography: "{typography.button}"
    rounded: "{rounded.md}"
    padding: "8px 12px"
  button-primary-hover:
    backgroundColor: "{colors.accent-hover}"
    textColor: "{colors.on-accent}"
    typography: "{typography.button}"
    rounded: "{rounded.md}"
    padding: "8px 12px"
  button-primary-pressed:
    backgroundColor: "{colors.accent-pressed}"
    textColor: "{colors.on-accent}"
    typography: "{typography.button}"
    rounded: "{rounded.md}"
    padding: "8px 12px"
  button-secondary:
    backgroundColor: "{colors.surface-2}"
    textColor: "{colors.ink}"
    typography: "{typography.button}"
    rounded: "{rounded.md}"
    padding: "7px 11px"
    border: "1px solid {colors.hairline}"
  button-quiet:
    backgroundColor: "transparent"
    textColor: "{colors.ink-muted}"
    typography: "{typography.button}"
    rounded: "{rounded.md}"
    padding: "7px 9px"
  icon-button:
    backgroundColor: "transparent"
    textColor: "{colors.ink-muted}"
    rounded: "{rounded.md}"
    size: "28px"
  panel:
    backgroundColor: "{colors.surface-1}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.sm}"
    border: "1px solid {colors.hairline}"
  panel-header:
    backgroundColor: "{colors.surface-1}"
    textColor: "{colors.ink-muted}"
    typography: "{typography.panel-title}"
    height: "32px"
    padding: "0 10px"
  text-input:
    backgroundColor: "{colors.canvas}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.md}"
    padding: "9px 10px"
    border: "1px solid {colors.hairline-strong}"
  text-input-focused:
    backgroundColor: "{colors.canvas}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.md}"
    padding: "9px 10px"
    border: "1px solid {colors.accent-focus}"
  tab-default:
    backgroundColor: "transparent"
    textColor: "{colors.ink-subtle}"
    typography: "{typography.body-sm}"
    padding: "8px 10px"
  tab-selected:
    backgroundColor: "{colors.surface-2}"
    textColor: "{colors.ink}"
    typography: "{typography.body-sm}"
    padding: "8px 10px"
    borderBottom: "2px solid {colors.accent}"
  status-neutral:
    backgroundColor: "{colors.surface-3}"
    textColor: "{colors.ink-muted}"
    typography: "{typography.caption}"
    rounded: "{rounded.sm}"
    padding: "3px 6px"
  status-success:
    backgroundColor: "{colors.success-soft}"
    textColor: "{colors.success}"
    typography: "{typography.caption}"
    rounded: "{rounded.sm}"
    padding: "3px 6px"
  status-warning:
    backgroundColor: "{colors.warning-soft}"
    textColor: "{colors.warning}"
    typography: "{typography.caption}"
    rounded: "{rounded.sm}"
    padding: "3px 6px"
  status-error:
    backgroundColor: "{colors.danger-soft}"
    textColor: "{colors.danger}"
    typography: "{typography.caption}"
    rounded: "{rounded.sm}"
    padding: "3px 6px"
  chat-user:
    backgroundColor: "{colors.surface-3}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.lg}"
    padding: "10px 12px"
  chat-gwen:
    backgroundColor: "{colors.surface-1}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.lg}"
    padding: "10px 12px"
    border: "1px solid {colors.hairline}"
  code-panel:
    backgroundColor: "{colors.canvas}"
    textColor: "{colors.ink-muted}"
    typography: "{typography.mono}"
    rounded: "{rounded.sm}"
    border: "1px solid {colors.hairline}"
  terminal-panel:
    backgroundColor: "{colors.canvas}"
    textColor: "{colors.ink-muted}"
    typography: "{typography.mono}"
    rounded: "{rounded.sm}"
    border: "1px solid {colors.hairline}"
  telemetry-panel:
    backgroundColor: "{colors.surface-1}"
    textColor: "{colors.ink}"
    typography: "{typography.mono-sm}"
    rounded: "{rounded.sm}"
    border: "1px solid {colors.hairline}"
  voice-control-idle:
    backgroundColor: "{colors.surface-2}"
    textColor: "{colors.ink}"
    rounded: "{rounded.full}"
    size: "44px"
  voice-control-active:
    backgroundColor: "{colors.accent-soft}"
    textColor: "{colors.accent-hover}"
    rounded: "{rounded.full}"
    size: "44px"
  divider:
    backgroundColor: "{colors.hairline}"
    height: "1px"
  tooltip:
    backgroundColor: "{colors.surface-4}"
    textColor: "{colors.ink}"
    typography: "{typography.caption}"
    rounded: "{rounded.sm}"
    padding: "5px 7px"

---

## 1. Design Intent

Gwen is a local AI companion presented through a desktop engineering workspace. Its interface borrows the credibility of a practical, homemade engineering workstation: project files, a code/editor surface, a terminal, useful diagnostics, compact system status, and deliberate visual restraint.

This is an **application UI, not a marketing site**. The design must support three connected modes without feeling like three unrelated products:

1. **Companion mode** — natural conversation, voice interaction, character expression, and recent context.
2. **Workspace mode** — tools, files, task activity, notes, and any work Gwen is doing on the user's behalf.
3. **Lab mode** — model selection, inference events, token throughput, memory use, latency, and debugging.

The companion experience is the default. Engineering panels are available when useful and must not permanently crowd out conversation or Gwen's character.

### Design principle

**Believable utility before cinematic decoration.** A panel exists because it answers a question or enables an action. The UI can feel cinematic through hierarchy, compactness, real activity, and a lived-in technical sensibility—not through neon, random numbers, fake scanlines, or floating holograms.

### Core characteristics

- Graphite-black canvas with slightly lifted charcoal panels.
- Restrained red accent used for selection, active controls, and a small amount of brand identity.
- Green, amber, red, and blue reserved for meaningful status and telemetry.
- Compact panel headers, thin borders, readable labels, and precise alignment.
- Sans-serif UI text paired with monospaced code, logs, IDs, and measured values.
- Hard-edged, practical panel geometry; moderate corner rounding only on controls and conversation bubbles.
- A character surface that feels intentionally composed, not like an emoji floating over a dashboard.
- Functional motion tied to state changes; no idle animation that constantly distracts or burns resources.
- Clear offline/local status. Never imply a cloud connection or tool action that did not happen.

### Do not imitate

Do not reproduce proprietary movie frames or pretend the interface is an exact screen from a film. Treat the film's workshop-software feel as a broad reference: grounded engineering tools, a compact interface, and technology with a reason to exist.

## 2. Color System

### Surfaces

- **Canvas** (`{colors.canvas}`): primary app background and deep work area.
- **Surface 1** (`{colors.surface-1}`): default panels, conversation region, sidebars, and editor chrome.
- **Surface 2** (`{colors.surface-2}`): raised controls, selected rows, inputs, and nested content.
- **Surface 3** (`{colors.surface-3}`): active list row, code selection background, and emphasized nested regions.
- **Surface 4** (`{colors.surface-4}`): menus, tooltips, popovers, and floating command palette.
- **Hairline** (`{colors.hairline}`): separators and default panel borders.
- **Hairline Strong** (`{colors.hairline-strong}`): input outlines, resizers, and important panel boundaries.

Surface changes—not heavy shadows—provide most of the depth. Keep adjacent surfaces close enough in value that the screen reads as one instrument rather than a stack of floating cards.

### Accent

- **Workbench Red** (`{colors.accent}`): selected navigation, primary action, recording/active interaction marker, and the occasional important highlight.
- **Accent Hover** (`{colors.accent-hover}`): hover and active emphasis.
- **Accent Pressed** (`{colors.accent-pressed}`): pressed state.
- **Accent Soft** (`{colors.accent-soft}`): subtle active-state background behind icons or status indicators.
- **Accent Focus** (`{colors.accent-focus}`): keyboard focus outline only.

Red must remain scarce. Do not apply it to every icon, panel border, chart, and label. If all elements are accent-colored, no element feels important.

### Semantic colors

- **Success green** (`{colors.success}`): completed actions, connected devices, healthy services, successful builds.
- **Warning amber** (`{colors.warning}`): degraded performance, elevated memory pressure, pending approval, or attention required.
- **Danger red** (`{colors.danger}`): failures, disconnected critical services, destructive actions, and errors. Use the semantic danger token rather than the brand accent when communicating an actual problem.
- **Info blue** (`{colors.info}`): neutral technical signals, selected telemetry series, and informational events.

Semantic color always accompanies a text label, icon, or shape difference. Do not communicate status by color alone.

### Telemetry colors

The telemetry series have stable identities to make live charts readable across sessions: red for pressure/latency, amber for queue or temperature-like signals, blue for memory/token rate, and green for healthy throughput. Choose series according to their meaning and keep the mapping consistent in every chart.

Do not show made-up values. When metrics are unavailable, display `Unavailable`, `Waiting for data`, or `Not measured` instead of sample numbers that look real.

### Contrast and restraint

- Primary content uses `ink`; secondary labels use `ink-muted`.
- `ink-subtle` is for low-priority metadata, not essential instructions.
- `ink-tertiary` is limited to disabled controls and very low-priority annotations.
- Never use a low-contrast label for an action the user needs to find.
- Avoid gradients in normal surfaces. A barely perceptible tint is acceptable only when it communicates a state, not atmosphere.

## 3. Typography

### Font families

- **UI sans:** Inter, with Geist Sans or the system sans-serif as fallbacks. Use for navigation, conversation, labels, buttons, and panel headings.
- **Technical mono:** JetBrains Mono, with Geist Mono or system monospace as fallbacks. Use for code, terminal output, paths, timestamps, model IDs, token counts, and measured values.

Do not depend on proprietary Linear fonts. Gwen must render consistently on Linux, Windows, and macOS.

### Hierarchy

| Token | Size | Weight | Usage |
|---|---:|---:|---|
| `display-xl` | 36px | 650 | Rare empty-state or setup heading; never a persistent dashboard title |
| `display-lg` | 28px | 620 | Setup and first-run screens |
| `display-md` | 22px | 600 | Main workspace title or major mode title |
| `headline` | 18px | 600 | Conversation title, settings section heading |
| `panel-title` | 13px | 600 | Panel headers and compact section headings |
| `body` | 14px | 400 | Main interface, conversation, forms |
| `body-sm` | 12px | 400 | Supporting controls and secondary descriptions |
| `caption` | 11px | 450 | Timestamps, compact metadata, status details |
| `label` | 11px | 600 | Uppercase-like technical labels; use sparingly |
| `button` | 12px | 600 | Button labels |
| `mono` | 12px | 400 | Editor, command output, file paths |
| `mono-sm` | 11px | 400 | Dense telemetry and compact logs |

### Type rules

- Desktop application chrome is compact. Do not use marketing-site-scale headlines in persistent app views.
- Prefer sentence case. Uppercase labels are allowed for short technical panel labels, not paragraphs.
- Reserve monospace for information that benefits from fixed-width alignment. Do not set ordinary conversation messages in monospace.
- Use tabular numerals for live metrics, counters, timestamps, and token rates to avoid layout jitter.
- Keep code line-height comfortable and preserve indentation.
- Do not use extreme negative tracking. Readability is more important than mimicking a brand's typography.

## 4. Application Layout

### Desktop frame

The desktop app fills the usable window. Avoid marketing-site containers, centered landing-page sections, giant hero areas, pricing cards, customer logos, or testimonial layouts.

The standard frame consists of:

1. **Title bar** — app name/mark, current workspace or conversation, and native window controls where available.
2. **Primary navigation rail** — compact entry points for Companion, Workspace, Lab, and Settings.
3. **Main work area** — conversation or selected workspace content.
4. **Context/character inspector** — optional area for Gwen's character presence, session context, task details, or system metrics. It can be hidden to give the main work area more room.
5. **Status bar** — concise local runtime, model, microphone, and task status when those states are known.

Do not show all possible panels at once by default. Start with the smallest composition that makes the current task clear, and let the user reveal advanced panels when they are useful.

### Recommended companion view

- Main area: conversation timeline, streaming response, and voice/text composer.
- Character area: Gwen's sprite/animation stage with a small, clear activity indicator.
- Optional context drawer: recent task, memory/context indicators, active tool, and model health.
- Footer/status line: current model, local/remote mode, microphone state, and active task state if applicable.

### Recommended lab view

- Left: model and runtime selection, configuration, and session list.
- Center: conversation/inference trace or selected test output.
- Bottom or secondary tab: logs and benchmark output.
- Right: measured stats such as prompt tokens, generated tokens, time-to-first-token, tokens per second, VRAM/RAM, and context length.
- Each statistic must show the source and unit where ambiguity is possible. Distinguish measured values, estimates, and configured limits.

### Resizing and panel behavior

- Panels use draggable splitters only where resizing provides genuine value.
- Support collapsing the navigation rail and context inspector.
- Persist panel choices if the app has settings storage; otherwise use stable defaults.
- Prevent a panel from shrinking until its controls become unusable. At the minimum width, collapse it or switch to tabs.
- Use one-pixel separators and a small gap between adjacent panels. Do not wrap every nested item in its own card.

### Density

This is an information-dense desktop tool, but it must remain scannable. Use compact spacing within technical panels and more breathing room around conversation content and Gwen's character. Dense does not mean cramped: group related information, align values, and avoid redundant labels.

## 5. Navigation and Window Chrome

### Navigation rail

Primary destinations:

- **Companion** — conversation, voice controls, character state.
- **Workspace** — tasks, notes, files, tool results, recent activity.
- **Lab** — model runtime, inference metrics, traces, and benchmarks.
- **Settings** — models, audio, appearance, permissions, and local data.

Use simple icons with short labels. The selected item gets a muted red marker or a subtle accent background, never a large glowing block. A tool or feature that is unavailable should be disabled with a clear reason where possible.

### Title bar

- Height: 36px.
- Show the current context and essential window actions.
- Keep branding compact; do not turn the title bar into a hero banner.
- If a workspace is busy, show a small activity indicator and a text label rather than an animated decoration.

### Status bar

- Height: 24px.
- Use for truthful, compact state: `Local model ready`, `Generating`, `Mic off`, `Task awaiting approval`, or `Runtime disconnected`.
- Do not overload this area with permanent CPU, memory, GPU, model, microphone, and network widgets simultaneously. Prioritize the active context; put deeper metrics in Lab.

## 6. Gwen Character and Expression System

Gwen is a character, not a decorative logo. Her visual representation should feel intentional, consistent, and lightweight.

### Character stage

- Give the character a defined region with a clear background and predictable bounds.
- Preserve the art's proportions and silhouette. Never stretch a sprite to fill arbitrary space.
- Avoid placing dense telemetry, controls, or chat text on top of the character.
- The character can have a subtle idle loop, but must settle into a still frame when the app is inactive or reduced motion is enabled.
- The character region may collapse to a small portrait/indicator in compact layouts. It must not force the conversation into a narrow column.

### Character states

The character state is driven by the real conversation pipeline, not random animation:

| State | Trigger | Visual response |
|---|---|---|
| `idle` | No active interaction | Low-motion idle or still pose |
| `listening` | Microphone capture is active | Attentive pose; subtle audio-reactive cue if input level is available |
| `processing` | A request is being processed | Focused pose; restrained indicator; no fake thought timer |
| `speaking` | Voice output is playing | Speaking animation synchronized to audio where feasible |
| `happy` | Positive response or explicit persona expression | Brief expression change, then return to neutral |
| `confused` | Gwen asks for clarification or reports uncertainty | Brief puzzled expression; do not use when the model is merely slow |
| `annoyed` | Explicitly chosen persona reaction, only when contextually appropriate | Controlled and brief expression; never obscure important errors |
| `error` | A real runtime, audio, or tool error occurs | Clear status message plus a restrained concerned/error pose |
| `offline` | Runtime is unavailable | Still/quiet pose with explicit offline status |

Do not claim the character is listening when microphone permission is denied or capture is stopped. Do not imply she is speaking while audio playback is paused. Character states must reflect actual application state.

### Sprite consistency

- Prefer a curated set of authored states or layered assets over unconstrained, on-the-fly image generation during runtime.
- Keep face shape, hair, outfit, outline weight, and palette consistent across expressions.
- If frames are sliced from a sprite sheet, validate transparent margins and frame bounds so neighboring frames do not bleed into one another.
- Store state-to-frame mapping in one place. Do not duplicate animation rules throughout UI components.
- Provide a still-image fallback for missing assets, reduced-motion preferences, or performance-sensitive operation.

### Persona and UI separation

The character's emotional tone belongs to Gwen's conversation/persona layer. The UI should not infer emotion solely from punctuation or a sentiment score. Let the conversation manager explicitly request a character state, and apply guardrails so important system errors and safety messages always remain clear.

## 7. Conversation and Voice Interaction

### Conversation timeline

- User messages and Gwen messages must have distinct, consistent alignment or surface treatment.
- Keep message widths comfortable for reading; do not stretch long text edge to edge across a wide desktop.
- Stream tokens into a stable message container. Avoid shifting the entire layout on every token.
- Show clear timestamps only when useful; avoid a timestamp on every tiny streamed chunk.
- Tool calls, summaries, and long reasoning/activity should use a compact expandable event row rather than being dumped into the user's conversation as raw logs.
- Code blocks use technical mono, syntax contrast, and copy controls. Long code may scroll horizontally within its own container.
- Provide visible controls to stop generation, retry a failed answer, copy output, and start a new conversation where supported.

### Composer

- Use a compact multi-line input that grows to a maximum height, then scrolls internally.
- Keyboard Enter/Shift+Enter behavior must be visible or documented and must not interfere with accessibility.
- Voice and send controls must be distinct. Do not make microphone capture a hidden side effect of clicking the composer.
- Show recording state, input device/permission errors, and the ability to stop capture.
- When generating audio, provide pause/stop controls and an obvious indication that Gwen is speaking.
- Support interruption: a new user input or explicit stop should cancel or queue work according to a clearly defined interaction contract.

### Voice states

- **Idle:** microphone inactive; neutral microphone icon.
- **Listening:** active capture indicator with clear stop/mute control.
- **Transcribing:** show interim recognized text only if the speech pipeline provides it.
- **Processing:** indicate that the request is being handled, without fake progress percentages.
- **Speaking:** indicate playback and offer stop/pause when available.
- **Error:** explain the actual issue, such as missing permission or unavailable device.

### Audio visualization

- A waveform or level meter is optional and only appears where it helps explain live audio.
- Visual amplitude must come from actual input/output audio levels when possible. If it is merely decorative, keep it subtle and do not label it as measured data.
- Animation should not run when audio is inactive.
- Respect reduced-motion settings and avoid flashing or rapid pulsing.

## 8. Engineering Workspace Panels

### Project/file tree

- Use compact rows with clear nesting, small icons, and visible selection.
- Distinguish folders, source files, models, logs, images, and generated artifacts.
- Avoid excessive colored file icons. Color should help recognition rather than decorate every row.
- Long paths truncate in the middle where possible and expose the full path via tooltip/copy action.
- Empty folders and failed loading states need useful explanations.

### Code editor

- Monospace text with stable line-number alignment.
- Use restrained syntax colors against the canvas; preserve contrast and semantic conventions.
- Highlight current line and selection with subtle surface changes, not large red blocks.
- Keep editor chrome compact and distinguish modified/unsaved state clearly.
- Long lines scroll within the editor instead of widening the whole window.
- Never present a static code mockup as an editable editor unless editing is actually supported.

### Terminal and logs

- Use a near-black inner canvas, monospaced content, and consistent log-level colors.
- Separate prompt/command, stdout, stderr, and system events through labels or icons in addition to color.
- New output should not force the user to lose their scroll position. Auto-follow only while the user is already at the bottom; show a `Jump to latest` affordance otherwise.
- Long logs need search, copy, and clear/export controls where appropriate.
- Do not fabricate commands, build success, model downloads, or completed tasks.

### Diagnostic and telemetry panels

Show only metrics that the running system actually exposes. Likely measures include:

- Model/runtime name and version.
- Time to first token and total response latency.
- Prompt and generated token counts.
- Token generation rate, with units.
- Context length/current context occupancy.
- CPU and system RAM usage.
- GPU utilization and VRAM usage when supported by the hardware/runtime.
- Queue/worker state and active tool status, when applicable.

Rules:

- Label units explicitly (`ms`, `tokens/s`, `MiB`, `%`).
- Distinguish `0` from `unknown`, `not supported`, and `not available`.
- Do not display invented values in production UI. Demo mode must be clearly labelled as simulated.
- Charts should answer a question. If a compact numeric value is clearer than a chart, use the number.
- Keep charts grid-light, with legible labels and stable series colors. Avoid decorative grids and excessive ticks.

### Task/activity panel

- Show a task's actual state: queued, running, needs approval, completed, failed, or cancelled.
- Display concise progress from real milestones only. Do not turn unknown-duration model work into fabricated percentage progress.
- Tool actions with side effects should be individually understandable and require approval when the action's risk warrants it.
- Preserve a small event history so users can understand what Gwen did and why.

## 9. Inputs, Buttons, and Controls

### Buttons

- Primary red button: one primary action per local context, such as Send, Run, or Confirm.
- Secondary button: common alternatives such as Cancel, Retry, or Open.
- Quiet button: low-priority navigation and inline actions.
- Icon-only buttons require tooltips and accessible names.
- Loading actions must remain understandable and avoid duplicate submission.
- Destructive actions use danger styling and clear wording; brand red alone is not enough to signal deletion.

### Inputs

- Use the same surface and border rules throughout the app.
- Focused inputs use an accessible focus outline, not only a change in fill color.
- Labels remain visible when a field is populated. Placeholder text is not a label.
- Show validation messages adjacent to the relevant control and explain how to recover.
- Disabled fields must be visibly disabled and should explain why when that is not obvious.

### Tabs and segmented controls

- Use tabs for different views of the same content (for example, Overview, Logs, Benchmarks).
- Use segmented controls only for a small set of mutually exclusive settings.
- Active state uses a subtle surface lift plus a thin accent marker; never fill the entire tab with saturated red by default.

### Menus and command palette

- Menus use surface 4, fine borders, and restrained depth.
- Keep actions grouped and keyboard navigable.
- The command palette should search real actions and destinations. Do not show nonfunctional sample commands.

## 10. Shapes, Borders, and Elevation

### Radius scale

| Token | Value | Usage |
|---|---:|---|
| `rounded.xs` | 3px | Dense badges and small status markers |
| `rounded.sm` | 4px | Panel corners and technical containers |
| `rounded.md` | 6px | Buttons and form inputs |
| `rounded.lg` | 8px | Conversation bubbles and larger controls |
| `rounded.xl` | 10px | Character stage or a large, intentional workspace region |
| `rounded.pill` | 9999px | Small status dots/pills only when the pill shape is appropriate |

Prefer modest rounding. Do not apply a 16–24px radius to every panel; that makes the engineering workspace look like a consumer dashboard template.

### Elevation

| Level | Treatment | Usage |
|---|---|---|
| 0 | Canvas, no border | Main app background and editor canvas |
| 1 | Surface 1 + hairline | Panels, sidebars, conversation region |
| 2 | Surface 2 + stronger hairline | Nested controls and selected rows |
| 3 | Surface 3/4 + hairline | Menus, command palette, floating popovers |
| Focus | 2px accent-focus outline | Keyboard focus |

Avoid large shadows and glows. Use z-index and border contrast to show overlap. A soft shadow may be used for a true floating menu if it improves separation, but it must not look atmospheric.

## 11. Motion and Feedback

Motion explains a state transition; it is not decoration.

- Typical UI transition: 120–180ms.
- Panel expand/collapse: 160–220ms where it does not impede interaction.
- Character expression transition: brief and asset-driven; avoid looping a full animation unnecessarily.
- Voice level animation runs only while real audio is active.
- Streaming indicators remain calm; no rapid pulsing or flashing.
- Respect OS reduced-motion settings. When enabled, replace motion with static state changes.
- Avoid parallax, animated scanlines, constantly moving backgrounds, fake typing cursors, and perpetual particle effects.
- Do not animate layout in a way that causes text or controls to jump while a user is interacting.

### Feedback states

Every async operation needs a useful set of states where relevant: idle, in progress, success, failure, and cancelled. Show the state in context, keep failure messages actionable, and never mark an operation successful before the runtime confirms it.

## 12. Accessibility and Usability

- All actions must be reachable by keyboard.
- Focus indicators remain visible on dark surfaces.
- Interactive targets should be at least 32px in dense desktop chrome and preferably 40–44px for prominent controls and touch use.
- Color is never the only status channel; pair it with icon/text/shape.
- Support OS reduced-motion settings.
- Provide accessible names for icon-only controls and meaningful labels for form fields.
- Avoid tiny low-contrast text for essential state or controls.
- Ensure screen readers can distinguish new messages, tool events, and runtime errors without announcing every streamed token.
- Do not trap keyboard focus in a non-modal panel.
- Keep keyboard shortcuts discoverable and avoid overriding common platform shortcuts without a strong reason.

## 13. Responsive and Window-Size Behavior

Gwen is a desktop-first app, not a responsive marketing site. Layout behavior is based on available window width, not a fixed set of marketing breakpoints.

| Available width | Layout behavior |
|---|---|
| 1440px and above | Navigation rail, main work area, optional inspector |
| 1200–1439px | Keep main area and inspector if both remain useful; allow inspector collapse |
| 900–1199px | Collapse inspector by default; allow navigation rail to compact |
| 680–899px | Main area takes priority; advanced panels become tabs or drawers |
| Below 680px | Single active workspace view; keep composer and primary controls available |

- The conversation composer must remain usable at all widths.
- Never let the character inspector push the conversation into an unreadable width; collapse it first.
- Technical panels should switch to tabs or drawers before text becomes cramped.
- Resizing the window must not reset the current conversation or task state.
- Prefer stable layouts over complicated responsive rearrangements.

## 14. Empty, Loading, Error, and Offline States

### Empty states

Explain what the user can do next. Example: `No active task` with an action to start a task, rather than a decorative illustration with no instruction.

### Loading states

- Use skeletons only when the eventual content has a stable shape.
- Use a small spinner or status message for brief actions.
- Avoid fabricated percentage progress.
- Do not replace a usable workspace with a full-screen loader for background work.

### Error states

- Identify which part failed: model runtime, microphone, output device, tool, or file operation.
- Explain one useful next action where possible.
- Preserve user input and conversation history after recoverable failures.
- Use danger color alongside a readable message; never rely only on a red icon.

### Offline/local runtime

Local-first is part of the product's identity. Make the runtime state explicit without making it alarming. Show whether the model is loaded, loading, stopped, unavailable, or remote when that fact is known. Do not imply data is local if the selected runtime sends requests elsewhere.

## 15. Privacy and Trust Signals

- Indicate microphone capture with an obvious, state-accurate control.
- Explain when a tool needs permission or approval.
- Distinguish model output from tool results and system events.
- Show which model/runtime is active when that affects latency, privacy, or capability.
- Do not show a green `Connected` label unless the relevant service is genuinely connected.
- Avoid collecting or displaying content merely to make the interface feel alive.
- Confirm before destructive actions or irreversible operations.

## 16. Do's and Don'ts

### Do

- Use graphite surfaces and fine borders to create the feel of a compact engineering workstation.
- Reserve red for the primary action, active navigation, and a small number of purposeful highlights.
- Use semantic colors consistently for real status and diagnostic meaning.
- Keep navigation, panel headers, and the status bar compact.
- Give Gwen a consistent character stage and state machine.
- Make conversation the default and expose the technical workspace when it helps.
- Use actual runtime events for listening, generating, speaking, task activity, and metrics.
- Use monospace for code, logs, paths, and measured values.
- Prefer functional panels over decorative dashboards.
- Keep keyboard navigation, reduced-motion support, and error recovery in scope from the beginning.

### Don't

- Do not retain Linear's lavender accent as the product identity.
- Do not use the old marketing-site composition of hero headlines, pricing cards, testimonials, logo strips, and 96px section gaps inside the desktop app.
- Do not turn every panel into a separate rounded card.
- Do not use neon cyan, multiple bright accents, heavy glows, scanlines, holographic rings, or atmospheric gradients as decoration.
- Do not display fake CPU, GPU, memory, token-rate, pressure, or connection values.
- Do not use giant uppercase labels or tiny unreadable metadata to make the UI appear technical.
- Do not let the character overlap or obscure active controls and conversation text.
- Do not animate the UI continuously when no interaction is occurring.
- Do not treat every response as a success state or every delay as an error.
- Do not reproduce exact copyrighted movie screens or imply the design is an official film interface.

## 17. Implementation Guidance

### Component architecture

Keep the design system independent of inference logic. The UI should consume explicit application state and render from it, rather than importing model runtime details into every component.

Suggested component areas:

- `AppShell` — title bar, navigation, workspace region, status bar.
- `CompanionView` — message timeline and composer.
- `CharacterStage` — sprite rendering and expression state.
- `VoiceControls` — microphone, transcription, playback, stop/interrupt actions.
- `WorkspaceView` — tasks, files, tool output, and recent activity.
- `LabView` — model/runtime configuration, metrics, logs, and benchmarks.
- `Panel`, `PanelHeader`, `StatusIndicator`, `MetricValue`, `EventRow` — reusable interface primitives.

### State contract

The UI should receive explicit, typed state such as:

- `runtimeStatus`: `offline | loading | ready | generating | error`.
- `audioInputStatus`: `inactive | listening | transcribing | error`.
- `audioOutputStatus`: `inactive | speaking | paused | error`.
- `characterState`: `idle | listening | processing | speaking | happy | confused | annoyed | error | offline`.
- `taskStatus`: `idle | queued | running | needs_approval | completed | failed | cancelled`.

Map these states to visible labels and animation rules in one place. Keep them synchronized with actual events, and define priority rules for overlapping states—for example, a runtime error must not be hidden by a happy character animation.

### Performance

- Prefer CSS transitions and a small number of active sprite frames over expensive full-screen effects.
- Pause character animation when the window is hidden or inactive when practical.
- Avoid redraw-heavy charts unless metrics are changing and the chart is visible.
- Keep telemetry sampling separate from UI render frequency.
- Ensure a character animation cannot starve audio playback or local inference resources.
- Render long logs and histories efficiently; virtualize only when the data volume justifies the complexity.

### Validation checklist

Before considering a screen complete, verify:

1. Every visible button performs a real action or is clearly disabled.
2. All statuses come from real application state.
3. No fabricated telemetry or fake progress is visible.
4. The selected, hover, focus, pressed, disabled, loading, and error states are defined where applicable.
5. Keyboard navigation works and focus is visible.
6. Reduced-motion behavior works.
7. Gwen's sprite proportions and frame boundaries remain consistent.
8. The layout remains useful when the inspector is hidden and the window is narrow.
9. Text is readable at normal desktop scale.
10. The interface remains calm when the model is idle and informative when it is active.

## 18. Known Gaps and Decisions to Confirm

- Exact desktop framework and native window chrome are not specified here.
- Final Gwen sprite dimensions, frame naming, and permitted animation frame rate should be defined in the character asset manifest.
- Telemetry fields depend on what the local inference engine and hardware expose; unavailable metrics must remain unavailable rather than simulated.
- Keyboard shortcuts should be documented in a separate interaction reference once the main workflows are implemented.
- The design system sets visual rules; it does not define model behavior, memory policy, tool permissions, or inference scheduling.

## 19. Iteration Guide

1. Build `AppShell` and the default companion view before implementing secondary dashboards.
2. Establish the token values in one source of truth and use semantic names instead of raw hex values in components.
3. Implement and test explicit state mapping for voice, runtime, task, and character states.
4. Add the workspace and Lab panels only after the core conversation flow is comfortable.
5. Validate at a normal desktop window size before optimizing very wide layouts.
6. Test with real runtime telemetry; clearly label any development fixture data.
7. Review contrast, focus order, reduced motion, and minimum-width behavior as part of each UI change.
8. Keep this file focused on Gwen's application UI. Document landing pages or public marketing pages separately if they are ever needed.

</design-context>

Use the design system above for all Gwen desktop UI you generate.
