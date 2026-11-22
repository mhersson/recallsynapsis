package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Above the OWASP minimum.
const (
	argonMemory    uint32 = 64 * 1024 // KiB
	argonTime      uint32 = 3
	argonThreads   uint8  = 4
	argonKeyLen    uint32 = 32
	argonSaltLen          = 16
	minPasswordLen        = 12
)

// phcPrefix starts every hash this package accepts. One set of parameters
// makes every verify cost the same, and no hash set from outside can make a
// sign-in cost more.
var phcPrefix = fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$", argon2.Version, argonMemory, argonTime, argonThreads)

// ErrInvalidHash reports a hash, stored or offered, that is not an argon2id
// PHC string with the parameters HashPassword uses. It holds nothing of the hash.
var ErrInvalidHash = errors.New("not an argon2id hash as recallsynapsis password --hash makes one")

// absent is verified when no password can match, so a name no user has and a
// user with no password cost what a wrong password does.
var absent = phc(make([]byte, argonSaltLen), make([]byte, argonKeyLen))

// HashPassword returns the argon2id hash of password as a PHC string, and
// refuses a password shorter than minPasswordLen characters.
func HashPassword(password string) (string, error) {
	if utf8.RuneCountInString(password) < minPasswordLen {
		return "", fmt.Errorf("the password needs at least %d characters", minPasswordLen)
	}

	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: generate salt: %w", err)
	}

	return phc(salt, argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)), nil
}

func phc(salt, key []byte) string {
	return phcPrefix + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key)
}

// VerifyPassword checks password against a PHC string. A wrong password is
// (false, nil); a malformed hash is ErrInvalidHash.
func VerifyPassword(password, encoded string) (bool, error) {
	salt, want, err := parsePHC(encoded)
	if err != nil {
		return false, err
	}

	got := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// parsePHC takes only the exact text phc writes, which refuses other
// parameters, and a newline the decoder would skip and a later start would
// take for a new hash.
func parsePHC(encoded string) (salt, key []byte, err error) {
	saltText, keyText, _ := strings.Cut(strings.TrimPrefix(encoded, phcPrefix), "$")
	salt, _ = base64.RawStdEncoding.DecodeString(saltText)
	key, _ = base64.RawStdEncoding.DecodeString(keyText)

	if len(salt) != argonSaltLen || len(key) != int(argonKeyLen) || phc(salt, key) != encoded {
		return nil, nil, ErrInvalidHash
	}

	return salt, key, nil
}
