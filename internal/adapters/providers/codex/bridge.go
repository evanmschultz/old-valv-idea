package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type bridgeManager struct {
	logger   *log.Logger
	ctx      context.Context
	cancel   context.CancelFunc
	listener net.Listener
	server   *http.Server
	baseURL  string
	mux      *http.ServeMux
	mu       sync.Mutex
	bridges  []*stdioBridge
}

type commandSpec struct {
	Command   string
	Args      []string
	Env       map[string]string
	ScopeRoot string
	Cwd       string
}

func newBridgeManager(ctx context.Context, logger *log.Logger) (*bridgeManager, error) {
	lifecycleCtx, cancel := context.WithCancel(context.Background())
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		cancel()
		return nil, fmt.Errorf("listen for MCP bridge: %w", err)
	}
	mux := http.NewServeMux()
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	manager := &bridgeManager{
		logger:   logger,
		ctx:      lifecycleCtx,
		cancel:   cancel,
		listener: listener,
		server:   server,
		baseURL:  fmt.Sprintf("http://host.docker.internal:%d", listener.Addr().(*net.TCPAddr).Port),
		mux:      mux,
	}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed && logger != nil {
			logger.Error("mcp bridge server exited", "error", err)
		}
	}()
	return manager, nil
}

func (m *bridgeManager) BridgeCommand(ctx context.Context, name string, spec commandSpec) (string, string, error) {
	resolvedCommand, warning, err := resolveCommand(spec)
	if err != nil {
		return "", "", err
	}
	command := exec.CommandContext(m.ctx, resolvedCommand.Command, resolvedCommand.Args...)
	command.Dir = resolvedCommand.Cwd
	command.Env = append(os.Environ(), resolvedCommand.Env...)

	client := mcp.NewClient(&mcp.Implementation{Name: "valv-mcp-bridge-client", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		return "", "", fmt.Errorf("connect stdio MCP command %q: %w", name, err)
	}

	server, err := buildProxyServer(ctx, session)
	if err != nil {
		_ = session.Close()
		return "", "", fmt.Errorf("proxy stdio MCP command %q: %w", name, err)
	}

	token := sanitizeBridgeName(name) + "-" + sanitizedToken()
	path := "/mcp/" + token
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	m.mux.Handle(path, handler)
	m.mux.Handle(path+"/", handler)

	bridge := &stdioBridge{
		name:    name,
		session: session,
	}
	m.mu.Lock()
	m.bridges = append(m.bridges, bridge)
	m.mu.Unlock()

	if m.logger != nil {
		m.logger.Debug("started MCP bridge", "name", name, "url", m.baseURL+path)
	}
	return m.baseURL + path, warning, nil
}

func (m *bridgeManager) Close() error {
	m.mu.Lock()
	bridges := append([]*stdioBridge(nil), m.bridges...)
	m.bridges = nil
	m.mu.Unlock()

	var errs []error
	if m.server != nil {
		if err := m.server.Close(); err != nil && err != http.ErrServerClosed {
			errs = append(errs, err)
		}
	}
	for _, bridge := range bridges {
		if err := bridge.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if m.cancel != nil {
		m.cancel()
	}
	return errorsJoin(errs...)
}

type stdioBridge struct {
	name    string
	session *mcp.ClientSession
}

func (b *stdioBridge) Close() error {
	if b.session == nil {
		return nil
	}
	err := b.session.Close()
	if shouldIgnoreBridgeCloseError(err) {
		return nil
	}
	return err
}

type resolvedCommand struct {
	Command string
	Args    []string
	Env     []string
	Cwd     string
}

func resolveCommand(spec commandSpec) (resolvedCommand, string, error) {
	command := strings.TrimSpace(spec.Command)
	if command == "" {
		return resolvedCommand{}, "", fmt.Errorf("command is required")
	}
	cwd := strings.TrimSpace(spec.Cwd)
	if cwd == "" {
		cwd = strings.TrimSpace(spec.ScopeRoot)
	}
	warning := ""
	if filepath.IsAbs(command) {
		return resolvedCommand{
			Command: command,
			Args:    append([]string(nil), spec.Args...),
			Env:     flattenEnv(spec.Env),
			Cwd:     cwd,
		}, warning, nil
	}
	if strings.Contains(command, string(filepath.Separator)) && strings.TrimSpace(spec.ScopeRoot) != "" {
		command = filepath.Join(spec.ScopeRoot, command)
		warning = "relative stdio MCP command is executed on the host with the project/profile scope as its base path"
	}
	return resolvedCommand{
		Command: command,
		Args:    append([]string(nil), spec.Args...),
		Env:     flattenEnv(spec.Env),
		Cwd:     cwd,
	}, warning, nil
}

func flattenEnv(values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for key, value := range values {
		out = append(out, fmt.Sprintf("%s=%s", key, value))
	}
	return out
}

func buildProxyServer(ctx context.Context, session *mcp.ClientSession) (*mcp.Server, error) {
	server := mcp.NewServer(&mcp.Implementation{Name: "valv-mcp-bridge", Version: "0.1.0"}, &mcp.ServerOptions{
		CompletionHandler: func(ctx context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
			return session.Complete(ctx, req.Params)
		},
	})

	var capabilities *mcp.ServerCapabilities
	if initResult := session.InitializeResult(); initResult != nil {
		capabilities = initResult.Capabilities
	}
	if capabilities == nil || capabilities.Tools != nil {
		for tool, err := range session.Tools(ctx, nil) {
			if err != nil {
				return nil, err
			}
			current := *tool
			server.AddTool(&current, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				var arguments any
				if len(req.Params.Arguments) > 0 {
					if err := json.Unmarshal(req.Params.Arguments, &arguments); err != nil {
						return nil, fmt.Errorf("decode tool arguments for %q: %w", current.Name, err)
					}
				}
				return session.CallTool(ctx, &mcp.CallToolParams{Name: current.Name, Arguments: arguments})
			})
		}
	}

	if capabilities != nil && capabilities.Prompts != nil {
		for prompt, err := range session.Prompts(ctx, nil) {
			if err != nil {
				return nil, err
			}
			current := *prompt
			server.AddPrompt(&current, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				return session.GetPrompt(ctx, &mcp.GetPromptParams{Name: current.Name, Arguments: req.Params.Arguments})
			})
		}
	}

	if capabilities != nil && capabilities.Resources != nil {
		for resource, err := range session.Resources(ctx, nil) {
			if err != nil {
				return nil, err
			}
			current := *resource
			server.AddResource(&current, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				return session.ReadResource(ctx, &mcp.ReadResourceParams{URI: req.Params.URI})
			})
		}

		for resourceTemplate, err := range session.ResourceTemplates(ctx, nil) {
			if err != nil {
				return nil, err
			}
			current := *resourceTemplate
			server.AddResourceTemplate(&current, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				return session.ReadResource(ctx, &mcp.ReadResourceParams{URI: req.Params.URI})
			})
		}
	}
	return server, nil
}

func rewriteLoopbackURL(raw string) (string, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return "", false, nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false, err
	}
	host := strings.TrimSpace(parsed.Hostname())
	switch host {
	case "127.0.0.1", "localhost", "::1":
		if port := strings.TrimSpace(parsed.Port()); port != "" {
			parsed.Host = net.JoinHostPort("host.docker.internal", port)
		} else {
			parsed.Host = "host.docker.internal"
		}
		return parsed.String(), true, nil
	default:
		return raw, false, nil
	}
}

func sanitizedToken() string {
	return strings.ReplaceAll(fmt.Sprintf("%d", time.Now().UTC().UnixNano()), "-", "")
}

func sanitizeBridgeName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "server"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "server"
	}
	return out
}

func shouldIgnoreBridgeCloseError(err error) bool {
	if err == nil {
		return false
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "broken pipe") ||
		strings.Contains(lower, "connection reset by peer") ||
		strings.Contains(lower, "file already closed")
}
