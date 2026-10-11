package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hrodrig/kpm/internal/cli"
	"github.com/hrodrig/kpm/internal/exitcode"
)

func main() {
	os.Exit(runMain())
}

// runMain runs the CLI under a signal-aware context and returns a process
// exit code (see exitcode.Of). SIGINT/SIGTERM cancel the context so a future
// sampler daemon can exit cleanly.
func runMain() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cli.ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return exitcode.Of(err)
	}
	return exitcode.Success
}
