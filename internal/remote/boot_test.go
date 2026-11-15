package remote

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
)

// gwend starts Gwen's core when it starts and finds none running, so the
// phone has terminals after a login. It does so once: a core that stays
// down is not launched again on every reconnect, and with no core
// installed nothing is launched at all.
func TestRunStartsTheCoreOnce(t *testing.T) {
	for _, tc := range []struct {
		name      string
		installed bool
		want      int32
	}{
		{name: "a core is installed", installed: true, want: 1},
		{name: "no core is installed", installed: false, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			old := systemCore
			systemCore = filepath.Join(home, "none")
			t.Cleanup(func() { systemCore = old })
			hc := filepath.Join(home, ".local", "lib", "gwen", "herdr-core")
			if tc.installed {
				require.NoError(t, os.MkdirAll(filepath.Dir(hc), 0o755))
				require.NoError(t, os.WriteFile(hc, nil, 0o755))
			}
			clk := clock.NewFake(testutil.At("09:00:00"))
			db := testutil.NewDBWithClock(t, clk)
			cfg := config.Defaults()
			cfg.Remote.KeepAwake = config.KeepAwakeNever
			cfg.Remote.Push = false
			var dials, launches atomic.Int32
			var argv atomic.Value
			svc, err := New(Options{DB: db, Repos: store.NewRepos(db), Clock: clk, Home: home,
				Config: func() config.Config { return cfg },
				Launch: func(_ context.Context, unit string, a []string) error {
					argv.Store(append([]string{unit}, a...))
					launches.Add(1)
					return nil
				},
				Dial: func(context.Context, string) (*herdr.Client, error) {
					dials.Add(1)
					return nil, errors.New("no core")
				},
			})
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan struct{})
			go func() { svc.Run(ctx); close(done) }()
			t.Cleanup(func() { cancel(); <-done })

			// Each kick ends a wait, so the core is dialled again.
			for n := int32(1); n <= 3; n++ {
				require.Eventually(t, func() bool { return dials.Load() >= n }, 5*time.Second, time.Millisecond)
				svc.kick <- struct{}{}
			}
			require.Eventually(t, func() bool { return dials.Load() >= 4 }, 5*time.Second, time.Millisecond)
			require.Equal(t, tc.want, launches.Load())
			if tc.installed {
				require.Equal(t, []string{coreUnit, "env", "HC_SESSION=" + herdr.Session, hc, "server", "start"}, argv.Load())
			}
		})
	}
}
