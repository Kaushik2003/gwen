// Package voice turns speech into text and text into speech: it records the
// microphone through PipeWire's pw-record and transcribes it with NVIDIA's
// Parakeet TDT 110M (English) on this computer, and speaks through pw-play
// with Kokoro 82M on this computer or with Fish Audio online. Parakeet and
// Kokoro run through sherpa-onnx, and nothing they hear or say leaves the
// machine; Fish sends the text it says to Fish's servers.
package voice

import (
	"archive/tar"
	"compress/bzip2"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Model is a speech model downloaded once: an archive of which only Files
// are kept, each checked against its SHA-256, and Trees.
type Model struct {
	Name  string            // its directory under the models directory
	URL   string            // a .tar.bz2
	Size  int64             // of the archive, for progress when the server sends no length
	Files map[string]string // file name → SHA-256 of its contents
	// Trees are directories of the archive kept whole, such as a voice's
	// espeak-ng-data, under the same path. Their files have no sums of their
	// own: Sum, the SHA-256 of the whole archive, vouches for them.
	Trees []string
	Sum   string
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

// Installed reports whether every file and tree is in place. Install
// checked their contents, so this only looks for them.
func (m Model) Installed(root string) bool {
	for name := range m.Files {
		if info, err := os.Stat(filepath.Join(m.Dir(root), name)); err != nil || !info.Mode().IsRegular() {
			return false
		}
	}
	for _, tree := range m.Trees {
		if info, err := os.Stat(filepath.Join(m.Dir(root), tree)); err != nil || !info.IsDir() {
			return false
		}
	}
	return true
}

// Remove deletes the model.
func (m Model) Remove(root string) error { return os.RemoveAll(m.Dir(root)) }

// progressStep is how many downloaded bytes pass between progress calls.
const progressStep = 1 << 20

// Install downloads the archive, keeps Files and Trees, checks each file and
// the archive, and only then moves them into place, replacing any earlier
// copy. progress gets the bytes downloaded so far and the total, every MiB or
// so, and once at the end.
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

	var raw io.Reader = resp.Body
	var whole hash.Hash
	if m.Sum != "" {
		whole = sha256.New()
		raw = io.TeeReader(resp.Body, whole)
	}
	body := &counter{r: raw, step: func(n int64) { progress(min(n, total), total) }}
	tr := tar.NewReader(bzip2.NewReader(body))
	var got []string
	trees := map[string]bool{}
	// With Trees the whole archive is read, as they have no list of files.
	for len(m.Trees) > 0 || len(got) < len(m.Files) {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("download the speech model: %w", err)
		}
		if tree, rel := m.inTree(h.Name); tree != "" {
			if err := writeTreeFile(tmp, rel, h, tr); err != nil {
				return err
			}
			trees[tree] = true
			continue
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
	for _, tree := range m.Trees {
		if !trees[tree] {
			return fmt.Errorf("the speech model download has no %s", tree)
		}
	}
	if whole != nil {
		// Hash what the archive reader left unread, then the whole.
		if _, err := io.Copy(io.Discard, body); err != nil {
			return fmt.Errorf("download the speech model: %w", err)
		}
		if hex.EncodeToString(whole.Sum(nil)) != m.Sum {
			return errors.New("the downloaded speech model is damaged; try again")
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

// inTree returns the tree an archive entry is in and its path from the
// model's directory, or "" for neither. Entries sit under one top directory.
func (m Model) inTree(name string) (tree, rel string) {
	_, rel, ok := strings.Cut(path.Clean(name), "/")
	if !ok {
		return "", ""
	}
	for _, t := range m.Trees {
		if rel == t || strings.HasPrefix(rel, t+"/") {
			return t, rel
		}
	}
	return "", ""
}

// writeTreeFile puts a directory or regular file of a tree under dir; other
// kinds of entry are skipped. rel must stay inside dir.
func writeTreeFile(dir, rel string, h *tar.Header, r io.Reader) error {
	if !filepath.IsLocal(rel) {
		return fmt.Errorf("the speech model download has a bad path: %s", h.Name)
	}
	dst := filepath.Join(dir, filepath.FromSlash(rel))
	switch h.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(dst, 0o755)
	case tar.TypeReg:
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, r); err != nil {
			f.Close()
			return fmt.Errorf("download the speech model: %w", err)
		}
		return f.Close()
	}
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
