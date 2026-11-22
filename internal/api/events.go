package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/mhersson/recallsynapsis/internal/module"
)

// backlog is how many changes a stream holds for a client that reads behind
// them; one more ends the stream.
const backlog = 256

// Hub gives each change to the open event streams of the user whose change
// it is, and keeps none: a stream hears only what is changed while it is open.
type Hub struct {
	stop      <-chan struct{}
	keepAlive time.Duration // how often a stream sends a comment line, so a proxy keeps it open
	lifetime  time.Duration // how long a stream lasts, so a revoked token or an ended session is refused when it opens again
	writeWait time.Duration // how long one write may wait on its client, which by then has stopped reading

	mu      sync.Mutex
	streams map[chan module.Change]int64 // each open stream's changes, and its user
}

// NewHub returns a hub whose streams end once stop is closed.
func NewHub(stop <-chan struct{}) *Hub {
	return &Hub{stop: stop, keepAlive: 15 * time.Second, lifetime: time.Hour, writeWait: 30 * time.Second, streams: map[chan module.Change]int64{}}
}

// Changed gives c to the open streams of the caller ctx carries, and waits on
// none: a stream with no room for c ends, since its page would never learn it
// missed c, and reads afresh when it opens the stream again.
func (h *Hub) Changed(ctx context.Context, c module.Change) {
	caller, ok := module.CallerFrom(ctx)
	if !ok {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for ch, user := range h.streams {
		if user != caller.UserID {
			continue
		}

		select {
		case ch <- c:
		default:
			delete(h.streams, ch)
			close(ch)
		}
	}
}

// join opens a stream of the user's changes; leave ends it.
func (h *Hub) join(user int64) (changes <-chan module.Change, leave func()) {
	ch := make(chan module.Change, backlog)

	h.mu.Lock()
	h.streams[ch] = user
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		delete(h.streams, ch)
		h.mu.Unlock()
	}
}

// events streams the caller's changes, one unnamed event each, until the
// client leaves, the stream falls behind, its lifetime ends or the server
// stops. The browser then opens it again by itself.
func (s *server) events(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	changes, leave := s.Hub.join(caller(r).UserID)
	defer leave()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no") // else a proxy such as nginx holds the events back
	w.WriteHeader(http.StatusOK)

	alive := time.NewTicker(s.Hub.keepAlive)
	defer alive.Stop()

	end := time.After(s.Hub.lifetime)
	rc := http.NewResponseController(w)

	// A browser waits seconds of its own choosing before it opens a stream that ended.
	_, err := io.WriteString(w, "retry: 1000\n\n")

	for err == nil {
		// A client that takes no write within writeWait has stopped reading: the
		// failed write ends the stream.
		_ = rc.SetWriteDeadline(time.Now().Add(s.Hub.writeWait))

		if err = rc.Flush(); err != nil {
			break
		}

		select {
		case <-ctx.Done():
			return nil
		case <-s.Hub.stop:
			return nil
		case <-end:
			return nil
		case <-alive.C:
			_, err = io.WriteString(w, ":\n\n")
		case c, ok := <-changes:
			if !ok {
				return nil
			}

			data, _ := json.Marshal(c) // a Change always encodes
			_, err = fmt.Fprintf(w, "data: %s\n\n", data)
		}
	}

	// A failed write ends ctx, so an error while ctx lives is the server's.
	if ctx.Err() == nil {
		slog.ErrorContext(ctx, "request failed", "route", r.Pattern, "err", err)
	}

	return nil
}
