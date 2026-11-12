// The only code that talks to gwend (plan.md 3.15).
//
// gwend serves its API to this mod on 127.0.0.1 with a token; both are in
// app.json in Gwen's runtime folder. Every Gwen tab is a copy of this mod and
// shares this module, so there is one connection and one set of lists for all
// of them: `gwen` below. A copy joins with gwen.attach(cx) in its init and
// re-renders when gwen.changed() resolves.
//
// `api` has one function per daemon route, named as the Go client names them
// (internal/client). A failed call throws an ApiError: {code, message, details}.

import { exec } from "herdr";

/** A failure as the daemon reports it. code "daemon_not_running" means no answer at all. */
export class ApiError extends Error {
  constructor(code, message, details) {
    super(message);
    this.code = code;
    this.details = details ?? {};
  }
}

/** Any thrown value as an ApiError. */
export function apiError(e) {
  if (e instanceof ApiError) return e;
  return new ApiError("internal", String(e?.message ?? e), {});
}

/** A rev mismatch: refetch and say so, never retry. */
export const isStale = (e) => e.code === "conflict" && e.details.field === "rev";

const NOT_RUNNING = "daemon_not_running";

// Where gwend wrote its port and token: $XDG_RUNTIME_DIR/gwen/app.json, as
// internal/config.DefaultPaths resolves it.
const READ_APP = 'cat "${XDG_RUNTIME_DIR:-${TMPDIR:-/tmp}/gwen-$(id -u)}/gwen/app.json" 2>/dev/null || cat "${TMPDIR:-/tmp}/gwen-$(id -u)/app.json"';

const state = {
  conn: null, // {port, token}
  connecting: null, // the promise reading app.json, while it runs
};

async function connect(force) {
  if (state.conn && !force) return state.conn;
  if (!state.connecting) {
    state.connecting = (async () => {
      try {
        const r = await exec("sh", ["-c", READ_APP], { timeout: 5000 });
        const info = JSON.parse(r.stdout);
        if (!info.port || !info.token) throw new Error("no port");
        state.conn = { port: info.port, token: info.token };
      } catch {
        state.conn = null;
      } finally {
        state.connecting = null;
      }
      return state.conn;
    })();
  }
  return state.connecting;
}

function url(conn, path, query) {
  let out = `http://127.0.0.1:${conn.port}${path}`;
  const parts = [];
  for (const [k, v] of Object.entries(query ?? {})) {
    if (v === undefined || v === null || v === "" || v === false) continue;
    parts.push(`${encodeURIComponent(k)}=${encodeURIComponent(v === true ? "true" : String(v))}`);
  }
  if (parts.length) out += `?${parts.join("&")}`;
  return out;
}

async function send(conn, method, path, query, body) {
  const options = { method, headers: { Authorization: `Bearer ${conn.token}` } };
  if (body !== undefined) {
    options.headers["Content-Type"] = "application/json";
    options.body = JSON.stringify(body);
  }
  return fetch(url(conn, path, query), options);
}

/**
 * One request. body undefined sends none. Returns the decoded JSON, or null
 * for an empty answer. A daemon that restarted has a new port and token, so a
 * refused connection or a 401 reads app.json again and tries once more.
 */
export async function request(method, path, query, body) {
  let res = null;
  for (const retry of [false, true]) {
    const conn = await connect(retry);
    if (!conn) throw new ApiError(NOT_RUNNING, "Gwen isn't running", {});
    try {
      res = await send(conn, method, path, query, body);
    } catch {
      res = null;
    }
    if (res && res.status !== 401) break;
    res = null;
  }
  if (!res) {
    state.conn = null;
    throw new ApiError(NOT_RUNNING, "Gwen isn't running", {});
  }
  const text = await res.text();
  let data = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      throw new ApiError("internal", `Gwen answered something this screen can't read (${res.status})`, {});
    }
  }
  if (!res.ok) {
    const err = data?.error ?? {};
    throw new ApiError(err.code ?? "internal", err.message ?? `the request failed (${res.status})`, err.details ?? {});
  }
  return data;
}

const get = (path, query) => request("GET", path, query);
const post = (path, body, query) => request("POST", path, query, body ?? {});
const patch = (path, body) => request("PATCH", path, undefined, body);
const put = (path, body) => request("PUT", path, undefined, body);
const del = (path, query) => request("DELETE", path, query);
const id = encodeURIComponent;

/** Every daemon route, named as internal/client names it. Requests and replies are the wire JSON. */
export const api = {
  Health: () => get("/v1/health"),
  Status: () => get("/v1/status"),
  ClockIn: (req) => post("/v1/day/clock-in", req),
  ClockOut: () => post("/v1/day/clock-out"),
  BreakStart: () => post("/v1/break/start"),
  BreakEnd: () => post("/v1/break/end"),
  Switch: (req) => post("/v1/session/switch", req),
  Snooze: () => post("/v1/nudge/snooze"),

  ListDays: (from, to) => get("/v1/days", { from, to }),
  GetDay: (day) => get(`/v1/days/${id(day)}`),
  PatchDay: (day, req) => patch(`/v1/days/${id(day)}`, req),
  CreateSegment: (req) => post("/v1/segments", req),
  PatchSegment: (sid, req) => patch(`/v1/segments/${id(sid)}`, req),
  SplitSegment: (sid, req) => post(`/v1/segments/${id(sid)}/split`, req),
  DeleteSegment: (sid) => del(`/v1/segments/${id(sid)}`),

  /** archived: "true", "false", or "all". */
  ListProjects: (archived) => get("/v1/projects", { archived }),
  CreateProject: (req) => post("/v1/projects", req),
  GetProject: (pid) => get(`/v1/projects/${id(pid)}`),
  PatchProject: (pid, req) => patch(`/v1/projects/${id(pid)}`, req),
  DeleteProject: (pid) => del(`/v1/projects/${id(pid)}`),

  /** q: {project_id, goal_id, status, due_before, templates}. */
  ListTasks: (q) => get("/v1/tasks", { project_id: q?.project_id, goal_id: q?.goal_id, status: q?.status, due_before: q?.due_before, templates: q?.templates }),
  CreateTask: (req) => post("/v1/tasks", req),
  GetTask: (tid) => get(`/v1/tasks/${id(tid)}`),
  PatchTask: (tid, req) => patch(`/v1/tasks/${id(tid)}`, req),
  CompleteTask: (tid, req) => post(`/v1/tasks/${id(tid)}/complete`, req),
  ReopenTask: (tid) => post(`/v1/tasks/${id(tid)}/reopen`),
  DeleteTask: (tid) => del(`/v1/tasks/${id(tid)}`),
  DeleteTasks: (req) => post("/v1/tasks/delete", req),

  StatsSummary: (from, to) => get("/v1/stats/summary", { from, to }),
  StatsHeatmap: (year) => get("/v1/stats/heatmap", { year }),

  GetConfig: () => get("/v1/config"),
  /** patch: {section: {key: value}}. */
  PatchConfig: (p) => patch("/v1/config", p),
  NotifyTest: () => post("/v1/notify/test"),
  NotifyPreview: () => post("/v1/notify/preview"),
  Nudges: (card) => get("/v1/nudges", card ? { card: "1" } : undefined),
  ActOnNudge: (kind, action) => post("/v1/nudges/act", { kind, action }),

  ListGoals: (status) => get("/v1/goals", { status }),
  CreateGoal: (req) => post("/v1/goals", req),
  GetGoal: (gid) => get(`/v1/goals/${id(gid)}`),
  PatchGoal: (gid, req) => patch(`/v1/goals/${id(gid)}`, req),
  /** openTasks: also delete the goal's open tasks. */
  DeleteGoal: (gid, openTasks) => del(`/v1/goals/${id(gid)}`, openTasks ? { tasks: "open" } : undefined),
  PreviewGoal: (req) => post("/v1/goals/preview", req),
  ListCommitments: () => get("/v1/commitments"),
  CreateCommitment: (req) => post("/v1/commitments", req),
  PatchCommitment: (cid, req) => patch(`/v1/commitments/${id(cid)}`, req),
  DeleteCommitment: (cid) => del(`/v1/commitments/${id(cid)}`),
  GetPlan: (day) => get("/v1/plan", { day }),
  GeneratePlan: (req) => post("/v1/plan/generate", req),
  SetDayHours: (req) => post("/v1/plan/hours", req),
  PatchPlanItem: (iid, req) => patch(`/v1/plan/items/${id(iid)}`, req),
  MovePlanTask: (req) => post("/v1/plan/move", req),
  ScheduleTask: (req) => post("/v1/plan/schedule", req),
  UnscheduleTask: (req) => post("/v1/plan/unschedule", req),
  Briefing: () => get("/v1/briefing"),

  GetReview: (weekStart) => get(`/v1/reviews/${id(weekStart)}`),
  SaveReview: (weekStart, req) => put(`/v1/reviews/${id(weekStart)}`, req),
  EnergyReport: (days) => get("/v1/energy", { days: days > 0 ? days : undefined }),
  LogEnergy: (req) => post("/v1/energy", req),
  DeleteEnergy: (eid) => del(`/v1/energy/${id(eid)}`),

  CalendarStatus: () => get("/v1/calendar/status"),
  CalendarAuthStart: () => post("/v1/calendar/auth/start"),
  CalendarSync: () => post("/v1/calendar/sync"),
  CalendarCalendars: () => get("/v1/calendar/calendars"),

  GoalBreakdown: (goalID, req) => post(`/v1/goals/${id(goalID)}/breakdown`, req),
  PlanChat: (req) => post("/v1/plan/chat", req),
  Retro: (req) => post("/v1/retro", req),
  GetLLMRun: (rid) => get(`/v1/llm/runs/${id(rid)}`),
  AcceptLLMRun: (rid, req) => post(`/v1/llm/runs/${id(rid)}/accept`, req),
  RejectLLMRun: (rid) => post(`/v1/llm/runs/${id(rid)}/reject`),
  AssistantChat: (req) => post("/v1/assistant/chat", req),
  AssistantSelf: () => get("/v1/assistant/self"),
  ResetAssistantSelf: () => del("/v1/assistant/self"),
  AssistantSelfHistory: (limit) => get("/v1/assistant/self/history", { limit: limit > 0 ? limit : undefined }),
  ListThreads: (limit, offset) => get("/v1/assistant/threads", { limit: limit > 0 ? limit : undefined, offset: offset > 0 ? offset : undefined }),
  GetThread: (tid) => get(`/v1/assistant/threads/${id(tid)}`),
  DeleteThread: (tid) => del(`/v1/assistant/threads/${id(tid)}`),
  SearchThreads: (q, limit) => get("/v1/assistant/search", { q, limit: limit > 0 ? limit : undefined }),

  MemoryStatus: () => get("/v1/memory/status"),
  /** q: {status, key, subject, q, hidden, limit, offset}. */
  ListClaims: (q) => get("/v1/memory/claims", { status: q?.status, key: q?.key, subject: q?.subject, q: q?.q, hidden: q?.hidden, limit: q?.limit > 0 ? q.limit : undefined, offset: q?.offset > 0 ? q.offset : undefined }),
  GetClaim: (cid) => get(`/v1/memory/claims/${id(cid)}`),
  EditClaim: (cid, req) => patch(`/v1/memory/claims/${id(cid)}`, req),
  ConfirmClaim: (cid) => post(`/v1/memory/claims/${id(cid)}/confirm`),
  RejectClaim: (cid) => post(`/v1/memory/claims/${id(cid)}/reject`),
  HideClaim: (cid, req) => post(`/v1/memory/claims/${id(cid)}/hide`, req),
  ForgetClaim: (cid) => del(`/v1/memory/claims/${id(cid)}`),
  ExplainMemory: (ref) => get("/v1/memory/explain", { ref }),
  Remember: (req, channel) => post("/v1/memory/remember", req, { channel }),
  RecallMemory: (req) => post("/v1/memory/recall", req),
  ListConflicts: (status) => get("/v1/memory/conflicts", { status }),
  ResolveConflict: (cid, req) => post(`/v1/memory/conflicts/${id(cid)}/resolve`, req),
  ListEntities: () => get("/v1/memory/entities"),
  MemoryProfile: () => get("/v1/memory/profile"),
  ListResources: () => get("/v1/memory/resources"),
  AddResource: (req) => post("/v1/memory/resources", req),
  RemoveResource: (rid) => del(`/v1/memory/resources/${id(rid)}`),
  RebuildMemory: (req) => post("/v1/memory/rebuild", req),
  SleepMemory: () => post("/v1/memory/sleep"),
  ExportMemory: () => get("/v1/memory/export"),
  ForgetAllMemory: (confirm) => post("/v1/memory/forget-all", { confirm }),

  SyncStatus: () => get("/v1/sync/status"),
  SyncNow: () => post("/v1/sync/now"),

  CodeSessions: () => get("/v1/code/sessions"),
  StartCodeSession: (req) => post("/v1/code/sessions", req),
  RemoteStatus: () => get("/v1/remote/status"),
  RemotePairCode: () => post("/v1/remote/pair-code"),
  RemoteDevices: () => get("/v1/remote/devices"),
  RevokeRemoteDevice: (did) => del(`/v1/remote/devices/${id(did)}`),
  SetRemoteHooks: (on) => post("/v1/remote/hooks", { on }),

  /** {stored: {name: bool}}: which credential files exist. Values are never sent back. */
  Credentials: () => get("/v1/credentials"),
  SetCredential: (name, value) => put(`/v1/credentials/${id(name)}`, { value }),
  /** Shows a Gwen screen in this window: today, gwen, tasks, plan, review, connections, memory, settings, or an old name like "board". */
  OpenApp: (screen) => post("/v1/app/open", { screen }),
};

const isStatus = (v) => v && typeof v === "object" && "state" in v && "server_now_at" in v;

/**
 * What every Gwen tab shares: whether the daemon answers, its status and
 * configuration, the live projects and open tasks, and a version number per
 * kind of data that goes up when the daemon says it changed. Screens compare
 * the versions they loaded with these and reload when they differ.
 */
class Gwen {
  constructor() {
    this.up = null; // null until the first answer
    this.status = null;
    this.config = null;
    this.projects = []; // live, not archived
    this.tasks = []; // open
    this.versions = { days: 0, tasks: 0, projects: 0, plan: 0, goals: 0, integration: 0, memory: 0, remote: 0, config: 0 };
    this.lastAttribution = { project_id: null, task_id: null };
    this.skew = 0;
    this.rev = 0; // goes up on every change of anything above
    this.toasts = []; // {id, text, level, at}: shown by every tab's shell
    this.navigate = null; // {screen, sub, n}: a tab asked to be shown on a sub-screen
    this.waiters = [];
    this.seq = null; // the long poll's place in the event stream
    this.polling = null; // {promise, at}
    this.refreshing = null;
    this.toastId = 0;
  }

  // Change notification ---------------------------------------------------

  /** Resolves the next time anything shared changes. */
  changed() {
    return new Promise((resolve) => this.waiters.push(resolve));
  }

  bump() {
    this.rev++;
    const waiters = this.waiters;
    this.waiters = [];
    for (const w of waiters) w();
  }

  /** The daemon's clock now. */
  now() {
    return Date.now() - this.skew;
  }

  /** The work day now belongs to, "YYYY-MM-DD". */
  today() {
    if (this.status?.work_day) return this.status.work_day.day;
    const rollover = this.config?.tracking?.day_rollover ?? "04:00";
    const d = new Date(this.now());
    const [h, m] = rollover.split(":").map(Number);
    if (d.getHours() * 60 + d.getMinutes() < h * 60 + m) d.setDate(d.getDate() - 1);
    const pad = (n) => String(n).padStart(2, "0");
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  }

  setStatus(s) {
    this.skew = Date.now() - s.server_now_at;
    if (s.state !== "off") this.lastAttribution = { project_id: s.project_id ?? null, task_id: s.task_id ?? null };
    this.status = s;
    this.up = true;
  }

  // Toasts ------------------------------------------------------------------

  /**
   * Shows a short message at the bottom of every Gwen tab. level: "info" |
   * "success" | "error". action: {label, run()} adds a button, such as Undo.
   */
  toast(text, level = "info", action = null) {
    this.toasts = [...this.toasts.filter((x) => Date.now() - x.at < 6000), { id: ++this.toastId, text, level, action, at: Date.now() }];
    this.bump();
  }

  // Calls -------------------------------------------------------------------

  /** Reports a failed call the way every screen does, and returns it as an ApiError. */
  fail(e) {
    const err = apiError(e);
    if (err.code === NOT_RUNNING) {
      if (this.up !== false) {
        this.up = false;
        this.bump();
      }
    } else if (isStale(err)) {
      this.toast("Changed elsewhere, so it was reloaded");
      this.refresh();
    } else this.toast(err.message, "error");
    return err;
  }

  /** Runs an API call, adopting a returned Status and reporting failures. Returns undefined when it failed. */
  async act(call) {
    try {
      const out = await call();
      if (isStatus(out)) {
        this.setStatus(out);
        this.bump();
      }
      return out;
    } catch (e) {
      this.fail(e);
      return undefined;
    }
  }

  /** As act, for calls that return nothing: true when the call succeeded. */
  async run(call) {
    try {
      const out = await call();
      if (isStatus(out)) {
        this.setStatus(out);
        this.bump();
      }
      return true;
    } catch (e) {
      this.fail(e);
      return false;
    }
  }

  // Lists -------------------------------------------------------------------

  async loadProjects() {
    this.projects = (await api.ListProjects("false")).projects ?? [];
    this.versions.projects++;
  }

  async loadTasks() {
    this.tasks = (await api.ListTasks({ status: "open" })).tasks ?? [];
    this.versions.tasks++;
  }

  /** Loads everything shared again. Calls made while one runs join it. */
  refresh() {
    if (!this.refreshing) {
      this.refreshing = (async () => {
        try {
          const [s, c] = await Promise.all([api.Status(), api.GetConfig()]);
          this.setStatus(s);
          this.config = c;
          await Promise.all([this.loadProjects(), this.loadTasks()]);
          for (const k of ["days", "plan", "goals", "integration", "memory", "remote", "config"]) this.versions[k]++;
        } catch (e) {
          const err = apiError(e);
          if (err.code === NOT_RUNNING) this.up = false;
          else this.toast(err.message, "error");
        } finally {
          this.refreshing = null;
          this.bump();
        }
      })();
    }
    return this.refreshing;
  }

  // Events ------------------------------------------------------------------

  apply(ev) {
    switch (ev.name) {
      case "state_changed":
        this.setStatus(ev.data);
        break;
      case "config_changed":
        this.config = ev.data;
        this.versions.config++;
        break;
      case "day_changed":
        this.versions.days++;
        break;
      case "projects_changed":
        return this.loadProjects().catch(() => {});
      case "tasks_changed":
        return this.loadTasks().catch(() => {});
      case "plan_changed":
        this.versions.plan++;
        break;
      case "goals_changed":
        this.versions.goals++;
        break;
      case "integration_changed":
        this.versions.integration++;
        break;
      case "memory_changed":
        this.versions.memory++;
        break;
      case "remote_changed":
        this.versions.remote++;
        break;
    }
    return null;
  }

  /** One long poll: waits up to 25 s for events and applies them. */
  async pollOnce() {
    if (this.up !== true || this.seq === null) {
      await this.refresh();
      if (this.up !== true) return false;
      this.seq = (await get("/v1/events/poll")).seq;
      return true;
    }
    const out = await get("/v1/events/poll", { after: String(this.seq) });
    if (out.reset) {
      this.seq = out.seq;
      await this.refresh();
      return true;
    }
    this.seq = out.seq;
    if (out.events.length === 0) return true;
    for (const ev of out.events) await this.apply(ev);
    this.bump();
    return true;
  }

  /**
   * Joins a tab to the shared connection. Each tab runs this loop, but only
   * one request is ever in flight: the others wait on the same promise. A
   * poll whose tab was closed never settles, so nobody waits on one for more
   * than 40 s.
   */
  attach(cx) {
    cx.spawn(async (cx) => {
      for (;;) {
        const now = Date.now();
        if (!this.polling || now - this.polling.at > 40000) {
          const mine = { at: now, promise: null };
          mine.promise = this.pollOnce().then(
            (ok) => ok,
            (e) => {
              const err = apiError(e);
              if (err.code === NOT_RUNNING || err.code === "unauthorized") {
                this.up = false;
                this.seq = null;
                this.bump();
              }
              return false;
            },
          ).finally(() => {
            if (this.polling === mine) this.polling = null;
          });
          this.polling = mine;
        }
        const ok = await Promise.race([this.polling.promise, cx.sleep(41000).then(() => null)]);
        // Down: look again every three seconds, so the tabs come back by themselves.
        if (ok === false) await cx.sleep(3000);
      }
    });
  }
}

// One for every tab: module code runs once and is shared by the copies.
export const gwen = new Gwen();

/**
 * The tracking commands with the tray menu's visibility rules, for Today,
 * the status line, and every list with a start button.
 */
export function tracking() {
  const state = gwen.status?.state ?? "off";
  const working = state === "working" || state === "idle_pending";
  const onBreak = state === "break_auto" || state === "break_manual";
  const clockIn = (project_id, task_id) => gwen.act(() => api.ClockIn({ project_id, task_id }));
  const switchTo = (project_id, task_id) => gwen.act(() => api.Switch({ project_id, task_id }));
  return {
    state,
    working,
    onBreak,
    canStartBreak: working || state === "break_auto",
    clockIn,
    switchTo,
    /** Starts tracking a task: clocks in, switches to it, or ends the break on it. */
    async startTask(t) {
      const project = t.project_id ?? null;
      if (state === "off") return clockIn(project, t.id);
      const s = await switchTo(project, t.id);
      if (s && onBreak) return gwen.act(() => api.BreakEnd());
      return s;
    },
    /** Whether the clock is running on this task right now. */
    isTracking: (taskId) => working && gwen.status?.task_id === taskId,
    breakStart: () => gwen.act(() => api.BreakStart()),
    breakEnd: () => gwen.act(() => api.BreakEnd()),
    snooze: () => gwen.act(() => api.Snooze()),
    clockOut: () => gwen.act(() => api.ClockOut()),
  };
}
