import type { wire } from "../api";
import { BreakdownFlow } from "./ai";
import { Modal } from "./ui";

/** Goal detail → Break down (docs/08-clients.md#v3-additions), as a dialog over Goals. */
export default function Breakdown({ goal, onClose }: { goal: wire.Goal; onClose: () => void }) {
  return (
    <Modal
      title={`Break down “${goal.title}”`}
      description={
        goal.kind === "quantity"
          ? `The assistant lines up the next sessions' ${goal.unit || "units"}, each saying what to do. Today's session takes the first ones.`
          : "The assistant proposes tasks that reach this goal by its due day."
      }
      size="lg"
      onClose={onClose}
    >
      <BreakdownFlow goal={goal} onDone={onClose} />
    </Modal>
  );
}
