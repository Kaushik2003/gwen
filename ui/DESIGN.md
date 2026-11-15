# Gwen's look

One design system for the Gwen window (`ui/src` outside `mobile/`, plan.md 3.15) and the phone app (`ui/src/mobile`).
The approved mockups are the "Gwen, one app" canvas (plan.md 3.15); this file is the rule set they follow. The
reference implementations are `ui/src/index.css` with `ui/src/shell/` for the window, and `ui/src/mobile/mobile.css`
for the phone; when this file and those disagree, fix whichever is wrong in the same change.

## 0. The Gwen window

- **Palette.** Ghost by default (plan.md 5.2), Tide and Ember in Settings › Appearance. Tokens live in
  `ui/src/index.css` (`canvas`, `surface-1..4`, `line`, `ink`, `ink-muted`, `ink-subtle`, `accent`, the status
  colours) plus the shell's chrome: `chrome` (sidebar and side panel), `chrome-line` (their hairlines), and
  `chrome-deep` (tab bar, status bar, strips). Tracking colours (working, idle, break, off) never change with the
  theme: they match the tray icons.
- **Frame.** Sidebar 256 px; tab bar 44 px; status bar 30 px; side panel 340 px; a terminal's task strip 40 px.
  Base text 13 px in the chrome. Rows 26 to 30 px high, radii 7 px for rows and tabs, 10 px for the Now card,
  12 px for cards.
- **Sidebar** (top to bottom): brand and search, the Now card, Today, Inbox, Gwen, Plan, Review, Projects with
  their live sessions nested under them on a hairline guide, then Settings and the phone. The selected row is
  `surface-2` with an inset 1 px `line` ring; others are `ink-muted` and brighten on hover.
- **Session state** is a glyph beside its name, never colour alone: an amber dot with a soft ring for needs you, a
  thin blue spinner for working, a green tick for finished unread, a red dot for an error. Claude is marked with a
  small orange asterisk; a shell with a terminal icon.
- **Cards** are `surface-1` with a `line` hairline; a card's list rows are separated by `chrome-line`. One primary
  (accent) button per card.
- **Code and terminals** are Geist Mono. Diffs use `success` for added and `danger` for removed lines, tinted 7 to
  15% into the surface; long lines wrap in diffs and scroll sideways only in the file editor.
- **Keys.** Ctrl K (or Ctrl Shift P) opens everything; Ctrl W closes a tab, Ctrl Tab cycles, Ctrl 1 to 9 jump, Ctrl \
  shows the side panel. Inside a terminal Ctrl W, Ctrl K, and Ctrl \ belong to the program, so the window uses
  Ctrl Shift W and Ctrl Shift P there.

The phone follows the sections below.

## 1. Colour comes from the theme

Nothing in Gwen hard-codes a colour. A theme is a terminal palette: `background`, `foreground`, `selection`,
`cursor`, and 16 ANSI colours. Everything around the terminal is derived from it, so every theme is consistent
and a new theme needs nothing but its palette.

| Token | Derived from | Used for |
|---|---|---|
| `--bg` | `background` | the page, bars, the terminal |
| `--fg` | `foreground` | text |
| `--muted` | background mixed 55% toward foreground | secondary text, idle status, icons |
| `--surface` | background mixed 5% toward foreground | cards, fields, sheets |
| `--surface-2` | background mixed 9% toward foreground | pressed and selected surfaces, secondary buttons |
| `--border` | background mixed 13% toward foreground | every hairline |
| `--accent` | ANSI blue (`ansi[4]`) | the one accent: primary buttons, focus, selection, links |
| `--on-accent` | `background` | text on the accent |
| `--working` | ANSI yellow (`ansi[3]`) | an agent that is working |
| `--blocked` | ANSI red (`ansi[1]`) | an agent that needs you, errors, destructive actions |
| `--done` | ANSI blue (`ansi[4]`) | an agent that finished, the unread dot |
| `--ok` | ANSI green (`ansi[2]`) | added lines, success |

- The phone reads Gwen Terminal's active theme from the laptop (`GET /v1/code/spaces`, `theme`) and sets these
  variables, so the phone always matches the laptop. Herdr's built-in palettes are in `term/themes.json`; the default
  is **graphite** (`#17191d` on `#d2d4d9`).
- A theme is dark or light by its background. Never assume dark: use the tokens, and `color-mix` for tints.
- Status is never colour alone. A status dot always sits beside the status in words ("Needs you", "Working").
- Tints of a status colour (a waiting card, a diff line) are that colour mixed 7 to 15% into the surface.

## 2. Type

- **Geist** for interface text and **Geist Mono** for anything the computer said: paths, commands, branches, diffs,
  terminal output, key caps. Both ship with the app (`@fontsource-variable/geist`, `@fontsource-variable/geist-mono`);
  never load a font from the network.
- Sizes on the phone: 17/600 screen titles, 15/550 row titles, 15 body, 13 secondary lines, 12 to 12.5 meta and
  mono details, 11.5/600 uppercase section labels with 0.06em tracking. Fields are 16 so the phone doesn't zoom
  when one is focused.
- Weight carries hierarchy: 400 body, 500 to 550 for labels and buttons, 600 to 650 for titles. No italics for
  emphasis, no all-caps except section labels.
- Sentence case everywhere: "New session", "Open on laptop".

## 3. Space and shape

- A 4-point rhythm. Rows have 12 by 14 padding; screens have 14 side padding; gaps inside a row are 8 to 11.
- Radii: 8 small buttons and key caps, 10 buttons and fields, 12 option cards and waiting cards, 16 sheet tops, full
  for dots, chips and round buttons.
- Hairlines, not shadows. Surfaces are separated by a 1px `--border` line or a one-step surface change. The only
  shadow is under a toast.
- Touch targets are at least 42 high (44 for primary buttons). Icon buttons are 38 square inside a 52 bar.
- Respect the safe areas: bars add `env(safe-area-inset-top)`, bottom bars add `env(safe-area-inset-bottom)`.

## 4. Components

- **Bar**: title on the left (with a one-line subtitle in muted 12), icon buttons on the right, a back arrow on the
  left on pushed screens. One hairline below.
- **Row / card**: status dot, title, then a muted line; meta (time, unread dot) on the right. A pressed row turns
  `--surface`. Rows are full-width with a hairline between them, never floating cards with gaps.
- **Status dot**: 9px. Working pulses slowly; needs-you and error are red; done is blue; idle is muted; ended is a
  ring. Honour `prefers-reduced-motion`: no pulse.
- **Section label**: uppercase 11.5/600 muted, with a count in regular weight.
- **Buttons**: secondary is `--surface-2` with a hairline; primary is solid accent with `--on-accent` text, one per
  screen at most; danger is red text on the secondary surface. A busy button shows a small spinner in place of its
  label and is disabled.
- **Fields**: `--surface` with a hairline, accent border on focus, no glow.
- **Choice cards** (radio rows): a hairline card with a radio, a bold name and a muted explanation. The chosen one
  gets an accent border and a 9% accent tint.
- **Segmented control and switch**: `--surface` track; the chosen segment is `--surface-2` with a hairline ring;
  an on switch is solid accent.
- **Sheet**: slides from the bottom over a 50% black scrim, `--surface`, 16 radius on top, a grip, then a title and
  rows. Tapping outside closes it. Confirmations and menus are sheets, never browser dialogs.
- **Toast**: one line, bottom centre above the tab bar, gone after about three seconds. For results ("Pinned",
  "Queued behind what Claude is doing") and errors written as sentences.
- **Key bar**: a row of mono key caps above the keyboard (Ctrl, Esc, Tab, ⇧Tab, arrows, Enter). It scrolls
  sideways, and a key never takes the focus from the terminal.
- **Terminal**: the laptop's grid at its own size, in the theme's colours, scaled to the phone's width but never
  below half size (a very wide tab scrolls sideways); pinch zooms.

## 5. Words

- Plain sentences, said the way you would say them: "Can't reach fedora. It may be asleep, or Tailscale is off on
  this phone." Say what happened and what to do.
- Never show raw syntax, JSON, error codes, or internal names (GUI first). Paths and commands are the exception:
  they are the thing itself, set in mono.
- An empty screen says what belongs there and offers the one action that fills it.
- Every state of a screen is designed: loading (a spinner, never a blank), empty, error, offline (the last thing
  the laptop said, and when).

## 6. Do and don't

- Do take every colour from the tokens; do check a light theme (paper) as well as graphite.
- Do keep one accent. Status colours are for status only.
- Don't add gradients, glows, drop shadows, or decorative illustration.
- Don't invent a second navigation: on the laptop the sidebar, tabs, and Ctrl K are the navigation; on the phone
  it is the bottom tabs (Inbox, New, Spaces) and pushed screens with a back arrow.
