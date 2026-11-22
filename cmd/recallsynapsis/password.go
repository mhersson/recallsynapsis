package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mhersson/recallsynapsis/internal/auth"
)

func init() {
	commands["password"] = command{
		summary: "set the owner's password from standard input, ending every session, or print its hash with --hash",
		run:     runPassword,
	}
}

const passwordInputAdvice = "write it to a file only you can read with sh -c 'umask 077 && cat > FILE', run recallsynapsis password [--hash] < FILE, then remove FILE"

// runPassword never takes the password as an argument, where the process list
// and the shell's history would show it. It may run while serve does.
func runPassword(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("recallsynapsis password", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: recallsynapsis password [--hash | [--data-dir DIR] [--config FILE]] < FILE")
		flags.PrintDefaults()
	}

	hashFlag := flags.Bool("hash", false, "print the password's hash, for RECALLSYNAPSIS_PASSWORD_HASH, and touch no data directory")
	dirFlag, configFlag := dataFlags(flags)

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}

		return err
	}

	if flags.NArg() > 0 {
		return errors.New("the password is read from standard input, never an argument: " + passwordInputAdvice)
	}

	if isTerminal(stdin) {
		return errors.New("the password is read from standard input, not a terminal: " + passwordInputAdvice)
	}

	data, err := io.ReadAll(stdin)
	if err != nil {
		return fmt.Errorf("standard input: %w", err)
	}

	hash, err := auth.HashPassword(strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r"))
	if err != nil {
		return err
	}

	if *hashFlag {
		fmt.Fprintln(stdout, hash)

		return nil
	}

	_, cfg, st, err := openData(ctx, *dirFlag, *configFlag)
	if err != nil {
		return err
	}
	defer st.Close()

	signin := auth.New(st, time.Now)

	owner, err := signin.EnsureUser(ctx, cfg.Owner)
	if err != nil {
		return err
	}

	if err := signin.SetPasswordHash(ctx, owner, hash); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "The password of %s is set and every session has ended.\n", cfg.Owner)

	return nil
}

// isTerminal reports whether r is a character device, as a terminal is.
func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}

	info, err := f.Stat()

	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
