package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"agent-platform/core/tools"
	clientpkg "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/mcptest"
)

func TestRemoteToolExecuteRaw(t *testing.T) {
	server := mcptest.NewUnstartedServer(t)
	server.AddTool(mcp.NewTool("echo"), func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("echo:" + request.GetArguments()["message"].(string)), nil
	})
	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("start mcptest server: %v", err)
	}
	defer server.Close()

	previous := connectClient
	connectClient = func(_ context.Context, _ Server) (*clientpkg.Client, func(), error) {
		return server.Client(), func() {}, nil
	}
	defer func() { connectClient = previous }()

	tool := remoteTool{server: Server{Name: "demo", Transport: "stdio"}, definition: mcp.NewTool("echo")}
	rawArgs, err := json.Marshal(map[string]string{"message": "hello"})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	result, err := tool.ExecuteRaw(context.Background(), rawArgs, tools.ExecutionEnv{})
	if err != nil {
		t.Fatalf("execute raw: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "echo:hello") {
		t.Fatalf("unexpected content: %+v", result.Content)
	}
}

func TestBridgeRegisterAll(t *testing.T) {
	server := mcptest.NewUnstartedServer(t)
	server.AddTool(mcp.NewTool("inspect"), func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("ok"), nil
	})
	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("start mcptest server: %v", err)
	}
	defer server.Close()

	previous := connectClient
	connectClient = func(_ context.Context, _ Server) (*clientpkg.Client, func(), error) {
		return server.Client(), func() {}, nil
	}
	defer func() { connectClient = previous }()

	workspace := t.TempDir()
	homeDir := t.TempDir()
	manager := NewManager(workspace, homeDir)
	if err := manager.Add(Server{Name: "demo", Endpoint: "ignored", Transport: "stdio", Scope: ScopeLocal}); err != nil {
		t.Fatalf("add server: %v", err)
	}
	registry := tools.NewRegistry()
	if err := NewBridge(manager).RegisterAll(context.Background(), registry); err != nil {
		t.Fatalf("register all: %v", err)
	}
	if _, ok := registry.Get("mcp.demo.inspect"); !ok {
		t.Fatalf("expected wrapped MCP tool to be registered")
	}
}
