package push

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"slices"
)

// recordSize is the aes128gcm record size; a push is one record (RFC 8291 section 4).
const recordSize = 4096

// Encrypt encrypts payload for the subscription keys p256dh and auth as one
// aes128gcm record (RFC 8291), with the salt and the server's key given.
func Encrypt(p256dh, auth, payload, salt []byte, server *ecdh.PrivateKey) ([]byte, error) {
	agent, err := ecdh.P256().NewPublicKey(p256dh)
	if err != nil {
		return nil, fmt.Errorf("subscription key: %w", err)
	}

	secret, err := server.ECDH(agent)
	if err != nil {
		return nil, fmt.Errorf("key agreement: %w", err)
	}

	public := server.PublicKey().Bytes()

	ikm, err := hkdf.Key(sha256.New, secret, auth, "WebPush: info\x00"+string(p256dh)+string(public), 32)
	if err != nil {
		return nil, fmt.Errorf("derive the key material: %w", err)
	}

	cek, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, fmt.Errorf("derive the content key: %w", err)
	}

	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, fmt.Errorf("derive the nonce: %w", err)
	}

	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, fmt.Errorf("cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cipher: %w", err)
	}

	// The key id is the server's uncompressed P-256 key, 65 octets.
	header := slices.Concat(salt, binary.BigEndian.AppendUint32(nil, recordSize), []byte{65}, public)

	return gcm.Seal(header, nonce, slices.Concat(payload, []byte{2}), nil), nil
}

// Seal encrypts payload for a subscription with a fresh salt and server key.
func Seal(p256dh, auth, payload []byte) ([]byte, error) {
	server, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("server key: %w", err)
	}

	salt := make([]byte, 16)
	_, _ = rand.Read(salt) // never returns an error

	return Encrypt(p256dh, auth, payload, salt, server)
}
