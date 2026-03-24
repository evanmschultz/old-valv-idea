package main

import (
	"context"
	"io"
	"os"

	"charm.land/fang/v2"
	"github.com/evanmschultz/valv/internal/cli"
)

func main() {
	os.Exit(realMain(context.Background(), os.Stdout, os.Stderr))
}

func realMain(ctx context.Context, stdout, stderr io.Writer) int {
	if err := run(ctx, stdout, stderr); err != nil {
		return 1
	}
	return 0
}

func run(ctx context.Context, stdout, stderr io.Writer) error {
	cmd, err := cli.NewRootCommand(ctx, stdout, stderr)
	if err != nil {
		return err
	}
	if err := fang.Execute(ctx, cmd); err != nil {
		return err
	}
	return nil
}
