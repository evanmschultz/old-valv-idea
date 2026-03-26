package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/log"
)

type commandFactory func(context.Context, string, ...string) *exec.Cmd

type SystemRunner struct {
	Binary     string
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	Env        []string
	newCommand commandFactory
}

func NewSystemRunner(binary string, stdin io.Reader, stdout, stderr io.Writer) SystemRunner {
	return SystemRunner{
		Binary: binary,
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}
}

func (r SystemRunner) Run(ctx context.Context, args []string) error {
	binary := strings.TrimSpace(r.Binary)
	if binary == "" {
		binary = "docker"
	}

	factory := r.newCommand
	if factory == nil {
		factory = exec.CommandContext
	}

	cmd := factory(ctx, binary, args...)
	cmd.Stdin = r.Stdin
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	if len(r.Env) > 0 {
		cmd.Env = append(os.Environ(), r.Env...)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %s %s: %w", binary, strings.Join(args, " "), err)
	}
	return nil
}

type QuietRunner struct {
	Binary     string
	Env        []string
	Logger     *log.Logger
	newCommand commandFactory
}

func NewQuietRunner(binary string, logger *log.Logger) QuietRunner {
	return QuietRunner{Binary: binary, Logger: logger}
}

func (r QuietRunner) Run(ctx context.Context, args []string) error {
	binary := strings.TrimSpace(r.Binary)
	if binary == "" {
		binary = "docker"
	}

	factory := r.newCommand
	if factory == nil {
		factory = exec.CommandContext
	}

	cmd := factory(ctx, binary, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if len(r.Env) > 0 {
		cmd.Env = append(os.Environ(), r.Env...)
	}

	err := cmd.Run()
	output := strings.TrimSpace(strings.Join([]string{stdout.String(), stderr.String()}, "\n"))
	if err != nil {
		if r.Logger != nil && output != "" {
			r.Logger.Debug("docker command failed", "args", strings.Join(args, " "), "output", truncateDockerOutput(output))
		}
		if len(args) >= 2 && args[0] == "buildx" && args[1] == "version" {
			return fmt.Errorf("run %s %s: docker buildx is required but unavailable: %w", binary, strings.Join(args, " "), err)
		}
		if output != "" {
			return fmt.Errorf("run %s %s: %w: %s", binary, strings.Join(args, " "), err, output)
		}
		return fmt.Errorf("run %s %s: %w", binary, strings.Join(args, " "), err)
	}

	if r.Logger != nil && output != "" {
		r.Logger.Debug("docker command output", "args", strings.Join(args, " "), "output", truncateDockerOutput(output))
	}
	return nil
}

func (r QuietRunner) Output(ctx context.Context, args []string) (string, error) {
	binary := strings.TrimSpace(r.Binary)
	if binary == "" {
		binary = "docker"
	}

	factory := r.newCommand
	if factory == nil {
		factory = exec.CommandContext
	}

	cmd := factory(ctx, binary, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if len(r.Env) > 0 {
		cmd.Env = append(os.Environ(), r.Env...)
	}

	err := cmd.Run()
	combined := strings.TrimSpace(strings.Join([]string{stdout.String(), stderr.String()}, "\n"))
	if err != nil {
		if r.Logger != nil && combined != "" {
			r.Logger.Debug("docker command output failed", "args", strings.Join(args, " "), "output", truncateDockerOutput(combined))
		}
		if combined != "" {
			return "", fmt.Errorf("run %s %s: %w: %s", binary, strings.Join(args, " "), err, combined)
		}
		return "", fmt.Errorf("run %s %s: %w", binary, strings.Join(args, " "), err)
	}

	trimmed := strings.TrimSpace(stdout.String())
	if r.Logger != nil && strings.TrimSpace(stderr.String()) != "" {
		r.Logger.Debug("docker command stderr", "args", strings.Join(args, " "), "output", truncateDockerOutput(stderr.String()))
	}
	return trimmed, nil
}

func truncateDockerOutput(value string) string {
	const limit = 4000
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= limit {
		return trimmed
	}
	return trimmed[:limit] + "...(truncated)"
}
