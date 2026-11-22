package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runVerb(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer

	code = run(args, bytes.NewReader(nil), &out, &errOut)

	return code, out.String(), errOut.String()
}

func register(t *testing.T, name string, fn func(context.Context, []string, io.Reader, io.Writer, io.Writer) error) {
	t.Helper()

	commands[name] = command{summary: "test verb", run: fn}

	t.Cleanup(func() { delete(commands, name) })
}

func TestVersion(t *testing.T) {
	code, stdout, stderr := runVerb("version")

	assert.Equal(t, 0, code)
	assert.Equal(t, "recallsynapsis dev\n", stdout)
	assert.Empty(t, stderr)
}

func TestUnknownVerb(t *testing.T) {
	code, stdout, stderr := runVerb("frobnicate")

	assert.Equal(t, 2, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "frobnicate")
}

func TestNoArguments(t *testing.T) {
	code, stdout, stderr := runVerb()

	assert.Equal(t, 2, code)
	assert.Empty(t, stdout)

	require.NotEmpty(t, commands)

	for name := range commands {
		assert.Contains(t, stderr, name)
	}
}

func TestHelpExitsZero(t *testing.T) {
	for _, arg := range []string{"help", "-h", "--help"} {
		t.Run(arg, func(t *testing.T) {
			code, stdout, stderr := runVerb(arg)

			assert.Equal(t, 0, code)
			assert.Empty(t, stderr)

			for name := range commands {
				assert.Contains(t, stdout, name)
			}
		})
	}
}

func TestVerbGetsItsArgumentsAndStreams(t *testing.T) {
	register(t, "echo", func(_ context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
		in, err := io.ReadAll(stdin)
		if err != nil {
			return err
		}

		_, err = fmt.Fprintf(stdout, "%q %s", args, in)
		if err != nil {
			return err
		}

		_, err = io.WriteString(stderr, "note")

		return err
	})

	var out, errOut bytes.Buffer

	code := run([]string{"echo", "a", "b"}, strings.NewReader("in"), &out, &errOut)

	assert.Equal(t, 0, code)
	assert.Equal(t, `["a" "b"] in`, out.String())
	assert.Equal(t, "note", errOut.String())
}

func TestVerbErrorExitsOne(t *testing.T) {
	register(t, "failing", func(context.Context, []string, io.Reader, io.Writer, io.Writer) error {
		return errors.New("boom")
	})

	code, stdout, stderr := runVerb("failing")

	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Equal(t, "recallsynapsis: boom\n", stderr)
}

// SIGHUP is left out: under nohup it is ignored on purpose, and the verb would wait.
func TestSignalEndsTheVerbsContext(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			register(t, "wait", func(ctx context.Context, _ []string, _ io.Reader, _, _ io.Writer) error {
				if err := syscall.Kill(syscall.Getpid(), sig); err != nil {
					return err
				}

				select {
				case <-ctx.Done():
					return nil
				case <-time.After(5 * time.Second):
					return errors.New("the context outlived the signal")
				}
			})

			code, _, stderr := runVerb("wait")

			assert.Equal(t, 0, code, stderr)
		})
	}
}
