// Package voice turns speech into text on this computer: it records the
// microphone through PipeWire's pw-record and transcribes it with NVIDIA's
// Parakeet TDT 110M (English) through sherpa-onnx. Nothing leaves the machine.
package voice

import (
	"archive/tar"
	"compress/bzip2"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
)

// Model is a speech model downloaded once: an archive of which only Files
// are kept, each checked against its SHA-256.
type Model struct {
	Name  string            // its directory under the models directory
	URL   string            // a .tar.bz2
	Size  int64             // of the archive, for progress when the server sends no length
	Files map[string]string // file name → SHA-256 of its contents
}

// The files of a Parakeet transducer.
const (
	encoderFile = "encoder.int8.onnx"
	decoderFile = "decoder.int8.onnx"
	joinerFile  = "joiner.int8.onnx"
	tokensFile  = "tokens.txt"
)

// Parakeet is NVIDIA's Parakeet TDT 110M, English, as sherpa-onnx int8:
// about 0.33 GB of memory while loaded and 50× faster than real time on four
// laptop cores. Licence CC-BY-4.0.
var Parakeet = Model{
	Name: "parakeet-tdt-110m-en-int8",
	URL:  "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-nemo-parakeet_tdt_transducer_110m-en-36000-int8.tar.bz2",
	Size: 104_337_827,
	Files: map[string]string{
		encoderFile: "0f35509ddeb9b39002fb077d979a9fe74f06eb0bc4dd5c34f512f82e5111d657",
		decoderFile: "f7c331c5504c2e593c76ed22b728e3f554af6c4a383dde862e719ced08b1da19",
		joinerFile:  "bf7dff69e9f2cdbe9943d70da358f38b361c115ba0105bae7e908e0d6ec782f6",
		tokensFile:  "450e56bd2f036fe5b6aa821865838cc5aa9d8b0106134ce9a9ba0664abe6cd10",
	},
}

// Dir is where the model lives under root.
func (m Model) Dir(root string) string { return filepath.Join(root, m.Name) }

// Installed reports whether every file is in place. Install checked their
// contents, so this only looks for them.
func (m Model) Installed(root string) bool {
	for name := range m.Files {
		if info, err := os.Stat(filepath.Join(m.Dir(root), name)); err != nil || !info.Mode().IsRegular() {
			return false
		}
	}
	return true
}

// Remove deletes the model.
func (m Model) Remove(root string) error { return os.RemoveAll(m.Dir(root)) }

// progressStep is how many downloaded bytes pass between progress calls.
const progressStep = 1 << 20

// Install downloads the archive, keeps Files, checks each one, and only then
// moves them into place, replacing any earlier copy. progress gets the bytes
// downloaded so far and the total, every MiB or so, and once at the end.
func (m Model) Install(ctx context.Context, hc *http.Client, root string, progress func(done, total int64)) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.URL, nil)
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("download the speech model: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download the speech model: the server answered %s", resp.Status)
	}
	total := resp.ContentLength
	if total <= 0 {
		total = m.Size
	}
	tmp, err := os.MkdirTemp(root, "."+m.Name+"-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	body := &counter{r: resp.Body, step: func(n int64) { progress(min(n, total), total) }}
	tr := tar.NewReader(bzip2.NewReader(body))
	var got []string
	for len(got) < len(m.Files) {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("download the speech model: %w", err)
		}
		name := path.Base(h.Name)
		sum, ok := m.Files[name]
		if !ok || h.Typeflag != tar.TypeReg || slices.Contains(got, name) {
			continue
		}
		if err := writeChecked(filepath.Join(tmp, name), tr, sum); err != nil {
			return err
		}
		got = append(got, name)
	}
	for name := range m.Files {
		if !slices.Contains(got, name) {
			return fmt.Errorf("the speech model download has no %s", name)
		}
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		return err
	}
	dst := m.Dir(root)
	if err := os.RemoveAll(dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	progress(total, total)
	return nil
}

// writeChecked copies r to path and fails unless its SHA-256 is sum.
func writeChecked(path string, r io.Reader, sum string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), r); err != nil {
		f.Close()
		return fmt.Errorf("download the speech model: %w", err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != sum {
		return fmt.Errorf("the downloaded %s is damaged; try again", filepath.Base(path))
	}
	return nil
}

// counter counts the bytes read through it and calls step every progressStep.
type counter struct {
	r    io.Reader
	n    int64
	last int64
	step func(n int64)
}

func (c *counter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	if c.n-c.last >= progressStep {
		c.last = c.n
		c.step(c.n)
	}
	return n, err
}
