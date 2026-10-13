package notify

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/kzark/gwen/assets"
)

// playTimeout bounds one play of the nudge sound.
const playTimeout = 10 * time.Second

// players play a sound file, best first. Plasma's own sound hints go through
// libcanberra, which cannot play MP3, so gwend plays the sound itself.
var players = [][]string{{"pw-play"}, {"paplay"}}

var (
	soundOnce sync.Once
	soundPath string
	soundErr  error
)

// nudgeSound writes the embedded sound to the user's cache once and returns
// its path.
func nudgeSound() (string, error) {
	soundOnce.Do(func() {
		dir, err := os.UserCacheDir()
		if err != nil {
			soundErr = err
			return
		}
		dir = filepath.Join(dir, "gwen")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			soundErr = err
			return
		}
		soundPath = filepath.Join(dir, "nudge.mp3")
		soundErr = os.WriteFile(soundPath, assets.NudgeSound, 0o600)
	})
	return soundPath, soundErr
}

// playSound plays the nudge sound with the first player found.
func playSound(ctx context.Context) error {
	path, err := nudgeSound()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, playTimeout)
	defer cancel()
	for _, p := range players {
		if _, err := exec.LookPath(p[0]); err != nil {
			continue
		}
		return exec.CommandContext(ctx, p[0], append(p[1:], path)...).Run()
	}
	return errors.New("no sound player: install pipewire-utils or pulseaudio-utils")
}
