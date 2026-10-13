import { Workflow } from "lucide-react";
import { Empty, PageHeader } from "../components/ui";

/** Runs, needs-you, and approvals (plan.md §5.3/§5.4) are kernel concepts that do not exist yet
 * (gwen-harness/harness.md's H1 "Kernel and agentic chat" is still unbuilt); this is a placeholder,
 * not an empty collection, so it says that plainly instead of implying a feature with no data. */
export default function Work() {
  return (
    <div className="flex flex-col gap-5">
      <PageHeader title="Work" />
      <Empty icon={Workflow} title="Not built yet">
        Runs, approvals, and the ledger need the permission kernel proposed in <code>gwen-harness/harness.md</code>.
        Nothing runs here until that lands.
      </Empty>
    </div>
  );
}
