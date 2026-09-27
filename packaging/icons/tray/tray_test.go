package tray_test

import (
	"bytes"
	"image/png"
	"testing"

	"github.com/kzark/gwen/packaging/icons/tray"
	"github.com/stretchr/testify/require"
)

func TestEveryIconExists(t *testing.T) {
	t.Parallel()
	for _, name := range []string{tray.Off, tray.Working, tray.Idle, tray.Break} {
		for _, px := range []int{22, 44} {
			b, err := tray.Icon(name, px)
			require.NoError(t, err, "%s %d", name, px)
			img, err := png.Decode(bytes.NewReader(b))
			require.NoError(t, err)
			require.Equal(t, px, img.Bounds().Dx())
		}
	}
	_, err := tray.Icon("nap", 22)
	require.Error(t, err)
}
