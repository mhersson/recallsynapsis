package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/mhersson/recallsynapsis/internal/api"
	"github.com/mhersson/recallsynapsis/internal/config"
	"github.com/mhersson/recallsynapsis/internal/store"
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

	dirFlag := flags.String("data-dir", "", "the data directory (default $RECALLSYNAPSIS_DATA_DIR, else $XDG_DATA_HOME/recallsynapsis, else ~/.local/share/recallsynapsis)")
	configFlag := flags.String("config", "", "the configuration file (default <data-dir>/recallsynapsis.toml)")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}

		return err
	}

	if flags.NArg() > 0 {
		return errors.New("serve takes no arguments; name the data directory with --data-dir")
	}

	dir, err := dataDir(*dirFlag)
	if err != nil {
		return err
	}

	cfg, err := config.Load(cmp.Or(*configFlag, filepath.Join(dir, "recallsynapsis.toml")))
	if err != nil {
		return err
	}

	if err := privateDataDir(dir); err != nil {
		return err
	}

	log := slog.New(slog.NewTextHandler(stderr, nil))
	slog.SetDefault(log)

	st, err := store.Open(ctx, filepath.Join(dir, "recallsynapsis.db"), nil)
	if err != nil {
		return err
	}

	// The store closes last, once every request has ended or been cut.
	defer func() {
		if err := st.Close(); err != nil {
			log.Warn("the database did not close cleanly", "err", err)
		}
	}()

	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	log.Info("listening", "address", ln.Addr().String(), "public_url", cfg.PublicURL, "data_dir", dir, "version", version)

	return serve(ctx, ln, logRequests(web.Handler(api.New(api.Deps{Version: version})), log), shutdownGrace, log)
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
