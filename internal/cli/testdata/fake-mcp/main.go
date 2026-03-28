package main

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fixtureInput struct {
	Name string `json:"name" jsonschema:"optional fixture argument"`
}

type fixtureOutput struct {
	Result string `json:"result" jsonschema:"fixture result"`
}

func handleFixtureTool(_ context.Context, _ *mcp.CallToolRequest, input fixtureInput) (*mcp.CallToolResult, fixtureOutput, error) {
	result := "fixture response"
	if input.Name != "" {
		result += ": " + input.Name
	}
	return nil, fixtureOutput{Result: result}, nil
}

func main() {
	server := mcp.NewServer(&mcp.Implementation{Name: "valv-fixture-mcp", Version: "0.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "fixture_tool", Description: "fixture MCP tool"}, handleFixtureTool)
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
