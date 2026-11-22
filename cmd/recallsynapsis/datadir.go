package main

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/mhersson/recallsynapsis/internal/auth"
	"github.com/mhersson/recallsynapsis/internal/config"
	"github.com/mhersson/recallsynapsis/internal/core"
	"github.com/mhersson/recallsynapsis/internal/store"
)

// migrations are every package's, auth's first: core's areas belong to its users.
var migrations = slices.Concat(auth.Migrations, core.Migrations)

// dataFlags adds --data-dir and --config to flags.
func dataFlags(flags *flag.FlagSet) (dirFlag, configFlag *string) {
	return flags.String("data-dir", "", "the data directory (default $RECALLSYNAPSIS_DATA_DIR, else $XDG_DATA_HOME/recallsynapsis, else ~/.local/share/recallsynapsis)"),
		flags.String("config", "", "the configuration file (default <data-dir>/recallsynapsis.toml)")
}

// openData resolves the data directory, reads the configuration, makes the
// directory private and opens the database with every migration applied.
func openData(ctx context.Context, dirFlag, configFlag string) (string, *config.Config, *store.Store, error) {
	dir, err := dataDir(dirFlag)
	if err != nil {
		return "", nil, nil, err
	}

	cfg, err := config.Load(cmp.Or(configFlag, filepath.Join(dir, "recallsynapsis.toml")))
	if err != nil {
		return "", nil, nil, err
	}

	if err := privateDataDir(dir); err != nil {
		return "", nil, nil, err
	}

	st, err := store.Open(ctx, filepath.Join(dir, "recallsynapsis.db"), migrations)
	if err != nil {
		return "", nil, nil, err
	}

	return dir, cfg, st, nil
}

// dataDir is the data directory: the flag, else RECALLSYNAPSIS_DATA_DIR, else
// $XDG_DATA_HOME/recallsynapsis, else ~/.local/share/recallsynapsis.
func dataDir(flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}

	if dir := os.Getenv("RECALLSYNAPSIS_DATA_DIR"); dir != "" {
		return dir, nil
	}

	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "recallsynapsis"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("data directory: %w", err)
	}

	return filepath.Join(home, ".local", "share", "recallsynapsis"), nil
}

// privateDataDir makes dir 0o700 when absent and refuses one the group or
// others have any permission on: SQLite makes its files with the umask's
// mode, so the directory's mode alone keeps them from the host's other users.
func privateDataDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("data directory: %w", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("data directory: %w", err)
	}

	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("the data directory %s can be read, written or entered by others; run chmod 700 %s", dir, dir)
	}

	return nil
}
