package remote

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kzark/gwen/internal/store"
)

// How pairing works (plan.md 3.14.4).
const (
	pairCodeLife   = 10 * time.Minute
	seenResolution = time.Minute
	// DeviceCookie is the cookie a paired phone carries.
	DeviceCookie = "gwen_device"
	// DeviceCookieAge is the cookie's Max-Age in seconds: 400 days.
	DeviceCookieAge = 400 * 24 * 60 * 60
)

// ErrBadPairCode is a pairing code that is wrong, used, or too old.
var ErrBadPairCode = errors.New("that pairing code is not valid any more; show a new one on the laptop")

// pairing holds the one live pairing code and when each phone was last
// recorded as seen.
type pairing struct {
	mu      sync.Mutex
	code    string
	expires time.Time
	seen    map[string]time.Time
}

// NewPairCode makes a single-use code valid for ten minutes; it replaces
// any earlier one.
func (s *Service) NewPairCode() (string, time.Time, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", time.Time{}, err
	}
	code := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:]))
	s.pair.mu.Lock()
	defer s.pair.mu.Unlock()
	s.pair.code, s.pair.expires = code, s.clk.Now().Add(pairCodeLife)
	return code, s.pair.expires, nil
}

// Pair spends a pairing code and returns the token for the phone's cookie.
func (s *Service) Pair(ctx context.Context, code, name string) (string, error) {
	code = strings.ToLower(strings.TrimSpace(code))
	s.pair.mu.Lock()
	ok := s.pair.code != "" && s.clk.Now().Before(s.pair.expires) &&
		subtle.ConstantTimeCompare([]byte(code), []byte(s.pair.code)) == 1
	if ok {
		s.pair.code = ""
	}
	s.pair.mu.Unlock()
	if !ok {
		return "", ErrBadPairCode
	}
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b[:])
	if _, err := s.repos.RemoteDevices.Create(ctx, name, tokenHash(token)); err != nil {
		return "", err
	}
	s.remoteChanged()
	return token, nil
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Device returns the paired phone holding token, and records that it called.
func (s *Service) Device(ctx context.Context, token string) (store.RemoteDevice, error) {
	if token == "" {
		return store.RemoteDevice{}, store.ErrNotFound
	}
	d, err := s.repos.RemoteDevices.ByTokenHash(ctx, tokenHash(token))
	if err != nil {
		return store.RemoteDevice{}, err
	}
	now := s.clk.Now()
	s.pair.mu.Lock()
	if s.pair.seen == nil {
		s.pair.seen = map[string]time.Time{}
	}
	record := now.Sub(s.pair.seen[d.ID]) >= seenResolution
	if record {
		s.pair.seen[d.ID] = now
	}
	s.pair.mu.Unlock()
	if record {
		_ = s.repos.RemoteDevices.Seen(ctx, d.ID, now)
	}
	return d, nil
}

// Devices lists the paired phones.
func (s *Service) Devices(ctx context.Context) ([]store.RemoteDevice, error) {
	return s.repos.RemoteDevices.List(ctx)
}

// Revoke unpairs a phone: its next request is refused, and its open terminal
// views and event stream end at once.
func (s *Service) Revoke(ctx context.Context, id string) error {
	if err := s.repos.RemoteDevices.Delete(ctx, id); err != nil {
		return err
	}
	s.unpaired(id)
	s.remoteChanged()
	return nil
}

// Owner is the Tailscale login allowed to use Gwen from a phone, "" before
// the remote was first turned on.
func (s *Service) Owner(ctx context.Context) string {
	owner, err := store.GetLocal(ctx, s.db.SQL(), store.KeyRemoteOwner)
	if err != nil {
		return ""
	}
	return owner
}

// claimOwner stores this laptop's Tailscale login as the owner, once.
func (s *Service) claimOwner(ctx context.Context, login string) {
	if login == "" || s.Owner(ctx) != "" {
		return
	}
	if err := store.SetLocal(ctx, s.db.SQL(), store.KeyRemoteOwner, login, s.clk.Now()); err == nil {
		s.remoteChanged()
	}
}

// Tailscale is what `tailscale status --json` says about this laptop.
type Tailscale struct {
	// State is ok, missing, stopped, or logged_out.
	State string
	Host  string // the laptop's tailnet DNS name, without the final dot
	Login string // the login this laptop is signed in as
	Certs bool   // HTTPS certificates are on in the tailnet
}

// tailscaleStatus reads the system tailscaled; it never changes anything
// and never runs as root.
func tailscaleStatus(ctx context.Context) Tailscale {
	bin, err := exec.LookPath("tailscale")
	if err != nil {
		return Tailscale{State: "missing"}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "status", "--json").Output()
	if err != nil {
		return Tailscale{State: "stopped"}
	}
	var st struct {
		BackendState string
		CertDomains  []string
		Self         struct {
			DNSName string
			UserID  json.Number
		}
		User map[string]struct{ LoginName string }
	}
	if json.Unmarshal(out, &st) != nil {
		return Tailscale{State: "stopped"}
	}
	t := Tailscale{State: "ok", Host: strings.TrimSuffix(st.Self.DNSName, "."), Certs: len(st.CertDomains) > 0}
	switch st.BackendState {
	case "Running":
	case "NeedsLogin", "NoState":
		t.State = "logged_out"
	default:
		t.State = "stopped"
	}
	if u, ok := st.User[st.Self.UserID.String()]; ok {
		t.Login = u.LoginName
	}
	return t
}

// servePublished reports whether Tailscale Serve publishes socket on port.
func servePublished(ctx context.Context, port int, socket string) bool {
	bin, err := exec.LookPath("tailscale")
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "serve", "status", "--json").Output()
	if err != nil {
		return false
	}
	var st struct {
		Web map[string]struct {
			Handlers map[string]struct{ Proxy string }
		}
	}
	if json.Unmarshal(out, &st) != nil {
		return false
	}
	suffix := fmt.Sprintf(":%d", port)
	for host, web := range st.Web {
		if !strings.HasSuffix(host, suffix) {
			continue
		}
		for _, h := range web.Handlers {
			if strings.Contains(h.Proxy, socket) {
				return true
			}
		}
	}
	return false
}

// phoneOrigin keeps the laptop's tailnet name for the WebSocket origin
// check, so a terminal view doesn't wait on `tailscale status` each time.
type phoneOrigin struct {
	mu   sync.Mutex
	host string
	at   time.Time
}

// PhoneOrigin reports whether origin is the address the phone app is
// published at: https://<this laptop's tailnet name>:<port>. Tailscale
// Serve hands requests on to the remote socket with Host "localhost", so a
// page's origin can't be compared with the request's Host there.
func (s *Service) PhoneOrigin(ctx context.Context, origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return false
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	if port != strconv.Itoa(s.cfg().Remote.Port) {
		return false
	}
	o := &s.origin
	o.mu.Lock()
	defer o.mu.Unlock()
	// A name that doesn't match is looked up again, at most once a minute,
	// in case the machine was renamed in the tailnet.
	if o.host == "" || !strings.EqualFold(o.host, u.Hostname()) && s.clk.Now().Sub(o.at) > time.Minute {
		if host := tailscaleStatus(ctx).Host; host != "" {
			o.host = host
		}
		o.at = s.clk.Now()
	}
	return o.host != "" && strings.EqualFold(o.host, u.Hostname())
}
