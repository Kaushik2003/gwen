package remote

import (
	"context"
	"sync"
	"time"

	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/wire"
)

// Limits of a terminal view (plan.md 3.14.7).
const (
	viewsPerDevice = 2
	maxKeysPerMsg  = 32
	maxKeyMsgsPerS = 50
	maxTypedText   = 4 << 10
	viewHistory    = 1000
)

// View is a phone's live view of one terminal. It only observes: output
// arrives through Next, and typing goes through herdr-core's API, so the
// phone never becomes the viewer whose size the terminal follows and the
// laptop's tab keeps its size.
type View struct {
	s        *Service
	device   string
	terminal string
	core     *herdr.Client
	out      *herdr.Attached

	mu     sync.Mutex
	cols   int
	rows   int
	typist *herdr.Attached // opened on the first typed text
	typed  herdr.Size      // the grid the typist last announced
	second int64
	msgs   int
	closed bool
}

// OpenView attaches to a terminal for a device; device "" is the local
// owner, who has no limit.
func (s *Service) OpenView(ctx context.Context, device, terminal string) (*View, error) {
	s.mu.Lock()
	t, c, err := s.terminal(terminal)
	if err == nil && device != "" && s.views[device] >= viewsPerDevice {
		err = fail(wire.CodeConflict, "this phone already has two terminals open; close one first")
	}
	if err == nil && device != "" {
		s.views[device]++
	}
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	out, err := c.Attach(ctx, herdr.AttachParams{Terminal: terminal, Observe: true, ResizeNotifications: true,
		History: viewHistory, Cols: t.Cols, Rows: t.Rows})
	if err != nil {
		s.viewDone(device)
		return nil, coreError(err)
	}
	v := &View{s: s, device: device, terminal: terminal, core: c, out: out, cols: out.Terminal.Cols, rows: out.Terminal.Rows}
	if v.cols == 0 || v.rows == 0 {
		v.cols, v.rows = t.Cols, t.Rows
	}
	return v, nil
}

func (s *Service) viewDone(device string) {
	if device == "" {
		return
	}
	s.mu.Lock()
	if s.views[device] > 0 {
		s.views[device]--
	}
	s.mu.Unlock()
}

// Size is the terminal's grid as last announced.
func (v *View) Size() (cols, rows int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.cols, v.rows
}

// Next returns the next frame of output: bytes, a new grid, or the end.
// After writing the bytes on, the caller returns their length with Credit.
func (v *View) Next(ctx context.Context) (herdr.Frame, error) {
	f, err := v.out.Next(ctx)
	if err == nil && f.Resize != nil {
		v.mu.Lock()
		v.cols, v.rows = f.Resize.Cols, f.Resize.Rows
		v.mu.Unlock()
	}
	return f, err
}

// Credit tells herdr-core that n bytes reached the phone.
func (v *View) Credit(n int) { v.out.Credit(n) }

// allow counts one message against the per-second limit.
func (v *View) allow() bool {
	now := time.Now().Unix()
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.second != now {
		v.second, v.msgs = now, 0
	}
	v.msgs++
	return v.msgs <= maxKeyMsgsPerS
}

// Keys presses named keys: enter, esc, tab, shift+tab, up, ctrl+c, ...
// Messages over the limits are dropped.
func (v *View) Keys(ctx context.Context, keys []string) error {
	if len(keys) == 0 || len(keys) > maxKeysPerMsg || !v.allow() {
		return nil
	}
	v.mu.Lock()
	closed := v.closed
	v.mu.Unlock()
	if closed {
		return nil
	}
	return v.core.Keys(ctx, v.terminal, keys)
}

// Text types characters as they are. herdr-core takes typed bytes only
// from a viewer that may type, so the view opens one whose size is the
// terminal's own grid: typing then changes nothing about the size.
func (v *View) Text(ctx context.Context, text string) error {
	if text == "" || len(text) > maxTypedText || !v.allow() {
		return nil
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return nil
	}
	grid := herdr.Size{Cols: v.cols, Rows: v.rows}
	if v.typist == nil {
		t, err := v.core.Attach(ctx, herdr.AttachParams{Terminal: v.terminal, History: 0, Cols: grid.Cols, Rows: grid.Rows})
		if err != nil {
			return coreError(err)
		}
		v.typist, v.typed = t, grid
		go func() { // its output is the same as the observer's: discard it
			for {
				f, err := t.Next(context.Background())
				if err != nil || f.Closed != nil {
					return
				}
				t.Credit(len(f.Data))
			}
		}()
	} else if v.typed != grid {
		if err := v.typist.Resize(grid.Cols, grid.Rows); err != nil {
			return coreError(err)
		}
		v.typed = grid
	}
	return v.typist.Write([]byte(text))
}

// Close detaches the view.
func (v *View) Close() {
	v.mu.Lock()
	if v.closed {
		v.mu.Unlock()
		return
	}
	v.closed = true
	typist := v.typist
	v.mu.Unlock()
	v.out.Close()
	if typist != nil {
		typist.Close()
	}
	v.s.viewDone(v.device)
}
