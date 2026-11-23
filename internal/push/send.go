package push

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mhersson/recallsynapsis/internal/module"
)

// maxMessage caps a message's JSON well under the 4096 octets of body every
// push service must accept (RFC 8030 section 7.2).
const maxMessage = 2048

// ErrGone is returned when the push service no longer knows the subscription.
var ErrGone = errors.New("push subscription gone")

// Subscription is a browser's Web Push subscription, its keys base64url as the browser gave them.
type Subscription struct {
	Endpoint, P256DH, Auth string
}

// Failure is a push that did not go through. Its Line names the push
// service's host and never the endpoint.
type Failure struct {
	Line   string // "HOST was not reached" when no answer came, else "HOST answered STATUS"
	Detail string // the error met, or the first line of the answer
}

func (f *Failure) Error() string {
	if f.Detail == "" {
		return f.Line
	}

	return f.Line + ": " + f.Detail
}

// message is what a notification shows, as the service worker reads it.
type message struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag"`
}

// NewClient is the client a Sender posts with: through the proxy the
// environment names, waiting at most 30 seconds. It follows no redirect, so a
// push reaches the endpoint's host and no other.
func NewClient() *http.Client {
	return &http.Client{
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// Check refuses a subscription Send could not push to: an endpoint that is not
// an https URL with a host, or keys that do not decode. Its error never quotes
// the endpoint.
func Check(sub Subscription) error {
	if u, err := url.Parse(sub.Endpoint); err != nil || u.Scheme != "https" || u.Host == "" {
		return errors.New("endpoint: want an https URL")
	}

	_, _, err := keys(sub)

	return err
}

// keys decodes sub's keys, base64url with or without padding: p256dh a P-256
// point and auth 16 octets, as RFC 8291 has them.
func keys(sub Subscription) (p256dh, auth []byte, err error) {
	p256dh, err = b64.DecodeString(strings.TrimRight(sub.P256DH, "="))
	if err == nil {
		_, err = ecdh.P256().NewPublicKey(p256dh)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("subscription key: %w", err)
	}

	auth, err = b64.DecodeString(strings.TrimRight(sub.Auth, "="))
	if err == nil && len(auth) != 16 {
		err = errors.New("want 16 octets")
	}

	if err != nil {
		return nil, nil, fmt.Errorf("subscription secret: %w", err)
	}

	return p256dh, auth, nil
}

// Sender posts messages to the push services subscriptions name.
type Sender struct {
	Client  *http.Client
	Signer  *Signer
	Contact string // the sender's contact in each token
}

// Send encrypts m for sub and posts it, to wait at the push service for ttl
// at high urgency. It returns ErrGone when the push service no longer knows
// sub, and a *Failure when it answered otherwise or not at all.
func (s *Sender) Send(ctx context.Context, sub Subscription, m module.Message, ttl time.Duration) error {
	payload, err := message(m).encode()
	if err != nil {
		return err
	}

	p256dh, auth, err := keys(sub)
	if err != nil {
		return err
	}

	body, err := Seal(p256dh, auth, payload)
	if err != nil {
		return err
	}

	authorization, err := s.Signer.Authorization(sub.Endpoint, s.Contact)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("push request: %w", err)
	}

	req.Header.Set("Authorization", authorization)
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", strconv.FormatInt(int64(ttl/time.Second), 10))
	req.Header.Set("Urgency", "high")

	if m.Tag != "" {
		req.Header.Set("Topic", m.Tag)
	}

	resp, err := s.Client.Do(req)
	if err != nil {
		// A *url.Error quotes the endpoint, which is a capability (RFC 8030 section 8.3).
		if ue, ok := errors.AsType[*url.Error](err); ok {
			err = ue.Err
		}

		return &Failure{Line: req.URL.Host + " was not reached", Detail: err.Error()}
	}

	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return ErrGone
	case resp.StatusCode >= 200 && resp.StatusCode <= 299:
		return nil
	}

	answer, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	line, _, _ := strings.Cut(strings.TrimSpace(string(answer)), "\n")

	return &Failure{Line: req.URL.Host + " answered " + strconv.Itoa(resp.StatusCode), Detail: strings.TrimSpace(line)}
}

// encode is m's JSON within maxMessage octets, its body cut at a rune boundary
// when it would not fit.
func (m message) encode() ([]byte, error) {
	for {
		b, err := json.Marshal(m)
		if err != nil {
			return nil, fmt.Errorf("push message: %w", err)
		}

		over := len(b) - maxMessage
		if over <= 0 || m.Body == "" {
			return b, nil
		}

		// An octet of the body is at least one octet of its JSON.
		cut := max(len(m.Body)-over, 0)
		for cut > 0 && !utf8.RuneStart(m.Body[cut]) {
			cut--
		}

		m.Body = m.Body[:cut]
	}
}
