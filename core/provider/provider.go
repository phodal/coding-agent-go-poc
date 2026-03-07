package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"agent-platform/core/tools"
	"agent-platform/core/types"
	"github.com/google/uuid"
)

type ToolDefinition struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type Request struct {
	Agent    types.AgentDefinition
	Model    string
	Messages []types.Message
	Tools    []ToolDefinition
	Skills   []types.SkillDefinition
}

type Response struct {
	Message    types.Message
	ToolCalls  []types.ToolCall
	StopReason types.StopReason
}

type Provider interface {
	Name() string
	CreateResponse(context.Context, Request) (Response, error)
}

type Registry struct {
	providers map[string]Provider
}

func NewRegistry() *Registry {
	return &Registry{providers: map[string]Provider{}}
}

func (r *Registry) Register(provider Provider) {
	r.providers[provider.Name()] = provider
}

func (r *Registry) Get(name string) (Provider, bool) {
	provider, ok := r.providers[name]
	return provider, ok
}

type LocalProvider struct{}

func (LocalProvider) Name() string { return "local" }

func (LocalProvider) CreateResponse(_ context.Context, req Request) (Response, error) {
	if len(req.Messages) == 0 {
		return Response{}, fmt.Errorf("no messages provided")
	}
	last := req.Messages[len(req.Messages)-1]
	if len(last.ToolResults) > 0 {
		result := last.ToolResults[0]
		text := "Tool execution failed"
		if result.Success {
			text = fmt.Sprintf("Tool %s completed successfully.\n\n%s", result.ToolName, joinContent(result.Content))
		} else if result.Error != "" {
			text = fmt.Sprintf("Tool %s failed: %s", result.ToolName, result.Error)
		}
		return Response{
			Message:    types.Message{ID: uuid.NewString(), Role: "assistant", CreatedAt: time.Now().UTC(), Content: []types.ContentPart{{Type: "text", Text: text}}},
			StopReason: types.StopCompletion,
		}, nil
	}

	userInput := strings.TrimSpace(joinContent(last.Content))
	toolNames := make([]string, 0, len(req.Tools))
	for _, tool := range req.Tools {
		toolNames = append(toolNames, tool.Name)
	}
	if strings.HasPrefix(userInput, "read ") {
		args, _ := json.Marshal(map[string]string{"path": strings.TrimSpace(strings.TrimPrefix(userInput, "read "))})
		return toolResponse("read_file", args), nil
	}
	if strings.HasPrefix(userInput, "write ") {
		body := strings.TrimSpace(strings.TrimPrefix(userInput, "write "))
		parts := strings.SplitN(body, "::", 2)
		if len(parts) == 2 {
			args, _ := json.Marshal(map[string]string{"path": strings.TrimSpace(parts[0]), "content": parts[1]})
			return toolResponse("write_file", args), nil
		}
	}
	if strings.HasPrefix(userInput, "bash ") {
		args, _ := json.Marshal(map[string]string{"command": strings.TrimSpace(strings.TrimPrefix(userInput, "bash "))})
		return toolResponse("bash", args), nil
	}
	if strings.HasPrefix(userInput, "ask ") {
		args, _ := json.Marshal(map[string]string{"question": strings.TrimSpace(strings.TrimPrefix(userInput, "ask "))})
		return toolResponse("ask_user", args), nil
	}

	summary := fmt.Sprintf(
		"Agent %s is running with provider local.\n\nAvailable tools: %s\nLoaded skills: %d\n\nPrefix your prompt with `read`, `write`, `bash`, or `ask` to trigger tool execution.",
		req.Agent.Name,
		strings.Join(toolNames, ", "),
		len(req.Skills),
	)
	return Response{
		Message:    types.Message{ID: uuid.NewString(), Role: "assistant", CreatedAt: time.Now().UTC(), Content: []types.ContentPart{{Type: "text", Text: summary}}},
		StopReason: types.StopCompletion,
	}, nil
}

func toolResponse(name string, args []byte) Response {
	call := types.ToolCall{ID: uuid.NewString(), Name: name, Args: args}
	return Response{
		Message:    types.Message{ID: uuid.NewString(), Role: "assistant", CreatedAt: time.Now().UTC(), ToolCalls: []types.ToolCall{call}, Content: []types.ContentPart{{Type: "text", Text: fmt.Sprintf("Requesting tool %s", name)}}},
		ToolCalls:  []types.ToolCall{call},
		StopReason: types.StopCompletion,
	}
}

func joinContent(parts []types.ContentPart) string {
	var builder strings.Builder
	for index, part := range parts {
		if index > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(part.Text)
	}
	return builder.String()
}

func DefinitionsFromDescriptors(descriptors []tools.Descriptor) []ToolDefinition {
	items := make([]ToolDefinition, 0, len(descriptors))
	for _, descriptor := range descriptors {
		items = append(items, ToolDefinition{
			Name:        descriptor.Name,
			Description: descriptor.Description,
			InputSchema: descriptor.InputSchema,
		})
	}
	return items
}
