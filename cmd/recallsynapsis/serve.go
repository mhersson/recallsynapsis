package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/mhersson/recallsynapsis/internal/api"
	"github.com/mhersson/recallsynapsis/internal/auth"
	"github.com/mhersson/recallsynapsis/internal/core"
	"github.com/mhersson/recallsynapsis/web"
)

func init() {
	commands["serve"] = command{
		summary: "serve the web app and the HTTP API until interrupted",
		run:     runServe,
	}
}

// shutdownGrace is how long open requests get once a signal stops the server.
const shutdownGrace = 5 * time.Second

func runServe(ctx context.Context, args []string, _ io.Reader, _, stderr io.Writer) error {
	flags := flag.NewFlagSet("recallsynapsis serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: recallsynapsis serve [--data-dir DIR] [--config FILE]")
		flags.PrintDefaults()
	}

	dirFlag, configFlag := dataFlags(flags)

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}

		return err
	}

	if flags.NArg() > 0 {
		return errors.New("serve takes no arguments; name the data directory with --data-dir")
	}

	dir, cfg, st, err := openData(ctx, *dirFlag, *configFlag)
	if err != nil {
		return err
	}

	log := slog.New(slog.NewTextHandler(stderr, nil))
	slog.SetDefault(log)

	// The store closes last, once every request has ended or been cut.
	defer func() {
		if err := st.Close(); err != nil {
			log.Warn("the database did not close cleanly", "err", err)
		}
	}()

	signin := auth.New(st, time.Now)

	owner, err := signin.EnsureUser(ctx, cfg.Owner)
	if err != nil {
		return err
	}

	areas := core.New(st)
	if err := areas.EnsurePersonal(ctx, owner); err != nil {
		return err
	}

	// A container is given the hash this way. The error never holds the value.
	if hash := os.Getenv("RECALLSYNAPSIS_PASSWORD_HASH"); hash != "" {
		if err := signin.SetPasswordHash(ctx, owner, hash); err != nil {
			return fmt.Errorf("RECALLSYNAPSIS_PASSWORD_HASH: %w", err)
		}
	}

	has, err := signin.HasPassword(ctx, owner)
	if err != nil {
		return err
	}

	// Sign-in answers a missing password as a wrong one, so the operator learns of it here.
	if !has {
		log.Warn("the owner has no password and cannot sign in; set one with recallsynapsis password", "owner", cfg.Owner)
	}

	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	log.Info("listening", "address", ln.Addr().String(), "public_url", cfg.PublicURL, "data_dir", dir, "version", version)

	h := api.New(api.Deps{Auth: signin, Core: areas, Version: version, PublicURL: cfg.PublicURL, Timezone: cfg.Timezone})

	return serve(ctx, ln, logRequests(web.Handler(h), log), shutdownGrace, log)
}

// serve serves h on ln until ctx ends, then gives the open requests grace to
// end and cuts those still open.
func serve(ctx context.Context, ln net.Listener, h http.Handler, grace time.Duration, log *slog.Logger) error {
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}
	served := make(chan error, 1)

	go func() { served <- srv.Serve(ln) }()

	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}

	gctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), grace)
	defer cancel()

	if err := srv.Shutdown(gctx); err != nil {
		log.Warn("cut the requests still open", "err", err)

		_ = srv.Close()
	}

	return nil
}

// logRequests logs one line per request. It holds no query, body or header,
// where a secret may travel.
func logRequests(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "took", time.Since(start))
	})
}

// statusRecorder keeps the status written. Unwrap lets http.ResponseController
// reach the connection through it.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
