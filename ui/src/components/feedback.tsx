import { CircleAlert, CircleCheck, Info, X } from "lucide-react";
import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from "react";
import { Button, Modal, cx } from "./ui";

type ToastTone = "info" | "success" | "error";
/** A button on a toast, such as Undo; pressing it runs it and dismisses the toast. */
export interface ToastAction {
  label: string;
  run: () => void;
}
export type Notify = (message: string, tone?: ToastTone, action?: ToastAction) => void;

interface Toast {
  id: number;
  message: string;
  tone: ToastTone;
  action?: ToastAction;
}

interface ConfirmOptions {
  title: string;
  body?: ReactNode;
  /** The confirming button's label: what will happen, such as "Delete goal". */
  confirm: string;
  /** Destroys something: the button is red. */
  danger?: boolean;
}

const ToastContext = createContext<Notify>(() => {});
const ConfirmContext = createContext<(o: ConfirmOptions) => Promise<boolean>>(async () => false);

/** Shows a short message in the corner: info, success, or an error that stays longer. One with an action stays long enough to use it. */
export function useToast(): Notify {
  return useContext(ToastContext);
}

/** Asks before doing something that cannot be undone; resolves true to go ahead. */
export function useConfirm(): (o: ConfirmOptions) => Promise<boolean> {
  return useContext(ConfirmContext);
}

/**
 * Toasts and confirmation dialogs. Both are drawn by the page, never by the
 * webview's own alert and confirm, which follow the system theme.
 */
export function FeedbackProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const next = useRef(1);
  const dismiss = useCallback((id: number) => setToasts((all) => all.filter((t) => t.id !== id)), []);
  const notify = useCallback<Notify>(
    (message, tone = "info", action) => {
      const id = next.current++;
      setToasts((all) => [...all.filter((t) => t.message !== message), { id, message, tone, action }].slice(-4));
      setTimeout(() => dismiss(id), tone === "error" ? 9000 : action ? 7000 : 3500);
    },
    [dismiss],
  );

  const [ask, setAsk] = useState<(ConfirmOptions & { resolve: (ok: boolean) => void }) | null>(null);
  const confirm = useCallback((o: ConfirmOptions) => new Promise<boolean>((resolve) => setAsk({ ...o, resolve })), []);
  const answer = (ok: boolean) => {
    ask?.resolve(ok);
    setAsk(null);
  };

  return (
    <ToastContext.Provider value={notify}>
      <ConfirmContext.Provider value={confirm}>
        {children}
        {ask && (
          <Modal
            title={ask.title}
            size="sm"
            onClose={() => answer(false)}
            footer={
              <>
                <Button onClick={() => answer(false)}>Cancel</Button>
                <Button tone={ask.danger ? "destroy" : "primary"} onClick={() => answer(true)} autoFocus>
                  {ask.confirm}
                </Button>
              </>
            }
          >
            {ask.body && <div className="text-sm leading-relaxed text-ink-muted">{ask.body}</div>}
          </Modal>
        )}
        <div aria-live="polite" className="pointer-events-none fixed right-4 bottom-4 z-[60] flex w-[24rem] max-w-[calc(100vw-2rem)] flex-col gap-2">
          {toasts.map((t) => (
            <ToastView key={t.id} toast={t} onClose={() => dismiss(t.id)} />
          ))}
        </div>
      </ConfirmContext.Provider>
    </ToastContext.Provider>
  );
}

function ToastView({ toast, onClose }: { toast: Toast; onClose: () => void }) {
  const Icon = toast.tone === "error" ? CircleAlert : toast.tone === "success" ? CircleCheck : Info;
  const color = toast.tone === "error" ? "text-danger" : toast.tone === "success" ? "text-working" : "text-accent-hover";
  return (
    <div
      role={toast.tone === "error" ? "alert" : "status"}
      className="animate-pop pointer-events-auto flex items-start gap-3 rounded-xl border border-line-strong bg-surface-3 py-3 pr-2 pl-3.5 shadow-xl shadow-black/60"
    >
      <Icon size={16} className={cx("mt-0.5 shrink-0", color)} aria-hidden />
      <p className="min-w-0 flex-1 text-[13px] leading-relaxed text-ink">{toast.message}</p>
      {toast.action && (
        <button
          type="button"
          onClick={() => {
            toast.action!.run();
            onClose();
          }}
          className="-my-0.5 shrink-0 rounded-md px-2 py-1 text-[13px] font-medium text-accent-hover hover:bg-surface-4"
        >
          {toast.action.label}
        </button>
      )}
      <button type="button" aria-label="Dismiss" onClick={onClose} className="grid size-6 shrink-0 place-items-center rounded-md text-ink-subtle hover:bg-surface-4 hover:text-ink">
        <X size={14} aria-hidden />
      </button>
    </div>
  );
}
