package remote

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/herdr/herdrtest"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// w27dSent are the texts the core was asked to paste, in order.
func w27dSent(t *testing.T, core *herdrtest.Core) []string {
	t.Helper()
	var out []string
	for _, call := range core.Calls("terminal.send") {
		var p struct {
			Terminal string `json:"terminal"`
			Text     string `json:"text"`
			Force    bool   `json:"force"`
		}
		require.NoError(t, call.Decode(&p))
		require.False(t, p.Force, "a prompt is never forced into a program")
		out = append(out, p.Text)
	}
	return out
}

// The phone sends each prompt with a UUID; a retry with the same one, after
// a dropped reply or a double tap, returns the first result and sends
// nothing more (plan.md 3.14.7). The result is sent, or queued when Claude
// is working.
func TestPromptIdem(t *testing.T) {
	t.Parallel()
	r := w27dReady(t)
	addr, id := r.claude(r.dir("proj"))
	prompt := func(text, idem string) string {
		t.Helper()
		res, err := r.svc.Prompt(t.Context(), id, wire.CodePrompt{Text: text, Idem: idem})
		require.NoError(t, err)
		return res.Result
	}

	require.Equal(t, wire.PromptSent, prompt("run the tests", "a1"))
	require.Equal(t, wire.PromptSent, prompt("run the tests", "a1"))
	require.Equal(t, []string{"run the tests"}, w27dSent(t, r.core), "the same idem sends once")

	require.Equal(t, wire.PromptSent, prompt("run the tests", "a2"))
	require.Len(t, w27dSent(t, r.core), 2, "a new idem is a new message")

	r.status(addr, "working")
	require.Equal(t, wire.PromptQueued, prompt("then commit", "a3"))
	require.Equal(t, wire.PromptQueued, prompt("then commit", "a3"), "the retry gets the first answer")
	require.Equal(t, []string{"run the tests", "run the tests", "then commit"}, w27dSent(t, r.core))

	// Compose follows the same rule, for any terminal.
	for range 2 {
		res, err := r.svc.SendTerminal(t.Context(), addr, wire.CodePrompt{Text: "ls", Idem: "c1"})
		require.NoError(t, err)
		require.Equal(t, wire.PromptQueued, res.Result)
	}
	require.Len(t, w27dSent(t, r.core), 4)
}

// The idem is optional in the API: without one, every prompt is a new
// message and is sent, as once() means for an empty key.
func TestPromptWithoutIdemAlwaysSends(t *testing.T) {
	t.Parallel()
	r := w27dReady(t)
	addr, id := r.claude(r.dir("proj"))
	for _, text := range []string{"run the tests", "and lint"} {
		res, err := r.svc.Prompt(t.Context(), id, wire.CodePrompt{Text: text})
		require.NoError(t, err)
		require.Equal(t, wire.PromptSent, res.Result)
	}
	for range 2 {
		_, err := r.svc.SendTerminal(t.Context(), addr, wire.CodePrompt{Text: "ls"})
		require.NoError(t, err)
	}
	require.Equal(t, []string{"run the tests", "and lint", "ls", "ls"}, w27dSent(t, r.core))
}

// herdr-core pastes a prompt only once you stop typing in that terminal at
// the laptop, so the call waits for it; a retry of the same message while
// it waits does not send it a second time.
func TestPromptWaitsWhileYouTypeAtTheLaptop(t *testing.T) {
	t.Parallel()
	r := w27dReady(t)
	_, id := r.claude(r.dir("proj"))
	typing := make(chan struct{})
	stopTyping := sync.OnceFunc(func() { close(typing) })
	t.Cleanup(stopTyping)
	r.core.Handle("terminal.send", func(call herdrtest.Call) (any, error) {
		<-typing
		return call.Builtin()
	})

	type result struct {
		res wire.CodePromptResult
		err error
	}
	send := func() <-chan result {
		ch := make(chan result, 1)
		go func() {
			res, err := r.svc.Prompt(t.Context(), id, wire.CodePrompt{Text: "add a test for it", Idem: "b1"})
			ch <- result{res, err}
		}()
		return ch
	}
	first := send()
	r.core.WaitCalls("terminal.send", 1)
	select {
	case got := <-first:
		t.Fatalf("the prompt went in while you were typing: %+v", got)
	default:
	}
	retry := send()
	stopTyping()
	for _, ch := range []<-chan result{first, retry} {
		got := w27dRecv(t, ch, "the prompt's result")
		require.NoError(t, got.err)
		require.Equal(t, wire.PromptSent, got.res.Result)
	}
	require.Len(t, r.core.Calls("terminal.send"), 1)
}

// Gwen never forces a prompt into a Claude that is asking something: a
// session that needs you refuses it with conflict, "Answer Claude first",
// and so does herdr-core when it sees Claude blocked first. A refused
// prompt is not remembered, so the phone's retry with the same idem goes
// through once Claude can take it.
func TestPromptConflict(t *testing.T) {
	t.Parallel()

	t.Run("the session needs you", func(t *testing.T) {
		t.Parallel()
		r := w27dReady(t)
		addr, id := r.claude(r.dir("proj"))
		r.status(addr, "blocked")
		require.Equal(t, store.CodeNeedsYou, r.card(id).State)
		_, err := r.svc.Prompt(t.Context(), id, wire.CodePrompt{Text: "yes", Idem: "e1"})
		require.Equal(t, wire.CodeConflict, w27dCode(err))
		require.EqualError(t, err, "Answer Claude first")
		require.Empty(t, r.core.Calls("terminal.send"))

		r.status(addr, "working")
		res, err := r.svc.Prompt(t.Context(), id, wire.CodePrompt{Text: "yes", Idem: "e1"})
		require.NoError(t, err)
		require.Equal(t, wire.PromptQueued, res.Result)
		require.Equal(t, []string{"yes"}, w27dSent(t, r.core))
	})

	t.Run("herdr-core says Claude is blocked", func(t *testing.T) {
		t.Parallel()
		r := w27dReady(t)
		_, id := r.claude(r.dir("proj"))
		var sends atomic.Int32
		r.core.Handle("terminal.send", func(call herdrtest.Call) (any, error) {
			if sends.Add(1) == 1 {
				return nil, herdrtest.Err("blocked", "t1 is blocked: it may be asking something")
			}
			return call.Builtin()
		})
		_, err := r.svc.Prompt(t.Context(), id, wire.CodePrompt{Text: "go on", Idem: "e2"})
		require.Equal(t, wire.CodeConflict, w27dCode(err))
		require.EqualError(t, err, "Answer Claude first")
		res, err := r.svc.Prompt(t.Context(), id, wire.CodePrompt{Text: "go on", Idem: "e2"})
		require.NoError(t, err)
		require.Equal(t, wire.PromptSent, res.Result)
		require.Len(t, r.core.Calls("terminal.send"), 2)
	})

	t.Run("the session ended", func(t *testing.T) {
		t.Parallel()
		r := w27dReady(t)
		addr, id := r.claude(r.dir("proj"))
		r.core.Finish(addr, herdr.CloseReason{Reason: "exited"})
		r.sync()
		_, err := r.svc.Prompt(t.Context(), id, wire.CodePrompt{Text: "hello?", Idem: "e3"})
		require.Equal(t, wire.CodeConflict, w27dCode(err))
		require.EqualError(t, err, "that session has ended; resume it to keep going")
		require.Empty(t, r.core.Calls("terminal.send"))
	})

	t.Run("empty or too long", func(t *testing.T) {
		t.Parallel()
		r := w27dReady(t)
		_, id := r.claude(r.dir("proj"))
		for _, text := range []string{"", " \n\t", strings.Repeat("x", 32<<10+1)} {
			_, err := r.svc.Prompt(t.Context(), id, wire.CodePrompt{Text: text})
			require.Equal(t, wire.CodeInvalidRequest, w27dCode(err))
		}
		_, err := r.svc.Prompt(t.Context(), id, wire.CodePrompt{Text: strings.Repeat("x", 32<<10)})
		require.NoError(t, err, "32 KiB is allowed")
		require.Len(t, r.core.Calls("terminal.send"), 1)
	})
}

// w27dKeys are n key names.
func w27dKeys(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = "down"
	}
	return keys
}

// A key message from the phone's terminal view names at most 32 keys, and a
// view takes at most 50 messages a second, typed text included; anything
// over is dropped, not queued (plan.md 3.14.7).
func TestViewKeyLimits(t *testing.T) {
	t.Parallel()
	r := w27dReady(t)
	addr := r.core.AddTerminal(herdr.Terminal{Cwd: r.dir("proj"), Argv: []string{"bash"}}).Address
	r.sync()
	open := func(t *testing.T) *View {
		t.Helper()
		v, err := r.svc.OpenView(t.Context(), "", addr)
		require.NoError(t, err)
		t.Cleanup(v.Close)
		return v
	}
	keyCalls := func(t *testing.T) [][]string {
		var out [][]string
		for _, call := range r.core.Calls("terminal.keys") {
			var p struct {
				Keys []string `json:"keys"`
			}
			require.NoError(t, call.Decode(&p))
			out = append(out, p.Keys)
		}
		return out
	}

	t.Run("32 names a message", func(t *testing.T) {
		v := open(t)
		require.NoError(t, v.Keys(t.Context(), w27dKeys(33)))
		require.NoError(t, v.Keys(t.Context(), nil))
		require.Empty(t, keyCalls(t), "too many names, or none, sends nothing")
		require.NoError(t, v.Keys(t.Context(), w27dKeys(32)))
		require.Equal(t, [][]string{w27dKeys(32)}, keyCalls(t))
	})

	t.Run("50 messages a second", func(t *testing.T) {
		// The limit counts by the wall clock's second, so a burst that
		// happens to straddle one is tried again.
		for attempt := 1; ; attempt++ {
			v := open(t)
			before := len(keyCalls(t))
			streams := len(r.core.Streams(addr))
			start := time.Now().Unix()
			for range 51 {
				require.NoError(t, v.Keys(t.Context(), []string{"up"}))
			}
			require.NoError(t, v.Text(t.Context(), "ls\r"))
			if time.Now().Unix() != start && attempt < 5 {
				continue
			}
			require.Equal(t, start, time.Now().Unix(), "five bursts in a row straddled a second")
			require.Len(t, keyCalls(t), before+50, "the 51st message is dropped")
			require.Len(t, r.core.Streams(addr), streams, "and so is typed text: no typing attach opened")

			// The next second starts afresh.
			v.mu.Lock()
			v.second--
			v.mu.Unlock()
			require.NoError(t, v.Keys(t.Context(), []string{"enter"}))
			require.Len(t, keyCalls(t), before+51)
			return
		}
	})
}

// w27dPhoto is a small JPEG, as a phone's camera would send.
func w27dPhoto(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for x := range 64 {
		for y := range 48 {
			img.Set(x, y, color.RGBA{R: uint8(x * 4), G: uint8(y * 5), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, nil))
	return buf.Bytes()
}

// Compose attaches a photo by putting it beside the terminal with
// terminal.put; the reply is where it landed, which the phone adds to the
// text. The name is only a file name, and 8 MB is herdr-core's limit
// (plan.md 3.14.7).
func TestPutPhoto(t *testing.T) {
	t.Parallel()
	r := w27dReady(t)
	addr := r.core.AddTerminal(herdr.Terminal{Cwd: r.dir("proj"), Argv: []string{"bash"}}).Address
	r.sync()
	put := func(name string, data []byte) (wire.CodePut, error) {
		return r.svc.Put(t.Context(), addr, name, data)
	}

	photo := w27dPhoto(t)
	got, err := put("IMG_0001.jpg", photo)
	require.NoError(t, err)
	require.Equal(t, path.Join("/tmp/herdrtest", addr, "IMG_0001.jpg"), got.Path, "the path herdr-core gave")
	saved, ok := r.core.File(addr, "IMG_0001.jpg")
	require.True(t, ok)
	require.Equal(t, photo, saved)

	// A name with folders in it is cut to its last element before it
	// leaves gwend.
	_, err = put("../../.ssh/authorized_keys", photo)
	require.NoError(t, err)
	var p struct {
		Name string `json:"name"`
	}
	calls := r.core.Calls("terminal.put")
	require.NoError(t, calls[len(calls)-1].Decode(&p))
	require.Equal(t, "authorized_keys", p.Name)

	big := bytes.Repeat([]byte{0xff}, herdr.MaxPut)
	_, err = put("scan.png", big)
	require.NoError(t, err, "exactly 8 MB fits")
	saved, _ = r.core.File(addr, "scan.png")
	require.Len(t, saved, herdr.MaxPut)

	n := len(r.core.Calls("terminal.put"))
	_, err = put("huge.png", append(big, 0))
	require.Equal(t, wire.CodeInvalidRequest, w27dCode(err))
	require.EqualError(t, err, "that file is too large; 8 MB at most")
	_, err = put("empty.jpg", nil)
	require.Equal(t, wire.CodeInvalidRequest, w27dCode(err))
	require.Len(t, r.core.Calls("terminal.put"), n, "refused before it reaches the core")

	_, err = r.svc.Put(t.Context(), "t99", "IMG_0002.jpg", photo)
	require.Equal(t, wire.CodeNotFound, w27dCode(err))
}
