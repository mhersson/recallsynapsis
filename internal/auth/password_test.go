package auth_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/auth"
)

func TestPasswordRoundTrip(t *testing.T) {
	t.Parallel()

	hash, err := auth.HashPassword("correct horse battery staple")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(hash, "$argon2id$v=19$m=65536,t=3,p=4$"))

	ok, err := auth.VerifyPassword("correct horse battery staple", hash)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = auth.VerifyPassword("wrong horse battery staple", hash)
	require.NoError(t, err)
	assert.False(t, ok)

	again, err := auth.HashPassword("correct horse battery staple")
	require.NoError(t, err)
	assert.NotEqual(t, hash, again, "each hash has its own salt")
}

func TestAShortPasswordIsRefused(t *testing.T) {
	t.Parallel()

	_, err := auth.HashPassword("elevenchars")
	require.Error(t, err)

	// Twelve characters of three bytes each: the minimum counts characters.
	_, err = auth.HashPassword("日本語日本語日本語日本語")
	require.NoError(t, err)

	_, err = auth.HashPassword("日本語日本語日本語日本")
	require.Error(t, err)
}

func TestOnlyThisBinarysHashesAreAccepted(t *testing.T) {
	t.Parallel()

	hash, err := auth.HashPassword("correct horse battery staple")
	require.NoError(t, err)

	salt, key, _ := strings.Cut(strings.TrimPrefix(hash, "$argon2id$v=19$m=65536,t=3,p=4$"), "$")

	for _, encoded := range []string{
		"",
		"$argon2i$v=19$m=65536,t=3,p=4$" + salt + "$" + key,
		"$argon2id$v=18$m=65536,t=3,p=4$" + salt + "$" + key,
		"$argon2id$v=19$m=4096,t=3,p=4$" + salt + "$" + key,
		"$argon2id$v=19$m=4194304,t=3,p=4$" + salt + "$" + key,
		"$argon2id$v=19$m=65536,t=99999,p=4$" + salt + "$" + key,
		"$argon2id$v=19$m=65536,t=3,p=1$" + salt + "$" + key,
		"$argon2id$v=19$m=65536,t=3,p=4$" + salt,
		"$argon2id$v=19$m=65536,t=3,p=4$$" + key,
		"$argon2id$v=19$m=65536,t=3,p=4$!!!$" + key,
		"$argon2id$v=19$m=65536,t=3,p=4$" + salt + "$" + key[:2],
		"$argon2id$v=19$m=65536,t=3,p=4$" + salt + "$" + key + "$",
		hash + "\n",
		hash[:40] + "\r\n" + hash[40:],
	} {
		ok, err := auth.VerifyPassword("correct horse battery staple", encoded)
		assert.False(t, ok, encoded)
		require.ErrorIs(t, err, auth.ErrInvalidHash, encoded)
	}
}
