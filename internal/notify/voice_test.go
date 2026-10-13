package notify

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/require"
)

var noon = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

func moment(at time.Time) Moment {
	return Moment{At: at, SoftIdle: 10 * time.Minute, HardIdle: 15 * time.Minute, BreakReminder: 15 * time.Minute,
		Repeat: 10 * time.Minute, Worked: 3*time.Hour + 22*time.Minute, Target: 12 * time.Hour}
}

// moods says the moods Gwen shows for kind across a minute of firings.
func moods(kind string, m Moment) map[string]bool {
	seen := map[string]bool{}
	for range 60 {
		m.At = m.At.Add(time.Second)
		seen[Speak(Notification{Kind: kind}, m).Mood] = true
	}
	return seen
}

func TestSpeakBreakFirmsUp(t *testing.T) {
	t.Parallel()
	m := moment(noon)
	for _, tc := range []struct {
		took  time.Duration
		moods []string
	}{
		{15 * time.Minute, []string{"smiling", "happy"}},
		{25 * time.Minute, []string{"winking", "smug"}},
		{35 * time.Minute, []string{"annoyed", "grumpy"}},
		{75 * time.Minute, []string{"angry", "crying"}},
	} {
		m.BreakFor = tc.took
		require.ElementsMatch(t, tc.moods, keys(moods("break_long", m)), "after %s", tc.took)
	}
	m.Worked, m.BreakFor = 12*time.Hour, 75*time.Minute
	require.ElementsMatch(t, []string{"love", "happy"}, keys(moods("break_long", m)), "gentle once the target is met")
}

func TestSpeakIdle(t *testing.T) {
	t.Parallel()
	n := Speak(idleNudge, moment(noon))
	require.NotEqual(t, idleNudge.Title, n.Title)
	require.Contains(t, n.Markup, "<b>10m</b>")
	require.NotContains(t, n.Body, "<", "the plain body has no markup")
	require.Equal(t, []string{"I'm here", "Take a break", "Snooze 10m"}, []string{n.Actions[0].Label, n.Actions[1].Label, n.Actions[2].Label})
	require.Equal(t, "Snooze 10m", idleNudge.Actions[2].Label, "the caller's actions are not changed")
	require.Equal(t, n, Speak(idleNudge, moment(noon)), "the same moment says the same thing")

	late := moods("idle", moment(time.Date(2026, 10, 13, 23, 30, 0, 0, time.UTC)))
	require.ElementsMatch(t, []string{"thinking", "crying"}, keys(late), "late at night she's sleepy, not teasing")
}

func TestSpeakClockIn(t *testing.T) {
	t.Parallel()
	m := moment(time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC))
	m.Briefing = "2 planned today & <more>"
	n := Speak(Notification{Kind: "clock_in", Actions: []Action{{"clock_in", "Clock in"}}}, m)
	require.Contains(t, []string{"Good morning! ☀️", "Morning. Ready to own today?"}, n.Title)
	require.Contains(t, n.Markup, "2 planned today &amp; &lt;more&gt;", "the briefing is escaped")
	require.Equal(t, "2 planned today & <more>\nClock in and I'll start counting.", n.Body)
	require.Equal(t, "Let's go", n.Actions[0].Label)
}

func TestSpeakUnknownKindPassesThrough(t *testing.T) {
	t.Parallel()
	n := Notification{Kind: KindTest, Title: "t", Body: "b"}
	require.Equal(t, n, Speak(n, moment(noon)))
}

func TestEveryMoodHasAFace(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"idle", "break_long", "break_active", "clock_in"} {
		for _, at := range []time.Time{noon, noon.Add(-6 * time.Hour), noon.Add(7 * time.Hour), noon.Add(11 * time.Hour)} {
			for took := 15 * time.Minute; took < 90*time.Minute; took += 10 * time.Minute {
				m := moment(at)
				m.BreakFor = took
				for mood := range moods(kind, m) {
					_, ok := face(mood)
					require.True(t, ok, "%s has a face", mood)
				}
			}
		}
	}
	_, ok := face("nope")
	require.False(t, ok)
}

func TestDesktopShowsGwen(t *testing.T) {
	t.Parallel()
	bus := &fakeBus{caps: []string{"body", "body-markup", "sound"}, sigs: make(chan *dbus.Signal)}
	plays := 0
	d := newDesktop(bus, func(context.Context) error { plays++; return nil })
	n := Speak(idleNudge, moment(noon))
	require.NoError(t, d.Notify(context.Background(), n))
	c := bus.notifys[0]
	require.Equal(t, n.Markup, c.body)
	img := c.hints["image-data"].Value().(imageData)
	require.Equal(t, int32(4), img.Channels)
	require.Len(t, img.Data, int(img.Stride*img.Height))
	require.Equal(t, 1, plays, "Gwen's sound plays")
	require.NotContains(t, c.hints, "sound-name", "not Plasma's own sound as well")
	bus.dnd = true
	require.NoError(t, d.Notify(context.Background(), n))
	require.Equal(t, 1, plays, "quiet under Do Not Disturb")

	plainBus := &fakeBus{caps: []string{"body"}, sigs: make(chan *dbus.Signal)}
	require.NoError(t, newDesktop(plainBus, silent).Notify(context.Background(), n))
	require.Equal(t, n.Body, plainBus.notifys[0].body, "no markup where the server cannot render it")
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestPlain(t *testing.T) {
	t.Parallel()
	require.Equal(t, "a & b\nc", plain("<b>a</b> &amp; <i>b</i><br/>c"))
	require.False(t, strings.Contains(plain(escape("<x>")), "&"))
}
