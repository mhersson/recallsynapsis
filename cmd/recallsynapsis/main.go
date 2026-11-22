// Package main is the recallsynapsis command: it dispatches the first argument
// to the verb registered under that name and turns the result into an exit code.
package main

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"os/signal"
	"slices"
	"syscall"
)

// command is one verb; each verb registers itself from an init function.
type command struct {
	summary string
	run     func(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error
}

var commands = map[string]command{}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)

		return 2
	}

	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		usage(stdout)

		return 0
	}

	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(stderr, "recallsynapsis: unknown command %q\n\n", args[0])
		usage(stderr)

		return 2
	}

	signals := []os.Signal{os.Interrupt, syscall.SIGTERM}
	// A hangup ends a verb as Ctrl-C does, unless nohup ignores it: Notify would undo that.
	if !signal.Ignored(syscall.SIGHUP) {
		signals = append(signals, syscall.SIGHUP)
	}

	ctx, stop := signal.NotifyContext(context.Background(), signals...)
	defer stop()
	// Restore default signal handling after the first signal, so a second
	// interrupt stops a verb that ignores ctx.
	context.AfterFunc(ctx, stop)

	if err := cmd.run(ctx, args[1:], stdin, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "recallsynapsis: %v\n", err)

		return 1
	}

	return 0
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: recallsynapsis <command> [arguments]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "commands:")

	for _, name := range slices.Sorted(maps.Keys(commands)) {
		fmt.Fprintf(w, "  %-12s %s\n", name, commands[name].summary)
	}
}
