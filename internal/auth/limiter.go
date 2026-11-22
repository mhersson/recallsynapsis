package auth

import "time"

const (
	limiterFreeFailures = 3
	limiterMaxShift     = 9 // 1s<<9 is past the cap, and a capped shift never overflows
	limiterMaxBlock     = 5 * time.Minute
	limiterPruneAfter   = time.Hour
	limiterPruneAbove   = 1024
)

// limiter blocks a sign-in key after repeated failures, with exponential
// backoff. It is in memory on purpose: a restart forgets it. Its caller holds
// Service.mu, which also makes each check and its count one step.
type limiter struct {
	entries map[string]*limiterEntry
	now     func() time.Time
}

type limiterEntry struct {
	failures     int
	blockedUntil time.Time
	lastActivity time.Time
}

func newLimiter(now func() time.Time) *limiter {
	return &limiter{entries: map[string]*limiterEntry{}, now: now}
}

// allow reports whether an attempt for key may proceed, or how long until one may.
func (l *limiter) allow(key string) (time.Duration, bool) {
	e, ok := l.entries[key]
	if !ok {
		return 0, true
	}

	now := l.now()
	if now.Before(e.blockedUntil) {
		return e.blockedUntil.Sub(now), false
	}

	return 0, true
}

// failure records a failed attempt. From the third failure on, the key is
// blocked for 2^(failures-3) seconds, up to five minutes.
func (l *limiter) failure(key string) {
	now := l.now()
	l.prune(now)

	e, ok := l.entries[key]
	if !ok {
		e = &limiterEntry{}
		l.entries[key] = e
	}

	e.failures++
	e.lastActivity = now

	if e.failures >= limiterFreeFailures {
		shift := min(e.failures-limiterFreeFailures, limiterMaxShift)
		e.blockedUntil = now.Add(min(time.Second<<shift, limiterMaxBlock))
	}
}

// reset forgets key after a sign-in that succeeded.
func (l *limiter) reset(key string) {
	delete(l.entries, key)
}

// prune drops idle entries once the map is large, which bounds memory under
// many failed sign-ins with varied names.
func (l *limiter) prune(now time.Time) {
	if len(l.entries) <= limiterPruneAbove {
		return
	}

	for k, e := range l.entries {
		if now.Sub(e.lastActivity) > limiterPruneAfter && now.After(e.blockedUntil) {
			delete(l.entries, k)
		}
	}
}
