// Gwen in the KDE panel: the day's worked timer and the task or project being
// tracked. A click drops down the now card (NowCard.qml) as Plasma's own
// popup; a right click has the tray's commands. It polls `gwen panel` every
// few seconds and ticks the timers in between.
import QtQuick
import QtQuick.Layouts
import org.kde.plasma.plasmoid
import org.kde.plasma.core as PlasmaCore
import org.kde.plasma.components as PlasmaComponents
import org.kde.plasma.plasma5support as P5Support
import org.kde.kirigami as Kirigami

PlasmoidItem {
    id: root

    property var line: ({ state: "down" })
    property real fetchedAt: Date.now()
    property real now: Date.now()

    readonly property bool off: line.state === "off" || line.state === "down"
    readonly property bool working: line.state === "working" || line.state === "idle_pending"
    readonly property bool onBreak: line.state === "break_auto" || line.state === "break_manual"
    readonly property real worked: (line.worked_ms || 0) + (working ? now - fetchedAt : 0)
    readonly property real breaks: (line.break_ms || 0) + (onBreak ? now - fetchedAt : 0)
    readonly property real since: (line.since_ms || 0) + (now - fetchedAt)
    readonly property string what: line.task || line.project || ""
    readonly property color stateColor: ({
        working: "#27a644", idle_pending: "#f2c94c", break_auto: "#4ea7fc", break_manual: "#4ea7fc"
    })[line.state] || "#8a8f98"
    readonly property string stateLabel: ({
        off: "Not clocked in", working: "Working", idle_pending: "Idle", break_auto: "On break", break_manual: "On break"
    })[line.state] || "Gwen isn't running"

    preferredRepresentation: compactRepresentation
    Plasmoid.backgroundHints: PlasmaCore.Types.NoBackground | PlasmaCore.Types.ConfigurableBackground
    toolTipMainText: off ? stateLabel : onBreak ? "On break · " + duration(since) : "Worked " + duration(worked) + " today"
    toolTipSubText: off ? "Click for the day at a glance"
        : [line.project + (line.task ? " — " + line.task : ""),
           line.target_ms > 0 ? Math.round(worked * 100 / line.target_ms) + "% of " + duration(line.target_ms) : ""].filter(s => s).join("\n")

    // "4:03:12" for a ticking timer.
    function clock(ms) {
        const s = Math.max(0, Math.floor(ms / 1000));
        const pad = n => String(n).padStart(2, "0");
        return Math.floor(s / 3600) + ":" + pad(Math.floor(s % 3600 / 60)) + ":" + pad(s % 60);
    }
    // "4:03 pm" of an instant, in the local zone (IST).
    function clock12(ms) {
        const d = new Date(ms);
        const h = d.getHours();
        return (h % 12 || 12) + ":" + String(d.getMinutes()).padStart(2, "0") + (h < 12 ? " am" : " pm");
    }
    // "7h 32m", "45m".
    function duration(ms) {
        const m = Math.max(0, Math.floor(ms / 60000));
        const h = Math.floor(m / 60);
        return h === 0 ? m + "m" : m % 60 === 0 ? h + "h" : h + "h " + (m % 60) + "m";
    }

    function refresh() { exec.connectSource("gwen panel"); }
    // Runs a command, then shows what it changed.
    function run(command) { exec.connectSource(command); }
    // Opens the dashboard, on a screen such as "inbox", or on Today for "".
    function openDashboard(screen) {
        root.expanded = false;
        exec.connectSource(screen ? "gwen-ui --screen " + screen : "gwen-ui");
    }
    // Tracks a project, "" for none, without a task: switches to it, or
    // clocks in on it when off the clock.
    function switchProject(id) {
        const flag = " --project " + (id || "none");
        run(line.state === "off" ? "gwen in" + flag : "gwen switch" + flag);
    }
    // Starts the plan's next task: clocks in on it, or switches to it.
    function startNext() {
        const n = line.next;
        if (!n) return;
        const flags = " --project " + n.project_id + " --task " + n.task_id;
        run(off ? "gwen in" + flags : onBreak ? "gwen switch" + flags + " && gwen back" : "gwen switch" + flags);
    }

    P5Support.DataSource {
        id: exec
        engine: "executable"
        connectedSources: []
        onNewData: (source, data) => {
            disconnectSource(source);
            if (source !== "gwen panel") {
                root.refresh();
                return;
            }
            try {
                root.line = JSON.parse(data["stdout"]);
            } catch (e) {
                root.line = { state: "down" };
            }
            root.fetchedAt = Date.now();
            root.now = root.fetchedAt;
        }
    }

    Timer {
        interval: 3000; running: true; repeat: true; triggeredOnStart: true
        onTriggered: root.refresh()
    }
    Timer {
        interval: 1000; running: true; repeat: true
        onTriggered: root.now = Date.now()
    }

    Plasmoid.contextualActions: [
        PlasmaCore.Action {
            text: "Clock in"
            icon.name: "media-playback-start"
            visible: root.line.state === "off"
            onTriggered: root.run("gwen in")
        },
        PlasmaCore.Action {
            text: "Start break"
            icon.name: "media-playback-pause"
            visible: root.working
            onTriggered: root.run("gwen break")
        },
        PlasmaCore.Action {
            text: "End break"
            icon.name: "media-playback-start"
            visible: root.onBreak
            onTriggered: root.run("gwen back")
        },
        PlasmaCore.Action {
            text: "Snooze nudges"
            icon.name: "notifications-disabled"
            visible: !root.off
            onTriggered: root.run("gwen snooze")
        },
        PlasmaCore.Action {
            text: "Clock out"
            icon.name: "media-playback-stop"
            visible: !root.off
            onTriggered: root.run("gwen out")
        },
        PlasmaCore.Action {
            isSeparator: true
        },
        PlasmaCore.Action {
            text: "Ask the assistant…"
            icon.name: "dialog-messages"
            onTriggered: root.openDashboard("assistant")
        },
        PlasmaCore.Action {
            text: "Capture to inbox…"
            icon.name: "mail-folder-inbox"
            onTriggered: root.openDashboard("inbox")
        },
        PlasmaCore.Action {
            text: "Task board"
            icon.name: "view-list-details"
            onTriggered: root.openDashboard("board")
        },
        PlasmaCore.Action {
            text: "Open dashboard"
            icon.name: "window-new"
            onTriggered: root.openDashboard("")
        }
    ]

    compactRepresentation: MouseArea {
        Layout.minimumWidth: row.implicitWidth + Kirigami.Units.smallSpacing * 2
        Layout.preferredWidth: Layout.minimumWidth
        hoverEnabled: true
        cursorShape: Qt.PointingHandCursor
        onClicked: root.expanded = !root.expanded

        RowLayout {
            id: row
            anchors.centerIn: parent
            spacing: Kirigami.Units.smallSpacing

            Rectangle {
                width: Kirigami.Units.smallSpacing * 2; height: width; radius: width / 2
                color: root.stateColor
            }
            PlasmaComponents.Label {
                text: root.line.state === "down" ? "Gwen off"
                    : root.line.state === "off" ? "Not clocked in"
                    : root.onBreak ? "Break " + root.duration(root.since)
                    : root.clock(root.worked)
                font.bold: root.working
                font.features: ({ "tnum": 1 })
            }
            PlasmaComponents.Label {
                visible: text !== "" && !root.off
                text: root.what
                opacity: 0.75
                elide: Text.ElideRight
                Layout.maximumWidth: Kirigami.Units.gridUnit * 10
            }
        }
    }

    // A fixed size: Plasma would otherwise stretch the popup around the card.
    fullRepresentation: NowCard {
        gwen: root
        Layout.minimumWidth: implicitWidth
        Layout.preferredWidth: implicitWidth
        Layout.maximumWidth: implicitWidth
        Layout.minimumHeight: implicitHeight
        Layout.preferredHeight: implicitHeight
        Layout.maximumHeight: implicitHeight
    }
}
