package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	openaihandler "github.com/evanmschultz/valv/internal/api/openai"
	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/output"
)

type openAIAPIService interface {
	openaihandler.Executor
	ValidateBinding(context.Context) error
	PruneExpiredRuntimes(context.Context) (int, error)
	Shutdown(context.Context) (int, error)
}

var openAIAPIServiceFactory = func(cmd *cobra.Command, paths config.Paths, projectPath string, workspaceAccess bool, runtimeTTL time.Duration) (openAIAPIService, func(), error) {
	return newOpenAIAPIService(cmd, paths, projectPath, workspaceAccess, runtimeTTL)
}

func newAPICommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "api",
		Short: "Run the Valv API surface",
		Long: strings.TrimSpace(`
Run the OpenAI-compatible Valv API surface backed by the current project's bound provider account.
`),
		Example: strings.TrimSpace(`
valv api serve
valv api serve --workspace --runtime-ttl 2m
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newAPIServeCommand(paths, opts))
	installBranchHelpCommands(cmd)
	return cmd
}

func newAPIServeCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	var listenAddr string
	var projectPath string
	var workspaceAccess bool
	var runtimeTTL time.Duration
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the OpenAI-compatible Valv API",
		Long: strings.TrimSpace(`
Serve the OpenAI-compatible /v1/chat/completions surface for the current bound project.

Important behavior:
- ` + "`--runtime-ttl`" + ` is the idle lifetime for warm API runtime containers, not an automatic shutdown timer for the HTTP server
- ` + "`--workspace`" + ` controls whether the bound project root is mounted into API runtime containers

Output fields:
- listen: TCP address the HTTP server is attempting to bind
- path: API route served by Valv
- workspace: whether runtime containers get the project workspace mount
- runtime ttl: idle timeout for warm runtime containers
- project: project root whose binding backs the API
`),
		Example: strings.TrimSpace(`
valv api serve
valv api serve --listen 127.0.0.1:18080 --runtime-ttl 30s
valv api serve --workspace --project /absolute/path/to/repo
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAPIServe(cmd, paths, opts, listenAddr, projectPath, workspaceAccess, runtimeTTL)
		},
	}
	cmd.Flags().StringVar(&listenAddr, "listen", "127.0.0.1:8080", "listen address for the Valv API")
	cmd.Flags().StringVar(&projectPath, "project", "", "project path whose binding should back the API server")
	cmd.Flags().BoolVar(&workspaceAccess, "workspace", false, "mount the bound project workspace into API runtime containers")
	cmd.Flags().DurationVar(&runtimeTTL, "runtime-ttl", 2*time.Minute, "idle TTL for warm API runtime containers")
	return cmd
}

func runAPIServe(cmd *cobra.Command, paths config.Paths, opts *rootOptions, listenAddr, projectPath string, workspaceAccess bool, runtimeTTL time.Duration) error {
	mode, err := commandOutputMode(cmd, opts)
	if err != nil {
		return fmt.Errorf("resolve output policy: %w", err)
	}
	if runtimeTTL <= 0 {
		return fmt.Errorf("api serve: runtime ttl must be greater than zero")
	}
	startPath := strings.TrimSpace(projectPath)
	if startPath == "" {
		startPath, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("api serve: resolve working directory: %w", err)
		}
	}
	service, closeStore, err := openAIAPIServiceFactory(cmd, paths, startPath, workspaceAccess, runtimeTTL)
	if err != nil {
		return fmt.Errorf("api serve: %w", err)
	}
	defer closeStore()
	if err := service.ValidateBinding(cmd.Context()); err != nil {
		return fmt.Errorf("api serve: validate binding: %w", err)
	}
	handler, err := openaihandler.NewHandler(service, openaihandler.Options{Logger: LoggerFromContext(cmd.Context())})
	if err != nil {
		return fmt.Errorf("api serve: initialize handler: %w", err)
	}
	mux := http.NewServeMux()
	mux.Handle(openaihandler.ChatCompletionsPath, handler)

	server := &http.Server{Addr: listenAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("api serve: listen: %w", err)
	}
	defer listener.Close()

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-cmd.Context().Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	go runAPIRuntimeSweeper(cmd.Context(), LoggerFromContext(cmd.Context()), service, runtimeTTL)

	boundAddr := listener.Addr().String()
	if err := output.WriteRecord(cmd.OutOrStdout(), mode, "API server listening", []output.Field{{Label: "listen", Value: boundAddr, Identifier: true}, {Label: "path", Value: openaihandler.ChatCompletionsPath, Identifier: true}, {Label: "workspace", Value: fmt.Sprintf("%t", workspaceAccess), Badge: true}, {Label: "runtime ttl", Value: runtimeTTL.String(), Identifier: true}, {Label: "project", Value: startPath, Muted: true}}); err != nil {
		return fmt.Errorf("api serve: write startup output: %w", err)
	}
	serveErr := server.Serve(listener)
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return fmt.Errorf("api serve: listen: %w", serveErr)
	}
	<-shutdownDone
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	removed, err := service.Shutdown(shutdownCtx)
	if err != nil {
		return fmt.Errorf("api serve: shutdown warm runtimes: %w", err)
	}
	logger := LoggerFromContext(cmd.Context())
	if removed > 0 && logger != nil {
		logger.Debug("api serve stopped warm runtimes on shutdown", "count", removed)
	}
	return nil
}

func runAPIRuntimeSweeper(ctx context.Context, logger *log.Logger, service interface {
	PruneExpiredRuntimes(context.Context) (int, error)
}, ttl time.Duration,
) {
	interval := ttl / 2
	if interval <= 0 {
		interval = ttl
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if interval > 30*time.Second {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweepCtx, cancel := context.WithTimeout(ctx, interval)
			removed, err := service.PruneExpiredRuntimes(sweepCtx)
			cancel()
			if err != nil {
				if logger != nil {
					logger.Debug("api runtime sweep failed", "error", err)
				}
				continue
			}
			if removed > 0 && logger != nil {
				logger.Debug("api runtime sweep removed expired containers", "count", removed)
			}
		}
	}
}
