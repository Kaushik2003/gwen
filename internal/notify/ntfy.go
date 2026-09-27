package notify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/kzark/gwen/internal/config"
)

// Ntfy posts nudges to an ntfy topic for the phone. It carries no action
// buttons: the phone cannot reach the daemon.
type Ntfy struct {
	client  *http.Client
	credDir string
	log     *slog.Logger

	mu  sync.Mutex
	cfg config.Ntfy
}

// NewNtfy returns the phone backend. The token is read from credDir at the
// moment of use.
func NewNtfy(client *http.Client, credDir string) *Ntfy {
	if client == nil {
		client = &http.Client{}
	}
	return &Ntfy{client: client, credDir: credDir, log: slog.Default()}
}

// Name is "phone".
func (p *Ntfy) Name() string { return "phone" }

// SetConfig applies the ntfy settings.
func (p *Ntfy) SetConfig(c config.Config) {
	p.mu.Lock()
	p.cfg = c.Ntfy
	p.mu.Unlock()
}

var tags = map[string]string{"idle": "hourglass_flowing_sand", "break_long": "coffee"}

// Withdraw is a no-op: ntfy cannot retract a push.
func (p *Ntfy) Withdraw(context.Context, string) error { return nil }

// Notify posts to the primary server, retrying once on the fallback after a
// network error or a 5xx. Errors never mention the topic.
func (p *Ntfy) Notify(ctx context.Context, n Notification) error {
	p.mu.Lock()
	cfg := p.cfg
	p.mu.Unlock()
	if cfg.Topic == "" {
		return errors.New("no ntfy topic is set")
	}
	token, err := config.ReadCredentialLine(p.credDir, config.CredNtfyToken)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("ntfy token: %w", err)
	}
	err = p.post(ctx, cfg.Server, cfg.Topic, token, n)
	var retry *retryable
	if errors.As(err, &retry) && cfg.FallbackServer != "" {
		p.log.Warn("ntfy failed, trying the fallback", "server", host(cfg.Server), "err", err)
		err = p.post(ctx, cfg.FallbackServer, cfg.Topic, token, n)
	}
	return err
}

// retryable marks a failure that the fallback server may not share.
type retryable struct{ err error }

func (r *retryable) Error() string { return r.err.Error() }
func (r *retryable) Unwrap() error { return r.err }

func (p *Ntfy) post(ctx context.Context, server, topic, token string, n Notification) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(server, "/")+"/"+url.PathEscape(topic), strings.NewReader(n.Body))
	if err != nil {
		return fmt.Errorf("ntfy %s: bad server address", host(server))
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.Header.Set("Title", mime.QEncoding.Encode("utf-8", n.Title))
	req.Header.Set("Priority", "4")
	if t := tags[n.Kind]; t != "" {
		req.Header.Set("Tags", t)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		// A *url.Error names the URL, and with it the topic; keep only the cause.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return &retryable{fmt.Errorf("ntfy %s: %w", host(server), err)}
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	switch {
	case resp.StatusCode >= 500:
		return &retryable{fmt.Errorf("ntfy %s: %s", host(server), resp.Status)}
	case resp.StatusCode >= 400:
		return fmt.Errorf("ntfy %s: %s", host(server), resp.Status)
	}
	return nil
}

// host is the server's host for messages; it never includes the path.
func host(server string) string {
	u, err := url.Parse(server)
	if err != nil || u.Host == "" {
		return "server"
	}
	return u.Host
}
