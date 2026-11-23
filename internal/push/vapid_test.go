package push_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/push"
)

func TestAuthorizationToken(t *testing.T) {
	key, err := push.NewKey()
	require.NoError(t, err)

	now := time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC)
	signer := push.NewSigner(key, func() time.Time { return now })

	header, err := signer.Authorization("https://push.example.net/p/JzLQ3raZ?x=1", "https://cavu.example.org")
	require.NoError(t, err)

	rest, ok := strings.CutPrefix(header, "vapid t=")
	require.True(t, ok, header)

	token, k, ok := strings.Cut(rest, ", k=")
	require.True(t, ok, header)
	assert.Equal(t, push.PublicKey(key), k)

	public, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), unb64(t, k))
	require.NoError(t, err)
	assert.True(t, public.Equal(&key.PublicKey))

	parts := strings.Split(token, ".")
	require.Len(t, parts, 3)
	assert.JSONEq(t, `{"typ":"JWT","alg":"ES256"}`, string(unb64(t, parts[0])))
	assert.JSONEq(t,
		fmt.Sprintf(`{"aud":"https://push.example.net","exp":%d,"sub":"https://cavu.example.org"}`, now.Add(12*time.Hour).Unix()),
		string(unb64(t, parts[1])))

	sig := unb64(t, parts[2])
	require.Len(t, sig, 64)

	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	assert.True(t, ecdsa.Verify(public, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])))
}

func TestAuthorizationReused(t *testing.T) {
	key, err := push.NewKey()
	require.NoError(t, err)

	start := time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC)
	now := start
	signer := push.NewSigner(key, func() time.Time { return now })
	authorization := func(endpoint string) string {
		t.Helper()

		h, err := signer.Authorization(endpoint, "https://cavu.example.org")
		require.NoError(t, err)

		return h
	}

	first := authorization("https://push.example.net/p/one")
	assert.Equal(t, first, authorization("https://push.example.net/p/two"), "same origin")
	assert.NotEqual(t, first, authorization("https://other.example.org/p/one"), "another origin")

	now = start.Add(11*time.Hour - time.Minute)

	assert.Equal(t, first, authorization("https://push.example.net/p/one"))

	now = start.Add(11 * time.Hour)

	assert.NotEqual(t, first, authorization("https://push.example.net/p/one"))
}

func TestKeyRoundTrip(t *testing.T) {
	key, err := push.NewKey()
	require.NoError(t, err)

	got, err := push.DecodeKey(push.EncodeKey(key))
	require.NoError(t, err)
	assert.True(t, key.Equal(got))

	for _, bad := range []string{"not a key", base64.RawURLEncoding.EncodeToString(make([]byte, 32))} {
		_, err = push.DecodeKey(bad)
		require.Error(t, err, bad)
	}
}
