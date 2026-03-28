package codex

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const helperEnv = "VALV_MCP_HELPER_PROCESS"

func TestBridgeManagerBridgesStdioServers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	manager, err := newBridgeManager(ctx, nil)
	if err != nil {
		t.Fatalf("newBridgeManager() error = %v", err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Fatalf("manager.Close() error = %v", err)
		}
	})

	url, warning, err := manager.BridgeCommand(ctx, "helper", commandSpec{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestBridgeHelperProcess"},
		Env: map[string]string{
			helperEnv: "1",
		},
		Cwd: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("BridgeCommand() error = %v", err)
	}
	if warning != "" {
		t.Fatalf("BridgeCommand() warning = %q, want empty", warning)
	}
	url = strings.Replace(url, "host.docker.internal", "127.0.0.1", 1)

	client := mcp.NewClient(&mcp.Implementation{Name: "bridge-test-client", Version: "v0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: url}, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Fatalf("session.Close() error = %v", err)
		}
	})

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools.Tools) != 1 || tools.Tools[0].Name != "echo" {
		t.Fatalf("ListTools() = %+v, want single echo tool", tools.Tools)
	}

	toolResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "echo",
		Arguments: map[string]any{"text": "hello"},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if toolResult.IsError {
		t.Fatalf("CallTool() returned error result: %+v", toolResult)
	}
	if len(toolResult.Content) != 1 {
		t.Fatalf("CallTool() content len = %d, want 1", len(toolResult.Content))
	}
	text, ok := toolResult.Content[0].(*mcp.TextContent)
	if !ok || text.Text != "{\"echo\":\"hello\"}" {
		t.Fatalf("CallTool() content = %#v, want JSON echo text", toolResult.Content[0])
	}

	prompts, err := session.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatalf("ListPrompts() error = %v", err)
	}
	if len(prompts.Prompts) != 1 || prompts.Prompts[0].Name != "greet" {
		t.Fatalf("ListPrompts() = %+v, want single greet prompt", prompts.Prompts)
	}

	promptResult, err := session.GetPrompt(ctx, &mcp.GetPromptParams{
		Name:      "greet",
		Arguments: map[string]string{"name": "Valv"},
	})
	if err != nil {
		t.Fatalf("GetPrompt() error = %v", err)
	}
	if len(promptResult.Messages) != 1 {
		t.Fatalf("GetPrompt() messages len = %d, want 1", len(promptResult.Messages))
	}
	promptText, ok := promptResult.Messages[0].Content.(*mcp.TextContent)
	if !ok || promptText.Text != "hello Valv" {
		t.Fatalf("GetPrompt() content = %#v, want greeting text", promptResult.Messages[0].Content)
	}

	resources, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("ListResources() error = %v", err)
	}
	if len(resources.Resources) != 1 || resources.Resources[0].URI != "file:///bridge" {
		t.Fatalf("ListResources() = %+v, want single bridge resource", resources.Resources)
	}

	resourceResult, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "file:///bridge"})
	if err != nil {
		t.Fatalf("ReadResource() error = %v", err)
	}
	if len(resourceResult.Contents) != 1 || resourceResult.Contents[0].Text != "bridge resource" {
		t.Fatalf("ReadResource() = %+v, want bridge resource text", resourceResult.Contents)
	}

	templates, err := session.ListResourceTemplates(ctx, nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates() error = %v", err)
	}
	if len(templates.ResourceTemplates) != 1 || templates.ResourceTemplates[0].URITemplate != "file:///items/{name}" {
		t.Fatalf("ListResourceTemplates() = %+v, want single template", templates.ResourceTemplates)
	}

	resourceResult, err = session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "file:///items/example"})
	if err != nil {
		t.Fatalf("ReadResource(template) error = %v", err)
	}
	if len(resourceResult.Contents) != 1 || resourceResult.Contents[0].Text != "template example" {
		t.Fatalf("ReadResource(template) = %+v, want template text", resourceResult.Contents)
	}
}

func TestBridgeManagerKeepsBridgeAliveAfterRequestContextCancellation(t *testing.T) {
	t.Parallel()

	manager, err := newBridgeManager(context.Background(), nil)
	if err != nil {
		t.Fatalf("newBridgeManager() error = %v", err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Fatalf("manager.Close() error = %v", err)
		}
	})

	requestCtx, cancel := context.WithCancel(context.Background())
	url, _, err := manager.BridgeCommand(requestCtx, "helper", commandSpec{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestBridgeHelperProcess"},
		Env: map[string]string{
			helperEnv: "1",
		},
		Cwd: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("BridgeCommand() error = %v", err)
	}
	cancel()

	url = strings.Replace(url, "host.docker.internal", "127.0.0.1", 1)
	client := mcp.NewClient(&mcp.Implementation{Name: "bridge-test-client", Version: "v0.0.1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: url}, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Fatalf("session.Close() error = %v", err)
		}
	})

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools.Tools) != 1 || tools.Tools[0].Name != "echo" {
		t.Fatalf("ListTools() = %+v, want single echo tool", tools.Tools)
	}
}

func TestBridgeManagerCloseSucceedsWithActiveStreamableClient(t *testing.T) {
	t.Parallel()

	manager, err := newBridgeManager(context.Background(), nil)
	if err != nil {
		t.Fatalf("newBridgeManager() error = %v", err)
	}

	url, _, err := manager.BridgeCommand(context.Background(), "helper", commandSpec{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestBridgeHelperProcess"},
		Env: map[string]string{
			helperEnv: "1",
		},
		Cwd: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("BridgeCommand() error = %v", err)
	}

	url = strings.Replace(url, "host.docker.internal", "127.0.0.1", 1)
	client := mcp.NewClient(&mcp.Implementation{Name: "bridge-test-client", Version: "v0.0.1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: url}, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	t.Cleanup(func() {
		_ = session.Close()
	})

	if err := manager.Close(); err != nil {
		t.Fatalf("manager.Close() error = %v", err)
	}
}

func TestBridgeManagerBridgesToolOnlyServers(t *testing.T) {
	t.Parallel()

	manager, err := newBridgeManager(context.Background(), nil)
	if err != nil {
		t.Fatalf("newBridgeManager() error = %v", err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Fatalf("manager.Close() error = %v", err)
		}
	})

	url, _, err := manager.BridgeCommand(context.Background(), "tool-only", commandSpec{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestBridgeToolOnlyHelperProcess"},
		Env: map[string]string{
			helperEnv: "1",
		},
		Cwd: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("BridgeCommand() error = %v", err)
	}

	url = strings.Replace(url, "host.docker.internal", "127.0.0.1", 1)
	client := mcp.NewClient(&mcp.Implementation{Name: "bridge-test-client", Version: "v0.0.1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: url}, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Fatalf("session.Close() error = %v", err)
		}
	})

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools.Tools) != 1 || tools.Tools[0].Name != "echo" {
		t.Fatalf("ListTools() = %+v, want single echo tool", tools.Tools)
	}
}

func TestRewriteLoopbackURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		changed bool
	}{
		{name: "localhost", input: "http://localhost:7389/mcp", want: "http://host.docker.internal:7389/mcp", changed: true},
		{name: "ipv4", input: "http://127.0.0.1:7389/mcp", want: "http://host.docker.internal:7389/mcp", changed: true},
		{name: "ipv6", input: "http://[::1]:7389/mcp", want: "http://host.docker.internal:7389/mcp", changed: true},
		{name: "remote", input: "https://example.com/mcp", want: "https://example.com/mcp", changed: false},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, changed, err := rewriteLoopbackURL(test.input)
			if err != nil {
				t.Fatalf("rewriteLoopbackURL() error = %v", err)
			}
			if got != test.want || changed != test.changed {
				t.Fatalf("rewriteLoopbackURL() = (%q, %t), want (%q, %t)", got, changed, test.want, test.changed)
			}
		})
	}
}

func TestShouldIgnoreBridgeCloseError(t *testing.T) {
	t.Parallel()

	if !shouldIgnoreBridgeCloseError(&exec.ExitError{}) {
		t.Fatal("shouldIgnoreBridgeCloseError(exec.ExitError) = false, want true")
	}
	if shouldIgnoreBridgeCloseError(context.Canceled) {
		t.Fatal("shouldIgnoreBridgeCloseError(context.Canceled) = true, want false")
	}
}

func TestBridgeHelperProcess(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		t.Skip("helper subprocess only")
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "bridge-helper", Version: "v0.0.1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "echo", Description: "echo text"}, func(_ context.Context, _ *mcp.CallToolRequest, input struct {
		Text string `json:"text"`
	}) (*mcp.CallToolResult, struct {
		Echo string `json:"echo"`
	}, error) {
		return nil, struct {
			Echo string `json:"echo"`
		}{Echo: input.Text}, nil
	})
	server.AddPrompt(&mcp.Prompt{Name: "greet"}, func(_ context.Context, request *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Messages: []*mcp.PromptMessage{
				{
					Role:    "user",
					Content: &mcp.TextContent{Text: "hello " + request.Params.Arguments["name"]},
				},
			},
		}, nil
	})
	resourceHandler := func(_ context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		switch request.Params.URI {
		case "file:///bridge":
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{URI: request.Params.URI, Text: "bridge resource"}},
			}, nil
		case "file:///items/example":
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{URI: request.Params.URI, Text: "template example"}},
			}, nil
		default:
			return nil, mcp.ResourceNotFoundError(request.Params.URI)
		}
	}
	server.AddResource(&mcp.Resource{URI: "file:///bridge"}, resourceHandler)
	server.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: "file:///items/{name}"}, resourceHandler)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		t.Fatalf("server.Run() error = %v", err)
	}
}

func TestBridgeToolOnlyHelperProcess(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		t.Skip("helper subprocess only")
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "bridge-helper", Version: "v0.0.1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "echo", Description: "echo text"}, func(_ context.Context, _ *mcp.CallToolRequest, input struct {
		Text string `json:"text"`
	}) (*mcp.CallToolResult, struct {
		Echo string `json:"echo"`
	}, error) {
		return nil, struct {
			Echo string `json:"echo"`
		}{Echo: input.Text}, nil
	})

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		t.Fatalf("server.Run() error = %v", err)
	}
}
