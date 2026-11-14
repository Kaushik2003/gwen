package herdr_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/kzark/gwen/internal/herdr"
	"github.com/stretchr/testify/require"
)

// VersionAtLeast compares dotted numbers as numbers, so 0.1.9 is older than
// 0.1.10, and tolerates how versions get written.
func TestVersionAtLeast(t *testing.T) {
	t.Parallel()
	cases := []struct {
		have, want string
		ok         bool
	}{
		{"0.1.20", "0.1.20", true},
		{"0.1.21", "0.1.20", true},
		{"0.1.19", "0.1.20", false},
		{"0.2.0", "0.1.20", true},
		{"1.0.0", "0.9.9", true},
		{"0.9.9", "1.0.0", false},
		{"0.1.9", "0.1.10", false},
		{"0.1.10", "0.1.9", true},
		{"v0.1.20", "0.1.20", true},
		{"0.1.20", "v0.1.20", true},
		{" 0.1.20\n", "0.1.20", true},
		{"0.1.20-rc1", "0.1.20", true}, // only the numbers count
		{"0.1.20+abc", "0.1.21", false},
		{"0.1.20 (abc123)", "0.1.20", true},
		{"0.1", "0.1.0", true},
		{"0.1", "0.1.1", false},
		{"", "0.0.1", false},
		{"garbage", "0.1.0", false}, // anything unparseable counts as 0
		{"garbage", "0.0.0", true},
	}
	for _, tc := range cases {
		require.Equal(t, tc.ok, herdr.VersionAtLeast(tc.have, tc.want), "VersionAtLeast(%q, %q)", tc.have, tc.want)
	}
}

// Gwen's core is the session named gwen, never the default core of a Herdr
// the person runs themselves (plan.md 3.14.5). Only the path is computed;
// nothing connects to it.
func TestDefaultSocket(t *testing.T) {
	fallback := filepath.Join(os.TempDir(), "herdr-core-"+strconv.Itoa(os.Getuid()), "gwen.sock")
	cases := []struct {
		runtime string
		want    string
	}{
		{"/run/user/1000", "/run/user/1000/herdr-core/gwen.sock"},
		{"", fallback},
		{"relative/dir", fallback},
	}
	for _, tc := range cases {
		t.Setenv("XDG_RUNTIME_DIR", tc.runtime)
		require.Equal(t, tc.want, herdr.DefaultSocket(), "XDG_RUNTIME_DIR=%q", tc.runtime)
	}
	require.Equal(t, "gwen", herdr.Session)
}
