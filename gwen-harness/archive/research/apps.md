## Driving any app from Gwen (KDE Plasma 6, Wayland): research report

Legend: **[V]** means I read it in the cited source. **[I]** means it is my own judgement or arithmetic.

### Bottom line

- **Most "computer use" doesn't need pixels.** UFO2 is the closest prior work. It found that adding API actions to GUI actions raised success from 16.3% to 24.5% with o1 and cut the average steps from 16.0 to 6.6 [V]. In its Windows runs, more than 62% of failures were control-detection failures, mostly in apps like LibreOffice that follow accessibility standards weakly [V]. OpenAI's current computer-use guide now recommends code execution over its `computer` tool for its newest model (GPT-6 Astra), because "one call can combine actions, loops, or conditional logic" [V]. Both points support the owner's "program files" idea.
- **Vision is now good enough to be a real fallback, but it is slow.** Anthropic revised Claude Opus 4.7's OSWorld-Verified score to 82.3%. OpenAI reports 78.7% for GPT-5.5. The original human baseline is 72.36% [V]. UFO2 measured about 10 s per LLM call [V].
- **Wayland is workable on Plasma 6.** Input goes through the RemoteDesktop portal: version 2 has `persist_mode=2` with restore tokens, and `ConnectToEIS` hands over a libei socket [V]. The structure of other apps comes from AT-SPI2 over D-Bus. Window management uses KWin scripts sent over D-Bus. All of it can be reached from Go with godbus [I].
- **Spotify has a policy problem, not just a technical one.** The Developer Policy says: "Do not create a voice-enabled SDA that enables a user to control Spotify with their voice or any kind of voice assistant that provides voice-control functionality" [V]. Gwen is a voice assistant. Since February 2026, Development Mode also needs a Premium owner account, allows 5 users, and gives each developer 1 Client ID [V]. The owner has to make an informed call here.
- **Recommendation [I].** Build a ladder of app adapters. Each adapter is a small "program file": a manifest, a short card, and executable verbs. A vision run that succeeds can be recorded and compiled into a deterministic adapter. Stagehand and Terminator already ship this record-then-replay pattern [V].

---

### 1. App-native APIs (top rung)

#### Spotify

**Web API status (all [V]):**

| Item | What applies now |
|---|---|
| Development Mode, since 11 Feb 2026 for new Client IDs and 9 Mar 2026 for existing ones | Owner needs Premium; up to 5 allowlisted users; 1 Development Mode Client ID per developer |
| Extended quota | Since 15 May 2025, organizations only: legally registered business, launched service, at least 250k MAU |
| Create a playlist | `POST /me/playlists` with scope `playlist-modify-private` or `-public`; the old `POST /users/{id}/playlists` was removed |
| Add tracks | `POST /playlists/{id}/items` replaces `/tracks`; the playlist field `tracks` is renamed `items` |
| Search | `GET /search` maximum `limit` dropped from 50 to 10 (default 5) |
| Removed for new apps since 27 Nov 2024 | Recommendations, Related Artists, Audio Features and Audio Analysis |
| Playback (`PUT /me/player/play`, with `context_uri` or `uris` and `device_id`) | Still available; scope `user-modify-playback-state`; "only works for users who have Spotify Premium" |

**Consequences:**
- Gwen must choose the tracks herself, using her own knowledge plus the user's listening history, then resolve each one with search. Spotify no longer recommends anything to new apps [I].
- The voice clause above applies to the Spotify Platform. Using the Web API from a voice-driven Gwen is exposed to it [I].

**Local control via MPRIS:**
- The MPRIS `Player` interface has `OpenUri`, `PlayPause`, `Next`, `Stop` and `Metadata` [V].
- The Linux Spotify client exposes `org.mpris.MediaPlayer2.spotify`. A 2019 community post shows `dbus-send … org.mpris.MediaPlayer2.Player.OpenUri string:'spotify:track:…'` [V]. I have not verified that it works for playlist URIs or on current clients.
- MPRIS is the operating system's media interface, not the Spotify Platform. That makes it the policy-safer path for voice commands like "pause" or "play my Focus playlist" [I]. It works on free accounts too [I].

**go-librespot (GPL-3.0) [V]:** a Spotify Connect client written in Go. It has a REST and WebSocket control API and MPRIS, and needs Premium. It would give Gwen an always-present playback device on the laptop or the Pi, so `PUT /me/player/play` never fails with "no active device" [I]. Gwen would run it as a separate process, which keeps GPL out of gwend [I].

#### VS Code

- **CLI, no install needed [V]:** `code --goto file:line:char`, `--diff a b`, `--merge`, `--reuse-window`, `--add`, `--wait`, and `code chat -m agent "<prompt>"`.
- **Extension API for a companion extension [V]:**
  - `languages.getDiagnostics()` and `onDidChangeDiagnostics` for errors and warnings.
  - The built-in commands `vscode.diff(left, right, title)`, `vscode.open`, and `vscode.executeCodeActionProvider(uri, range, kind)` (quick fixes).
  - The terminal shell integration API: "listen to commands run in terminals, read their raw output, exit code" through `shellIntegration.executeCommand({commandLine})` and `window.onDidEndTerminalShellExecution`.
  - VS Code also has a Language Model Tool API and an MCP server-definition API.
- **Prior art 1: Claude Code's own IDE protocol [V].**
  - The extension runs an MCP server over WebSocket on `127.0.0.1` at a random port.
  - It writes `~/.claude/ide/<port>.lock` (mode 0600 inside a 0700 directory) containing `pid`, `workspaceFolders`, `ideName`, `transport` and `authToken`. Clients send the token as `X-Claude-Code-Ide-Authorization`.
  - Its tools include `openFile`, `openDiff` (blocks until the user acts, then returns `FILE_SAVED` or `DIFF_REJECTED`), `getDiagnostics`, `getOpenEditors`, `saveDocument` and `executeCode`. Only `getDiagnostics` and `executeCode` are visible to the model.
  - An external Claude Code CLI connects with `/ide`. So when Gwen hands coding work to Claude Code (her `code.delegate`), VS Code integration can come for free [V]. I have not verified whether headless `-p` runs can attach.
- **Prior art 2: `tjx666/vscode-mcp` [V].** An extension plus an MCP server or a CLI, talking over a per-workspace Unix socket. Tools: `get_diagnostics`, `get_symbol_lsp_info`, `get_references`, `rename_symbol`, `open_files`, `execute_command` and `list_workspaces`. Its license is "Anti 996", so check it before reusing code [V]. Its design (Unix socket plus CLI, MCP optional) is exactly the "more direct than MCP" shape the owner described [I].

#### Word and Excel on Linux

There is no native Microsoft Office for Linux. The realistic targets are LibreOffice, OnlyOffice, and Office on the web [I].

- **File-level editing, the best default [I]:**
  - **Excelize** is pure Go, BSD-3, and handles XLSX/XLSM, charts, streaming, and `CalcCellValue`. Formula calculation is "in working processing" and does not support array or table formulas [V].
  - **openpyxl:** `data_only` gives either the formula or "the value stored the last time Excel read the sheet", and shapes are lost on load [V]. So any recalculation has to come from a real engine.
- **LibreOffice:**
  - `--headless` is for when "the application is controlled by external clients via the API" [V].
  - `--accept=<UNO-URL>` opens a UNO listener [V].
  - **unoserver** (MIT) keeps one LibreOffice running for `unoconvert` and `unocompare` [V].
  - Community LibreOffice MCP servers drive Writer through the UNO socket on port 2002 [V].
  - If Gwen starts the user's LibreOffice with `--accept`, she can edit the document that is open on screen, and the edits land in the user's undo stack [I].
- **Microsoft Graph:**
  - The Excel REST API works only on OneDrive for Business, SharePoint and Group drives. "Support for workbooks stored in OneDrive Consumer platform is still not available." Only Open XML files work, and it has persistent and non-persistent sessions [V].
  - Word has no document model in Graph. Microsoft's answer is to download the stream, edit it with a library, and upload it again [V].
  - So for personal Microsoft 365, Office on the web through the browser rung is the only live path [I].
- **OnlyOffice:**
  - Document Builder is AGPL-3.0 and dual-licensed, with Python (`pip install document-builder`), JS, C++ and .NET bindings [V].
  - Desktop Editors 9.2+ include an AI agent that acts as an MCP client [V]. That is not an inbound API for Gwen.

---

### 2. Desktop-level automation on Plasma 6 Wayland

**D-Bus and KWin [V]:**
- kdotool (Apache-2.0) generates a KWin script, loads it over D-Bus, runs it and deletes it. It supports `search`, `getactivewindow`, `windowactivate`, `windowmove`, `windowclose` and others, plus raw `kwinscript`.
- It explicitly "won't support" keyboard or mouse input.
- The same approach lets Gwen focus, move and list windows, and get each window's screen origin [I].

**Input injection:**
- **RemoteDesktop portal v2 [V]:**
  - Device bitmask: keyboard = 1, pointer = 2, touchscreen = 4.
  - `persist_mode`: 0 = none, 1 = while the app runs, 2 = until explicitly revoked. Each restore token works once.
  - Input goes either through `Notify*` D-Bus methods (relative motion, keycodes and keysyms; absolute motion needs a ScreenCast stream) or through `ConnectToEIS`, which returns a libei socket.
  - KDE persistence across reboots was fixed in 6.1.1 [V]. A 2026 report against Plasma 6.6.3 turned out to be the application's own bug [V].
  - The EI protocol is "a public protocol that may be used directly by clients" [V]. A small pure-Go EI client for keyboard and pointer is feasible [I]. For the first version, `Notify*` through godbus needs no libei at all [I].
- **Bypassing the prompt.** kwin-mcp calls KWin's private EIS D-Bus interface directly, "avoid[ing] the XDG RemoteDesktop portal, so there are no authorization dialogs" [V]. KWin's design says "clients are expected to go through the RemoteDesktop portal" [V]. Gwen should not bypass consent [I].
- **ydotool** works through uinput and the `ydotoold` daemon. It "usually requires root", ignores custom keyboard layouts, and is AGPL-3.0 [V]. It is the last resort only [I].
- **Go library gap.** `rymdport/portal` (Apache-2.0) implements Screenshot, OpenURI and Notification, but not RemoteDesktop, ScreenCast or GlobalShortcuts [V]. Gwen needs about 300 lines of godbus code of her own [I].

**Screenshots [V]:**
- `org.kde.KWin.ScreenShot2` is restricted. The calling app's `.desktop` file must carry `X-KDE-DBUS-Restricted-Interfaces` and live in a root-owned directory such as `/usr/share/applications`.
- kwin-mcp captures in about 30–70 ms per frame through ScreenShot2, with the Spectacle CLI as a fallback.
- So Gwen's distro package should install such a desktop file [I].

**AT-SPI2, the accessibility tree [V]:**
- It runs on its own accessibility D-Bus bus.
- Qt 6 turns on its bridge when `org.a11y.Status` `IsEnabled` or `ScreenReaderEnabled` is set, or when `QT_LINUX_ACCESSIBILITY_ALWAYS_ON` is set.
- VS Code and Electron need `--force-renderer-accessibility`.
- On Wayland, "clients cannot know their own window position". Qt reports extents relative to the window, so clicks miss unless the KWin window origin is added (kwin-mcp issue #51).
- I infer that Spotify (a Chromium-based client) has the same need for a flag. Unverified.

---

### 3. Vision-based computer use

- **Anthropic [V]:**
  - The current toolset is `computer_toolset_20260801`, with no beta header. On the Claude API, Claude 5.5 models accept only this toolset.
  - It has 17 actions, including `zoom` to a region.
  - It adds about 4,500 input tokens.
  - Recommended sizes are 1280×720 for desktop and 1280×800 for web. Images can be up to 2576 px on the long edge, counted as ⌈w/28⌉×⌈h/28⌉ visual tokens.
  - Classifiers scan screenshots for prompt injection.
- **OpenAI [V]:**
  - The `computer` tool takes batched `actions` arrays.
  - The guide recommends an "isolated browser or VM and an allow list".
  - It recommends code execution for GPT-6 Astra.
  - Operator was folded into ChatGPT agent in July 2025.
- **Scores:**
  - Human baseline on OSWorld: 72.36% [V].
  - Opus 4.7: 82.3%, revised by Anthropic [V].
  - GPT-5.5: 78.7% and GPT-5.4: 75.0% [V].
  - Agent S3 with Behavior Best-of-N: 72.6% (Dec 2025) [V].
  - UI-TARS-1.5 (Apache-2.0): 42.5%, and its 7B version 27.5% [V]. Open local models are far behind.
  - Aggregator sites list higher numbers, but they are self-reported and unverified [V].
- **Hybrid beats either alone.** In UFO2, UIA-only, OmniParser-only and hybrid detection scored 24.5%, 14.3% and 28.6% on OSWorld-W with o1 [V]. Speculative multi-action cut steps by up to 51.5% [V].
- **OS-Copilot / FRIDAY** "learns to control and self-improve on Excel and PowerPoint" by accumulating tools [V].
- **Cost and latency [I]:**
  - A 1280×800 screenshot is 46 × 29 = 1,334 visual tokens.
  - A 15-step task is about 20k image tokens plus 4.5k overhead, and about 2.5 minutes at roughly 10 s per step.
  - An API call takes under 1 s.
- **Failure modes [I, partly from UFO2]:** missed or mis-located controls, focus theft by the user, popups, small text (hence `zoom`), cross-app tasks (UFO2 managed only 9.1% on these [V]), and injected instructions in on-screen content.

---

### 4. Browser

- **Playwright MCP (Apache-2.0) [V]:**
  - Uses a persistent profile by default, or `--isolated`, `--user-data-dir` or `--storage-state`.
  - `--extension` attaches to existing logged-in tabs, and `--cdp-endpoint` attaches to a running browser.
  - It works from accessibility snapshots by default; vision needs `--caps=vision`.
  - Its own docs say `--allowed-origins` "does not serve as a security boundary".
  - They also say the Playwright CLI plus skills is "more token-efficient" than MCP.
- **Chrome DevTools MCP** (Apache-2.0) "exposes content of the browser instance to the MCP clients" [V].
- **agent-browser (Vercel, Apache-2.0, Rust CLI) [V]:**
  - Gives accessibility snapshot refs such as `@e2`, and a daemon keeps browser state between shell calls.
  - `--profile <name>` copies a Chrome profile to a temporary directory; `--auto-connect` attaches to a running Chrome.
  - It is a program-file-style tool that needs no MCP [I].
- **Stagehand v3 (Oct 2025)** caches agent actions, so a run made with a computer-use agent can be replayed "without further inference" [V].
- **Profile policy for Gwen [I]:**
  - Use her own Chromium profile by default.
  - Use the user's logged-in profile only through an explicit, revocable extension pairing, tab by tab.
  - Web apps such as the Spotify web player and Office on the web are the fallback rung for desktop apps.

---

### 5. Existing "control any app" frameworks

| Project | Platform and approach | Reusable for Gwen |
|---|---|---|
| **kwin-mcp** (MIT, Python) [V] | Plasma 6 Wayland. 30 tools. Runs apps in a `dbus-run-session` plus `kwin_wayland --virtual` sandbox, or attaches to the live session. Uses AT-SPI2, private EIS, and ScreenShot2 | The **virtual KWin session** idea, which works like UFO2's isolated "picture-in-picture" desktop. Not its consent bypass |
| **computer-use-linux** (MIT, Rust) [V] | AT-SPI, portals, ydotool fallback. Uses KWin scripting on KDE. Tools include `perform_action` and `set_value` | The tool vocabulary and the "doctor" and setup checks |
| **Terminator** (MIT) [V] | Windows only. "Playwright for Windows". Records workflows and calls AI "only when recovery is needed" | The record-and-replay design |
| **Windows-MCP** (MIT) [V] | UIA tree with optional vision; 0.2–0.5 s between actions | Proof that tree-first is fast |
| **UFO2** [V] | HostAgent plus per-app agents, GUI and API actions unified, hybrid detection | The architecture template |
| **Cua** (mostly MIT) [V] | Sandboxed Linux and macOS desktops for agents | Option for a disposable desktop |
| **stacyvm** (Apache-2.0, Go, pre-1.0) [V] | Firecracker or Docker sandboxes with exec and file APIs and about 28 ms snapshot restore (roadmap). No in-sandbox GUI yet | Host for a **disposable KDE desktop template** for risky vision tasks; adding GUI and VNC support is the owner's change to make [I] |

From Go, Gwen can reuse godbus, D-Bus calls into KWin scripts, wazero (pure Go, no cgo, Apache-2.0) [V], and subprocess CLIs. The Python and Rust projects are designs to borrow, not dependencies [I].

---

### 6. Safety

- **Irreversible classes, which always need a payload-bound approval [I]:** send, post or publish; purchases and subscriptions; delete or overwrite without a snapshot; changes to sharing, permissions or account security; force pushes; anything typed into password or payment fields.
- **Patterns from shipped products [V]:**
  - Operator asks the user to take over for credentials and payments, asks before "submitting an order or sending an email", and uses watch mode on email and finance sites plus a monitor model.
  - Claude in Chrome cut attack success from 23.6% to 11.2% with mitigations (35.7% to 0% on the browser-specific set). It has per-site permissions and blocked categories.
  - The residual risk is not zero, so content from the screen and accessibility tree is untrusted data [I].
- **Confining input [I]:**
  - Use one portal session owned by gwend, with only the device bits it needs and `persist_mode=2`, revocable in System Settings.
  - Run work that doesn't need the user's logins in a virtual KWin session or a stacyvm VM.
  - Never use uinput unless the user opts in.
- **Show and hand back control [I]:**
  - A "Gwen is driving" overlay: a colored border on the target window, a status in the tray and Plasma widget, and a spoken note.
  - Gwen pauses as soon as the user touches the mouse or keyboard.
  - A panic shortcut through the GlobalShortcuts portal.
  - Every action goes into the hash-chained ledger with a screenshot.
  - Plasma already shows portal notifications, such as "Input Capture started" [V].

---

### 7. Implications for Gwen

#### Adapter ladder

| Rung | Mechanism | Speed | Use when |
|---|---|---|---|
| 0 File | excelize, unoserver, python-docx in a sandbox | ms | The data is in a file and the app needn't be open |
| 1 Service API | Spotify Web API, Graph | ~100 ms | An official API exists and its terms allow the use |
| 2 App bridge | VS Code companion socket, LibreOffice UNO, Playwright/CDP, go-librespot | ms | The app is open and has a plugin, IPC or debug port |
| 3 Desktop IPC | MPRIS, KDE D-Bus, KWin scripts | ms | Common verbs: play, focus, open |
| 4 Accessibility | AT-SPI tree plus portal input | ~0.3 s per action plus LLM | Qt and GTK apps with good trees |
| 5 Vision | Claude computer-use toolset with `zoom` | ~10 s per step | Everything else, first-time tasks |

**Choosing a rung per app [I]:**
- A probe builds an app profile:
  - Is there an installed adapter whose health check passes?
  - Which D-Bus names does the app own (including `org.mpris.*`)?
  - How rich is its AT-SPI tree (count of named, actionable nodes in the focused window)?
  - Can it be reached through a debug port or extension?
- The probe result is cached in Gwen's self-model of her capabilities.
- Gwen picks the lowest working rung for each verb, not for each app. The same app can mix rungs.

#### An app adapter as a program file [I]

```
~/.local/share/gwen/apps/spotify/
  app.toml   # verbs with typed args and results, rung, risk class per verb, D-Bus names, hosts, secrets, sandbox profile
  card.md    # ≤150 tokens: the only text the model sees; one line per verb
  main.go | main.star | mod.wasm   # executable verbs (subprocess JSON-RPC, or wazero in-process)
  tests/     # fixtures and recorded traces, run inside a virtual KWin session
```

- Verbs run as functions inside Gwen's code-execution runtime, the "code mode" pattern. Intermediate data stays out of the model's context; Anthropic measured 150,000 tokens falling to 2,000 this way [V].
- **Self-writing loop:**
  1. When Gwen has used rung 4 or 5 for the same verb N times, she proposes compiling it.
  2. She turns the recorded trace into accessibility selectors with assertions, not coordinates.
  3. She tests it in a virtual session and goes through the v1 tool-forge approval.
  4. Afterwards it replays deterministically, and falls back to the LLM only when an assertion fails. This mirrors Stagehand and Terminator [V].
- She can also find a better rung on her own. For example, she might notice a D-Bus interface and wrap it instead of clicking.

#### (a) "Make a focus playlist and play it"

1. The `music.focus(mood, minutes)` adapter checks its paths in order:
   - **Web API is linked and the owner accepts the voice-policy risk:** Gwen drafts about 40 "artist – title" picks from her knowledge and the user's history. The program resolves them through `/search` (limit 10 or fewer, in parallel, with no LLM per result). It then calls `POST /me/playlists` (private) and `POST /playlists/{id}/items`. It makes sure a device exists (the desktop client, or go-librespot). Finally it calls `PUT /me/player/play` with `context_uri` and `device_id`.
   - **Otherwise:** MPRIS `OpenUri` on an existing playlist, with GUI or vision playlist creation as the last resort.
2. The model gets back one line: "Created 'Deep Work, Oct 12' (28 tracks, 1h52), playing on laptop".
3. Risk classes: creating a playlist is a reversible write, and playback is local. Neither needs a prompt once the adapter is approved.
4. Bonus: Gwen lowers the music through MPRIS while her TTS speaks.

#### (b) Help in VS Code

1. Ship `gwen-vscode`, a small TypeScript companion extension:
   - It opens a Unix socket under `$XDG_RUNTIME_DIR/gwen/` with a 0600 lock file and a random token, copying the Claude Code IDE design.
   - Its verbs: `open` (`vscode.open` or `code --goto`), `diff` (`vscode.diff`), `diagnostics` (errors only, at most 20, as path:line:message), `quickfix` (`executeCodeActionProvider`), `applyEdit` (so edits are undoable), `run` (shell-integration `executeCommand`, then the exit code and the last 40 lines), and allowlisted `command`.
   - Without the extension, she falls back to the `code` CLI flags.
2. Flow:
   1. Read diagnostics.
   2. Gwen, or Claude Code through `code.delegate` with `/ide`, writes the patch.
   3. `diff` shows it in the editor and blocks until the user accepts.
   4. Gwen applies it and runs the tests in a visible terminal.
   5. She summarizes the result.

#### (c) Edit an Excel or Word document

1. **Where the file lives decides the route:**
   - **A local file that isn't open:** excelize for cells, formats and charts, or python-docx for Word. Recalculate and render a PDF or PNG through unoserver, and check it with a single image.
   - **Open in LibreOffice:** edit live over UNO.
   - **Microsoft 365 business account:** use Graph Excel sessions.
   - **Personal Microsoft 365, or Word online:** use the browser rung in Gwen's profile.
2. Always snapshot first, write a new version, and show a text or cell diff before replacing the original. Overwriting is irreversible-class.

### Sources
- https://developer.spotify.com/blog/2026-02-06-update-on-developer-access-and-platform-security
- https://developer.spotify.com/documentation/web-api/references/changes/february-2026
- https://developer.spotify.com/documentation/web-api/concepts/quota-modes
- https://developer.spotify.com/documentation/web-api/reference/create-playlist
- https://developer.spotify.com/documentation/web-api/reference/start-a-users-playback
- https://developer.spotify.com/blog/2024-11-27-changes-to-the-web-api
- https://developer.spotify.com/policy
- https://techcrunch.com/2026/02/06/spotify-changes-developer-mode-api-to-require-premium-accounts-limits-test-users/
- https://github.com/ramsayleung/rspotify/issues/550
- https://specifications.freedesktop.org/mpris-spec/latest/Player_Interface.html
- https://community.spotify.com/t5/Desktop-Linux/Guide-Play-Spotify-links-in-running-client/m-p/4647375/highlight/true
- https://github.com/devgianlu/go-librespot
- https://code.visualstudio.com/docs/configure/command-line
- https://code.visualstudio.com/api/references/commands
- https://code.visualstudio.com/api/references/vscode-api
- https://code.visualstudio.com/updates/v1_93
- https://code.claude.com/docs/en/vs-code
- https://raw.githubusercontent.com/coder/claudecode.nvim/main/PROTOCOL.md
- https://github.com/tjx666/vscode-mcp
- https://github.com/microsoft/vscode/issues/84833
- https://learn.microsoft.com/en-us/graph/api/resources/excel?view=graph-rest-1.0
- https://learn.microsoft.com/en-sg/answers/questions/1615922/how-to-use-graph-api-to-edit-word-or-powerpoint-co
- https://help.libreoffice.org/latest/en-US/text/shared/guide/start_parameters.html
- https://github.com/unoconv/unoserver
- https://github.com/harshithb3304/libre-office-mcp
- https://github.com/ONLYOFFICE/DocumentBuilder
- https://api.onlyoffice.com/docs/desktop-editors/usage-api/connecting-mcp-servers/docspace-mcp/
- https://github.com/qax-os/excelize
- https://xuri.me/excelize/en/cell.html
- https://openpyxl.readthedocs.io/en/stable/tutorial.html
- https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.portal.RemoteDesktop.html
- https://bugs.kde.org/show_bug.cgi?id=480235
- https://github.com/lamco-admin/lamco-rdp-server/issues/51
- https://invent.kde.org/plasma/kwin/-/merge_requests/5496
- https://libinput.pages.freedesktop.org/libei/
- https://forum.kde.org/viewtopic.php%3Ff=305&t=176422.html
- https://discuss.kde.org/t/portal-input-capture-started-notification/30265
- https://github.com/jinliu/kdotool
- https://github.com/ReimuNotMoe/ydotool
- https://github.com/rymdport/portal
- https://wiki.freedesktop.org/www/Accessibility/AT-SPI2/
- https://codebrowser.dev/qt6/qtbase/src/gui/accessible/linux/dbusconnection.cpp.html
- https://github.com/isac322/kwin-mcp
- https://github.com/isac322/kwin-mcp/issues/51
- https://github.com/agent-sh/computer-use-linux
- https://platform.claude.com/docs/en/agents-and-tools/tool-use/computer-use-tool
- https://developers.openai.com/api/docs/guides/tools-computer-use
- https://openai.com/index/introducing-gpt-5-5/
- https://www.anthropic.com/news/claude-opus-4-8
- https://simular.ai/articles/simulars-computer-use-agent-outperforms-humans
- https://arxiv.org/abs/2510.02250
- https://huggingface.co/ByteDance-Seed/UI-TARS-1.5-7B
- https://llm-stats.com/benchmarks/osworld-verified
- https://arxiv.org/abs/2504.14603
- https://arxiv.org/html/2504.14603v2
- https://arxiv.org/abs/2503.11069
- https://arxiv.org/abs/2402.07456
- https://www.anthropic.com/engineering/code-execution-with-mcp
- https://github.com/microsoft/playwright-mcp
- https://github.com/ChromeDevTools/chrome-devtools-mcp
- https://github.com/vercel-labs/agent-browser
- https://browserbase.com/changelog/stagehand-v3
- https://github.com/mediar-ai/terminator
- https://github.com/CursorTouch/Windows-MCP
- https://github.com/trycua/cua
- https://github.com/StacyOS/stacyvm
- https://github.com/tetratelabs/wazero
- https://openai.com/index/introducing-operator/
- https://claude.com/resources/articles/claude-for-chrome