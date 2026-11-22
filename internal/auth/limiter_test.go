package auth

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLimiter() (*limiter, *time.Time) {
	clock := time.Unix(1000, 0)

	return newLimiter(func() time.Time { return clock }), &clock
}

func TestTheLimiterBacksOffFromTheThirdFailure(t *testing.T) {
	t.Parallel()

	l, clock := testLimiter()
	key := "ada|192.0.2.1"

	for range 2 {
		l.failure(key)

		_, ok := l.allow(key)
		require.True(t, ok)
	}

	l.failure(key)

	retry, ok := l.allow(key)
	require.False(t, ok)
	assert.Equal(t, time.Second, retry)

	*clock = clock.Add(time.Second)

	_, ok = l.allow(key)
	require.True(t, ok)

	l.failure(key)

	retry, ok = l.allow(key)
	require.False(t, ok)
	assert.Equal(t, 2*time.Second, retry)
}

func TestTheLimiterCapsAtFiveMinutes(t *testing.T) {
	t.Parallel()

	l, _ := testLimiter()

	for range 100 {
		l.failure("k")
	}

	retry, _ := l.allow("k")
	assert.Equal(t, 5*time.Minute, retry)
}

func TestTheLimiterResetsAndKeepsKeysApart(t *testing.T) {
	t.Parallel()

	l, _ := testLimiter()

	for range 5 {
		l.failure("a")
		l.failure("b")
	}

	l.reset("a")

	_, ok := l.allow("a")
	assert.True(t, ok)

	_, ok = l.allow("b")
	assert.False(t, ok)

	_, ok = l.allow("c")
	assert.True(t, ok)
}

func TestTheLimiterPrunesIdleEntries(t *testing.T) {
	t.Parallel()

	l, clock := testLimiter()

	for i := range limiterPruneAbove + 100 {
		l.failure(fmt.Sprintf("user%d|ip", i))
	}

	*clock = clock.Add(2 * time.Hour)

	l.failure("trigger|prune")

	assert.Len(t, l.entries, 1)
}
