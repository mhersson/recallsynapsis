package backup

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNextKeepsTheTimeOfDayAcrossTheClockChanges(t *testing.T) {
	t.Parallel()

	at, err := time.Parse("15:04", "03:00")
	require.NoError(t, err)

	// Oslo sets the clock forward on 2026-03-29 and back on 2026-10-25.
	for _, tt := range []struct {
		after, want time.Time
		gap         time.Duration
	}{
		{time.Date(2026, 3, 28, 3, 0, 0, 0, oslo), time.Date(2026, 3, 29, 3, 0, 0, 0, oslo), 23 * time.Hour},
		{time.Date(2026, 10, 24, 3, 0, 0, 0, oslo), time.Date(2026, 10, 25, 3, 0, 0, 0, oslo), 25 * time.Hour},
		{time.Date(2026, 11, 20, 2, 59, 0, 0, oslo), time.Date(2026, 11, 20, 3, 0, 0, 0, oslo), time.Minute},
	} {
		got := next(tt.after, at)
		assert.Equal(t, tt.want, got)
		assert.Equal(t, tt.gap, got.Sub(tt.after))
	}
}

func TestNextIsOnceADay(t *testing.T) {
	t.Parallel()

	// Santiago, Havana and the Azores set the clock forward at midnight.
	zones := []string{"Europe/Oslo", "America/New_York", "America/Santiago", "America/Havana", "Atlantic/Azores", "Australia/Lord_Howe", "UTC"}
	// Among them the times of day the clock skips or repeats when it is set forward or back.
	times := []string{"00:00", "00:30", "01:30", "02:00", "02:30", "03:00", "23:59"}

	for _, name := range zones {
		zone, err := time.LoadLocation(name)
		require.NoError(t, err)

		for _, clock := range times {
			at, err := time.Parse("15:04", clock)
			require.NoError(t, err)

			// As the schedule does: the next is found from when the backup before it ended.
			prev := next(time.Date(2026, 1, 1, 0, 0, 0, 0, zone), at)
			for range 366 {
				got := next(prev.Add(time.Minute), at)

				y, m, d := prev.Date()
				require.Equal(t, time.Date(y, m, d+1, 12, 0, 0, 0, zone).Format(layout), got.Format(layout), "%s %s after %s", name, clock, prev)

				prev = got
			}
		}
	}
}

func TestNightly(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 3)
	f.b.dir = f.dir
	f.set(time.Date(2026, 11, 20, 12, 0, 0, 0, oslo))

	// The schedule's sleep hands the test what it would wait, and waits for the test to wake it.
	slept, woke := make(chan time.Duration), make(chan struct{})
	f.b.sleep = func(ctx context.Context, d time.Duration) error {
		select {
		case slept <- d:
		case <-ctx.Done():
			return ctx.Err()
		}

		select {
		case <-woke:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	var logged bytes.Buffer

	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)

		f.b.Nightly(ctx, slog.New(slog.NewJSONHandler(&logged, nil)))
	}()

	wait := <-slept
	assert.Equal(t, 15*time.Hour, wait, "from noon to three at night")

	// night moves the clock on by the wait and wakes the schedule; the next
	// wait comes once the night's backup is done.
	night := func() {
		f.set(f.now().Add(wait))

		woke <- struct{}{}

		wait = <-slept
		assert.Equal(t, 24*time.Hour, wait)
	}

	night()
	assert.Equal(t, []string{"recallsynapsis-20261121.db"}, names(t, f.dir))

	// A file where the backup's files/ goes fails the backup once its snapshot is written.
	require.NoError(t, os.Mkdir(filepath.Join(f.data, "files"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, "files"), nil, 0o600))
	night()
	assert.Equal(t, []string{"files", "recallsynapsis-20261121.db"}, names(t, f.dir))

	require.NoError(t, os.Remove(filepath.Join(f.dir, "files")))
	night()
	assert.Equal(t, []string{"files", "recallsynapsis-20261121.db", "recallsynapsis-20261123.db"}, names(t, f.dir))

	cancel()

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("the schedule did not stop with its context")
	}

	assert.Equal(t, []string{"INFO", "ERROR", "INFO"}, levels(t, &logged), "each night is logged, the failed one as an error")
}

func TestNightlyLogsARefusedDirectoryAtStart(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		mode   os.FileMode
		levels []string
	}{
		{0o700, nil},
		{0o750, []string{"ERROR"}},
	} {
		f := newFixture(t, 3)
		f.b.dir = f.dir
		require.NoError(t, os.Mkdir(f.dir, tt.mode))

		slept := make(chan struct{})
		f.b.sleep = func(ctx context.Context, _ time.Duration) error {
			close(slept)
			<-ctx.Done()

			return ctx.Err()
		}

		var logged bytes.Buffer

		ctx, cancel := context.WithCancel(t.Context())
		stopped := make(chan struct{})

		go func() {
			defer close(stopped)

			f.b.Nightly(ctx, slog.New(slog.NewJSONHandler(&logged, nil)))
		}()

		<-slept
		cancel()
		<-stopped

		assert.Equal(t, tt.levels, levels(t, &logged), tt.mode)
	}
}

// levels lists the level of each JSON record in logged.
func levels(t *testing.T, logged *bytes.Buffer) []string {
	t.Helper()

	var list []string

	scan := bufio.NewScanner(logged)
	for scan.Scan() {
		var line struct{ Level string }
		require.NoError(t, json.Unmarshal(scan.Bytes(), &line))

		list = append(list, line.Level)
	}

	return list
}

func TestSleepEndsWithItsContext(t *testing.T) {
	t.Parallel()

	require.NoError(t, sleep(t.Context(), time.Millisecond))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.ErrorIs(t, sleep(ctx, time.Hour), context.Canceled)
}
