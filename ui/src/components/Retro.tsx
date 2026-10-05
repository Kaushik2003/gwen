import { ScrollText } from "lucide-react";
import { RetroFlow } from "./ai";
import { Panel } from "./ui";

/** Stats → Week → Retro (docs/08-clients.md#v3-additions). */
export default function Retro() {
  return (
    <Panel title="Weekly retro" icon={ScrollText}>
      <RetroFlow />
    </Panel>
  );
}
