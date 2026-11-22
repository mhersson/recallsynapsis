package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// newToken returns 256 random bits as 43 characters, and their hash. Only the
// hash is ever stored, so a leaked database yields no usable token.
func newToken() (raw, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("auth: generate token: %w", err)
	}

	raw = base64.RawURLEncoding.EncodeToString(buf)

	return raw, hashToken(raw), nil
}

// hashToken returns the hex SHA-256 of a raw token. A token carries 256
// random bits, so a fast hash is enough and a lookup by it reveals nothing.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))

	return hex.EncodeToString(sum[:])
}
