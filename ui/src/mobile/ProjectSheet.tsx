import { FolderIcon } from "lucide-react";
import { useState } from "react";
import { ApiError, api, type Project, type ProjectFolder } from "./api";
import { useDay } from "./day";
import { Browser, type Made } from "./NewSession";
import { useStore } from "./store";
import { Sheet, baseName, tilde } from "./ui";

// A new project from the phone, or a folder for one that has none (the phone redo). The folder is one the phone may open
// (Settings › Phone's folders on the laptop), picked or made with the same picker as a new session, so Claude and shells can
// start there right after.

const colours = ["#4f8bf5", "#3ec1af", "#e9935a", "#b57edc", "#d72638", "#27a644", "#f2b33d", "#8a8f98"];

export function ProjectSheet({ project, onClose, onMade }: {
  /** Given, the sheet gives this project its folder; left out, it makes a new project. */
  project?: Project;
  onClose: () => void;
  onMade?: (p: Project) => void;
}) {
  const { toast } = useStore();
  const day = useDay();
  const [name, setName] = useState("");
  const [colour, setColour] = useState(() => colours[day.projects.length % colours.length]);
  const [folder, setFolder] = useState("");
  const [made, setMade] = useState<Made | null>(null);
  const [browsing, setBrowsing] = useState<"" | "browse" | "new">(project ? "browse" : "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const pick = (path: string, m?: Made) => {
    setFolder(path);
    setMade(m ?? null);
    setBrowsing("");
  };
  const where: ProjectFolder = folder ? { folder, create_folder: !!made, git_init: made?.gitInit } : {};
  const ready = project ? !!folder : !!name.trim();

  const go = async () => {
    setBusy(true);
    setError("");
    try {
      const p = project ? await api.projectFolder(project.id, where) : await api.createProject({ name: name.trim(), color: colour, ...where });
      toast(project ? `${p.name} works in ${tilde(p.folder)}` : `Made ${p.name}`);
      day.reload();
      onMade?.(p);
      onClose();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "That didn't work.");
      setBusy(false);
    }
  };

  if (browsing) {
    return (
      <Browser
        start={folder}
        roots={[]}
        making={browsing === "new"}
        onPick={pick}
        onClose={() => (project && !folder ? onClose() : setBrowsing(""))}
        onError={(m) => (setError(m), setBrowsing(""))}
      />
    );
  }

  return (
    <Sheet title={project ? `A folder for ${project.name}` : "New project"} onClose={onClose}>
      <div className="pad col" style={{ gap: 16 }}>
        {!project && (
          <>
            <label className="col" style={{ gap: 6 }}>
              <span className="card-meta">Name</span>
              <input className="field" autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="What it's called" enterKeyHint="done" />
            </label>
            <div className="col" style={{ gap: 6 }}>
              <span className="card-meta" id="pj-colour">Colour</span>
              <div role="group" aria-labelledby="pj-colour" style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
                {colours.map((c, i) => (
                  <button
                    key={c}
                    type="button"
                    aria-label={`Colour ${i + 1}`}
                    aria-pressed={colour === c}
                    onClick={() => setColour(c)}
                    style={{ width: 44, height: 44, borderRadius: 12, display: "grid", placeItems: "center", border: `2px solid ${colour === c ? "var(--fg)" : "transparent"}` }}
                  >
                    <span style={{ width: 26, height: 26, borderRadius: 8, background: c }} />
                  </button>
                ))}
              </div>
            </div>
          </>
        )}
        <div className="col" style={{ gap: 6 }}>
          <span className="card-meta">Folder on the laptop</span>
          {folder ? (
            <div className="row" style={{ minHeight: 44, padding: "0 12px", borderRadius: 12, background: "var(--card)", border: "1px solid var(--border-2)" }}>
              <FolderIcon size={17} aria-hidden style={{ flex: "none", color: "var(--muted)" }} />
              <span className="grow col" style={{ gap: 1, padding: "6px 0" }}>
                <span className="clip" style={{ fontWeight: 500 }}>
                  {baseName(folder)}
                  {made ? " (new)" : ""}
                </span>
                <span className="clip mono muted" style={{ fontSize: 11.5 }}>
                  {tilde(folder)}
                </span>
              </span>
              <button type="button" className="btn" style={{ minHeight: 36, padding: "0 10px" }} onClick={() => setBrowsing("browse")}>
                Change
              </button>
            </div>
          ) : (
            <div className="buttons">
              <button type="button" className="btn flex" onClick={() => setBrowsing("browse")}>
                Pick a folder
              </button>
              <button type="button" className="btn flex" onClick={() => setBrowsing("new")}>
                New folder
              </button>
            </div>
          )}
          <p className="card-meta">
            {project
              ? "Claude and shells for it start here."
              : folder
                ? "Claude and shells for it start here."
                : "Optional. With a folder you can start Claude and shells in it from here."}
          </p>
        </div>
        {error && (
          <p className="card-meta" role="alert" style={{ color: "var(--red)" }}>
            {error}
          </p>
        )}
        <div className="buttons">
          <button type="button" className="btn flex" onClick={onClose} disabled={busy}>
            Cancel
          </button>
          <button type="button" className="btn primary flex" disabled={!ready || busy} onClick={go}>
            {busy ? <span className="spin" /> : project ? "Use this folder" : "Make the project"}
          </button>
        </div>
      </div>
    </Sheet>
  );
}
