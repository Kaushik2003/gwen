// Drops Gwen's now card down from the tray icon: an app cannot place its own
// window on Wayland, and KWin's placement rules pass it by, so this moves the
// card under the cursor, against the panel edge nearer it, on that screen.
// The card is titled once it is mapped, so it is placed when the title comes.
const title = "Gwen · now";

function place(w) {
    const c = workspace.cursorPos;
    const area = workspace.clientArea(KWin.PlacementArea, workspace.screenAt(c), workspace.currentDesktop);
    const g = w.frameGeometry;
    const gap = 6;
    const x = Math.max(area.x + gap, Math.min(c.x - g.width / 2, area.x + area.width - g.width - gap));
    const y = c.y < area.y + area.height / 2 ? area.y + gap : area.y + area.height - g.height - gap;
    w.frameGeometry = { x: Math.round(x), y: Math.round(y), width: g.width, height: g.height };
}

workspace.windowAdded.connect((w) => {
    if (w.resourceClass != "gwen-ui") return;
    if (w.caption == title) return place(w);
    const titled = () => {
        if (w.caption != title) return;
        w.captionChanged.disconnect(titled);
        place(w);
    };
    w.captionChanged.connect(titled);
});
