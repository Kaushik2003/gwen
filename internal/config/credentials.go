package config

import (
	"bytes"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Credential file names (docs/07-integrations.md#credentials).
const (
	CredNtfyToken    = "ntfy_token"
	CredGoogleClient = "google_client.json"
	CredGoogleToken  = "google_token.json"
	CredLLMAPIKey    = "llm_api_key"
	CredSyncToken    = "sync_token"
)

// CredentialsDir returns the credentials directory under a data directory.
func CredentialsDir(dataDir string) string { return filepath.Join(dataDir, "credentials") }

// ReadCredential returns the contents of a credential file. Files are read at
// the moment of use and never cached. A missing file is an error satisfying
// errors.Is(err, fs.ErrNotExist).
func ReadCredential(dir, name string) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil, fmt.Errorf("read credential %s: %w", name, err)
	}
	return b, nil
}

// ReadCredentialLine returns a one-line credential with surrounding whitespace
// removed.
func ReadCredentialLine(dir, name string) (string, error) {
	b, err := ReadCredential(dir, name)
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(b)), nil
}

// WriteCredential writes a credential file atomically with mode 0600, creating
// dir with mode 0700.
func WriteCredential(dir, name string, data []byte) error {
	if name == "" || filepath.Base(name) != name {
		return fmt.Errorf("write credential: invalid name %q", name)
	}
	if err := writeFileAtomic(filepath.Join(dir, name), data, 0o700); err != nil {
		return fmt.Errorf("write credential %s: %w", name, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("write credential %s: %w", name, err)
	}
	return nil
}

// NewTopic returns a fresh ntfy topic: "gwen-" and 24 lowercase base32
// characters from crypto/rand.
func NewTopic() string {
	var b [15]byte         // 15 bytes encode to exactly 24 base32 characters
	_, _ = rand.Read(b[:]) // never fails; see crypto/rand.Read
	return "gwen-" + strings.ToLower(base32.StdEncoding.EncodeToString(b[:]))
}
