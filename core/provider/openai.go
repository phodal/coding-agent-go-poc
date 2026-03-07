package provider

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"agent-platform/core/types"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

type OpenAIConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

type OpenAIProvider struct {
	client openai.Client
	config OpenAIConfig
}

func LoadOpenAIConfigFromEnv() OpenAIConfig {
	return OpenAIConfig{
		BaseURL: firstNonEmpty(os.Getenv("AGENT_PLATFORM_OPENAI_BASE_URL"), os.Getenv("OPENAI_BASE_URL")),
		APIKey:  firstNonEmpty(os.Getenv("AGENT_PLATFORM_OPENAI_API_KEY"), os.Getenv("OPENAI_API_KEY")),
		Model:   firstNonEmpty(os.Getenv("AGENT_PLATFORM_OPENAI_MODEL"), os.Getenv("OPENAI_MODEL")),
	}
}

func NewOpenAIProvider(config OpenAIConfig) OpenAIProvider {
	options := make([]option.RequestOption, 0, 2)
	if config.BaseURL != "" {
		options = append(options, option.WithBaseURL(config.BaseURL))
	}
	if config.APIKey != "" {
		options = append(options, option.WithAPIKey(config.APIKey))
	}
	return OpenAIProvider{client: openai.NewClient(options...), config: config}
}

func (p OpenAIProvider) Name() string { return "openai" }

func (p OpenAIProvider) CreateResponse(ctx context.Context, req Request) (Response, error) {
	if len(req.Messages) == 0 {
		return Response{}, fmt.Errorf("no messages provided")
	}
	if p.config.APIKey == "" {
		return Response{}, fmt.Errorf("missing OpenAI API key: set AGENT_PLATFORM_OPENAI_API_KEY or OPENAI_API_KEY")
	}
	model := firstNonEmpty(req.Model, req.Agent.Model, p.config.Model)
	if model == "" || model == "local" {
		return Response{}, fmt.Errorf("missing OpenAI model: set agent model or AGENT_PLATFORM_OPENAI_MODEL")
	}

	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages)+1)
	if systemPrompt := buildSystemPrompt(req.Agent, req.Skills); systemPrompt != "" {
		messages = append(messages, openai.DeveloperMessage(systemPrompt))
	}
	for _, message := range req.Messages {
		messages = append(messages, mapMessage(message)...)
	}

	params := openai.ChatCompletionNewParams{
		Messages: messages,
		Model:    shared.ChatModel(model),
	}
	if len(req.Tools) > 0 {
		params.Tools = make([]openai.ChatCompletionToolUnionParam, 0, len(req.Tools))
		for _, tool := range req.Tools {
			params.Tools = append(params.Tools, openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
				Name:        tool.Name,
				Description: openai.String(tool.Description),
				Parameters:  openai.FunctionParameters(tool.InputSchema),
			}))
		}
	}

	completion, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return Response{}, err
	}
	if len(completion.Choices) == 0 {
		return Response{}, fmt.Errorf("empty completion choices")
	}

	choice := completion.Choices[0]
	toolCalls := make([]types.ToolCall, 0, len(choice.Message.ToolCalls))
	for _, union := range choice.Message.ToolCalls {
		if union.Type != "function" {
			continue
		}
		call := union.AsFunction()
		toolCalls = append(toolCalls, types.ToolCall{
			ID:   call.ID,
			Name: call.Function.Name,
			Args: []byte(call.Function.Arguments),
		})
	}

	message := types.Message{
		ID:        completion.ID,
		Role:      "assistant",
		CreatedAt: time.Now().UTC(),
		ToolCalls: toolCalls,
	}
	if content := strings.TrimSpace(choice.Message.Content); content != "" {
		message.Content = []types.ContentPart{{Type: "text", Text: content}}
	}
	if message.ID == "" {
		message.ID = fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	}

	return Response{Message: message, ToolCalls: toolCalls, StopReason: mapStopReason(choice.FinishReason)}, nil
}

func buildSystemPrompt(agent types.AgentDefinition, skills []types.SkillDefinition) string {
	parts := make([]string, 0, len(skills)+1)
	if strings.TrimSpace(agent.SystemPrompt) != "" {
		parts = append(parts, strings.TrimSpace(agent.SystemPrompt))
	}
	for _, skill := range skills {
		if strings.TrimSpace(skill.Content) == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("Skill %s:\n%s", skill.Name, strings.TrimSpace(skill.Content)))
	}
	return strings.Join(parts, "\n\n")
}

func mapMessage(message types.Message) []openai.ChatCompletionMessageParamUnion {
	text := joinContent(message.Content)
	switch message.Role {
	case "user":
		return []openai.ChatCompletionMessageParamUnion{openai.UserMessage(text)}
	case "assistant":
		assistant := openai.ChatCompletionAssistantMessageParam{}
		if strings.TrimSpace(text) != "" {
			assistant.Content.OfString = openai.String(text)
		}
		for _, toolCall := range message.ToolCalls {
			assistant.ToolCalls = append(assistant.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: toolCall.ID,
					Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      toolCall.Name,
						Arguments: string(toolCall.Args),
					},
				},
			})
		}
		if strings.TrimSpace(text) == "" && len(assistant.ToolCalls) == 0 {
			return nil
		}
		return []openai.ChatCompletionMessageParamUnion{{OfAssistant: &assistant}}
	case "tool":
		items := make([]openai.ChatCompletionMessageParamUnion, 0, len(message.ToolResults))
		for _, result := range message.ToolResults {
			payload := result.Error
			if result.Success {
				payload = joinContent(result.Content)
			}
			items = append(items, openai.ToolMessage(payload, result.ToolCallID))
		}
		return items
	default:
		if strings.TrimSpace(text) == "" {
			return nil
		}
		return []openai.ChatCompletionMessageParamUnion{openai.UserMessage(text)}
	}
}

func mapStopReason(_ string) types.StopReason {
	return types.StopCompletion
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
