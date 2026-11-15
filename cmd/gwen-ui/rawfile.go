package main

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/kzark/gwen/internal/repo"
)

// rawFilePath is where the window reads a project file's bytes, for what the
// file tab shows rather than edits: PDFs, images, and the images a Markdown
// preview links to. ?dir= is the project's folder and ?path= the file in it;
// repo.Resolve keeps the path inside the folder, as every repo binding does.
const rawFilePath = "/_gwen/file"

// rawFiles serves rawFilePath ahead of the frontend's assets.
func rawFiles(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != rawFilePath {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		q := r.URL.Query()
		abs, err := repo.Resolve(q.Get("dir"), q.Get("path"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		f, err := os.Open(abs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil || !st.Mode().IsRegular() {
			http.Error(w, "not a file", http.StatusNotFound)
			return
		}
		// The tab follows the file on disk, so never answer from a cache.
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// An SVG or HTML file opened this way must not run script in the window.
		w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src * data:; style-src 'unsafe-inline'; sandbox")
		http.ServeContent(w, r, filepath.Base(abs), st.ModTime(), f)
	})
}
