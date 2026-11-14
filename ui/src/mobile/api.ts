// The phone app's link to the laptop: gwend's code routes on the remote socket, published to the tailnet by Tailscale Serve
// (plan.md 3.14). The types mirror internal/wire/code.go; Gwen's own (your day, tasks, her chats) are the window's wire types.
import type { wire } from "../wailsjs/go/models";

export type DayStatus = wire.Status;
export type Plan = wire.Plan;
export type PlanItem = wire.PlanItem;
export type Task = wire.Task;
export type Project = wire.Project;
export type AgentChat = wire.AgentChat;
export type AgentChatDetail = wire.AgentChatDetail;
export type AgentStep = wire.AgentStep;
export type Approval = wire.Approval;
export type AgentStatus = wire.AgentStatus;
/** A project's folder as the phone picks it: an existing folder, or a new one (made, maybe as a git repository). */
export interface ProjectFolder { folder?: string; create_folder?: boolean; git_init?: boolean }

export type SessionState = "starting" | "working" | "needs_you" | "done" | "idle" | "error" | "ended";

export interface Option { key: string; label: string; kind: "allow" | "always" | "deny" | "option" }

export interface Waiting {
  kind: "permission" | "question";
  held: boolean;
  tool: string;
  summary: string;
  question: string;
  options: Option[];
  note: string;
  held_until: number | null;
}

export interface Terminal { terminal: string; name: string; cwd: string; status: string; app: string; session_id: string; cols: number; rows: number }

export interface Session {
  id: string;
  terminal: string;
  name: string;
  cwd: string;
  branch: string;
  sandbox: boolean;
  state: SessionState;
  need: "" | "permission" | "question" | "other";
  step: string;
  last_message: string;
  permission_mode: string;
  claude_session_id: string;
  pinned: boolean;
  unread: boolean;
  window: boolean;
  notice: string;
  waiting: Waiting | null;
  helpers: Terminal[];
  /** What it changed, for its card; null until the laptop has counted. */
  changes: Stat | null;
  started_at: number;
  updated_at: number;
  ended_at: number | null;
}

export interface Stat { files: number; added: number; removed: number }

export interface SessionList { sessions: Session[]; core: "ok" | "down" | "outdated"; message: string; machine: string }

export interface Theme {
  name: string;
  dark: boolean;
  background: string;
  foreground: string;
  cursor: string;
  selection: string;
  accent: string;
  surface: string;
  border: string;
  muted: string;
  ansi: string[];
}

export interface Tab { id: string; name: string; terminals: Terminal[] }
export interface Space { id: string; name: string; folder: string; tabs: Tab[] }
export interface Spaces { spaces: Space[]; loose: Terminal[]; theme: Theme }

export interface TranscriptEvent {
  cursor: number;
  kind: "you" | "claude" | "tool" | "notice";
  at: number;
  text: string;
  tool: string;
  ok: boolean;
  added: number;
  removed: number;
  output: string;
}
export interface Transcript { events: TranscriptEvent[]; before: number | null; after: number }

export interface Change { path: string; old_path: string; status: string; added: number; removed: number; binary: boolean; before_session: boolean }
export interface Changes { git: boolean; branch: string; head: string; files: Change[] }
export interface Diff { path: string; diff: string; truncated: boolean; binary: boolean; size: number }

export interface Folder { name: string; path: string; git: boolean; branch: string }
export interface Folders { path: string; parent: string; folders: Folder[]; roots: string[] }
export interface Conversation { claude_session_id: string; title: string; active_at: number; sandbox: boolean; session_id: string }

/** Phone notifications: the laptop's public key, and whether this phone is subscribed (and where). */
export interface PushState { key: string; subscribed: boolean; endpoint: string }

export interface StartSession { dir: string; create?: boolean; git_init?: boolean; sandbox?: boolean; prompt?: string; window?: boolean; resume?: string }

/** A failure the laptop explained, or that the phone could not reach it at all. */
export class ApiError extends Error {
  constructor(
    public code: string,
    message: string,
    public details: Record<string, unknown> = {},
    public status = 0,
  ) {
    super(message);
  }
}

/** Called when the laptop says this phone is not paired (any 401). */
let onUnpaired: () => void = () => {};
export function setOnUnpaired(fn: () => void) {
  onUnpaired = fn;
}

async function request<T>(method: string, path: string, body?: unknown, raw?: BodyInit, signal?: AbortSignal): Promise<T> {
  const headers: Record<string, string> = {};
  if (method !== "GET") headers["X-Gwen"] = "1";
  let payload: BodyInit | undefined = raw;
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
    payload = JSON.stringify(body);
  } else if (raw !== undefined) {
    headers["Content-Type"] = "application/octet-stream";
  }
  let res: Response;
  try {
    res = await fetch(path, { method, headers, body: payload, credentials: "same-origin", signal });
  } catch (e) {
    if ((e as Error).name === "AbortError") throw e;
    throw new ApiError("offline", "Can't reach the laptop.");
  }
  if (res.status === 204) return undefined as T;
  let data: any = null;
  try {
    data = await res.json();
  } catch {
    // Tailscale answers with its own page when the laptop is off
  }
  if (!res.ok) {
    if (res.status === 401 && path !== "/pair") onUnpaired();
    const err = data?.error;
    if (err?.code) throw new ApiError(err.code, sentence(err.message), err.details ?? {}, res.status);
    throw new ApiError(res.status >= 502 ? "offline" : "internal", res.status >= 502 ? "Can't reach the laptop." : "Something went wrong on the laptop.", {}, res.status);
  }
  return data as T;
}

/** The daemon writes messages as lowercase sentences; show them as sentences. */
function sentence(s: string): string {
  if (!s) return "Something went wrong.";
  const t = s.charAt(0).toUpperCase() + s.slice(1);
  return /[.!?]$/.test(t) ? t : t + ".";
}

const q = (params: Record<string, string | number | undefined>) => {
  const u = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== "") u.set(k, String(v));
  const s = u.toString();
  return s ? "?" + s : "";
};
const enc = encodeURIComponent;

export const api = {
  pair: (code: string, name: string) => request<void>("POST", "/pair", { code, name }),
  sessions: (signal?: AbortSignal) => request<SessionList>("GET", "/v1/code/sessions", undefined, undefined, signal),
  session: (id: string) => request<Session>("GET", `/v1/code/sessions/${enc(id)}`),
  start: (req: StartSession) => request<Session>("POST", "/v1/code/sessions", req),
  prompt: (id: string, text: string, idem: string) => request<{ result: "sent" | "queued" }>("POST", `/v1/code/sessions/${enc(id)}/prompt`, { text, idem }),
  control: (id: string, action: string) => request<Session>("POST", `/v1/code/sessions/${enc(id)}/control`, { action }),
  answer: (id: string, key: string, message?: string) => request<{ ok: boolean; message: string }>("POST", `/v1/code/sessions/${enc(id)}/answer`, { key, message }),
  mark: (id: string, m: { pinned?: boolean; seen?: boolean }) => request<Session>("POST", `/v1/code/sessions/${enc(id)}/mark`, m),
  transcript: (id: string, p: { before?: number; after?: number; limit?: number }) => request<Transcript>("GET", `/v1/code/sessions/${enc(id)}/transcript${q(p)}`),
  changes: (id: string) => request<Changes>("GET", `/v1/code/sessions/${enc(id)}/changes`),
  diff: (id: string, path: string) => request<Diff>("GET", `/v1/code/sessions/${enc(id)}/changes/diff${q({ path })}`),
  spaces: (signal?: AbortSignal) => request<Spaces>("GET", "/v1/code/spaces", undefined, undefined, signal),
  newTerminal: (space: string) => request<{ terminal: string }>("POST", `/v1/code/spaces/${enc(space)}/terminal`, {}),
  send: (terminal: string, text: string, idem: string) => request<{ result: string }>("POST", `/v1/code/terminals/${enc(terminal)}/send`, { text, idem }),
  put: (terminal: string, file: File) => request<{ path: string }>("POST", `/v1/code/terminals/${enc(terminal)}/put${q({ name: file.name || "photo.jpg" })}`, undefined, file),
  folders: (path: string) => request<Folders>("GET", `/v1/code/folders${q({ path })}`),
  recentFolders: () => request<Folders>("GET", "/v1/code/recent-folders"),
  conversations: (dir: string) => request<{ conversations: Conversation[] }>("GET", `/v1/code/conversations${q({ dir })}`),
  transcribe: (pcm: ArrayBuffer) => request<{ text: string }>("POST", "/v1/code/transcribe", undefined, pcm),
  pushState: () => request<PushState>("GET", "/v1/code/push"),
  pushSubscribe: (sub: PushSubscriptionJSON) => request<PushState>("PUT", "/v1/code/push", sub),
  pushUnsubscribe: () => request<void>("DELETE", "/v1/code/push"),
  pushTest: () => request<void>("POST", "/v1/code/push/test"),

  // Gwen herself (api/phone.go): your tracking, today's plan, tasks and projects, her chats and approvals.
  status: () => request<DayStatus>("GET", "/v1/status"),
  clockIn: (project_id: string | null, task_id: string | null) => request<DayStatus>("POST", "/v1/day/clock-in", { project_id, task_id }),
  clockOut: () => request<DayStatus>("POST", "/v1/day/clock-out", {}),
  breakStart: () => request<DayStatus>("POST", "/v1/break/start", {}),
  breakEnd: () => request<DayStatus>("POST", "/v1/break/end", {}),
  switchTo: (project_id: string | null, task_id: string | null) => request<DayStatus>("POST", "/v1/session/switch", { project_id, task_id }),
  plan: (day?: string) => request<Plan>("GET", `/v1/plan${q({ day })}`),
  projects: () => request<wire.ProjectList>("GET", "/v1/projects"),
  /** A new project; its folder must be one the phone may open, made first with create_folder. */
  createProject: (p: ProjectFolder & { name: string; color?: string }) => request<Project>("POST", "/v1/projects", p),
  /** Gives a project its folder, one the phone may open. */
  projectFolder: (id: string, f: ProjectFolder) => request<Project>("POST", `/v1/projects/${enc(id)}/folder`, f),
  tasks: (p: { status?: string } = {}) => request<wire.TaskList>("GET", `/v1/tasks${q(p)}`),
  createTask: (t: Partial<wire.CreateTaskRequest> & { title: string }) => request<unknown>("POST", "/v1/tasks", t),
  patchTask: (id: string, t: Partial<wire.PatchTaskRequest>) => request<unknown>("PATCH", `/v1/tasks/${enc(id)}`, t),
  completeTask: (id: string) => request<unknown>("POST", `/v1/tasks/${enc(id)}/complete`, {}),
  reopenTask: (id: string) => request<unknown>("POST", `/v1/tasks/${enc(id)}/reopen`, {}),
  /** Hand it to an agent: a Claude session in the project's folder, linked to the task, which then waits on Claude. */
  handOff: (id: string, h: { project_id?: string | null; brief: string; sandbox: boolean }) => request<Session>("POST", `/v1/tasks/${enc(id)}/hand-off`, h),
  agentStatus: () => request<AgentStatus>("GET", "/v1/agent/status"),
  agentChats: (project?: string) => request<wire.AgentChatList>("GET", `/v1/agent/chats${q({ project })}`),
  agentChat: (id: string) => request<AgentChatDetail>("GET", `/v1/agent/chats/${enc(id)}`),
  createAgentChat: (project_id: string | null) => request<AgentChat>("POST", "/v1/agent/chats", { project_id }),
  sendAgent: (id: string, text: string) => request<AgentChat>("POST", `/v1/agent/chats/${enc(id)}/messages`, { text }),
  stopAgent: (id: string) => request<unknown>("POST", `/v1/agent/chats/${enc(id)}/stop`, {}),
  approvals: () => request<wire.ApprovalList>("GET", "/v1/approvals?status=pending"),
  decide: (id: string, approve: boolean, note?: string) =>
    request<Approval>("POST", `/v1/approvals/${enc(id)}`, { decision: approve ? "approve" : "reject", payload: note ? { note } : undefined }),
};

/** A fresh idempotency key: the same send is never typed twice (plan.md 3.14.7). */
export function idem(): string {
  return crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

export interface EventHandlers {
  session: (s: Session) => void;
  removed: (id: string) => void;
  spaces: () => void;
  /** Gwen's own events (your day, tasks, plan, her chats and approvals), by name, with their data. */
  gwen?: (name: GwenEvent, data: any) => void;
  /** The stream opened (true) or dropped (false). */
  online: (up: boolean) => void;
}

/** The laptop pings the stream every 25 s; this long without hearing anything and the stream is taken as dead. */
const SILENT_MS = 60_000;
/** Away from the screen this long, the stream is opened again on return: a phone freezes a background page and its connection with it. */
const FROZEN_MS = 3_000;

/** Follows the laptop's event stream, reconnecting by itself. Returns a function that stops it. */
export function follow(h: EventHandlers): () => void {
  let es: EventSource | null = null;
  let timer: number | undefined;
  let stopped = false;
  // When anything last came in on the stream, and when the page was last put away.
  let heard = Date.now();
  let hiddenAt = 0;
  const open = () => {
    if (stopped) return;
    window.clearTimeout(timer);
    es?.close();
    const stream = new EventSource("/v1/code/events");
    es = stream;
    heard = Date.now();
    const on = (name: string, fn: (data: unknown) => void) =>
      stream.addEventListener(name, (e) => {
        heard = Date.now();
        let data: unknown = null;
        try {
          data = JSON.parse((e as MessageEvent).data);
        } catch {
          // an event without data
        }
        fn(data);
      });
    stream.onopen = () => {
      heard = Date.now();
      h.online(true);
    };
    on("ping", () => {});
    on("code_session", (d) => h.session(d as Session));
    on("code_session_removed", (d) => h.removed((d as { id: string }).id));
    on("code_spaces", () => h.spaces());
    for (const name of gwenEvents) on(name, (d) => h.gwen?.(name, d));
    stream.onerror = () => {
      if (es !== stream) return;
      // EventSource retries by itself on a dropped stream, but gives up on an HTTP error (a laptop that is asleep).
      h.online(false);
      stream.close();
      es = null;
      timer = window.setTimeout(open, 4000);
    };
  };
  open();
  // A connection that died without saying so (a phone that slept, a network that changed) never errors: the pings give it away.
  const watchdog = window.setInterval(() => {
    if (document.visibilityState === "visible" && es && Date.now() - heard > SILENT_MS) open();
  }, 10_000);
  const wake = () => {
    if (document.visibilityState === "hidden") {
      hiddenAt = Date.now();
      return;
    }
    if (stopped) return;
    if (!es || (hiddenAt && Date.now() - hiddenAt > FROZEN_MS) || Date.now() - heard > SILENT_MS) open();
    hiddenAt = 0;
  };
  const back = () => {
    if (!stopped && (!es || Date.now() - heard > SILENT_MS)) open();
  };
  document.addEventListener("visibilitychange", wake);
  window.addEventListener("online", back);
  window.addEventListener("pageshow", back);
  return () => {
    stopped = true;
    window.clearTimeout(timer);
    window.clearInterval(watchdog);
    es?.close();
    document.removeEventListener("visibilitychange", wake);
    window.removeEventListener("online", back);
    window.removeEventListener("pageshow", back);
  };
}

/** The events of Gwen herself the phone's stream carries (api/phone.go phoneEvents). */
export const gwenEvents = ["state_changed", "day_changed", "tasks_changed", "plan_changed", "projects_changed", "agent_changed", "agent_text", "agent_step", "approval_requested", "approval_decided"] as const;
export type GwenEvent = (typeof gwenEvents)[number];

/** The live view of a terminal: bytes in, keys and typed text out. */
export interface View {
  keys: (keys: string[]) => void;
  text: (text: string) => void;
  close: () => void;
}

export function openView(
  terminal: string,
  on: { data: (bytes: Uint8Array) => void; size: (cols: number, rows: number) => void; closed: (reason: string) => void },
): View {
  const ws = new WebSocket(`${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/v1/code/terminals/${enc(terminal)}/view`);
  ws.binaryType = "arraybuffer";
  let done = false;
  const finish = (reason: string) => {
    if (done) return;
    done = true;
    on.closed(reason);
  };
  ws.onmessage = (e) => {
    if (typeof e.data !== "string") return on.data(new Uint8Array(e.data as ArrayBuffer));
    try {
      const msg = JSON.parse(e.data);
      if (msg.cols) on.size(msg.cols, msg.rows);
      if (msg.closed) finish(msg.closed);
    } catch {
      // not ours
    }
  };
  ws.onclose = () => finish("lost");
  const send = (msg: object) => {
    if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(msg));
  };
  return {
    keys: (keys) => {
      for (let i = 0; i < keys.length; i += 32) send({ keys: keys.slice(i, i + 32) });
    },
    // gwend takes at most 4 KB of typed text per message: a long paste goes in pieces, never splitting a character.
    text: (text) => {
      for (let i = 0; i < text.length; ) {
        let end = Math.min(text.length, i + 1000);
        if (end < text.length && /[\ud800-\udbff]/.test(text[end - 1])) end--;
        send({ text: text.slice(i, end) });
        i = end;
      }
    },
    close: () => {
      done = true;
      ws.close();
    },
  };
}
