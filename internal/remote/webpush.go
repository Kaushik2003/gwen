package remote

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/wire"
)

// Web Push (RFC 8030, 8291, 8292): the phone app subscribes in its service
// worker, and while you are away from the laptop gwend sends it a
// notification through the browser's own push service when a session needs
// you, finishes, or stops, or Gwen's agent waits for your OK. The payload is
// encrypted for the phone, so the push service never reads it. Without a
// subscribed phone, pushes go to ntfy as before.

// Credential files: the server's signing key (VAPID) and the phones'
// subscriptions, whose endpoints let anyone holding them push to the phone.
const (
	credWebPushKey  = "web_push_vapid.pem"
	credWebPushSubs = "web_push_subscriptions.json"
)

// pushHosts are the push services a subscription may point at: Chrome
// (FCM), Firefox, Safari, and Edge. Gwen posts nowhere else.
var pushHosts = []string{"fcm.googleapis.com", "push.services.mozilla.com", "push.apple.com", "notify.windows.com"}

// webPushMu guards the credential files.
var webPushMu sync.Mutex

// PushSubscription is a browser's PushSubscription.toJSON().
type PushSubscription struct {
	Endpoint       string `json:"endpoint"`
	ExpirationTime *int64 `json:"expirationTime"`
	Keys           struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// PushState is what the phone app needs to subscribe: the server's public
// key, and whether this phone is subscribed already (and with which
// endpoint, so a renewed subscription is sent again).
type PushState struct {
	Key        string `json:"key"`
	Subscribed bool   `json:"subscribed"`
	Endpoint   string `json:"endpoint"`
}

type pushSub struct {
	Device   string    `json:"device"`
	Endpoint string    `json:"endpoint"`
	P256dh   string    `json:"p256dh"`
	Auth     string    `json:"auth"`
	At       time.Time `json:"at"`
}

var b64 = base64.RawURLEncoding

// decodeB64 reads base64url with or without padding.
func decodeB64(s string) ([]byte, error) {
	return b64.DecodeString(strings.TrimRight(s, "="))
}

// vapidKey is the server's signing key, made on first use. webPushMu is held.
func (s *Service) vapidKey() (*ecdsa.PrivateKey, error) {
	raw, err := config.ReadCredential(s.opts.CredDir, credWebPushKey)
	if err == nil {
		block, _ := pem.Decode(raw)
		if block == nil {
			return nil, errors.New("the web push key is unreadable")
		}
		return x509.ParseECPrivateKey(block.Bytes)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	if err := config.WriteCredential(s.opts.CredDir, credWebPushKey, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})); err != nil {
		return nil, err
	}
	return key, nil
}

func publicBytes(key *ecdsa.PrivateKey) []byte {
	pub, err := key.PublicKey.ECDH()
	if err != nil {
		return nil
	}
	return pub.Bytes()
}

// subs reads the subscriptions. webPushMu is held.
func (s *Service) subs() []pushSub {
	raw, err := config.ReadCredential(s.opts.CredDir, credWebPushSubs)
	if err != nil {
		return nil
	}
	var list []pushSub
	_ = json.Unmarshal(raw, &list)
	return list
}

func (s *Service) saveSubs(list []pushSub) error {
	if list == nil {
		list = []pushSub{}
	}
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return config.WriteCredential(s.opts.CredDir, credWebPushSubs, b)
}

// PushState says what a phone needs to subscribe.
func (s *Service) PushState(device string) (PushState, error) {
	webPushMu.Lock()
	defer webPushMu.Unlock()
	key, err := s.vapidKey()
	if err != nil {
		return PushState{}, fail(wire.CodeUnavailable, "phone notifications can't be set up here: %v", err)
	}
	st := PushState{Key: b64.EncodeToString(publicBytes(key))}
	for _, sub := range s.subs() {
		if sub.Device == device {
			st.Subscribed, st.Endpoint = true, sub.Endpoint
		}
	}
	return st, nil
}

// Subscribe keeps a phone's subscription, replacing the one it had.
func (s *Service) Subscribe(device string, sub PushSubscription) error {
	if device == "" {
		return fail(wire.CodeInvalidRequest, "only a paired phone gets notifications")
	}
	u, err := url.Parse(sub.Endpoint)
	if err != nil || u.Scheme != "https" || !slices.ContainsFunc(pushHosts, func(h string) bool {
		return u.Hostname() == h || strings.HasSuffix(u.Hostname(), "."+h)
	}) {
		return fail(wire.CodeInvalidRequest, "that isn't a push service Gwen knows")
	}
	p256dh, err1 := decodeB64(sub.Keys.P256dh)
	auth, err2 := decodeB64(sub.Keys.Auth)
	if err1 != nil || err2 != nil || len(auth) != 16 {
		return fail(wire.CodeInvalidRequest, "the subscription's keys aren't valid")
	}
	if _, err := ecdh.P256().NewPublicKey(p256dh); err != nil {
		return fail(wire.CodeInvalidRequest, "the subscription's keys aren't valid")
	}
	webPushMu.Lock()
	defer webPushMu.Unlock()
	list := slices.DeleteFunc(s.subs(), func(x pushSub) bool { return x.Device == device || x.Endpoint == sub.Endpoint })
	list = append(list, pushSub{Device: device, Endpoint: sub.Endpoint, P256dh: b64.EncodeToString(p256dh),
		Auth: b64.EncodeToString(auth), At: s.clk.Now()})
	return s.saveSubs(list)
}

// Unsubscribe forgets a phone's subscription.
func (s *Service) Unsubscribe(device string) error {
	webPushMu.Lock()
	defer webPushMu.Unlock()
	return s.saveSubs(slices.DeleteFunc(s.subs(), func(x pushSub) bool { return x.Device == device }))
}

// pushMessage is what the service worker shows.
type pushMessage struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	// URL is a path in the phone app, opened when the notification is tapped.
	URL string `json:"url"`
	// Tag replaces an earlier notification about the same thing.
	Tag string `json:"tag"`
}

// webPush sends m to every subscribed phone that is still paired, or to
// device alone when it is not "". It returns how many took it. A phone
// whose subscription the push service no longer knows is forgotten.
func (s *Service) webPush(ctx context.Context, m pushMessage, device string) int {
	webPushMu.Lock()
	list := s.subs()
	var key *ecdsa.PrivateKey
	if len(list) > 0 {
		key, _ = s.vapidKey()
	}
	webPushMu.Unlock()
	if key == nil {
		return 0
	}
	paired := map[string]bool{}
	if devices, err := s.Devices(ctx); err == nil {
		for _, d := range devices {
			paired[d.ID] = true
		}
	}
	payload, _ := json.Marshal(m)
	sent := 0
	var gone []string
	for _, sub := range list {
		if !paired[sub.Device] || (device != "" && sub.Device != device) {
			continue
		}
		status, err := sendWebPush(ctx, key, sub, payload, m.Tag)
		switch {
		case err != nil:
			slog.Debug("web push", "err", err)
		case status == http.StatusNotFound || status == http.StatusGone:
			gone = append(gone, sub.Endpoint)
		case status >= 200 && status < 300:
			sent++
		default:
			slog.Debug("web push refused", "status", status)
		}
	}
	if len(gone) > 0 {
		webPushMu.Lock()
		_ = s.saveSubs(slices.DeleteFunc(s.subs(), func(x pushSub) bool { return slices.Contains(gone, x.Endpoint) }))
		webPushMu.Unlock()
	}
	return sent
}

// TestPush sends a notification to one phone now, away or not.
func (s *Service) TestPush(ctx context.Context, device string) error {
	if s.webPush(ctx, pushMessage{Title: "Gwen", Body: "Notifications are on. You'll hear from me here when something needs you.",
		URL: "/", Tag: "test"}, device) == 0 {
		return fail(wire.CodeUnavailable, "the notification didn't go through; turn notifications off and on again")
	}
	return nil
}

// sendWebPush posts one encrypted message (RFC 8291) signed for the push
// service (RFC 8292) and returns the service's status.
func sendWebPush(ctx context.Context, key *ecdsa.PrivateKey, sub pushSub, payload []byte, topic string) (int, error) {
	body, err := encryptPush(sub, payload)
	if err != nil {
		return 0, err
	}
	u, err := url.Parse(sub.Endpoint)
	if err != nil {
		return 0, err
	}
	jwt, err := vapidJWT(key, u.Scheme+"://"+u.Host)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", "3600")
	req.Header.Set("Urgency", "high")
	if topic != "" {
		// A newer message with the same topic replaces one not yet delivered.
		sum := sha256.Sum256([]byte(topic))
		req.Header.Set("Topic", b64.EncodeToString(sum[:24]))
	}
	req.Header.Set("Authorization", "vapid t="+jwt+", k="+b64.EncodeToString(publicBytes(key)))
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		// A *url.Error names the endpoint; keep only the cause.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return 0, fmt.Errorf("push service %s: %w", u.Hostname(), err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	return resp.StatusCode, nil
}

// vapidJWT signs the claim that this server sends to aud (RFC 8292).
func vapidJWT(key *ecdsa.PrivateKey, aud string) (string, error) {
	header := b64.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`))
	claims, _ := json.Marshal(map[string]any{"aud": aud, "exp": time.Now().Add(12 * time.Hour).Unix(), "sub": "mailto:gwen@localhost.localdomain"})
	unsigned := header + "." + b64.EncodeToString(claims)
	sum := sha256.Sum256([]byte(unsigned))
	r, sv, err := ecdsa.Sign(rand.Reader, key, sum[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	sv.FillBytes(sig[32:])
	return unsigned + "." + b64.EncodeToString(sig), nil
}

// encryptPush encrypts payload for a subscription: aes128gcm with one record
// (RFC 8188), keyed by ECDH with the phone's key and its auth secret.
func encryptPush(sub pushSub, payload []byte) ([]byte, error) {
	uaRaw, err := decodeB64(sub.P256dh)
	if err != nil {
		return nil, err
	}
	auth, err := decodeB64(sub.Auth)
	if err != nil {
		return nil, err
	}
	ua, err := ecdh.P256().NewPublicKey(uaRaw)
	if err != nil {
		return nil, err
	}
	as, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	secret, err := as.ECDH(ua)
	if err != nil {
		return nil, err
	}
	asPub := as.PublicKey().Bytes()
	info := append(append([]byte("WebPush: info\x00"), uaRaw...), asPub...)
	ikm, err := hkdf.Key(sha256.New, secret, auth, string(info), 32)
	if err != nil {
		return nil, err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	cek, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	// One record: the payload, then the last-record delimiter.
	sealed := gcm.Seal(nil, nonce, append(slices.Clone(payload), 2), nil)
	head := make([]byte, 0, 16+4+1+len(asPub)+len(sealed))
	head = append(head, salt...)
	head = binary.BigEndian.AppendUint32(head, 4096)
	head = append(head, byte(len(asPub)))
	head = append(head, asPub...)
	return append(head, sealed...), nil
}
