package runtime

import (
	"context"
	"fmt"
	"os"
	"slices"
	"time"

	"agent-platform/core/agent/session_memory"
	jobcore "agent-platform/core/jobs"
	"agent-platform/core/permission"
	"agent-platform/core/provider"
	"agent-platform/core/resource"
	mcpresource "agent-platform/core/resource/mcp"
	"agent-platform/core/session"
	"agent-platform/core/tools"
	"agent-platform/core/types"

	"github.com/google/uuid"
)

type StartRequest struct {
	Workspace         string
	Prompt            string
	SessionID         string
	AgentName         string
	ProviderName      string
	PermissionHandler permission.Handler
}

type TurnResult struct {
	Session types.Session
	Output  types.Message
}

type Runtime struct {
	providers *provider.Registry
	tools     *tools.Registry
	sessions  *session.Service
	resources *resource.Loader
	memory    *session_memory.Manager
	jobs      *jobcore.Manager
}

func New(workspace string) (*Runtime, error) {
	resources, err := resource.NewLoader(workspace)
	if err != nil {
		return nil, err
	}
	providers := provider.NewRegistry()
	providers.Register(provider.LocalProvider{})
	providers.Register(provider.NewOpenAIProvider(provider.LoadOpenAIConfigFromEnv()))
	toolRegistry := tools.NewRegistry()
	toolRegistry.MustRegister(tools.BuiltinReadFile())
	toolRegistry.MustRegister(tools.BuiltinWriteFile())
	toolRegistry.MustRegister(tools.BuiltinBash())
	toolRegistry.MustRegister(tools.BuiltinAskUser())
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	mcpManager := mcpresource.NewManager(workspace, homeDir)
	if err := mcpresource.NewBridge(mcpManager).RegisterAll(context.Background(), toolRegistry); err != nil {
		return nil, err
	}
	return &Runtime{
		providers: providers,
		tools:     toolRegistry,
		sessions:  session.NewService(workspace),
		resources: resources,
		memory:    session_memory.NewManager(workspace),
		jobs:      jobcore.NewManager(workspace),
	}, nil
}

func (r *Runtime) Start(ctx context.Context, req StartRequest) (TurnResult, error) {
	permissionHandler := req.PermissionHandler
	if permissionHandler == nil {
		permissionHandler = permission.DefaultHandler
	}

	var sess types.Session
	var err error
	if req.SessionID != "" {
		sess, err = r.sessions.Load(req.SessionID)
	} else {
		sess, err = r.sessions.New(req.Workspace)
	}
	if err != nil {
		return TurnResult{}, err
	}

	agent, skills, err := r.resolveAgentContext(req.AgentName)
	if err != nil {
		return TurnResult{}, err
	}
	availableTools := r.allowedTools(agent.Tools)
	providerName := req.ProviderName
	if providerName == "" {
		providerName = defaultProviderName(agent)
	}
	selectedProvider, ok := r.providers.Get(providerName)
	if !ok {
		return TurnResult{}, fmt.Errorf("provider %s not registered", providerName)
	}

	userMessage := types.Message{
		ID:        uuid.NewString(),
		Role:      "user",
		CreatedAt: time.Now().UTC(),
		Content:   []types.ContentPart{{Type: "text", Text: req.Prompt}},
	}
	sess.Messages = append(sess.Messages, userMessage)
	sess.AgentState.CurrentAgent = agent.Name

	var final types.Message
	for i := 0; i < sess.AgentState.MaxIterations; i++ {
		sess.AgentState.Iteration++
		response, err := selectedProvider.CreateResponse(ctx, provider.Request{Agent: agent, Model: agent.Model, Messages: sess.Messages, Tools: availableTools, Skills: skills})
		if err != nil {
			return TurnResult{}, err
		}
		final = response.Message
		sess.Messages = append(sess.Messages, response.Message)
		sess.AgentState.CurrentStopReason = response.StopReason

		if len(response.ToolCalls) == 0 {
			break
		}

		for _, toolCall := range response.ToolCalls {
			if _, allowed := findToolDefinition(availableTools, toolCall.Name); !allowed {
				return TurnResult{}, fmt.Errorf("tool %s is not allowed for agent %s", toolCall.Name, agent.Name)
			}
			tool, ok := r.tools.Get(toolCall.Name)
			if !ok {
				return TurnResult{}, fmt.Errorf("tool %s not registered", toolCall.Name)
			}
			permissionReq, err := tool.EvaluatePermission(ctx, toolCall.Args, tools.ExecutionEnv{Workspace: req.Workspace, SessionID: sess.ID})
			if err != nil {
				return TurnResult{}, err
			}
			decision, err := permissionHandler(ctx, permissionReq)
			if err != nil {
				return TurnResult{}, err
			}
			if decision.Decision != permission.DecisionAllow {
				denied := types.Message{
					ID:        uuid.NewString(),
					Role:      "tool",
					CreatedAt: time.Now().UTC(),
					ToolResults: []types.ToolResult{{
						ToolCallID: toolCall.ID,
						ToolName:   toolCall.Name,
						Success:    false,
						Error:      decision.Reason,
					}},
				}
				sess.Messages = append(sess.Messages, denied)
				final = denied
				break
			}

			result, err := tool.ExecuteRaw(ctx, toolCall.Args, tools.ExecutionEnv{Workspace: req.Workspace, SessionID: sess.ID})
			toolResult := types.ToolResult{ToolCallID: toolCall.ID, ToolName: toolCall.Name, Success: result.Success, Content: result.Content, Error: result.Error, Metadata: result.Metadata}
			toolMsg := types.Message{ID: uuid.NewString(), Role: "tool", CreatedAt: time.Now().UTC(), ToolResults: []types.ToolResult{toolResult}}
			sess.Messages = append(sess.Messages, toolMsg)
			sess.AgentState.ToolBreakdown[toolCall.Name]++
			final = toolMsg
			if err != nil {
				break
			}
		}
	}

	if err := r.sessions.Save(sess); err != nil {
		return TurnResult{}, err
	}
	if _, err := r.memory.Update(sess); err != nil {
		return TurnResult{}, err
	}
	_ = r.jobs.Upsert(jobcore.Job{
		ID:        sess.ID,
		Workspace: req.Workspace,
		SessionID: sess.ID,
		Status:    jobcore.StatusRunning,
		StartedAt: sess.CreatedAt,
		UpdatedAt: time.Now().UTC(),
	})
	return TurnResult{Session: sess, Output: final}, nil
}

func (r *Runtime) resolveAgentContext(name string) (types.AgentDefinition, []types.SkillDefinition, error) {
	agents, err := r.resources.LoadAgents()
	if err != nil {
		return types.AgentDefinition{}, nil, err
	}
	skills, err := r.resources.LoadSkills()
	if err != nil {
		return types.AgentDefinition{}, nil, err
	}
	defaultAgent := types.AgentDefinition{
		Name:         "default",
		Description:  "Default local agent",
		Model:        "local",
		Tools:        []string{"read_file", "write_file", "bash", "ask_user"},
		SystemPrompt: "You are a local development agent.",
		Location:     types.LocationBuiltin,
	}
	if len(agents) == 0 {
		return defaultAgent, skills, nil
	}
	if name == "" {
		return agents[0], skills, nil
	}
	index := slices.IndexFunc(agents, func(agent types.AgentDefinition) bool { return agent.Name == name })
	if index < 0 {
		return types.AgentDefinition{}, nil, fmt.Errorf("agent %s not found", name)
	}
	return agents[index], skills, nil
}

func (r *Runtime) allowedTools(names []string) []provider.ToolDefinition {
	if len(names) == 0 {
		return provider.DefinitionsFromDescriptors(r.tools.List())
	}
	items := make([]provider.ToolDefinition, 0, len(names))
	for _, name := range names {
		tool, ok := r.tools.Get(name)
		if !ok {
			continue
		}
		descriptor := tool.Descriptor()
		items = append(items, provider.ToolDefinition{Name: descriptor.Name, Description: descriptor.Description, InputSchema: descriptor.InputSchema})
	}
	return items
}

func defaultProviderName(agent types.AgentDefinition) string {
	if agent.Model == "" || agent.Model == "local" {
		return "local"
	}
	return "openai"
}

func findToolDefinition(definitions []provider.ToolDefinition, name string) (provider.ToolDefinition, bool) {
	for _, item := range definitions {
		if item.Name == name {
			return item, true
		}
	}
	return provider.ToolDefinition{}, false
}
