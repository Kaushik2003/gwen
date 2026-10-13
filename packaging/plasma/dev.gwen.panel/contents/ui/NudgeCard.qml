// The nudge card: Gwen's reminder, risen from the panel widget in place of a
// desktop notification while the widget runs (internal/notify/card.go). Her
// face in the nudge's mood, her line, a live strip that counts down or up,
// and the nudge's buttons. Colours and type follow ui/DESIGN.md, as the now
// card does.
import QtQuick
import QtQuick.Layouts
import org.kde.plasma.components as PlasmaComponents
import org.kde.kirigami as Kirigami

Item {
    id: card

    // The PlasmoidItem: line, now, since, worked, face(), stateColor,
    // actOnNudge(), dismissNudge(), talk().
    required property var gwen
    readonly property var nudge: gwen.nudge || ({ kind: "", title: "", markup: "", body: "", mood: "", actions: [], at: 0 })
    readonly property var next: gwen.line.next || null

    readonly property string family: inter.status === FontLoader.Ready ? inter.font.family : ""
    FontLoader {
        id: inter
        source: Qt.resolvedUrl("../fonts/InterVariable.ttf")
    }
    readonly property color ink: "#f7f8f8"
    readonly property color muted: "#d0d6e0"
    readonly property color subtle: "#8a8f98"
    readonly property color faint: "#62666d"
    readonly property color accent: "#5e6ad2"
    readonly property color tint: gwen.stateColor

    // What the live strip shows: a countdown while idle, the break ticking
    // up on a break, the next block when clocking in.
    readonly property string strip: nudge.kind === "idle" && nudge.until_at ? "countdown"
        : (nudge.kind === "break_long" || nudge.kind === "break_active") ? "break"
        : nudge.kind === "clock_in" && next ? "next"
        : ""
    readonly property real remaining: nudge.until_at ? Math.max(0, nudge.until_at - gwen.now) : 0

    implicitWidth: 384
    implicitHeight: panel.implicitHeight

    // Rises and fades in each time a new nudge arrives.
    function enter() {
        rise.restart();
        spring.restart();
    }
    ParallelAnimation {
        id: rise
        NumberAnimation { target: panel; property: "opacity"; from: 0; to: 1; duration: 200; easing.type: Easing.OutCubic }
        NumberAnimation { target: lift; property: "y"; from: 14; to: 0; duration: 320; easing.type: Easing.OutBack }
    }

    // "4:32" of a duration; "1:04:32" past an hour.
    function clock(ms) {
        const s = Math.max(0, Math.floor(ms / 1000));
        const pad = n => String(n).padStart(2, "0");
        const h = Math.floor(s / 3600);
        return (h > 0 ? h + ":" + pad(Math.floor(s % 3600 / 60)) : Math.floor(s / 60)) + ":" + pad(s % 60);
    }

    component Caption: PlasmaComponents.Label {
        font.family: card.family
        font.pixelSize: 10
        font.weight: Font.Medium
        font.letterSpacing: 0.6
        font.capitalization: Font.AllUppercase
        color: card.faint
    }
    component NudgeButton: Rectangle {
        id: button
        property string text
        property bool primary: false
        signal clicked
        implicitHeight: 32
        implicitWidth: label.implicitWidth + 28
        radius: 8
        color: primary ? (area.pressed ? "#5e69d1" : area.containsMouse ? "#828fff" : card.accent) : (area.containsMouse ? "#18191a" : "#141516")
        border.color: primary ? "transparent" : (area.containsMouse ? "#3e3e44" : "#34343a")
        Behavior on color { ColorAnimation { duration: 120 } }
        PlasmaComponents.Label {
            id: label
            anchors.centerIn: parent
            font.family: card.family
            text: button.text
            color: button.primary ? "#ffffff" : card.ink
            font.pixelSize: 13
            font.weight: Font.Medium
        }
        MouseArea {
            id: area
            anchors.fill: parent
            hoverEnabled: true
            cursorShape: Qt.PointingHandCursor
            onClicked: button.clicked()
        }
    }

    Item {
        id: lift
        width: card.width
        height: panel.implicitHeight

        Rectangle {
            id: panel
            width: parent.width
            implicitHeight: body.implicitHeight + 32
            radius: 16
            color: "#0f1011"
            border.color: "#23252a"
            // The faint top-edge highlight of a lifted panel.
            Rectangle {
                anchors.top: parent.top
                anchors.topMargin: 1
                anchors.horizontalCenter: parent.horizontalCenter
                width: parent.width - 2 * parent.radius
                height: 1
                color: Qt.rgba(1, 1, 1, 0.07)
            }

            ColumnLayout {
                id: body
                anchors.fill: parent
                anchors.margins: 16
                spacing: 14

                RowLayout {
                    spacing: 14
                    Layout.fillWidth: true

                    // Gwen, in the nudge's mood: she springs in, breathes, and
                    // a click talks to her.
                    Item {
                        id: face
                        Layout.alignment: Qt.AlignTop
                        implicitWidth: 64
                        implicitHeight: 64
                        transformOrigin: Item.Bottom
                        scale: faceArea.containsMouse ? 1.08 : 1
                        Behavior on scale { NumberAnimation { duration: 160; easing.type: Easing.OutBack } }
                        Image {
                            anchors.fill: parent
                            source: card.gwen.face(card.nudge.mood || card.gwen.mood)
                            sourceSize: Qt.size(128, 128)
                            fillMode: Image.PreserveAspectFit
                            smooth: true
                            mipmap: true
                            transformOrigin: Item.Bottom
                            SequentialAnimation on scale {
                                loops: Animation.Infinite
                                running: card.visible
                                NumberAnimation { to: 1.04; duration: 1900; easing.type: Easing.InOutSine }
                                NumberAnimation { to: 1; duration: 1900; easing.type: Easing.InOutSine }
                            }
                        }
                        SequentialAnimation {
                            id: spring
                            NumberAnimation { target: face; property: "rotation"; from: -12; to: 5; duration: 240; easing.type: Easing.OutQuad }
                            NumberAnimation { target: face; property: "rotation"; to: 0; duration: 300; easing.type: Easing.OutBack }
                        }
                        MouseArea {
                            id: faceArea
                            anchors.fill: parent
                            hoverEnabled: true
                            cursorShape: Qt.PointingHandCursor
                            onClicked: card.gwen.talk()
                        }
                        PlasmaComponents.ToolTip { text: "Talk to Gwen"; visible: faceArea.containsMouse }
                    }

                    ColumnLayout {
                        Layout.fillWidth: true
                        Layout.alignment: Qt.AlignTop
                        spacing: 4
                        RowLayout {
                            Layout.fillWidth: true
                            spacing: 6
                            Rectangle {
                                implicitWidth: 6
                                implicitHeight: 6
                                radius: 3
                                color: card.tint
                            }
                            Caption {
                                Layout.fillWidth: true
                                text: "Gwen · " + card.gwen.clock12(card.nudge.at)
                            }
                            // Close: hides the card; the nudge itself stays until
                            // what it is about changes.
                            Rectangle {
                                implicitWidth: 22
                                implicitHeight: 22
                                radius: 6
                                color: closeArea.containsMouse ? "#18191a" : "transparent"
                                Kirigami.Icon {
                                    anchors.centerIn: parent
                                    source: "window-close-symbolic"
                                    color: closeArea.containsMouse ? card.ink : card.subtle
                                    isMask: true
                                    implicitWidth: 14
                                    implicitHeight: 14
                                }
                                MouseArea {
                                    id: closeArea
                                    anchors.fill: parent
                                    hoverEnabled: true
                                    cursorShape: Qt.PointingHandCursor
                                    onClicked: card.gwen.dismissNudge()
                                }
                            }
                        }
                        PlasmaComponents.Label {
                            Layout.fillWidth: true
                            font.family: card.family
                            text: card.nudge.title
                            textFormat: Text.PlainText
                            color: card.ink
                            font.pixelSize: 16
                            font.weight: Font.DemiBold
                            font.letterSpacing: -0.2
                            wrapMode: Text.Wrap
                        }
                        PlasmaComponents.Label {
                            Layout.fillWidth: true
                            visible: text !== ""
                            font.family: card.family
                            text: card.nudge.markup || card.nudge.body
                            textFormat: card.nudge.markup ? Text.StyledText : Text.PlainText
                            color: card.muted
                            font.pixelSize: 13
                            lineHeight: 1.35
                            wrapMode: Text.Wrap
                        }
                    }
                }

                // The live strip.
                Rectangle {
                    Layout.fillWidth: true
                    visible: card.strip !== ""
                    implicitHeight: card.strip === "next" ? 38 : 52
                    radius: 10
                    color: "#141516"
                    border.color: "#23252a"
                    ColumnLayout {
                        anchors.fill: parent
                        anchors.leftMargin: 12
                        anchors.rightMargin: 12
                        anchors.topMargin: 9
                        anchors.bottomMargin: 10
                        spacing: 7
                        RowLayout {
                            Layout.fillWidth: true
                            Caption {
                                Layout.fillWidth: true
                                elide: Text.ElideRight
                                text: card.strip === "countdown" ? (card.remaining > 0 ? "Counts as a break in" : "Counted as a break")
                                    : card.strip === "break" ? "On break"
                                    : card.next ? "Up next · " + card.gwen.clock12(card.next.start_at) : ""
                            }
                            PlasmaComponents.Label {
                                font.family: card.family
                                font.features: ({ "tnum": 1 })
                                font.pixelSize: 13
                                font.weight: Font.DemiBold
                                color: card.ink
                                elide: Text.ElideRight
                                Layout.maximumWidth: 220
                                text: card.strip === "countdown" ? card.clock(card.remaining)
                                    : card.strip === "break" ? card.clock(card.nudge.started_at ? card.gwen.now - card.nudge.started_at : card.gwen.since)
                                    : card.next ? card.next.title : ""
                            }
                        }
                        // Countdown: the time left before idle becomes a break.
                        // Break: today's work against the target.
                        Rectangle {
                            Layout.fillWidth: true
                            visible: card.strip !== "next"
                            implicitHeight: 4
                            radius: 2
                            color: "#23252a"
                            Rectangle {
                                readonly property real span: card.nudge.until_at ? card.nudge.until_at - card.nudge.at : 1
                                readonly property real fraction: card.strip === "countdown" ? card.remaining / Math.max(1, span)
                                    : card.gwen.line.target_ms > 0 ? Math.min(1, card.gwen.worked / card.gwen.line.target_ms) : 0
                                width: parent.width * fraction
                                height: parent.height
                                radius: 2
                                color: card.strip === "countdown" ? card.tint : "#27a644"
                                Behavior on width { NumberAnimation { duration: 900; easing.type: Easing.Linear } }
                            }
                        }
                    }
                }

                // The nudge's buttons: the first is the one she hopes for.
                RowLayout {
                    Layout.fillWidth: true
                    visible: card.nudge.actions.length > 0
                    spacing: 8
                    Repeater {
                        model: card.nudge.actions
                        NudgeButton {
                            required property var modelData
                            required property int index
                            Layout.fillWidth: true
                            Layout.preferredWidth: 1 // equal widths
                            text: modelData.label
                            primary: index === 0
                            onClicked: card.gwen.actOnNudge(modelData.id)
                        }
                    }
                }
            }

            // Hovering keeps a passing nudge up.
            HoverHandler { id: hover }
        }
    }

    readonly property bool hovered: hover.hovered
}
