package specengine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"agent-platform/core/provider"
	"agent-platform/core/resource"
	"agent-platform/core/types"
)

type GenerateRequest struct {
	Workspace    string
	Prompt       string
	AgentName    string
	ProviderName string
}

type GenerateResult struct {
	SpecID    string            `json:"specId"`
	Directory string            `json:"directory"`
	Files     map[string]string `json:"files"`
}

type Service struct {
	workspace string
	resources *resource.Loader
	providers *provider.Registry
}

func New(workspace string) (*Service, error) {
	resources, err := resource.NewLoader(workspace)
	if err != nil {
		return nil, err
	}
	providers := provider.NewRegistry()
	providers.Register(provider.LocalProvider{})
	providers.Register(provider.NewOpenAIProvider(provider.LoadOpenAIConfigFromEnv()))
	return &Service{workspace: workspace, resources: resources, providers: providers}, nil
}

func (s *Service) Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return GenerateResult{}, fmt.Errorf("prompt is required")
	}
	agent, skills, err := s.resolveAgentContext(req.AgentName)
	if err != nil {
		return GenerateResult{}, err
	}
	providerName := req.ProviderName
	if providerName == "" {
		if agent.Model == "" || agent.Model == "local" {
			providerName = "local"
		} else {
			providerName = "openai"
		}
	}
	selectedProvider, ok := s.providers.Get(providerName)
	if !ok {
		return GenerateResult{}, fmt.Errorf("provider %s not registered", providerName)
	}

	specID := fmt.Sprintf("%s-%s", time.Now().UTC().Format("20060102-150405"), slugify(req.Prompt))
	directory := filepath.Join(req.Workspace, ".agent-platform", "specs", specID)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return GenerateResult{}, err
	}

	files := map[string]string{}
	requirements, err := s.generateStage(ctx, selectedProvider, agent, skills, "requirements", req.Prompt, nil)
	if err != nil {
		return GenerateResult{}, err
	}
	requirementsPath := filepath.Join(directory, "requirements.md")
	if err := os.WriteFile(requirementsPath, []byte(requirements), 0o644); err != nil {
		return GenerateResult{}, err
	}
	files["requirements"] = requirementsPath

	design, err := s.generateStage(ctx, selectedProvider, agent, skills, "design", req.Prompt, map[string]string{"requirements": requirements})
	if err != nil {
		return GenerateResult{}, err
	}
	designPath := filepath.Join(directory, "design.md")
	if err := os.WriteFile(designPath, []byte(design), 0o644); err != nil {
		return GenerateResult{}, err
	}
	files["design"] = designPath

	tasks, err := s.generateStage(ctx, selectedProvider, agent, skills, "tasks", req.Prompt, map[string]string{"requirements": requirements, "design": design})
	if err != nil {
		return GenerateResult{}, err
	}
	tasksPath := filepath.Join(directory, "tasks.md")
	if err := os.WriteFile(tasksPath, []byte(tasks), 0o644); err != nil {
		return GenerateResult{}, err
	}
	files["tasks"] = tasksPath

	checkpoint := map[string]any{
		"specId":      specID,
		"prompt":      req.Prompt,
		"provider":    providerName,
		"agent":       agent.Name,
		"generatedAt": time.Now().UTC().Format(time.RFC3339),
		"files":       files,
	}
	checkpointContent, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return GenerateResult{}, err
	}
	checkpointPath := filepath.Join(directory, "checkpoint.json")
	if err := os.WriteFile(checkpointPath, checkpointContent, 0o644); err != nil {
		return GenerateResult{}, err
	}
	files["checkpoint"] = checkpointPath

	return GenerateResult{SpecID: specID, Directory: directory, Files: files}, nil
}

func (s *Service) generateStage(ctx context.Context, selected provider.Provider, agent types.AgentDefinition, skills []types.SkillDefinition, stage string, prompt string, previous map[string]string) (string, error) {
	if selected.Name() == "local" {
		return fallbackStage(stage, prompt, previous), nil
	}
	stagePrompt := buildStagePrompt(stage, prompt, previous)
	response, err := selected.CreateResponse(ctx, provider.Request{
		Agent: types.AgentDefinition{
			Name:         agent.Name + "-spec-" + stage,
			Description:  "Spec generation stage",
			Model:        agent.Model,
			SystemPrompt: buildStageSystemPrompt(agent.SystemPrompt, stage),
		},
		Model:    agent.Model,
		Messages: []types.Message{{ID: stage, Role: "user", CreatedAt: time.Now().UTC(), Content: []types.ContentPart{{Type: "text", Text: stagePrompt}}}},
		Skills:   skills,
		Tools:    nil,
	})
	if err != nil {
		return fallbackStage(stage, prompt, previous), nil
	}
	content := strings.TrimSpace(joinParts(response.Message.Content))
	if content == "" {
		return fallbackStage(stage, prompt, previous), nil
	}
	return content, nil
}

func (s *Service) resolveAgentContext(name string) (types.AgentDefinition, []types.SkillDefinition, error) {
	agents, err := s.resources.LoadAgents()
	if err != nil {
		return types.AgentDefinition{}, nil, err
	}
	skills, err := s.resources.LoadSkills()
	if err != nil {
		return types.AgentDefinition{}, nil, err
	}
	defaultAgent := types.AgentDefinition{
		Name:         "spec-writer",
		Description:  "Specification authoring agent",
		Model:        provider.LoadOpenAIConfigFromEnv().Model,
		SystemPrompt: "You are a rigorous software architect. Produce concrete, implementation-ready engineering artifacts.",
		Location:     types.LocationBuiltin,
	}
	if defaultAgent.Model == "" {
		defaultAgent.Model = "local"
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

func buildStageSystemPrompt(base string, stage string) string {
	stageInstruction := map[string]string{
		"requirements": "Write a requirements document with goals, scope, constraints, functional requirements, non-functional requirements, risks, and acceptance criteria.",
		"design":       "Write a technical design with architecture, components, data flow, interfaces, failure modes, observability, and rollout notes.",
		"tasks":        "Write an implementation plan with ordered tasks, validation steps, and open questions.",
	}[stage]
	parts := []string{strings.TrimSpace(base), stageInstruction}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func buildStagePrompt(stage string, prompt string, previous map[string]string) string {
	var builder strings.Builder
	builder.WriteString("Project request:\n")
	builder.WriteString(strings.TrimSpace(prompt))
	builder.WriteString("\n\nStage: ")
	builder.WriteString(stage)
	if len(previous) > 0 {
		keys := make([]string, 0, len(previous))
		for key := range previous {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			builder.WriteString("\n\nReference ")
			builder.WriteString(key)
			builder.WriteString(":\n")
			builder.WriteString(strings.TrimSpace(previous[key]))
		}
	}
	return builder.String()
}

func fallbackStage(stage string, prompt string, previous map[string]string) string {
	switch stage {
	case "requirements":
		return fmt.Sprintf("# Requirements\n\n## Problem\n%s\n\n## Goals\n- Deliver the requested capability end-to-end.\n- Keep the implementation testable and observable.\n\n## Functional Requirements\n- Accept the user input and execute the primary workflow.\n- Persist the artifacts needed to continue work safely.\n- Expose enough CLI surface for local iteration.\n\n## Non-Functional Requirements\n- Fail with clear errors.\n- Keep file layout predictable.\n- Preserve local-first execution.\n\n## Acceptance Criteria\n- A developer can run the workflow locally.\n- Core paths have automated coverage.\n- Generated outputs are stored under .agent-platform/specs.\n", strings.TrimSpace(prompt))
	case "design":
		return fmt.Sprintf("# Design\n\n## Architecture\n- CLI command accepts a spec prompt and selects a provider.\n- Spec engine generates three artifacts: requirements, design, and tasks.\n- Outputs are checkpointed under a stable spec directory.\n\n## Components\n- Command layer for argument parsing.\n- Spec service for stage orchestration.\n- Provider abstraction for model-backed authoring.\n- Fallback renderer when no remote model is configured.\n\n## Data Flow\n1. Receive prompt.\n2. Resolve agent and provider.\n3. Generate stage documents in order.\n4. Persist documents and checkpoint metadata.\n\n## Inputs\n- User prompt\n- Optional reference docs\n\n## Risks\n- Remote provider may fail; fallback keeps the flow usable.\n- Poor prompts can produce vague specs; stage-specific prompts constrain that.\n\n## References\n%s\n", strings.Join(sortedPrevious(previous), "\n\n"))
	default:
		return fmt.Sprintf("# Tasks\n\n1. Finalize requirements and scope for: %s\n2. Validate design boundaries and interfaces.\n3. Implement the runtime changes in small, testable increments.\n4. Add unit and integration tests for the critical flow.\n5. Run build and test verification before release.\n", strings.TrimSpace(prompt))
	}
}

func sortedPrevious(previous map[string]string) []string {
	keys := make([]string, 0, len(previous))
	for key := range previous {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	items := make([]string, 0, len(keys))
	for _, key := range keys {
		items = append(items, "## "+strings.Title(key)+"\n"+strings.TrimSpace(previous[key]))
	}
	return items
}

func joinParts(parts []types.ContentPart) string {
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		items = append(items, part.Text)
	}
	return strings.Join(items, "\n")
}

func slugify(input string) string {
	cleaned := strings.ToLower(strings.TrimSpace(input))
	cleaned = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(cleaned, "-")
	cleaned = strings.Trim(cleaned, "-")
	if cleaned == "" {
		return "spec"
	}
	if len(cleaned) > 40 {
		return cleaned[:40]
	}
	return cleaned
}
