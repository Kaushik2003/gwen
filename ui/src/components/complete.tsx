import { useState, type ReactNode } from "react";
import { App, wire } from "../api";
import { useDaemon } from "../daemon";
import { useToast } from "./feedback";
import { Button, Field, Input, Modal } from "./ui";

/**
 * Completing and reopening a task. A task that covers several units of a
 * quantity goal first asks how many were done, as gwen task done --qty does;
 * a step, or a task whose steps count instead, never asks. Every tick can be
 * undone from its toast, and undoing gives back exactly what was there: a
 * task keeps its steps when ticked.
 */
export function useCompleteTask(): {
  complete: (t: wire.Task, hasSteps?: boolean) => void;
  reopen: (t: wire.Task) => void;
  dialog: ReactNode;
} {
  const d = useDaemon();
  const notify = useToast();
  const [asking, setAsking] = useState<wire.Task | null>(null);
  const [qty, setQty] = useState("");

  async function done(t: wire.Task, quantity?: number) {
    const req = wire.CompleteTaskRequest.createFrom(quantity != null ? { quantity_done: quantity } : {});
    if (await d.act(() => App.CompleteTask(t.id, req)))
      notify(`Completed ${t.title}`, "success", { label: "Undo", run: () => void d.act(() => App.ReopenTask(t.id)) });
  }

  const dialog = asking && (
    <Modal
      title={`Complete ${asking.title}`}
      size="sm"
      onClose={() => setAsking(null)}
      footer={
        <>
          <Button onClick={() => setAsking(null)}>Cancel</Button>
          <Button
            tone="primary"
            disabled={!(Number(qty) >= 0) || qty === ""}
            onClick={() => {
              done(asking, Number(qty));
              setAsking(null);
            }}
          >
            Complete
          </Button>
        </>
      }
    >
      <Field label="How many did you do?" hint={`This task planned ${asking.quantity}. Any shortfall moves to the goal's next session.`}>
        <Input type="number" min={0} value={qty} onChange={(e) => setQty(e.target.value)} autoFocus className="w-28" />
      </Field>
    </Modal>
  );

  return {
    complete(t, hasSteps = false) {
      if (t.quantity != null && t.quantity > 1 && !t.parent_id && !hasSteps) {
        setQty(String(t.quantity));
        setAsking(t);
      } else done(t);
    },
    reopen(t) {
      d.act(() => App.ReopenTask(t.id));
    },
    dialog,
  };
}
