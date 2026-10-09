// The now card: the day at a glance as a small grid of tiles, dropped down
// from the panel widget by Plasma, as its popup. Colours follow ui/DESIGN.md.
import QtQuick
import QtQuick.Layouts
import QtQuick.Shapes
import org.kde.plasma.components as PlasmaComponents
import org.kde.plasma.extras as PlasmaExtras
import org.kde.kirigami as Kirigami

Rectangle {
    id: card

    // The PlasmoidItem: line, now, worked, since, the state flags, run().
    required property var gwen

    readonly property int pad: 10
    readonly property int gap: 8
    readonly property real cell: (implicitWidth - pad * 2 - gap * 2) / 3
    readonly property var line: gwen.line
    readonly property bool off: line.state === "off"
    readonly property bool down: line.state === "down"
    readonly property var next: line.next || null

    implicitWidth: 420
    implicitHeight: (down ? downTile.implicitHeight : grid.implicitHeight) + pad * 2
    radius: 18
    color: "#010102"
    border.color: "#23252a"

    // Inter, as in the dashboard, whatever the system font is.
    readonly property string family: inter.status === FontLoader.Ready ? inter.font.family : ""
    FontLoader {
        id: inter
        source: Qt.resolvedUrl("../fonts/InterVariable.ttf")
    }

    readonly property color ink: "#f7f8f8"
    readonly property color subtle: "#8a8f98"
    readonly property color faint: "#62666d"
    readonly property color accent: "#5e6ad2"

    // The energy check-in levels, 1 first, and their colours.
    readonly property var energyLevels: ["Drained", "Low", "Okay", "Good", "Peak"]
    readonly property var energyColors: ["#eb5757", "#f2994a", "#8a8f98", "#4cb782", "#27a644"]

    // Switch project: Unassigned, then the live projects, the tracked one ticked.
    PlasmaExtras.Menu {
        id: projectMenu
        visualParent: projectRow
    }
    Component {
        id: projectItem
        PlasmaExtras.MenuItem {}
    }
    function openProjects() {
        projectMenu.clearMenuItems();
        const add = (id, name) => {
            const item = projectItem.createObject(projectMenu, { text: name, checkable: true, checked: (line.project_id || "") === id });
            item.clicked.connect(() => gwen.switchProject(id));
            projectMenu.addMenuItem(item);
        };
        add("", "Unassigned");
        for (const p of line.projects || []) add(p.id, p.name);
        projectMenu.open(0, projectRow.height + 4); // under the project name
    }

    component Tile: Rectangle {
        radius: 14
        color: "#0f1011"
        border.color: "#23252a"
    }
    component Caption: PlasmaComponents.Label {
        font.family: card.family
        font.pixelSize: 10
        font.weight: Font.Medium
        font.letterSpacing: 0.6
        font.capitalization: Font.AllUppercase
        color: card.faint
        elide: Text.ElideRight
    }
    component CardButton: Rectangle {
        id: button
        property string text
        property string iconName
        property color iconColor: card.ink
        property bool primary: false
        signal clicked
        implicitHeight: 36
        radius: 10
        color: primary ? (area.containsMouse ? "#828fff" : card.accent) : (area.containsMouse ? "#18191a" : "#141516")
        border.color: primary ? "transparent" : "#34343a"
        RowLayout {
            anchors.centerIn: parent
            spacing: 6
            Kirigami.Icon {
                source: button.iconName
                color: button.primary ? "#ffffff" : button.iconColor
                isMask: true
                implicitWidth: 16
                implicitHeight: 16
            }
            PlasmaComponents.Label {
                font.family: card.family
                text: button.text
                color: button.primary ? "#ffffff" : card.ink
                font.pixelSize: 13
                font.weight: Font.Medium
            }
        }
        MouseArea {
            id: area
            anchors.fill: parent
            hoverEnabled: true
            cursorShape: Qt.PointingHandCursor
            onClicked: button.clicked()
        }
    }

    // Gwen is not running: one tile that says so and starts it.
    Tile {
        id: downTile
        visible: card.down
        anchors.fill: parent
        anchors.margins: card.pad
        implicitHeight: 220
        ColumnLayout {
            anchors.centerIn: parent
            spacing: 10
            Image {
                Layout.alignment: Qt.AlignHCenter
                Layout.preferredWidth: 80
                Layout.preferredHeight: 80
                source: card.gwen.face(card.gwen.mood)
                sourceSize: Qt.size(160, 160)
                fillMode: Image.PreserveAspectFit
                smooth: true
                mipmap: true
            }
            PlasmaComponents.Label {
                font.family: card.family
                Layout.alignment: Qt.AlignHCenter
                text: "Gwen isn't running"
                color: card.ink
                font.pixelSize: 15
                font.weight: Font.Medium
            }
            CardButton {
                Layout.alignment: Qt.AlignHCenter
                implicitWidth: 140
                text: "Start Gwen"
                iconName: "media-playback-start"
                primary: true
                onClicked: card.gwen.run("systemctl --user start gwend.service")
            }
        }
    }

    GridLayout {
        id: grid
        visible: !card.down
        anchors.fill: parent
        anchors.margins: card.pad
        columns: 3
        columnSpacing: card.gap
        rowSpacing: card.gap

        // What is tracked, and the day's worked timer.
        Tile {
            Layout.row: 0
            Layout.column: 0
            Layout.columnSpan: 2
            Layout.rowSpan: 2
            Layout.preferredWidth: card.cell * 2 + card.gap
            Layout.preferredHeight: 64 * 2 + card.gap
            gradient: Gradient {
                orientation: Gradient.Horizontal
                GradientStop { position: 0; color: card.off ? "#0f1011" : Qt.tint("#0f1011", Qt.alpha(card.gwen.stateColor, 0.14)) }
                GradientStop { position: 0.75; color: "#0f1011" }
            }
            // Gwen in the corner, her face following the state: she breathes,
            // springs when her mood changes, and a click talks to her. The
            // task and project stop short of her.
            Item {
                id: face
                anchors.right: parent.right
                anchors.bottom: parent.bottom
                anchors.rightMargin: 6
                anchors.bottomMargin: 4
                width: 86
                height: 86
                scale: faceArea.containsMouse ? 1.08 : 1
                transformOrigin: Item.Bottom
                Behavior on scale { NumberAnimation { duration: 160; easing.type: Easing.OutBack } }
                Image {
                    id: faceImage
                    anchors.fill: parent
                    source: card.gwen.face(card.gwen.mood)
                    sourceSize: Qt.size(width * 2, height * 2)
                    fillMode: Image.PreserveAspectFit
                    smooth: true
                    mipmap: true
                    transformOrigin: Item.Bottom
                    onSourceChanged: spring.restart()
                    SequentialAnimation on scale {
                        loops: Animation.Infinite
                        running: card.gwen.expanded
                        NumberAnimation { to: 1.035; duration: 2100; easing.type: Easing.InOutSine }
                        NumberAnimation { to: 1; duration: 2100; easing.type: Easing.InOutSine }
                    }
                    SequentialAnimation on rotation {
                        loops: Animation.Infinite
                        running: card.gwen.expanded
                        NumberAnimation { to: -1.5; duration: 2600; easing.type: Easing.InOutSine }
                        NumberAnimation { to: 1.2; duration: 2600; easing.type: Easing.InOutSine }
                    }
                }
                SequentialAnimation {
                    id: spring
                    NumberAnimation { target: face; property: "rotation"; from: -10; to: 4; duration: 260; easing.type: Easing.OutQuad }
                    NumberAnimation { target: face; property: "rotation"; to: 0; duration: 280; easing.type: Easing.OutBack }
                }
                // A mic on hover says what the click does.
                Rectangle {
                    anchors.left: parent.left
                    anchors.bottom: parent.bottom
                    anchors.bottomMargin: 6
                    width: 24
                    height: 24
                    radius: 12
                    color: card.accent
                    border.color: "#010102"
                    border.width: 2
                    opacity: faceArea.containsMouse ? 1 : 0
                    Behavior on opacity { NumberAnimation { duration: 120 } }
                    Kirigami.Icon {
                        anchors.centerIn: parent
                        source: "audio-input-microphone"
                        color: "#ffffff"
                        isMask: true
                        implicitWidth: 13
                        implicitHeight: 13
                    }
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
                anchors.fill: parent
                anchors.margins: 12
                spacing: 0
                RowLayout {
                    spacing: 6
                    Rectangle {
                        implicitWidth: 7
                        implicitHeight: 7
                        radius: 3.5
                        color: card.gwen.stateColor
                    }
                    PlasmaComponents.Label {
                        font.family: card.family
                        text: card.gwen.stateLabel
                        color: card.gwen.stateColor
                        font.pixelSize: 12
                        font.weight: Font.Medium
                    }
                    PlasmaComponents.Label {
                        font.family: card.family
                        visible: !card.off
                        text: "since " + card.gwen.clock12(card.gwen.now - card.gwen.since)
                        color: card.subtle
                        font.pixelSize: 12
                    }
                }
                Row {
                    Layout.topMargin: 4
                    visible: !card.off
                    PlasmaComponents.Label {
                        font.family: card.family
                        text: card.gwen.clock(card.gwen.worked).slice(0, -3)
                        color: card.gwen.onBreak ? card.subtle : card.ink
                        font.pixelSize: 44
                        font.weight: Font.DemiBold
                        font.variableAxes: ({ "wght": 600 })
                        font.letterSpacing: -1.5
                        font.features: ({ "tnum": 1 })
                    }
                    PlasmaComponents.Label {
                        font.family: card.family
                        text: card.gwen.clock(card.gwen.worked).slice(-3)
                        color: card.faint
                        font.pixelSize: 44
                        font.weight: Font.DemiBold
                        font.variableAxes: ({ "wght": 600 })
                        font.letterSpacing: -1.5
                        font.features: ({ "tnum": 1 })
                    }
                }
                PlasmaComponents.Label {
                    font.family: card.family
                    Layout.topMargin: 6
                    visible: card.off
                    text: "Off the clock"
                    color: card.subtle
                    font.pixelSize: 28
                    font.weight: Font.DemiBold
                }
                Item { Layout.fillHeight: true }
                PlasmaComponents.Label {
                    font.family: card.family
                    Layout.fillWidth: true
                    Layout.rightMargin: face.width + 4
                    text: card.line.task || (card.off ? "Nothing tracked right now" : "No task")
                    color: card.ink
                    font.pixelSize: 14
                    font.weight: Font.Medium
                    elide: Text.ElideRight
                }
                RowLayout {
                    id: projectRow
                    spacing: 6
                    Rectangle {
                        implicitWidth: 6
                        implicitHeight: 6
                        radius: 3
                        color: card.off ? card.faint : (card.line.color || card.subtle)
                    }
                    PlasmaComponents.Label {
                        font.family: card.family
                        Layout.maximumWidth: card.cell * 2 - 60 - face.width
                        text: card.off ? "Clock in on a project" : (card.line.project || "")
                        color: projectHover.hovered ? card.ink : card.subtle
                        font.pixelSize: 12
                        elide: Text.ElideRight
                    }
                    Kirigami.Icon {
                        source: "arrow-down"
                        color: projectHover.hovered ? card.ink : card.subtle
                        isMask: true
                        implicitWidth: 12
                        implicitHeight: 12
                    }
                    // Handlers, not items: the whole row is the button.
                    HoverHandler {
                        id: projectHover
                        cursorShape: Qt.PointingHandCursor
                    }
                    TapHandler {
                        onTapped: card.openProjects()
                    }
                }
            }
        }

        // The time in IST, and the date.
        Tile {
            Layout.row: 0
            Layout.column: 2
            Layout.preferredWidth: card.cell
            Layout.preferredHeight: 64
            ColumnLayout {
                anchors.fill: parent
                anchors.margins: 12
                spacing: 0
                Caption {
                    Layout.fillWidth: true
                    text: Qt.formatDate(new Date(card.gwen.now), "ddd d MMM") + " · IST"
                }
                Item { Layout.fillHeight: true }
                PlasmaComponents.Label {
                    font.family: card.family
                    text: card.gwen.clock12(card.gwen.now)
                    color: card.ink
                    font.pixelSize: 21
                    font.weight: Font.DemiBold
                    font.features: ({ "tnum": 1 })
                }
            }
        }

        // Progress to the daily target.
        Tile {
            id: target
            Layout.row: 1
            Layout.column: 2
            Layout.preferredWidth: card.cell
            Layout.preferredHeight: 64
            readonly property real fraction: card.line.target_ms > 0 ? Math.min(1, card.gwen.worked / card.line.target_ms) : 0
            readonly property bool met: card.line.target_ms > 0 && card.gwen.worked >= card.line.target_ms
            RowLayout {
                anchors.fill: parent
                anchors.margins: 10
                spacing: 8
                Item {
                    implicitWidth: 42
                    implicitHeight: 42
                    Shape {
                        anchors.fill: parent
                        preferredRendererType: Shape.CurveRenderer
                        ShapePath {
                            strokeColor: "#23252a"
                            strokeWidth: 4.5
                            fillColor: "transparent"
                            PathAngleArc { centerX: 21; centerY: 21; radiusX: 18.75; radiusY: 18.75; startAngle: 0; sweepAngle: 360 }
                        }
                        ShapePath {
                            strokeColor: target.met ? "#27a644" : card.accent
                            strokeWidth: 4.5
                            fillColor: "transparent"
                            capStyle: ShapePath.RoundCap
                            PathAngleArc { centerX: 21; centerY: 21; radiusX: 18.75; radiusY: 18.75; startAngle: -90; sweepAngle: 360 * target.fraction }
                        }
                    }
                    PlasmaComponents.Label {
                        font.family: card.family
                        anchors.centerIn: parent
                        text: Math.round(target.fraction * 100) + "%"
                        color: card.ink
                        font.pixelSize: 11
                        font.weight: Font.DemiBold
                    }
                }
                ColumnLayout {
                    spacing: 0
                    Caption { text: "Target" }
                    PlasmaComponents.Label {
                        font.family: card.family
                        text: target.met ? "Met" : card.gwen.duration(card.line.target_ms - card.gwen.worked)
                        color: card.ink
                        font.pixelSize: 13
                        font.weight: Font.DemiBold
                    }
                    PlasmaComponents.Label {
                        font.family: card.family
                        visible: !target.met
                        text: "left of " + card.gwen.duration(card.line.target_ms)
                        color: card.faint
                        font.pixelSize: 11
                    }
                }
            }
        }

        // How long this stretch of work, or this break, has lasted.
        Tile {
            Layout.row: 2
            Layout.column: 0
            Layout.preferredWidth: card.cell
            Layout.preferredHeight: 76
            ColumnLayout {
                anchors.fill: parent
                anchors.margins: 12
                spacing: 0
                Caption { text: card.gwen.onBreak ? "This break" : "This stretch" }
                Item { Layout.fillHeight: true }
                PlasmaComponents.Label {
                    font.family: card.family
                    text: card.off ? "—" : card.gwen.duration(card.gwen.since)
                    color: card.ink
                    font.pixelSize: 17
                    font.weight: Font.DemiBold
                }
                PlasmaComponents.Label {
                    font.family: card.family
                    text: "Breaks " + card.gwen.duration(card.gwen.breaks)
                    color: card.faint
                    font.pixelSize: 11
                }
            }
        }

        // The plan block on now, or the next one.
        Tile {
            Layout.row: 2
            Layout.column: 1
            Layout.columnSpan: 2
            Layout.preferredWidth: card.cell * 2 + card.gap
            Layout.preferredHeight: 76
            ColumnLayout {
                anchors.fill: parent
                anchors.margins: 12
                spacing: 0
                RowLayout {
                    Caption {
                        Layout.fillWidth: true
                        text: card.next && card.next.now ? "On the plan now" : "Next up"
                    }
                    Caption {
                        visible: card.line.more > 0
                        text: card.line.more + " more today"
                        font.capitalization: Font.MixedCase
                        font.letterSpacing: 0
                    }
                }
                Item { Layout.fillHeight: true }
                RowLayout {
                    visible: card.next !== null
                    spacing: 8
                    PlasmaComponents.Label {
                        font.family: card.family
                        Layout.fillWidth: true
                        text: card.next ? card.next.title : ""
                        color: card.ink
                        font.pixelSize: 14
                        font.weight: Font.Medium
                        elide: Text.ElideRight
                    }
                    PlasmaComponents.Label {
                        font.family: card.family
                        text: !card.next ? "" : card.next.now ? "until " + card.gwen.clock12(card.next.start_at + card.next.minutes * 60000)
                            : card.gwen.clock12(card.next.start_at) + " · " + card.gwen.duration(card.next.minutes * 60000)
                        color: card.subtle
                        font.pixelSize: 12
                    }
                    CardButton {
                        visible: card.next !== null && !(card.gwen.working && card.line.task === card.next.title)
                        implicitWidth: 30
                        implicitHeight: 30
                        iconName: "media-playback-start"
                        onClicked: card.gwen.startNext()
                    }
                }
                PlasmaComponents.Label {
                    font.family: card.family
                    visible: card.next === null
                    text: "Nothing else planned today."
                    color: card.subtle
                    font.pixelSize: 13
                }
            }
        }

        // How your energy is now, for biological prime time.
        Tile {
            id: energyTile
            Layout.row: 3
            Layout.column: 0
            Layout.columnSpan: 3
            Layout.fillWidth: true
            Layout.preferredHeight: 62
            readonly property var last: card.line.energy || null
            ColumnLayout {
                anchors.fill: parent
                anchors.leftMargin: 12
                anchors.rightMargin: 12
                anchors.topMargin: 9
                anchors.bottomMargin: 9
                spacing: 7
                RowLayout {
                    Caption {
                        Layout.fillWidth: true
                        text: "Energy now"
                    }
                    Caption {
                        text: energyTile.last ? card.energyLevels[energyTile.last.level - 1] + " at " + card.gwen.clock12(energyTile.last.at) : "Not logged today"
                        font.capitalization: Font.MixedCase
                        font.letterSpacing: 0
                    }
                }
                RowLayout {
                    spacing: 6
                    Repeater {
                        model: 5
                        Rectangle {
                            id: chip
                            required property int index
                            readonly property bool logged: energyTile.last !== null && energyTile.last.level === index + 1
                            Layout.fillWidth: true
                            implicitHeight: 24
                            radius: 8
                            color: logged ? Qt.tint("#141516", Qt.alpha(card.energyColors[index], 0.22)) : chipArea.containsMouse ? "#18191a" : "#141516"
                            border.color: logged ? card.energyColors[index] : "#34343a"
                            RowLayout {
                                anchors.centerIn: parent
                                spacing: 5
                                Rectangle {
                                    implicitWidth: 6
                                    implicitHeight: 6
                                    radius: 3
                                    color: card.energyColors[chip.index]
                                }
                                PlasmaComponents.Label {
                                    font.family: card.family
                                    text: card.energyLevels[chip.index]
                                    color: card.ink
                                    font.pixelSize: 11
                                    font.weight: Font.Medium
                                }
                            }
                            MouseArea {
                                id: chipArea
                                anchors.fill: parent
                                hoverEnabled: true
                                cursorShape: Qt.PointingHandCursor
                                onClicked: card.gwen.run("gwen energy " + (chip.index + 1))
                            }
                        }
                    }
                }
            }
        }

        // The everyday commands.
        RowLayout {
            Layout.row: 4
            Layout.column: 0
            Layout.columnSpan: 3
            Layout.fillWidth: true
            spacing: card.gap
            CardButton {
                visible: card.off
                Layout.fillWidth: true
                text: "Clock in"
                iconName: "media-playback-start"
                primary: true
                onClicked: card.gwen.run("gwen in")
            }
            CardButton {
                visible: card.gwen.onBreak
                Layout.fillWidth: true
                text: "End break"
                iconName: "media-playback-start"
                primary: true
                onClicked: card.gwen.run("gwen back")
            }
            CardButton {
                visible: card.gwen.working
                Layout.fillWidth: true
                text: "Break"
                iconName: "media-playback-pause"
                iconColor: "#4ea7fc"
                onClicked: card.gwen.run("gwen break")
            }
            CardButton {
                visible: !card.off
                Layout.fillWidth: true
                text: "Clock out"
                iconName: "media-playback-stop"
                iconColor: "#eb5757"
                onClicked: card.gwen.run("gwen out")
            }
            CardButton {
                Layout.fillWidth: true
                text: "Talk"
                iconName: "audio-input-microphone"
                iconColor: "#828fff"
                onClicked: card.gwen.talk()
            }
            CardButton {
                Layout.fillWidth: true
                text: "Dashboard"
                iconName: "window-new"
                onClicked: card.gwen.openDashboard("")
            }
        }
    }
}
