package herdr_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/herdr/herdrtest"
	"github.com/stretchr/testify/require"
)

// next returns the stream's next frame.
func next(t *testing.T, s *herdr.Stream) herdr.Frame {
	t.Helper()
	f, err := s.Next(testCtx(t))
	require.NoError(t, err)
	return f
}

// readN reads exactly n bytes of output, returning credit for each frame
// when credit is set, as a relay does once it has passed the bytes on.
func readN(t *testing.T, s *herdr.Stream, n int, credit bool) []byte {
	t.Helper()
	var out []byte
	for len(out) < n {
		f := next(t, s)
		require.Nil(t, f.Closed, "the stream ended early")
		out = append(out, f.Data...)
		if credit {
			s.Credit(len(f.Data))
		}
	}
	require.Len(t, out, n, "a frame went past what was expected")
	return out
}

// lastFrame skips output to the stream's Closed frame and checks that
// nothing follows it.
func lastFrame(t *testing.T, s *herdr.Stream) herdr.Frame {
	t.Helper()
	for {
		f := next(t, s)
		if f.Closed != nil {
			_, err := s.Next(testCtx(t))
			require.ErrorIs(t, err, io.EOF)
			return f
		}
	}
}

func attach(t *testing.T, c *herdr.Client, p herdr.AttachParams) *herdr.Attached {
	t.Helper()
	a, err := c.Attach(testCtx(t), p)
	require.NoError(t, err)
	return a
}

// An observer that asks for resize notifications learns the grid before
// its first paint and again at every change, with a fresh paint after it;
// the phone draws at exactly the laptop's size from these.
func TestAttach(t *testing.T) {
	t.Parallel()
	core := herdrtest.New(t, herdrtest.Options{})
	t1 := core.AddTerminal(herdr.Terminal{Cols: 120, Rows: 40})
	core.SetScreen(t1.Address, "$ ls\nmain.go")
	c := core.Connect()

	view := attach(t, c, herdr.AttachParams{Terminal: t1.Address, Observe: true, ResizeNotifications: true, History: 1000, Cols: 120, Rows: 40})
	require.JSONEq(t, `{"terminal":"t1","observe":true,"resize_notifications":true,"history":1000,"cols":120,"rows":40}`,
		string(core.WaitCalls("terminal.attach", 1)[0].Params))
	require.True(t, view.Resizes)
	require.Equal(t, t1.Address, view.Terminal.Address)
	require.Equal(t, 120, view.Terminal.Cols)

	require.Equal(t, &herdr.Size{Cols: 120, Rows: 40}, next(t, view.Stream).Resize)
	require.Equal(t, herdrtest.Paint("$ ls\nmain.go"), next(t, view.Stream).Data)

	core.Output(t1.Address, []byte("hello\r\n"))
	require.Equal(t, []byte("hello\r\n"), next(t, view.Stream).Data)

	core.Resize(t1.Address, 100, 30)
	require.Equal(t, &herdr.Size{Cols: 100, Rows: 30}, next(t, view.Stream).Resize)
	require.Equal(t, herdrtest.Paint("$ ls\nmain.go"), next(t, view.Stream).Data)

	// Without the request there are no Resize frames: the first frame is
	// the paint.
	plain := attach(t, c, herdr.AttachParams{Terminal: t1.Address, Observe: true, Cols: 100, Rows: 30})
	require.False(t, plain.Resizes)
	require.Equal(t, herdrtest.Paint("$ ls\nmain.go"), next(t, plain.Stream).Data)

	_, err := c.Attach(testCtx(t), herdr.AttachParams{Terminal: "t9", Observe: true})
	require.True(t, herdr.IsKind(err, "not_found"), "%v", err)
}

// The core sends a stream only as much as its credit allows, and exactly
// as much more as the client returns. A stalled stream holds up neither
// calls nor other streams on the connection.
func TestStreamCredit(t *testing.T) {
	t.Parallel()
	const window = 1024
	core := herdrtest.New(t, herdrtest.Options{Window: window})
	t1 := core.AddTerminal(herdr.Terminal{})
	t2 := core.AddTerminal(herdr.Terminal{})
	c := core.Connect()
	ctx := testCtx(t)

	view := attach(t, c, herdr.AttachParams{Terminal: t1.Address, Observe: true, Cols: 80, Rows: 24})
	s := core.WaitStreams(t1.Address, 1)[0]
	paint := herdrtest.Paint("")
	out := bytes.Repeat([]byte("a"), 4096)
	core.Output(t1.Address, out)
	total := len(paint) + len(out)

	got := readN(t, view.Stream, window, false)
	require.Equal(t, append(paint, out[:window-len(paint)]...), got)
	s.WaitSent(window)
	require.Equal(t, window, s.Sent(), "nothing past the window before credit")
	require.Equal(t, total-window, s.Queued())

	_, err := c.List(ctx)
	require.NoError(t, err, "calls go ahead of a stalled stream")
	other := attach(t, c, herdr.AttachParams{Terminal: t2.Address, Observe: true, Cols: 80, Rows: 24})
	core.Output(t2.Address, []byte("other"))
	require.Equal(t, append(herdrtest.Paint(""), "other"...), readN(t, other.Stream, len(paint)+5, false))

	view.Credit(512)
	s.WaitCredited(512)
	s.WaitSent(window + 512)
	readN(t, view.Stream, 512, false)
	require.Equal(t, window+512, s.Sent(), "exactly the credit returned")

	view.Credit(total)
	readN(t, view.Stream, total-window-512, false)
	require.Zero(t, s.Queued())
	require.Equal(t, 512+total, s.Credited())

	view.Credit(0) // nothing to return sends nothing
	view.Credit(-5)
	_, err = c.List(ctx)
	require.NoError(t, err)
	require.Equal(t, 512+total, s.Credited())
}

// A viewer more than 4 MiB behind gets a fresh paint instead of the
// backlog (plan.md 3.14.6): a phone that stalls never makes gwend or the
// core hold megabytes for it, and it catches up at once.
func TestRepaintWhenFarBehind(t *testing.T) {
	t.Parallel()
	core := herdrtest.New(t, herdrtest.Options{}) // herdr-core's limits: 256 KiB of credit, 4 MiB behind
	t1 := core.AddTerminal(herdr.Terminal{})
	core.SetScreen(t1.Address, "SCREEN")
	paint := herdrtest.Paint("SCREEN")
	c := core.Connect()
	view := attach(t, c, herdr.AttachParams{Terminal: t1.Address, Observe: true, Cols: 80, Rows: 24})

	// The viewer reads nothing while 5 MiB of output arrives, 64 KiB at a
	// time, each block its own letter.
	const block, blocks = 64 << 10, 80
	for i := range blocks {
		core.Output(t1.Address, bytes.Repeat([]byte{'a' + byte(i%26)}, block))
	}
	core.Output(t1.Address, []byte("END"))

	// Now it reads, returning credit as it goes.
	var got []byte
	for !bytes.HasSuffix(got, []byte("END")) {
		f := next(t, view.Stream)
		require.Nil(t, f.Closed)
		got = append(got, f.Data...)
		view.Credit(len(f.Data))
	}

	require.Less(t, len(got), blocks*block, "the backlog was dropped")
	require.True(t, bytes.HasPrefix(got, paint), "the first paint")
	at := bytes.LastIndex(got, paint)
	require.Positive(t, at, "a fresh paint after the stall")
	require.LessOrEqual(t, at, herdrtest.DefaultWindow, "before it, only what the first credit allowed")
	tail := got[at+len(paint) : len(got)-len("END")]
	require.LessOrEqual(t, len(tail), herdrtest.DefaultMaxPending)
	require.Zero(t, len(tail)%block, "after it, whole blocks of what came later")
	require.Equal(t, byte('a'+(blocks-1)%26), tail[len(tail)-1], "ending with the last block")
}

// Each way a stream can end reaches the caller with its reason, after any
// output queued before it, and nothing follows it.
func TestCloseReasons(t *testing.T) {
	t.Parallel()
	zero, nine, fifteen := 0, 9, 15
	cases := []struct {
		name string
		end  func(core *herdrtest.Core, s *herdrtest.Stream, view *herdr.Attached)
		want herdr.CloseReason
	}{
		{"the program exited", func(core *herdrtest.Core, _ *herdrtest.Stream, _ *herdr.Attached) {
			core.Finish("t1", herdr.CloseReason{Reason: "exited", ExitCode: &zero})
		}, herdr.CloseReason{Reason: "exited", ExitCode: &zero}},
		{"killed by a signal", func(core *herdrtest.Core, _ *herdrtest.Stream, _ *herdr.Attached) {
			core.Finish("t1", herdr.CloseReason{Reason: "killed", Signal: &nine})
		}, herdr.CloseReason{Reason: "killed", Signal: &nine}},
		{"a kept terminal lost its program", func(core *herdrtest.Core, _ *herdrtest.Stream, _ *herdr.Attached) {
			core.Lose("t1", herdr.CloseReason{Reason: "signaled", Signal: &fifteen})
		}, herdr.CloseReason{Reason: "signaled", Signal: &fifteen}},
		{"the core stopped", func(_ *herdrtest.Core, s *herdrtest.Stream, _ *herdr.Attached) {
			s.Close(herdr.CloseReason{Reason: "server_stop"})
		}, herdr.CloseReason{Reason: "server_stop"}},
		{"an empty CLOSE", func(_ *herdrtest.Core, s *herdrtest.Stream, _ *herdr.Attached) {
			s.Close(herdr.CloseReason{})
		}, herdr.CloseReason{Reason: "closed"}},
		{"the connection went away", func(core *herdrtest.Core, _ *herdrtest.Stream, _ *herdr.Attached) {
			core.DropConnections()
		}, herdr.CloseReason{Reason: "lost"}},
		{"this side detached", func(_ *herdrtest.Core, _ *herdrtest.Stream, view *herdr.Attached) {
			view.Close()
		}, herdr.CloseReason{Reason: "closed"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			core := herdrtest.New(t, herdrtest.Options{})
			t1 := core.AddTerminal(herdr.Terminal{})
			c := core.Connect()
			view := attach(t, c, herdr.AttachParams{Terminal: t1.Address, Observe: true, Cols: 80, Rows: 24})
			s := core.WaitStreams(t1.Address, 1)[0]
			require.Equal(t, herdrtest.Paint(""), next(t, view.Stream).Data)

			tc.end(core, s, view)
			f := lastFrame(t, view.Stream)
			require.Equal(t, tc.want, *f.Closed)
			s.WaitClosed()
		})
	}

	// A program's last output reaches the viewer before its CLOSE.
	t.Run("last output first", func(t *testing.T) {
		t.Parallel()
		core := herdrtest.New(t, herdrtest.Options{})
		t1 := core.AddTerminal(herdr.Terminal{})
		c := core.Connect()
		view := attach(t, c, herdr.AttachParams{Terminal: t1.Address, Observe: true, Cols: 80, Rows: 24})
		next(t, view.Stream)
		core.Output(t1.Address, []byte("bye\r\n"))
		core.Finish(t1.Address, herdr.CloseReason{Reason: "exited", ExitCode: &zero})
		require.Equal(t, []byte("bye\r\n"), next(t, view.Stream).Data)
		require.Equal(t, "exited", next(t, view.Stream).Closed.Reason)
	})

	// The core sees a detach and answers it; the stream is over on both
	// sides and the connection carries on.
	t.Run("detach reaches the core", func(t *testing.T) {
		t.Parallel()
		core := herdrtest.New(t, herdrtest.Options{})
		t1 := core.AddTerminal(herdr.Terminal{})
		c := core.Connect()
		view := attach(t, c, herdr.AttachParams{Terminal: t1.Address, Observe: true, Cols: 80, Rows: 24})
		s := core.WaitStreams(t1.Address, 1)[0]
		view.Close()
		s.WaitDetached()
		require.True(t, s.Closed())
		_, err := c.List(testCtx(t))
		require.NoError(t, err)
	})
}

// A program that ends as it is attached: its paint and CLOSE follow the
// attach reply at once and may arrive before Attach has registered the
// stream. They must be kept for it, not lost.
func TestFramesBeforeStreamRegistered(t *testing.T) {
	t.Parallel()
	core := herdrtest.New(t, herdrtest.Options{})
	t1 := core.AddTerminal(herdr.Terminal{})
	core.SetScreen(t1.Address, "last screen")
	core.Handle("terminal.attach", func(call herdrtest.Call) (any, error) {
		result, err := call.Builtin()
		if err == nil {
			streams := core.Streams(t1.Address)
			streams[len(streams)-1].Finish(herdr.CloseReason{Reason: "exited"})
		}
		return result, err
	})
	c := core.Connect()
	for range 20 { // each try races the reader against Attach once
		view := attach(t, c, herdr.AttachParams{Terminal: t1.Address, Observe: true, Cols: 80, Rows: 24})
		require.Equal(t, herdrtest.Paint("last screen"), next(t, view.Stream).Data)
		require.Equal(t, "exited", lastFrame(t, view.Stream).Closed.Reason)
	}
	require.Zero(t, herdr.EarlyFrames(c), "every early frame went to its stream")
}

// A typing viewer at the terminal's own grid changes nothing about its
// size, which is how the phone types without squeezing the laptop's tab
// (plan.md 3.14.6). Announcing another grid makes it the size the terminal
// follows, and observers are told. Observers can neither type nor resize.
func TestTypingAndResize(t *testing.T) {
	t.Parallel()
	core := herdrtest.New(t, herdrtest.Options{})
	t1 := core.AddTerminal(herdr.Terminal{Cols: 120, Rows: 40})
	c := core.Connect()

	obs := attach(t, c, herdr.AttachParams{Terminal: t1.Address, Observe: true, ResizeNotifications: true, Cols: 120, Rows: 40})
	require.NotNil(t, next(t, obs.Stream).Resize)
	next(t, obs.Stream)

	typist := attach(t, c, herdr.AttachParams{Terminal: t1.Address, Cols: 120, Rows: 40})
	require.False(t, typist.Resizes)
	next(t, typist.Stream)
	ts := core.WaitStreams(t1.Address, 2)[1]
	require.False(t, ts.Params().Observe)

	require.NoError(t, typist.Write([]byte("ls -la\r")))
	require.Equal(t, []byte("ls -la\r"), ts.WaitInput(7))
	require.Equal(t, []byte("ls -la\r"), core.Input(t1.Address))
	rec, _ := core.Terminal(t1.Address)
	require.Equal(t, 120, rec.Cols, "typing at its own grid keeps the size")

	require.NoError(t, typist.Resize(80, 24))
	require.Equal(t, []herdr.Size{{Cols: 80, Rows: 24}}, ts.WaitResizes(1))
	require.Equal(t, &herdr.Size{Cols: 80, Rows: 24}, next(t, obs.Stream).Resize)
	require.Equal(t, herdrtest.Paint(""), next(t, obs.Stream).Data)
	rec, _ = core.Terminal(t1.Address)
	require.Equal(t, [2]int{80, 24}, [2]int{rec.Cols, rec.Rows})

	observer := core.Streams(t1.Address)[0]
	require.NoError(t, obs.Write([]byte("x")))
	require.NoError(t, obs.Resize(10, 10))
	require.Equal(t, []herdr.Size{{Cols: 10, Rows: 10}}, observer.WaitResizes(1))
	require.Equal(t, []byte("x"), observer.Input(), "the bytes arrived")
	require.Equal(t, []byte("ls -la\r"), core.Input(t1.Address), "but never reached the program")
	rec, _ = core.Terminal(t1.Address)
	require.Equal(t, [2]int{80, 24}, [2]int{rec.Cols, rec.Rows})
}

// After a stream closes, herdr-core still answers the client's CLOSE with
// an empty CLOSE, and output already on the wire arrives after it. None of
// it has a stream any more, so the client must drop it: gwend keeps one
// connection for days and phones open and close views on it all the time.
func TestClosedStreamLeavesNothingBehind(t *testing.T) {
	t.Parallel()
	core := herdrtest.New(t, herdrtest.Options{})
	t1 := core.AddTerminal(herdr.Terminal{})
	c := core.Connect()
	ctx := testCtx(t)

	for range 3 {
		view := attach(t, c, herdr.AttachParams{Terminal: t1.Address, Observe: true, Cols: 80, Rows: 24})
		next(t, view.Stream)
		core.Output(t1.Address, []byte("in flight"))
		view.Close()
	}
	for _, s := range core.WaitStreams(t1.Address, 3) {
		s.WaitDetached()
	}
	// The core's answers are queued ahead of this reply, so the client has
	// read them all once it returns.
	_, err := c.List(ctx)
	require.NoError(t, err)
	require.Zero(t, herdr.EarlyFrames(c))
	require.Zero(t, herdr.GoneChannels(c), "each closed channel is forgotten once the core answers")
}
