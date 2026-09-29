package gcal

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

// AuthWindow is how long the authorization listener waits for Google.
const AuthWindow = 5 * time.Minute

// AuthStart begins authorization (docs/07-integrations.md#authorization): it
// listens on a loopback port and returns the Google URL to open. On a valid
// callback the token is stored and OnAttempt runs. Starting again replaces an
// authorization in progress.
func (s *Service) AuthStart(ctx context.Context) (string, error) {
	cfg, err := s.oauthConfig()
	if err != nil {
		return "", err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("authorization listener: %w", err)
	}
	cfg.RedirectURL = fmt.Sprintf("http://%s/callback", ln.Addr())
	state, err := randomState()
	if err != nil {
		ln.Close()
		return "", err
	}
	verifier := oauth2.GenerateVerifier()
	url := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce, oauth2.S256ChallengeOption(verifier))

	authCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), AuthWindow)
	s.mu.Lock()
	if s.authStop != nil {
		s.authStop()
	}
	s.authStop = cancel
	s.mu.Unlock()

	done := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /callback", func(w http.ResponseWriter, r *http.Request) {
		err := s.callback(authCtx, cfg, state, verifier, r)
		if errors.Is(err, errBadState) {
			http.Error(w, "This sign-in link is not the one Gwen started. Start again from Gwen.", http.StatusBadRequest)
			return // keep waiting for the real callback
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err != nil {
			slog.Warn("calendar authorization failed", "err", err)
			fmt.Fprintf(w, "<p>Gwen could not connect to Google Calendar: %s</p>", html.EscapeString(err.Error()))
		} else {
			fmt.Fprint(w, "<p>Gwen is connected to Google Calendar. You can close this tab.</p>")
		}
		select {
		case <-done:
		default:
			close(done)
		}
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	go func() {
		select {
		case <-done:
		case <-authCtx.Done():
		}
		cancel()
		shutdown, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		srv.Shutdown(shutdown)
	}()
	return url, nil
}

var errBadState = errors.New("state does not match")

// callback validates state, exchanges the code with the PKCE verifier, and
// stores the token.
func (s *Service) callback(ctx context.Context, cfg *oauth2.Config, state, verifier string, r *http.Request) error {
	q := r.URL.Query()
	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) != 1 {
		return errBadState
	}
	defer s.o.OnAttempt(Changes{})
	if e := q.Get("error"); e != "" {
		s.fail(fmt.Errorf("google refused access: %s", e))
		return fmt.Errorf("google refused access: %s", e)
	}
	tok, err := cfg.Exchange(s.ctx(ctx), q.Get("code"), oauth2.VerifierOption(verifier))
	if err != nil {
		s.fail(fmt.Errorf("exchange the code: %w", err))
		return fmt.Errorf("exchange the code: %w", err)
	}
	if err := s.saveToken(tok); err != nil {
		s.fail(err)
		return err
	}
	slog.Info("calendar connected")
	s.mu.Lock()
	s.lastErr = ""
	s.mu.Unlock()
	s.PlanChanged() // sync soon
	return nil
}

func (s *Service) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastErr = err.Error()
}

func randomState() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("authorization state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
