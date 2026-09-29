// The hub build (VITE_GWEN_TARGET=hub) talks to the sync hub over fetch
// instead of the Wails bindings: the read-only /v1 subset of
// docs/07-integrations.md#read-only-dashboard, behind the session cookie
// that POST /login sets. Anything else rejects as unavailable.

type Query = Record<string, string | undefined>;

function fail(code: string, message: string): never {
  // Errors travel as the wire error body in JSON, as the host passes them.
  throw JSON.stringify({ code, message, details: {} });
}

async function get<T>(path: string, query?: Query): Promise<T> {
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(query ?? {})) if (v) params.set(k, v);
  const url = params.size ? `${path}?${params}` : path;
  let resp: Response;
  try {
    resp = await fetch(url, { credentials: "same-origin" });
  } catch {
    fail("daemon_not_running", "The hub is not reachable");
  }
  if (!resp.ok) {
    let body: { error?: { code: string; message: string } } | null = null;
    try {
      body = await resp.json();
    } catch {
      // not a JSON error body
    }
    fail(body?.error?.code ?? (resp.status === 401 ? "unauthorized" : "internal"), body?.error?.message ?? resp.statusText);
  }
  return resp.json();
}

const methods: Record<string, (...args: any[]) => Promise<unknown>> = {
  Health: () => get("/v1/health"),
  ListProjects: (archived: string) => get("/v1/projects", { archived }),
  GetProject: (id: string) => get(`/v1/projects/${encodeURIComponent(id)}`),
  ListTasks: (q: { project_id?: string; goal_id?: string; status?: string; due_before?: string; templates?: boolean }) =>
    get("/v1/tasks", {
      project_id: q.project_id, goal_id: q.goal_id, status: q.status, due_before: q.due_before,
      templates: q.templates ? "true" : undefined,
    }),
  GetTask: (id: string) => get(`/v1/tasks/${encodeURIComponent(id)}`),
  ListDays: (from: string, to: string) => get("/v1/days", { from, to }),
  GetDay: (day: string) => get(`/v1/days/${encodeURIComponent(day)}`),
  StatsSummary: (from: string, to: string) => get("/v1/stats/summary", { from, to }),
  StatsHeatmap: (year: number) => get("/v1/stats/heatmap", { year: String(year) }),
  GetPlan: (day: string) => get("/v1/plan", { day }),
  ListGoals: (status: string) => get("/v1/goals", { status }),
  GetGoal: (id: string) => get(`/v1/goals/${encodeURIComponent(id)}`),
  ListCommitments: () => get("/v1/commitments"),
};

/** The hub's stand-in for the Wails App bindings. */
export const hubApp = new Proxy(methods, {
  get: (target, name: string) =>
    target[name] ?? (() => Promise.reject(JSON.stringify({ code: "unavailable", message: "Not available on the hub", details: {} }))),
});

/** Signs in with the hub's sync token; true when the token was right. */
export async function login(token: string): Promise<boolean> {
  const resp = await fetch("/login", {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ token }),
  });
  return resp.ok;
}
