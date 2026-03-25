package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	openaihandler "github.com/evanmschultz/valv/internal/api/openai"
	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/output"
)

func newAPICommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "api",
		Short: "Run the Valv API surface",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newAPIServeCommand(paths, opts))
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
		Args:  cobra.NoArgs,
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
	startPath := strings.TrimSpace(projectPath)
	if startPath == "" {
		startPath, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("api serve: resolve working directory: %w", err)
		}
	}
	service, closeStore, err := newOpenAIAPIService(cmd, paths, startPath, workspaceAccess, runtimeTTL)
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
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-cmd.Context().Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	if err := output.WriteRecord(cmd.OutOrStdout(), mode, "API server starting", []output.Field{{Label: "listen", Value: listenAddr, Identifier: true}, {Label: "path", Value: openaihandler.ChatCompletionsPath, Identifier: true}, {Label: "workspace", Value: fmt.Sprintf("%t", workspaceAccess), Badge: true}, {Label: "runtime ttl", Value: runtimeTTL.String(), Identifier: true}, {Label: "project", Value: startPath, Muted: true}}); err != nil {
		return fmt.Errorf("api serve: write startup output: %w", err)
	}
	serveErr := server.ListenAndServe()
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return fmt.Errorf("api serve: listen: %w", serveErr)
	}
	<-shutdownDone
	return nil
}
