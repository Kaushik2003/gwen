import { ArrowLeft, ChevronRight, CornerLeftUp, Folder as FolderIcon, FolderGit2, FolderPlus } from "lucide-react";
import { useEffect, useState } from "react";
import { ApiError, api, type Conversation, type Folder, type Folders } from "./api";
import { useStore } from "./store";
import { Sheet, Switch, agoWords, baseName, tilde } from "./ui";

/** Start Claude in a folder on the laptop: a recent one, one you browse to, or a new one; or continue an earlier conversation. */
export function NewSession() {
  const { back, go, upsert, toast } = useStore();
  const [recent, setRecent] = useState<Folder[] | null>(null);
  const [roots, setRoots] = useState<string[]>([]);
  const [dir, setDir] = useState("");
  const [browse, setBrowse] = useState(false);
  const [sandbox, setSandbox] = useState(false);
  const [window_, setWindow] = useState(true);
  const [prompt, setPrompt] = useState("");
  const [convs, setConvs] = useState<Conversation[]>([]);
  const [resume, setResume] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    api.recentFolders().then(
      (f) => (setRecent(f.folders), setRoots(f.roots)),
      () => setRecent([]),
    );
  }, []);
  useEffect(() => {
    setConvs([]);
    setResume("");
    if (!dir) return;
    let live = true;
    api.conversations(dir).then((c) => live && setConvs(c.conversations), () => {});
    return () => {
      live = false;
    };
  }, [dir]);

  const start = async () => {
    setBusy(true);
    setError("");
    try {
      const conv = convs.find((c) => c.claude_session_id === resume);
      // A conversation that is running already has a card: open it instead of starting a second Claude on it.
      if (conv?.session_id) return go({ name: "session", id: conv.session_id }, true);
      const s = await api.start({ dir, window: window_, prompt: prompt.trim() || undefined, ...(resume ? { resume, sandbox: sandbox || undefined } : { sandbox }) });
      upsert(s);
      go({ name: "session", id: s.id }, true);
    } catch (e) {
      const err = e as ApiError;
      if (err.details?.reason === "running") return go({ name: "session", id: String(err.details.session_id) }, true);
      setError(err.message ?? "Couldn't start the session.");
      setBusy(false);
    }
  };

  return (
    <>
      <header className="bar">
        <button className="icon-btn" aria-label="Back" onClick={back}><ArrowLeft size={21} /></button>
        <h1 className="grow">New session</h1>
      </header>
      <main className="page">
        <div className="pad">
          <div>
            <span className="label">Folder</span>
            {dir ? (
              <button className="option on" onClick={() => setBrowse(true)}>
                <FolderIcon size={18} style={{ flex: "none", marginTop: 1 }} />
                <span className="grow col">
                  <b>{baseName(dir)}</b>
                  <span className="small muted mono clip">{tilde(dir, roots)}</span>
                </span>
                <span className="small" style={{ color: "var(--accent)" }}>Change</span>
              </button>
            ) : (
              <div className="col" style={{ gap: 8 }}>
                {recent === null && <span className="spin" />}
                {recent?.slice(0, 6).map((f) => (
                  <button key={f.path} className="option" onClick={() => setDir(f.path)}>
                    {f.git ? <FolderGit2 size={18} style={{ flex: "none", marginTop: 1 }} /> : <FolderIcon size={18} style={{ flex: "none", marginTop: 1 }} />}
                    <span className="grow col">
                      <b className="clip">{f.name}</b>
                      <span className="small muted mono clip">{tilde(f.path, roots)}{f.branch && ` · ${f.branch}`}</span>
                    </span>
                  </button>
                ))}
                <button className="btn full" onClick={() => setBrowse(true)}>Browse folders</button>
              </div>
            )}
          </div>

          {dir && (
            <>
              {convs.length > 0 && (
                <div>
                  <span className="label">Start fresh, or continue a conversation</span>
                  <div className="col" style={{ gap: 6 }}>
                    <button className={`option ${resume === "" ? "on" : ""}`} onClick={() => setResume("")}>
                      <span className="radio" />
                      <b>New conversation</b>
                    </button>
                    {convs.map((c) => (
                      <button key={c.claude_session_id} className={`option ${resume === c.claude_session_id ? "on" : ""}`} onClick={() => (setResume(c.claude_session_id), setSandbox(c.sandbox))}>
                        <span className="radio" />
                        <span className="grow col" style={{ gap: 2 }}>
                          <span className="clip2">{c.title}</span>
                          <span className="small muted">
                            {agoWords(c.active_at)}{c.sandbox && " · sandbox"}{c.session_id && " · running now"}
                          </span>
                        </span>
                      </button>
                    ))}
                  </div>
                </div>
              )}

              <div>
                <span className="label">How it runs</span>
                <div className="col" style={{ gap: 6 }}>
                  <button className={`option ${!sandbox ? "on" : ""}`} onClick={() => setSandbox(false)}>
                    <span className="radio" />
                    <span className="grow col">
                      <b>Normal</b>
                      <span className="small muted">Your own Claude, with your own permission rules.</span>
                    </span>
                  </button>
                  <button className={`option ${sandbox ? "on" : ""}`} onClick={() => setSandbox(true)}>
                    <span className="radio" />
                    <span className="grow col">
                      <b>Sandbox</b>
                      <span className="small muted">Claude only sees this folder and its own files: not your other folders, SSH keys or desktop. It can't push over SSH.</span>
                    </span>
                  </button>
                </div>
              </div>

              <div className="row">
                <span className="grow col">
                  <b>Open a window on the laptop</b>
                  <span className="small muted">A tab in Gwen, ready when you sit down.</span>
                </span>
                <Switch on={window_} onChange={setWindow} label="Open a window on the laptop" />
              </div>

              {!resume && (
                <div>
                  <label className="label" htmlFor="first">First message (optional)</label>
                  <textarea id="first" className="field" rows={3} placeholder="What should Claude do?" value={prompt} onChange={(e) => setPrompt(e.target.value)} />
                </div>
              )}
              {error && <p className="error" role="alert">{error}</p>}
              <button className="btn primary full" disabled={busy} onClick={start}>
                {busy ? <span className="spin" /> : convs.find((c) => c.claude_session_id === resume)?.session_id ? "Open it" : resume ? "Continue" : "Start Claude"}
              </button>
            </>
          )}
        </div>
      </main>
      {browse && <Browser start={dir} roots={roots} onClose={() => setBrowse(false)} onPick={(p) => (setDir(p), setBrowse(false))} onError={toast} />}
    </>
  );
}

/** The folder picker: folders only, inside the folders the laptop allows. */
function Browser({ start, roots, onPick, onClose, onError }: { start: string; roots: string[]; onPick: (path: string) => void; onClose: () => void; onError: (msg: string) => void }) {
  const [at, setAt] = useState<Folders | null>(null);
  const [making, setMaking] = useState(false);
  const [name, setName] = useState("");
  const [gitInit, setGitInit] = useState(true);
  const [busy, setBusy] = useState(false);
  const { go, upsert } = useStore();

  const load = (path: string) =>
    api.folders(path).then(setAt, (e) => {
      if (path) load("");
      else onError(e instanceof ApiError ? e.message : "Couldn't list folders.");
    });
  useEffect(() => {
    load(start ? start.replace(/\/[^/]+\/?$/, "") : "");
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // A new folder is made when the session starts in it, so nothing is created by browsing.
  const create = async () => {
    if (!at || !name.trim()) return;
    setBusy(true);
    try {
      const s = await api.start({ dir: `${at.path}/${name.trim()}`, create: true, git_init: gitInit });
      upsert(s);
      go({ name: "session", id: s.id }, true);
    } catch (e) {
      onError(e instanceof ApiError ? e.message : "Couldn't make that folder.");
      setBusy(false);
    }
  };

  return (
    <Sheet title={at ? tilde(at.path, at.roots.length ? at.roots : roots) : "Folders"} onClose={onClose}>
      {!at ? (
        <div className="empty"><span className="spin" /></div>
      ) : making ? (
        <div className="pad" style={{ paddingTop: 0 }}>
          <input className="field" autoFocus placeholder="New folder name" value={name} onChange={(e) => setName(e.target.value)} />
          <div className="row">
            <span className="grow">Make it a git repository</span>
            <Switch on={gitInit} onChange={setGitInit} label="Make it a git repository" />
          </div>
          <p className="small muted">Claude starts in the new folder right away.</p>
          <div className="row">
            <button className="btn grow" onClick={() => setMaking(false)}>Back</button>
            <button className="btn primary grow" disabled={busy || !name.trim() || name.startsWith(".") || name.includes("/")} onClick={create}>
              {busy ? <span className="spin" /> : "Create and start"}
            </button>
          </div>
        </div>
      ) : (
        <>
          <div className="pad" style={{ paddingTop: 0, paddingBottom: 8 }}>
            <button className="btn primary full" onClick={() => onPick(at.path)}>Use {baseName(at.path)}</button>
          </div>
          {at.parent && <button className="item" onClick={() => load(at.parent)}><CornerLeftUp size={18} className="muted" /> Up</button>}
          {at.folders.map((f) => (
            <button key={f.path} className="item" onClick={() => load(f.path)}>
              {f.git ? <FolderGit2 size={18} className="muted" /> : <FolderIcon size={18} className="muted" />}
              <span className="grow clip">{f.name}</span>
              {f.branch && <span className="small muted mono clip" style={{ maxWidth: "35%" }}>{f.branch}</span>}
              <ChevronRight size={16} className="muted" />
            </button>
          ))}
          {at.folders.length === 0 && <p className="muted" style={{ padding: "8px 16px" }}>No folders in here.</p>}
          <button className="item" onClick={() => setMaking(true)}><FolderPlus size={18} className="muted" /> New folder here</button>
        </>
      )}
    </Sheet>
  );
}
