package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"agent-platform/core/permission"
	"agent-platform/core/tools"
	"agent-platform/core/types"
	clientpkg "github.com/mark3labs/mcp-go/client"
	transportpkg "github.com/mark3labs/mcp-go/client/transport"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

type Bridge struct {
	manager *Manager
}

var connectClient = newClient

func NewBridge(manager *Manager) *Bridge {
	return &Bridge{manager: manager}
}

func (b *Bridge) RegisterAll(ctx context.Context, registry *tools.Registry) error {
	servers, err := b.manager.ListAll()
	if err != nil {
		return err
	}
	for _, server := range servers {
		wrapped, err := b.wrapServer(ctx, server)
		if err != nil {
			return err
		}
		for _, tool := range wrapped {
			if err := registry.Register(tool); err != nil && !strings.Contains(err.Error(), "already registered") {
				return err
			}
		}
	}
	return nil
}

func (b *Bridge) wrapServer(ctx context.Context, server Server) ([]tools.Tool, error) {
	client, cleanup, err := connectClient(ctx, server)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	listing, err := client.ListTools(ctx, mcpgo.ListToolsRequest{})
	if err != nil {
		return nil, err
	}
	items := make([]tools.Tool, 0, len(listing.Tools))
	for _, definition := range listing.Tools {
		items = append(items, remoteTool{server: server, definition: definition})
	}
	return items, nil
}

type remoteTool struct {
	server     Server
	definition mcpgo.Tool
}

func (t remoteTool) Descriptor() tools.Descriptor {
	return tools.Descriptor{
		Name:        fmt.Sprintf("mcp.%s.%s", t.server.Name, t.definition.Name),
		Description: chooseDescription(t.definition.Description, "MCP tool"),
		Category:    "mcp",
		InputSchema: map[string]any{"type": t.definition.InputSchema.Type, "properties": t.definition.InputSchema.Properties, "required": t.definition.InputSchema.Required},
	}
}

func (t remoteTool) EvaluatePermission(_ context.Context, rawArgs []byte, _ tools.ExecutionEnv) (permission.Request, error) {
	risk := types.RiskMedium
	if t.definition.Annotations.DestructiveHint != nil && *t.definition.Annotations.DestructiveHint {
		risk = types.RiskHigh
	}
	if t.definition.Annotations.ReadOnlyHint != nil && *t.definition.Annotations.ReadOnlyHint {
		risk = types.RiskLow
	}
	return permission.Request{
		ToolName:  t.Descriptor().Name,
		RiskLevel: risk,
		Summary:   fmt.Sprintf("Call MCP tool %s on server %s with args %s", t.definition.Name, t.server.Name, string(rawArgs)),
	}, nil
}

func (t remoteTool) ExecuteRaw(ctx context.Context, rawArgs []byte, _ tools.ExecutionEnv) (tools.ExecutionResult, error) {
	client, cleanup, err := connectClient(ctx, t.server)
	if err != nil {
		return tools.ExecutionResult{Success: false, Error: err.Error()}, err
	}
	defer cleanup()

	arguments := map[string]any{}
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &arguments); err != nil {
			return tools.ExecutionResult{Success: false, Error: err.Error()}, err
		}
	}
	result, err := client.CallTool(ctx, mcpgo.CallToolRequest{Params: mcpgo.CallToolParams{Name: t.definition.Name, Arguments: arguments}})
	if err != nil {
		return tools.ExecutionResult{Success: false, Error: err.Error()}, err
	}
	content := make([]types.ContentPart, 0, len(result.Content))
	for _, item := range result.Content {
		content = append(content, types.ContentPart{Type: contentType(item), Text: encodeContent(item)})
	}
	return tools.ExecutionResult{Success: !result.IsError, Content: content, Metadata: map[string]any{"server": t.server.Name}}, nil
}

func newClient(ctx context.Context, server Server) (*clientpkg.Client, func(), error) {
	var (
		client *clientpkg.Client
		err    error
	)
	switch strings.ToLower(server.Transport) {
	case "stdio":
		parts := strings.Fields(server.Endpoint)
		if len(parts) == 0 {
			return nil, nil, fmt.Errorf("empty stdio endpoint for mcp server %s", server.Name)
		}
		client, err = clientpkg.NewStdioMCPClient(parts[0], environmentList(server.Environment), parts[1:]...)
	case "http", "streamablehttp", "streamable-http":
		transport, transportErr := transportpkg.NewStreamableHTTP(server.Endpoint)
		if transportErr != nil {
			return nil, nil, transportErr
		}
		client = clientpkg.NewClient(transport)
	case "sse":
		client, err = clientpkg.NewSSEMCPClient(server.Endpoint)
	default:
		return nil, nil, fmt.Errorf("unsupported mcp transport %s", server.Transport)
	}
	if err != nil {
		return nil, nil, err
	}
	startupCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := client.Start(startupCtx); err != nil {
		_ = client.Close()
		return nil, nil, err
	}
	request := mcpgo.InitializeRequest{}
	request.Params.ProtocolVersion = mcpgo.LATEST_PROTOCOL_VERSION
	request.Params.ClientInfo = mcpgo.Implementation{Name: "agent-platform", Version: "0.1.0"}
	request.Params.Capabilities = mcpgo.ClientCapabilities{}
	if _, err := client.Initialize(startupCtx, request); err != nil {
		_ = client.Close()
		return nil, nil, err
	}
	return client, func() { _ = client.Close() }, nil
}

func chooseDescription(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "MCP tool"
}

func environmentList(environment map[string]string) []string {
	items := make([]string, 0, len(environment))
	for key, value := range environment {
		items = append(items, key+"="+value)
	}
	return items
}

func encodeContent(content mcpgo.Content) string {
	switch value := content.(type) {
	case mcpgo.TextContent:
		return value.Text
	case mcpgo.ImageContent:
		return value.Data
	case mcpgo.EmbeddedResource:
		payload, _ := json.Marshal(value.Resource)
		return string(payload)
	default:
		payload, _ := json.Marshal(value)
		return string(payload)
	}
}

func contentType(content mcpgo.Content) string {
	switch value := content.(type) {
	case mcpgo.TextContent:
		return value.Type
	case mcpgo.ImageContent:
		return value.Type
	case mcpgo.EmbeddedResource:
		return value.Type
	default:
		return "json"
	}
}
