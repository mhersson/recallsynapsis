package backup

import (
	"context"
	"log/slog"
	"time"
)

// Nightly backs up into backup.dir every day at backup.at in the configured
// zone, and logs how each backup went, until ctx ends.
func (b *Backup) Nightly(ctx context.Context, log *slog.Logger) {
	// Settings shows a directory every backup is refused as one not backed up
	// yet, so the log says so at start, not first at night.
	if err := privateDir(b.dir); err != nil {
		log.Error("every backup will fail", "err", err)
	}

	for {
		now := b.now().In(b.zone)
		if b.sleep(ctx, next(now, b.at).Sub(now)) != nil {
			return
		}

		// A backup the stop cut short did not fail.
		if path, err := b.Run(ctx, b.dir); err == nil {
			log.Info("backed up", "snapshot", path)
		} else if ctx.Err() == nil {
			log.Error("the backup failed", "err", err)
		}
	}
}

// next is the first time after now at at's hour and minute in now's zone. It is
// found from each day, not by adding 24 hours, so a day the clock is set forward
// or back has one too: time.Date gives a time of day the clock skips or repeats
// as the same one instant each time.
func next(now, at time.Time) time.Time {
	y, m, d := now.Date()

	for ; ; d++ {
		t := time.Date(y, m, d, at.Hour(), at.Minute(), 0, 0, now.Location())

		// West of UTC a time the clock skips at midnight comes out on the day
		// before; the moment the clock jumps stands in for it.
		if noon := time.Date(y, m, d, 12, 0, 0, 0, now.Location()); t.Day() != noon.Day() && t.Before(noon) {
			_, t = t.ZoneBounds()
		}

		if t.After(now) {
			return t
		}
	}
}

// sleep waits d, or until ctx ends.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
