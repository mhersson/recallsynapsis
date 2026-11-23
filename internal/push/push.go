// Package push sends each user's notifications to every browser in which they
// turned them on, as Web Push with no push library: it encrypts a message for
// a browser's subscription (RFC 8291) and posts it, signed with the server's
// key (RFC 8292), to the push service the subscription names. It forgets a
// browser its push service says is gone, and keeps on a browser's row how the
// last message to it failed, until one goes through. It owns the tables
// push_subscriptions and push_key.
package push

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

// migrations make the module's schema. A browser's id is AUTOINCREMENT, so the
// id a page keeps of a browser that was removed never comes to name another.
// The key is one row, the signing key EncodeKey gives.
var migrations = []store.Migration{{Version: 1, SQL: `
CREATE TABLE push_subscriptions (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	endpoint   TEXT NOT NULL UNIQUE,
	p256dh     TEXT NOT NULL,
	auth       TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	failed_at  INTEGER,
	failure    TEXT NOT NULL DEFAULT ''
);

CREATE INDEX push_subscriptions_user ON push_subscriptions (user_id);

CREATE TABLE push_key (
	id  INTEGER PRIMARY KEY CHECK (id = 1),
	key TEXT NOT NULL
);
`}}

const name = "push"

// changed is what an open page hears of a change of a browser.
var changed = module.Change{Module: name, Kind: "browser"}

type push struct {
	module.Deps
	client *http.Client
	mu     sync.Mutex
	signer *Signer // nil until the key is first needed
}

// New builds the push module.
func New(d module.Deps) module.Module {
	return build(d, NewClient())
}

func build(d module.Deps, client *http.Client) module.Module {
	p := &push{Deps: d, client: client}

	return module.Module{
		Name:       name,
		Migrations: migrations,
		Deliver:    p.deliver,
		Routes: []module.Route{
			{Pattern: "GET /api/push", Access: module.Owner, Serve: p.serveGet},
			{Pattern: "PUT /api/push", Access: module.Owner, Serve: p.serveRegister},
			{Pattern: "DELETE /api/push/{id}", Access: module.Owner, Serve: p.serveRemove},
			{Pattern: "DELETE /api/push", Access: module.Owner, Serve: p.serveRemoveAll},
		},
	}
}

// sign gives the signer of the server's key, made and kept the first time it
// is needed. The insert keeps a row already there, so a server that made one
// at the same moment reads the key kept first, as this one does.
func (p *push) sign(ctx context.Context) (*Signer, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.signer != nil {
		return p.signer, nil
	}

	fresh, err := NewKey()
	if err != nil {
		return nil, fmt.Errorf("push: make the key: %w", err)
	}

	if _, err := p.Store.ExecContext(ctx, `INSERT INTO push_key (id, key) VALUES (1, ?) ON CONFLICT DO NOTHING`, EncodeKey(fresh)); err != nil {
		return nil, fmt.Errorf("push: keep the key: %w", err)
	}

	var kept string
	if err := p.Store.QueryRowContext(ctx, `SELECT key FROM push_key`).Scan(&kept); err != nil {
		return nil, fmt.Errorf("push: read the key: %w", err)
	}

	key, err := DecodeKey(kept)
	if err != nil {
		return nil, err
	}

	p.signer = NewSigner(key, p.Now)

	return p.signer, nil
}

// browser is one of the caller's browsers, as GET /api/push gives it.
type browser struct {
	ID       int64      `json:"id"`
	FailedAt *time.Time `json:"failed_at"` // null when the last message went through
	Failure  string     `json:"failure"`   // how it failed, naming the push service's host
}

// serveGet answers the server's public key, in how many of the caller's
// browsers notifications are on, and the browser ?browser= names, null when
// it is none of the caller's.
func (p *push) serveGet(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	c, _ := module.CallerFrom(ctx)

	var out struct {
		Key     string   `json:"key"`
		On      int      `json:"on"`
		Browser *browser `json:"browser"`
	}

	if v := r.URL.Query().Get("browser"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return &module.Refusal{Status: http.StatusBadRequest, Text: "browser is the id of a browser"}
		}

		b := browser{ID: id}

		var failed sql.NullInt64

		err = p.Store.QueryRowContext(ctx, `SELECT failed_at, failure FROM push_subscriptions WHERE id = ? AND user_id = ?`,
			id, c.UserID).Scan(&failed, &b.Failure)

		switch {
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return fmt.Errorf("push: read a browser: %w", err)
		default:
			if failed.Valid {
				at := time.UnixMilli(failed.Int64).UTC()
				b.FailedAt = &at
			}

			out.Browser = &b
		}
	}

	if err := p.Store.QueryRowContext(ctx, `SELECT count(*) FROM push_subscriptions WHERE user_id = ?`, c.UserID).Scan(&out.On); err != nil {
		return fmt.Errorf("push: count the browsers: %w", err)
	}

	s, err := p.sign(ctx)
	if err != nil {
		return err
	}

	out.Key = PublicKey(s.key)
	module.Reply(w, http.StatusOK, out)

	return nil
}

// serveRegister registers the browser of the subscription
// {"endpoint","keys":{"p256dh","auth"}} for the caller and answers {"id"}. A
// browser registered again, by any user, keeps its id and becomes the caller's.
func (p *push) serveRegister(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256DH string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
	}

	if err := module.Decode(w, r, &in); err != nil {
		return err
	}

	sub := Subscription{Endpoint: in.Endpoint, P256DH: in.Keys.P256DH, Auth: in.Keys.Auth}
	if err := Check(sub); err != nil {
		return &module.Refusal{Status: http.StatusBadRequest, Text: err.Error()}
	}

	ctx := r.Context()
	c, _ := module.CallerFrom(ctx)

	var id int64
	if err := p.Store.QueryRowContext(ctx, `INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth, created_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (endpoint) DO UPDATE SET user_id = excluded.user_id, p256dh = excluded.p256dh, auth = excluded.auth
		RETURNING id`, c.UserID, sub.Endpoint, sub.P256DH, sub.Auth, p.Now().UnixMilli()).Scan(&id); err != nil {
		return fmt.Errorf("push: register a browser: %w", err)
	}

	p.Changed(ctx, changed)
	module.Reply(w, http.StatusOK, map[string]int64{"id": id})

	return nil
}

// serveRemove turns the caller's browser of the id in the path off.
func (p *push) serveRemove(w http.ResponseWriter, r *http.Request) error {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return fmt.Errorf("browser %q: %w", r.PathValue("id"), store.ErrNotFound)
	}

	ctx := r.Context()
	c, _ := module.CallerFrom(ctx)

	res, err := p.Store.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE id = ? AND user_id = ?`, id, c.UserID)
	if err != nil {
		return fmt.Errorf("push: remove a browser: %w", err)
	}

	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return fmt.Errorf("browser %d: %w", id, store.ErrNotFound)
	}

	p.Changed(ctx, changed)
	w.WriteHeader(http.StatusNoContent)

	return nil
}

// serveRemoveAll turns every browser of the caller off.
func (p *push) serveRemoveAll(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	c, _ := module.CallerFrom(ctx)

	if _, err := p.Store.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE user_id = ?`, c.UserID); err != nil {
		return fmt.Errorf("push: remove the browsers: %w", err)
	}

	p.Changed(ctx, changed)
	w.WriteHeader(http.StatusNoContent)

	return nil
}

// target is a browser a message goes to.
type target struct {
	id     int64
	sub    Subscription
	failed bool // whether the last message to it failed
}

// targets are the user's browsers.
func (p *push) targets(ctx context.Context, userID int64) ([]target, error) {
	rows, err := p.Store.QueryContext(ctx, `SELECT id, endpoint, p256dh, auth, failed_at IS NOT NULL FROM push_subscriptions
		WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, fmt.Errorf("push: list the browsers: %w", err)
	}
	defer rows.Close()

	var out []target

	for rows.Next() {
		var t target
		if err := rows.Scan(&t.id, &t.sub.Endpoint, &t.sub.P256DH, &t.sub.Auth, &t.failed); err != nil {
			return nil, fmt.Errorf("push: list the browsers: %w", err)
		}

		out = append(out, t)
	}

	return out, rows.Err()
}

// deliver sends m to every browser of the user, one after the other, and
// returns once each was tried or ctx ended. A message waits at a push service
// until the day ends in the configured zone. A browser its push service says
// is gone is forgotten; any other failure is logged and kept on the browser's
// row until a message to it goes through. A failed message is not sent again.
func (p *push) deliver(ctx context.Context, userID int64, m module.Message) {
	targets, err := p.targets(ctx, userID)

	var signer *Signer
	if err == nil && len(targets) > 0 {
		signer, err = p.sign(ctx)
	}

	if err != nil {
		if ctx.Err() == nil {
			slog.ErrorContext(ctx, "push: deliver", "user", userID, "err", err)
		}

		return
	}

	now := p.Now().In(p.Zone)
	y, mo, d := now.Date()
	sender := Sender{Client: p.client, Signer: signer, Contact: p.PublicURL}
	ttl := time.Date(y, mo, d+1, 0, 0, 0, 0, p.Zone).Sub(now)
	tell := false

	for _, t := range targets {
		err := sender.Send(ctx, t.sub, m, ttl)
		if ctx.Err() != nil {
			break
		}

		var (
			query string
			args  []any
		)

		f, failed := errors.AsType[*Failure](err)

		switch {
		case errors.Is(err, ErrGone):
			query, args = `DELETE FROM push_subscriptions WHERE id = ?`, []any{t.id}
		case failed:
			slog.WarnContext(ctx, "push: a message did not go through", "browser", t.id, "err", err)

			query, args = `UPDATE push_subscriptions SET failed_at = ?, failure = ? WHERE id = ?`, []any{p.Now().UnixMilli(), f.Line, t.id}
		case err != nil:
			slog.ErrorContext(ctx, "push: send", "browser", t.id, "err", err)

			continue
		case t.failed:
			query, args = `UPDATE push_subscriptions SET failed_at = NULL, failure = '' WHERE id = ?`, []any{t.id}
		default:
			continue
		}

		if _, err := p.Store.ExecContext(ctx, query, args...); err != nil {
			slog.ErrorContext(ctx, "push: keep how a message went", "browser", t.id, "err", err)

			continue
		}

		tell = true
	}

	// The change is the user's, whoever sent the message.
	if tell {
		p.Changed(module.WithCaller(ctx, module.Caller{UserID: userID}), changed)
	}
}
