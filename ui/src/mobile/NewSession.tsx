import { Check, ChevronRight, CornerLeftUp, Folder as FolderIcon, FolderGit2, FolderPlus, Lock, Mic, Plus, X } from "lucide-react";
import { useEffect, useState } from "react";
import { ApiError, api, type Conversation, type Folder, type Folders } from "./api";
import { HowItRuns } from "./SessionScreen";
import { useStore } from "./store";
import { Dock, Sheet, Switch, agoWords, baseName, lasted, tilde } from "./ui";
import { useHoldToTalk } from "./voice";

/** A folder the session will make when it starts. */
export interface Made { gitInit: boolean }

/** Start Claude in a folder on the laptop: a recent one, one you browse to, or a new one; or continue an earlier conversation. */
export function NewSession() {
  const { back, go, upsert, toast, sessions } = useStore();
  const [recent, setRecent] = useState<Folder[] | null>(null);
  const [roots, setRoots] = useState<string[]>([]);
  const [dir, setDir] = useState("");
  const [made, setMade] = useState<Made | null>(null);
  const [browse, setBrowse] = useState<"" | "browse" | "new">("");
  const [changing, setChanging] = useState(false);
  const [sandbox, setSandbox] = useState(false);
  const [window_, setWindow] = useState(true);
  const [prompt, setPrompt] = useState("");
  const [convs, setConvs] = useState<Conversation[]>([]);
  const [resume, setResume] = useState("");
  const [more, setMore] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const talk = useHoldToTalk((heard) => setPrompt((p) => (p && !/\s$/.test(p) ? p + " " : p) + heard), toast);

  useEffect(() => {
    api.recentFolders().then(
      (f) => {
        setRecent(f.folders);
        setRoots(f.roots);
        // The folder used last is the likeliest one.
        setDir((d) => d || f.folders[0]?.path || "");
      },
      () => setRecent([]),
    );
  }, []);
  useEffect(() => {
    setConvs([]);
    setResume("");
    setMore(false);
    if (!dir || made) return;
    let live = true;
    api.conversations(dir).then((c) => live && setConvs(c.conversations), () => {});
    return () => {
      live = false;
    };
  }, [dir, made]);

  const pick = (path: string, m: Made | null = null) => {
    setDir(path);
    setMade(m);
    setChanging(false);
    setBrowse("");
  };
  const conv = convs.find((c) => c.claude_session_id === resume);
  const start = async () => {
    setBusy(true);
    setError("");
    try {
      // A conversation that is running already has a card: open it instead of starting a second Claude on it.
      if (conv?.session_id) return go({ name: "session", id: conv.session_id }, true);
      const s = await api.start({
        dir, window: window_, sandbox, ...(made ? { create: true, git_init: made.gitInit } : {}), ...(resume ? { resume } : { prompt: prompt.trim() || undefined }),
      });
      upsert(s);
      go({ name: "session", id: s.id }, true);
    } catch (e) {
      const err = e as ApiError;
      if (err.details?.reason === "running") return go({ name: "session", id: String(err.details.session_id) }, true);
      setError(err.message ?? "Couldn't start the session.");
      setBusy(false);
    }
  };

  const known = recent?.find((f) => f.path === dir);
  const folderRow = (path: string, name: string, sub: string, on: boolean, onClick: () => void) => (
    <button key={path} className="pick" role="radio" aria-checked={on} onClick={onClick}>
      <FolderIcon size={18} aria-hidden />
      <span className="what">
        <span className="clip">{name}</span>
        <span className="clip">{sub}</span>
      </span>
      {on && <Check size={18} strokeWidth={2.4} style={{ color: "var(--accent-2)", flex: "none" }} aria-hidden />}
    </button>
  );
  const subOf = (f: Folder) => tilde(f.path, roots) + (f.branch ? ` · ${f.branch}` : "");
  const madeSub = made ? `New folder${made.gitInit ? " with git" : ""} · made when it starts` : "";
  const shown = more ? convs : convs.slice(0, 4);
  const collapsed = !!dir && convs.length > 0 && !changing;

  return (
    <>
      <header className="head titled">
        <h1 className="title">New session</h1>
        <button className="ghost soft" aria-label="Close" onClick={back}><X size={20} /></button>
      </header>
      <main className="page">
        <div className="form">
          <section aria-labelledby="folder-title">
            <h2 id="folder-title" className="group-title">Folder</h2>
            {collapsed ? (
              <div className="picks">
                <button className="pick" onClick={() => setChanging(true)} aria-label={`Folder: ${baseName(dir)}. Change`}>
                  <FolderIcon size={18} style={{ color: "var(--accent-2)" }} aria-hidden />
                  <span className="what">
                    <span className="clip" style={{ fontWeight: 600 }}>{baseName(dir)}</span>
                    <span className="clip">{made ? madeSub : known ? subOf(known) : tilde(dir, roots)}</span>
                  </span>
                  <span className="change">Change</span>
                </button>
              </div>
            ) : (
              <div className="picks" role="radiogroup" aria-labelledby="folder-title">
                {recent === null && <div className="pick"><span className="spin" /></div>}
                {dir && !known && folderRow(dir, baseName(dir), made ? madeSub : tilde(dir, roots), true, () => setChanging(false))}
                {recent?.slice(0, 4).map((f) => folderRow(f.path, f.name, subOf(f), f.path === dir, () => pick(f.path)))}
                <div className="split">
                  <button onClick={() => setBrowse("browse")}>Browse folders…</button>
                  <button onClick={() => setBrowse("new")}>New folder…</button>
                </div>
              </div>
            )}
          </section>

          {convs.length > 0 && (
            <section aria-labelledby="conv-title">
              <h2 id="conv-title" className="group-title">Conversation</h2>
              <div className="picks" role="radiogroup" aria-labelledby="conv-title">
                <button className="pick" role="radio" aria-checked={resume === ""} onClick={() => setResume("")}>
                  <Plus size={18} aria-hidden />
                  <span className="what">
                    <span>New conversation</span>
                    <span>Start fresh in this folder</span>
                  </span>
                </button>
                {shown.map((c) => {
                  const on = resume === c.claude_session_id;
                  const running = c.session_id ? sessions.find((x) => x.id === c.session_id) : undefined;
                  return (
                    <button key={c.claude_session_id} className="pick" role="radio" aria-checked={on} onClick={() => (setResume(c.claude_session_id), setSandbox(c.sandbox))}>
                      <span className="mark" aria-hidden>
                        {on ? <Check size={18} strokeWidth={2.4} style={{ color: "var(--accent-2)" }} /> : c.session_id ? <span className="led on" /> : null}
                      </span>
                      <span className="what">
                        <span className="clip">{c.title || "Untitled conversation"}</span>
                        <span className="row" style={{ gap: 6 }}>
                          {c.session_id
                            ? `Running now${running && !running.window ? ", no window" : ""}${running ? ` · ${running.state === "working" ? "working" : `idle ${lasted(running.updated_at)}`}` : ""}`
                            : agoWords(c.active_at)}
                          {c.sandbox && <span className="sandbox"><Lock size={11} strokeWidth={2.4} aria-hidden />Sandbox</span>}
                        </span>
                      </span>
                      {c.session_id && <span className="go">Open</span>}
                    </button>
                  );
                })}
                {convs.length > shown.length && (
                  <button className="pick" onClick={() => setMore(true)}>
                    <span className="mark" aria-hidden />
                    <span className="what"><span style={{ color: "var(--accent-text)" }}>{convs.length - shown.length} older conversations</span></span>
                  </button>
                )}
              </div>
            </section>
          )}

          {!conv?.session_id && (
            <section aria-labelledby="how-title">
              <h2 id="how-title" className="group-title">How it runs</h2>
              <HowItRuns sandbox={sandbox} onChange={setSandbox} short={!!resume} last={conv ? conv.sandbox : undefined} />
            </section>
          )}

          {!resume && (
            <section>
              <label htmlFor="first" className="group-title">First message (optional)</label>
              <div className="first">
                <textarea id="first" rows={3} placeholder="What should Claude do?" value={prompt} onChange={(e) => setPrompt(e.target.value)} />
                <button className={`round ${talk.state === "listening" ? "rec" : ""}`} aria-label="Hold to talk" {...talk.bind}>
                  {talk.state === "hearing" ? <span className="spin" /> : <Mic size={20} />}
                </button>
              </div>
            </section>
          )}

          {!conv?.session_id && (
            <div className="toggle">
              <span className="what" id="window-what">
                <span>Open a window on the laptop</span>
                <span>Off: it runs in the background until you open it</span>
              </span>
              <Switch on={window_} onChange={setWindow} label="Open a window on the laptop" />
            </div>
          )}
          {error && <p className="error" role="alert">{error}</p>}
        </div>
      </main>
      <Dock className="cta">
        <button className="btn primary big full" disabled={busy || !dir} onClick={start}>
          {busy ? <span className="spin" /> : conv?.session_id ? "Open the running session" : resume ? "Continue conversation" : "Start session"}
        </button>
      </Dock>
      {browse && (
        <Browser
          start={dir}
          roots={roots}
          making={browse === "new"}
          onClose={() => setBrowse("")}
          onPick={pick}
          onError={toast}
        />
      )}
    </>
  );
}

/** The folder picker: folders only, inside the folders the laptop allows. A new folder is only named here; it is made when the session starts. */
export function Browser({ start, roots, making: makingFirst, onPick, onClose, onError }: {
  start: string; roots: string[]; making: boolean; onPick: (path: string, made?: Made) => void; onClose: () => void; onError: (msg: string) => void;
}) {
  const [at, setAt] = useState<Folders | null>(null);
  const [making, setMaking] = useState(makingFirst);
  const [name, setName] = useState("");
  const [gitInit, setGitInit] = useState(true);

  const load = (path: string) =>
    api.folders(path).then(setAt, (e) => {
      if (path) load("");
      else onError(e instanceof ApiError ? e.message : "Couldn't list folders.");
    });
  useEffect(() => {
    load(start ? start.replace(/\/[^/]+\/?$/, "") : "");
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const rootsNow = at?.roots.length ? at.roots : roots;
  const bad = !name.trim() || name.startsWith(".") || name.includes("/");
  return (
    <Sheet title={making ? "New folder" : at ? tilde(at.path, rootsNow) : "Folders"} onClose={onClose}>
      {!at ? (
        <div className="empty"><span className="spin" /></div>
      ) : making ? (
        <div className="pad">
          <label className="sr" htmlFor="new-folder">New folder name</label>
          <input id="new-folder" className="field" autoFocus placeholder="New folder name" value={name} onChange={(e) => setName(e.target.value)} />
          <p className="card-meta">In {tilde(at.path, rootsNow)}. It is made when you go ahead.</p>
          <div className="toggle" style={{ padding: 0 }}>
            <span className="what"><span>Make it a git repository</span></span>
            <Switch on={gitInit} onChange={setGitInit} label="Make it a git repository" />
          </div>
          <div className="buttons">
            <button className="btn flex" onClick={() => setMaking(false)}>Choose where</button>
            <button className="btn primary flex" disabled={bad} onClick={() => onPick(`${at.path}/${name.trim()}`, { gitInit })}>Use this name</button>
          </div>
        </div>
      ) : (
        <>
          <div className="pad" style={{ paddingBottom: 8 }}>
            <button className="btn primary full" onClick={() => onPick(at.path)}>Use {baseName(at.path)}</button>
          </div>
          {at.parent && <button className="item" onClick={() => load(at.parent)}><CornerLeftUp size={18} /> Up</button>}
          {at.folders.map((f) => (
            <button key={f.path} className="item" onClick={() => load(f.path)}>
              {f.git ? <FolderGit2 size={18} /> : <FolderIcon size={18} />}
              <span className="grow clip">{f.name}</span>
              {f.branch && <span className="card-meta mono clip" style={{ maxWidth: "35%" }}>{f.branch}</span>}
              <ChevronRight size={16} />
            </button>
          ))}
          {at.folders.length === 0 && <p className="note">No folders in here.</p>}
          <button className="item" onClick={() => setMaking(true)}><FolderPlus size={18} /> New folder here</button>
        </>
      )}
    </Sheet>
  );
}
