package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

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
