package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"charm.land/fang/v2"
	"github.com/evanmschultz/valv/internal/cli"
	"github.com/evanmschultz/valv/internal/config"
	"github.com/spf13/cobra"
)

var signalNotifyContext = signal.NotifyContext

func main() {
	ctx, stop := newMainContext()
	defer stop()
	os.Exit(realMain(ctx, os.Stdout, os.Stderr))
}

func newMainContext() (context.Context, context.CancelFunc) {
	return signalNotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
}

func realMain(ctx context.Context, stdout, stderr io.Writer) int {
	if err := run(ctx, stdout, stderr); err != nil {
		return 1
	}
	return 0
}

func run(ctx context.Context, stdout, stderr io.Writer) error {
	cmd, err := buildRootCommand(ctx, stdout, stderr)
	if err != nil {
		return err
	}
	if err := fang.Execute(
		ctx,
		cmd,
		fang.WithoutManpage(),
		fang.WithoutCompletions(),
		fang.WithoutVersion(),
		fang.WithErrorHandler(renderFangError),
	); err != nil {
		return err
	}
	return nil
}

func buildRootCommand(ctx context.Context, stdout, stderr io.Writer) (*cobra.Command, error) {
	if homeDir := strings.TrimSpace(os.Getenv("VALV_TEST_HOME_DIR")); homeDir != "" {
		paths, err := config.ResolvePaths(homeDir)
		if err != nil {
			return nil, fmt.Errorf("resolve test override paths: %w", err)
		}
		return cli.NewRootCommandWithPaths(ctx, stdout, stderr, paths)
	}
	return cli.NewRootCommand(ctx, stdout, stderr)
}

func renderFangError(w io.Writer, styles fang.Styles, err error) {
	if err == nil {
		return
	}
	if _, writeErr := fmt.Fprintln(w, styles.ErrorHeader.String()); writeErr != nil {
		return
	}
	lineStyle := styles.ErrorText.UnsetTransform()
	for _, line := range strings.Split(strings.TrimRight(err.Error(), "\n"), "\n") {
		if _, writeErr := fmt.Fprintln(w, lineStyle.Render(line)); writeErr != nil {
			return
		}
	}
}
