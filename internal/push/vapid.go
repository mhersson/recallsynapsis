package push

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"time"
)

const (
	// tokenLife is how long a token is valid; RFC 8292 allows at most 24 hours.
	tokenLife = 12 * time.Hour
	// tokenRenew is how long before its expiry a token is replaced.
	tokenRenew = time.Hour
)

var (
	b64       = base64.RawURLEncoding
	jwtHeader = b64.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`))
)

// Signer makes the VAPID Authorization header (RFC 8292). A token serves every
// endpoint of its audience, the push service's origin, until it is renewed. It
// is safe for concurrent use.
type Signer struct {
	key    *ecdsa.PrivateKey
	now    func() time.Time
	mu     sync.Mutex
	tokens map[string]token // by audience
}

type token struct {
	header  string
	expires time.Time
}

// NewSigner signs with key, the instance's VAPID key.
func NewSigner(key *ecdsa.PrivateKey, now func() time.Time) *Signer {
	return &Signer{key: key, now: now, tokens: map[string]token{}}
}

// Authorization is the header for a push to endpoint; a newly made token
// carries subject as the sender's contact.
func (s *Signer) Authorization(endpoint, subject string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("endpoint: %w", err)
	}

	aud := u.Scheme + "://" + u.Host

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	if t, ok := s.tokens[aud]; ok && now.Before(t.expires.Add(-tokenRenew)) {
		return t.header, nil
	}

	expires := now.Add(tokenLife)

	claims, err := json.Marshal(struct {
		Aud string `json:"aud"`
		Exp int64  `json:"exp"`
		Sub string `json:"sub"`
	}{aud, expires.Unix(), subject})
	if err != nil {
		return "", fmt.Errorf("claims: %w", err)
	}

	signed := jwtHeader + "." + b64.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signed))

	r, sig, err := ecdsa.Sign(rand.Reader, s.key, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}

	// JWS wants the raw r||s, each 32 octets, not ASN.1.
	raw := make([]byte, 64)
	r.FillBytes(raw[:32])
	sig.FillBytes(raw[32:])

	header := "vapid t=" + signed + "." + b64.EncodeToString(raw) + ", k=" + PublicKey(s.key)
	s.tokens[aud] = token{header: header, expires: expires}

	return header, nil
}

// NewKey makes a VAPID key.
func NewKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

// EncodeKey is key's private scalar, base64url, as the database keeps it.
func EncodeKey(key *ecdsa.PrivateKey) string {
	b, _ := key.Bytes() // a key from NewKey or DecodeKey is valid

	return b64.EncodeToString(b)
}

// DecodeKey reads a key EncodeKey wrote.
func DecodeKey(s string) (*ecdsa.PrivateKey, error) {
	b, err := b64.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("vapid key: %w", err)
	}

	key, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), b)
	if err != nil {
		return nil, fmt.Errorf("vapid key: %w", err)
	}

	return key, nil
}

// PublicKey is key's public half as a browser's applicationServerKey wants it:
// the uncompressed point, base64url.
func PublicKey(key *ecdsa.PrivateKey) string {
	b, _ := key.PublicKey.Bytes() // a key from NewKey or DecodeKey is valid

	return b64.EncodeToString(b)
}
