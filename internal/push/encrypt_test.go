package push_test

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/push"
)

// RFC 8291 appendix A, base64url.
const (
	rfcPlaintext     = "V2hlbiBJIGdyb3cgdXAsIEkgd2FudCB0byBiZSBhIHdhdGVybWVsb24"
	rfcServerPrivate = "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"
	rfcAgentPublic   = "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"
	rfcAgentPrivate  = "q1dXpw3UpT5VOmu_cf_v6ih07Aems3njxI-JWgLcM94"
	rfcSalt          = "DGv6ra1nlYgDCS1FRnbzlw"
	rfcAuth          = "BTBZMqHH6r4Tts7J_aSIgg"
	rfcHeader        = "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A8"
	rfcCiphertext    = "8pfeW0KbunFT06SuDKoJH9Ql87S1QUrdirN6GcG7sFz1y1sqLgVi1VhjVkHsUoEsbI_0LpXMuGvnzQ"
)

func TestEncryptRFC8291Vector(t *testing.T) {
	server, err := ecdh.P256().NewPrivateKey(unb64(t, rfcServerPrivate))
	require.NoError(t, err)

	agent, err := ecdh.P256().NewPrivateKey(unb64(t, rfcAgentPrivate))
	require.NoError(t, err)
	require.Equal(t, unb64(t, rfcAgentPublic), agent.PublicKey().Bytes())

	got, err := push.Encrypt(unb64(t, rfcAgentPublic), unb64(t, rfcAuth), unb64(t, rfcPlaintext), unb64(t, rfcSalt), server)
	require.NoError(t, err)
	assert.Equal(t, append(unb64(t, rfcHeader), unb64(t, rfcCiphertext)...), got)

	b := browser{key: agent, auth: unb64(t, rfcAuth)}
	assert.Equal(t, unb64(t, rfcPlaintext), b.open(t, got))
}

func TestSealRoundTrip(t *testing.T) {
	b := newBrowser(t)
	payload := []byte(`{"title":"Ready for review · cavu","body":"Fix the sign-in page"}`)

	first, err := push.Seal(b.key.PublicKey().Bytes(), b.auth, payload)
	require.NoError(t, err)

	second, err := push.Seal(b.key.PublicKey().Bytes(), b.auth, payload)
	require.NoError(t, err)

	assert.Equal(t, payload, b.open(t, first))
	assert.Equal(t, payload, b.open(t, second))
	assert.NotEqual(t, first[:16], second[:16], "a fresh salt")
	assert.NotEqual(t, first[21:86], second[21:86], "a fresh server key")
}

// browser holds the private half of a subscription, as a browser keeps it.
type browser struct {
	key  *ecdh.PrivateKey
	auth []byte
}

func newBrowser(t *testing.T) browser {
	t.Helper()

	key, err := ecdh.P256().GenerateKey(rand.Reader)
	require.NoError(t, err)

	auth := make([]byte, 16)
	_, _ = rand.Read(auth)

	return browser{key: key, auth: auth}
}

// open decrypts an aes128gcm body as the browser does (RFC 8291 section 3.4).
func (b browser) open(t *testing.T, body []byte) []byte {
	t.Helper()
	require.Greater(t, len(body), 21)

	salt, idlen := body[:16], int(body[20])
	require.Equal(t, uint32(4096), binary.BigEndian.Uint32(body[16:20]), "record size")
	require.Greater(t, len(body), 21+idlen)

	keyid, record := body[21:21+idlen], body[21+idlen:]

	server, err := ecdh.P256().NewPublicKey(keyid)
	require.NoError(t, err)

	secret, err := b.key.ECDH(server)
	require.NoError(t, err)

	ikm, err := hkdf.Key(sha256.New, secret, b.auth, "WebPush: info\x00"+string(b.key.PublicKey().Bytes())+string(keyid), 32)
	require.NoError(t, err)

	cek, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	require.NoError(t, err)

	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	require.NoError(t, err)

	block, err := aes.NewCipher(cek)
	require.NoError(t, err)

	gcm, err := cipher.NewGCM(block)
	require.NoError(t, err)

	plain, err := gcm.Open(nil, nonce, record, nil)
	require.NoError(t, err)
	require.NotEmpty(t, plain)
	require.Equal(t, byte(2), plain[len(plain)-1], "padding delimiter")

	return plain[:len(plain)-1]
}

func unb64(t *testing.T, s string) []byte {
	t.Helper()

	b, err := base64.RawURLEncoding.DecodeString(s)
	require.NoError(t, err)

	return b
}
