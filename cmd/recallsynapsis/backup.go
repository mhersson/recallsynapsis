package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mhersson/recallsynapsis/internal/backup"
	"github.com/mhersson/recallsynapsis/internal/module"
)

func init() {
	commands["backup"] = command{
		summary: "make one backup now, into backup.dir or the directory --to names",
		run:     runBackup,
	}
}

// runBackup may run while serve does: a snapshot is a read, which SQLite lets
// run beside the writer.
func runBackup(ctx context.Context, args []string, _ io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("recallsynapsis backup", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: recallsynapsis backup [--to DIR] [--data-dir DIR] [--config FILE]")
		flags.PrintDefaults()
	}

	to := flags.String("to", "", "the directory to back up into, kept to backup.keep snapshots as backup.dir is (default backup.dir)")
	dirFlag, configFlag := dataFlags(flags)

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}

		return err
	}

	if flags.NArg() > 0 {
		return errors.New("backup takes no arguments; name the directory with --to")
	}

	dir, err := dataDir(*dirFlag)
	if err != nil {
		return err
	}

	// Opening a data directory with no database makes an empty one, whose
	// snapshot would replace the day's real one.
	if _, err := os.Stat(filepath.Join(dir, "recallsynapsis.db")); err != nil {
		return fmt.Errorf("no database to back up: %w", err)
	}

	d, err := openData(ctx, dir, *configFlag, func(context.Context, module.Change) {})
	if err != nil {
		return err
	}
	defer d.store.Close()

	into := cmp.Or(*to, d.cfg.Backup.Dir)
	if into == "" {
		return errors.New("there is no backup directory: set backup.dir in the configuration, or name one with --to")
	}

	b, err := backup.New(d.store.DB, d.dir, d.cfg)
	if err != nil {
		return err
	}

	path, err := b.Run(ctx, into)
	if err != nil {
		return err
	}

	fmt.Fprintln(stdout, path)

	return nil
}
