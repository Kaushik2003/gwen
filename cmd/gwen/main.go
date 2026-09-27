// Command gwen is the command-line client for the Gwen daemon
// (docs/08-clients.md#cli).
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/wire"
)

var version = "dev"

// Exit codes (docs/08-clients.md#cli).
const (
	exitOK       = 0
	exitAPI      = 1
	exitUsage    = 2
	exitNoDaemon = 3
)

const notRunning = "Gwen isn't running. Start it with: systemctl --user start gwend"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	c := &cli{
		newAPI: func(socket string) client.API { return client.New(socket) },
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
		clk:    clock.Real(),
		loc:    time.Local,
	}
	os.Exit(c.execute(ctx, os.Args[1:]))
}

// usageError is a mistake in the command line: exit code 2.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func usagef(format string, args ...any) error { return &usageError{fmt.Sprintf(format, args...)} }

// execute runs a command line and returns the exit code.
func (c *cli) execute(ctx context.Context, args []string) int {
	root := newRootCmd(c)
	root.SetArgs(args)
	root.SetIn(c.stdin)
	root.SetOut(c.stdout)
	root.SetErr(c.stderr)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return exitOK
	}
	var (
		ue *usageError
		ae *client.APIError
	)
	switch {
	case errors.Is(err, client.ErrDaemonNotRunning):
		fmt.Fprintln(c.stderr, notRunning)
		return exitNoDaemon
	case errors.As(err, &ue), errors.Is(err, client.ErrAmbiguousID), isCobraUsage(err):
		fmt.Fprintln(c.stderr, "gwen:", err)
		fmt.Fprintln(c.stderr, "Run 'gwen --help' for usage.")
		return exitUsage
	case errors.As(err, &ae):
		msg := ae.Message
		if ae.Code == wire.CodeConflict && ae.Details["field"] == "rev" {
			msg = "Changed elsewhere — reloaded"
		}
		fmt.Fprintln(c.stderr, "gwen:", msg)
		return exitAPI
	case errors.Is(err, context.Canceled):
		return exitOK
	}
	fmt.Fprintln(c.stderr, "gwen:", err)
	return exitAPI
}

// isCobraUsage recognizes the usage errors Cobra itself returns.
func isCobraUsage(err error) bool {
	msg := err.Error()
	for _, prefix := range []string{"unknown command", "unknown flag", "unknown shorthand flag",
		"accepts ", "requires ", "invalid argument", "flag needs an argument", "required flag"} {
		if strings.HasPrefix(msg, prefix) {
			return true
		}
	}
	return false
}
