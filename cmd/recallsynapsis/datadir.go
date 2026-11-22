package main

import (
	"fmt"
	"os"
	"path/filepath"
)

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
