import { X } from "lucide-react";
import { useEffect } from "react";
import AssistantChat from "../components/AssistantChat";
import { IconButton } from "../components/ui";
import { useDaemon } from "../daemon";
import { Quit } from "../wailsjs/runtime/runtime";

/**
 * The talk window the panel widget opens: Gwen large, listening at once, the
 * same conversation as Assistant → Chat. Drag it by its top; hanging up, ×,
 * or Esc closes it.
 */
export default function Talk() {
  const d = useDaemon();
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && Quit();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return (
    <div className="relative flex h-full flex-col overflow-hidden border border-line-strong bg-canvas select-none">
      <div className="absolute inset-x-0 top-0 z-10 flex h-12 items-center justify-end px-2" style={{ "--wails-draggable": "drag" } as React.CSSProperties}>
        <IconButton icon={X} label="Close" tone="ghost" onClick={Quit} style={{ "--wails-draggable": "no-drag" } as React.CSSProperties} />
      </div>
      {d.up === false ? (
        <div className="grid flex-1 place-items-center px-6 text-center">
          <div>
            <div className="text-[15px] font-medium text-ink">Gwen isn't running</div>
            <p className="mt-1 text-xs text-ink-subtle">Start it from the panel widget, then talk again.</p>
          </div>
        </div>
      ) : (
        <AssistantChat call onHangUp={Quit} />
      )}
    </div>
  );
}
