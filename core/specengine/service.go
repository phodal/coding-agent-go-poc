package specengine

import (
	"context"
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
	Workspace      string
	Prompt         string
	TaskName       string
	AgentName      string
	ProviderName   string
	ReviewRounds   int
	CheckpointMode CheckpointMode
}

type GenerateResult struct {
	SpecID       string            `json:"specId"`
	Directory    string            `json:"directory"`
	Files        map[string]string `json:"files"`
	StatePath    string            `json:"statePath"`
	CurrentStage SpecStage         `json:"currentStage"`
	Validation   ValidationResult  `json:"validation"`
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
	if req.Workspace == "" {
		req.Workspace = s.workspace
	}
	if req.ReviewRounds <= 0 {
		req.ReviewRounds = 1
	}
	if req.CheckpointMode == "" {
		req.CheckpointMode = CheckpointModeAuto
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

	taskName := strings.TrimSpace(req.TaskName)
	if taskName == "" {
		taskName = humanizeTaskName(req.Prompt)
	}
	specID := fmt.Sprintf("%s-%s", time.Now().UTC().Format("20060102-150405"), slugify(taskName))
	directory := filepath.Join(req.Workspace, ".agent-platform", "specs", specID)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return GenerateResult{}, err
	}
	state := SpecState{
		SpecID:         specID,
		TaskName:       taskName,
		Prompt:         strings.TrimSpace(req.Prompt),
		SpecDir:        directory,
		CurrentStage:   StageInitialized,
		ReviewRound:    req.ReviewRounds,
		CheckpointMode: req.CheckpointMode,
		GeneratedAt:    time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
		ProviderName:   providerName,
		AgentName:      agent.Name,
		StatePath:      filepath.Join(directory, "spec-state.json"),
	}
	if err := s.saveState(state); err != nil {
		return GenerateResult{}, err
	}

	files := map[string]string{"state": state.StatePath}
	requirements, err := s.generateStage(ctx, selectedProvider, agent, skills, "requirements", req.Prompt, nil)
	if err != nil {
		return GenerateResult{}, err
	}
	state.RequirementPath, err = writeArtifact(directory, "01-requirement.md", requirements)
	if err != nil {
		return GenerateResult{}, err
	}
	files["requirements"] = state.RequirementPath
	state.CurrentStage = StageRequirementsComplete
	if err := s.saveState(state); err != nil {
		return GenerateResult{}, err
	}

	verifyRequirements, err := s.generateStage(ctx, selectedProvider, agent, skills, "verify_requirements", req.Prompt, map[string]string{"requirements": requirements})
	if err != nil {
		return GenerateResult{}, err
	}
	state.VerifyPath, err = writeArtifact(directory, "01-verify-requirement.md", verifyRequirements)
	if err != nil {
		return GenerateResult{}, err
	}
	files["verifyRequirements"] = state.VerifyPath
	state.CurrentStage = StageVerifyRequirementsComplete
	if err := s.saveState(state); err != nil {
		return GenerateResult{}, err
	}

	checkpoint1, err := s.writeCheckpoint(directory, state, "checkpoint-01-requirements-to-design.json", StageCheckpointDesignApproved, "requirements_complete", "Transition from requirements to design stage")
	if err != nil {
		return GenerateResult{}, err
	}
	state.Checkpoints = append(state.Checkpoints, checkpoint1)
	state.CheckpointPaths = append(state.CheckpointPaths, checkpoint1.ArtifactPath)
	files["checkpointRequirementsToDesign"] = checkpoint1.ArtifactPath
	state.CurrentStage = StageCheckpointDesignApproved
	if err := s.saveState(state); err != nil {
		return GenerateResult{}, err
	}
	if req.CheckpointMode == CheckpointModeStop {
		validation, validationErr := s.Validate(state.SpecDir)
		return GenerateResult{SpecID: state.SpecID, Directory: state.SpecDir, Files: files, StatePath: state.StatePath, CurrentStage: state.CurrentStage, Validation: validation}, validationErr
	}

	hld, err := s.generateStage(ctx, selectedProvider, agent, skills, "high_level_design", req.Prompt, map[string]string{"requirements": requirements, "verify_requirements": verifyRequirements})
	if err != nil {
		return GenerateResult{}, err
	}
	state.HLDPath, err = writeArtifact(directory, "02-high-level-design.md", hld)
	if err != nil {
		return GenerateResult{}, err
	}
	files["highLevelDesign"] = state.HLDPath
	state.CurrentStage = StageHLDComplete
	if err := s.saveState(state); err != nil {
		return GenerateResult{}, err
	}

	for round := 1; round <= req.ReviewRounds; round++ {
		review, err := s.generateStage(ctx, selectedProvider, agent, skills, "high_level_design_review", req.Prompt, map[string]string{"requirements": requirements, "verify_requirements": verifyRequirements, "high_level_design": hld, "review_round": fmt.Sprintf("%d", round)})
		if err != nil {
			return GenerateResult{}, err
		}
		path, err := writeArtifact(directory, fmt.Sprintf("03-high-level-design-review-round-%d.md", round), review)
		if err != nil {
			return GenerateResult{}, err
		}
		state.HLDReviewPaths = append(state.HLDReviewPaths, path)
		files[fmt.Sprintf("highLevelDesignReviewRound%d", round)] = path
	}
	state.CurrentStage = StageHLDReviewComplete
	if err := s.saveState(state); err != nil {
		return GenerateResult{}, err
	}

	checkpoint2, err := s.writeCheckpoint(directory, state, "checkpoint-02-hld-review-to-lld.json", StageCheckpointLLDApproved, "high_level_design_review_complete", "Transition from HLD review to LLD stage")
	if err != nil {
		return GenerateResult{}, err
	}
	state.Checkpoints = append(state.Checkpoints, checkpoint2)
	state.CheckpointPaths = append(state.CheckpointPaths, checkpoint2.ArtifactPath)
	files["checkpointHldToLld"] = checkpoint2.ArtifactPath
	state.CurrentStage = StageCheckpointLLDApproved
	if err := s.saveState(state); err != nil {
		return GenerateResult{}, err
	}

	taskPlan, err := s.generateTaskPlan(ctx, selectedProvider, agent, skills, req.Prompt, requirements, verifyRequirements, hld)
	if err != nil {
		return GenerateResult{}, err
	}
	overviewBody, err := s.generateStage(ctx, selectedProvider, agent, skills, "low_level_design_overview", req.Prompt, map[string]string{"requirements": requirements, "verify_requirements": verifyRequirements, "high_level_design": hld, "task_plan": renderTaskPlanReference(taskPlan)})
	if err != nil {
		return GenerateResult{}, err
	}
	lldDir := filepath.Join(directory, "04-low-level-design")
	if err := os.MkdirAll(lldDir, 0o755); err != nil {
		return GenerateResult{}, err
	}
	state.Tasks = make([]TaskArtifact, 0, len(taskPlan))
	for index, plan := range taskPlan {
		taskID := fmt.Sprintf("task-%02d", index+1)
		fileName := fmt.Sprintf("task-%02d-%s.md", index+1, slugify(plan.Title))
		path, err := writeArtifact(lldDir, fileName, renderTaskMarkdown(taskID, plan))
		if err != nil {
			return GenerateResult{}, err
		}
		artifact := TaskArtifact{ID: taskID, Title: plan.Title, Summary: plan.Summary, FilePath: path, Dependencies: plan.Dependencies, Files: plan.Files, TestFocus: plan.TestFocus, Status: TaskStatusPending}
		state.Tasks = append(state.Tasks, artifact)
		state.LLDTaskPaths = append(state.LLDTaskPaths, path)
		files[fmt.Sprintf("lldTask%02d", index+1)] = path
	}
	state.LLDOverviewPath, err = writeArtifact(lldDir, "overview.md", renderLLDOverview(taskName, overviewBody, state.Tasks))
	if err != nil {
		return GenerateResult{}, err
	}
	files["lowLevelDesignOverview"] = state.LLDOverviewPath
	state.CurrentStage = StageLLDTasksComplete
	if err := s.saveState(state); err != nil {
		return GenerateResult{}, err
	}

	for round := 1; round <= req.ReviewRounds; round++ {
		review, err := s.generateStage(ctx, selectedProvider, agent, skills, "low_level_design_review", req.Prompt, map[string]string{"requirements": requirements, "verify_requirements": verifyRequirements, "high_level_design": hld, "low_level_design_overview": renderLLDOverview(taskName, overviewBody, state.Tasks), "low_level_design_tasks": renderTaskArtifactsReference(state.Tasks), "review_round": fmt.Sprintf("%d", round)})
		if err != nil {
			return GenerateResult{}, err
		}
		path, err := writeArtifact(directory, fmt.Sprintf("05-low-level-design-review-round-%d.md", round), review)
		if err != nil {
			return GenerateResult{}, err
		}
		state.LLDReviewPaths = append(state.LLDReviewPaths, path)
		files[fmt.Sprintf("lowLevelDesignReviewRound%d", round)] = path
	}
	state.CurrentStage = StageLLDReviewComplete
	if err := s.saveState(state); err != nil {
		return GenerateResult{}, err
	}

	checkpoint3, err := s.writeCheckpoint(directory, state, "checkpoint-03-lld-review-to-implementation.json", StageReadyForImplementation, "low_level_design_review_complete", "Transition from LLD review to implementation stage")
	if err != nil {
		return GenerateResult{}, err
	}
	state.Checkpoints = append(state.Checkpoints, checkpoint3)
	state.CheckpointPaths = append(state.CheckpointPaths, checkpoint3.ArtifactPath)
	files["checkpointLldToImplementation"] = checkpoint3.ArtifactPath
	state.CurrentStage = StageReadyForImplementation
	if err := s.saveState(state); err != nil {
		return GenerateResult{}, err
	}

	validation, err := s.Validate(state.SpecDir)
	if err != nil {
		return GenerateResult{}, err
	}
	return GenerateResult{SpecID: state.SpecID, Directory: state.SpecDir, Files: files, StatePath: state.StatePath, CurrentStage: state.CurrentStage, Validation: validation}, nil
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

func (s *Service) generateTaskPlan(ctx context.Context, selected provider.Provider, agent types.AgentDefinition, skills []types.SkillDefinition, prompt string, requirements string, verifyRequirements string, hld string) ([]taskPlan, error) {
	if selected.Name() == "local" {
		return fallbackTaskPlan(prompt), nil
	}
	response, err := selected.CreateResponse(ctx, provider.Request{
		Agent: types.AgentDefinition{
			Name:         agent.Name + "-spec-low-level-design-tasks",
			Description:  "Detailed design task planner",
			Model:        agent.Model,
			SystemPrompt: buildStageSystemPrompt(agent.SystemPrompt, "low_level_design_tasks"),
		},
		Model:    agent.Model,
		Messages: []types.Message{{ID: "low-level-design-tasks", Role: "user", CreatedAt: time.Now().UTC(), Content: []types.ContentPart{{Type: "text", Text: buildStagePrompt("low_level_design_tasks", prompt, map[string]string{"requirements": requirements, "verify_requirements": verifyRequirements, "high_level_design": hld})}}}},
		Skills:   skills,
		Tools:    nil,
	})
	if err != nil {
		return fallbackTaskPlan(prompt), nil
	}
	plans, parseErr := parseTaskPlan(joinParts(response.Message.Content))
	if parseErr != nil || len(plans) == 0 {
		return fallbackTaskPlan(prompt), nil
	}
	return plans, nil
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
		"requirements":              "Produce only the requirement artifact. Capture goals, scope, constraints, assumptions, functional requirements, non-functional requirements, and acceptance criteria.",
		"verify_requirements":       "Produce a verification strategy artifact. Define UT, IT, E2E, manual checks, expected evidence, and done criteria before implementation starts.",
		"high_level_design":         "Produce only the HLD artifact. Focus on modules, interfaces, data flow, affected files, rollout, observability, and testing strategy. Do not drop into implementation detail.",
		"high_level_design_review":  "Produce only the HLD review artifact. Evaluate coverage, over-design risk, missing interfaces, unclear boundaries, and blockers. Separate blocking issues from advisory notes.",
		"low_level_design_overview": "Produce the LLD overview artifact. Summarize sequencing, task dependencies, file ownership, testing focus, and status tracking expectations.",
		"low_level_design_tasks":    "Return only JSON: an array of 3 to 6 task objects. Each object must contain title, summary, files, dependencies, and testFocus.",
		"low_level_design_review":   "Produce only the LLD review artifact. Review task granularity, dependency order, missing edge cases, test adequacy, and readiness for implementation.",
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
	builder.WriteString("\n\nFollow the documented workflow exactly. Only generate the artifact required for this stage.")
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
		return fmt.Sprintf("# Requirement\n\n## Task\n%s\n\n## Goals\n- Clarify what must be delivered before design and coding start.\n- Provide a reviewable source of truth for later stages.\n\n## Scope\n- In scope: the requested feature and its direct engineering consequences.\n- Out of scope: unrelated refactors and speculative platform work.\n\n## Constraints\n- Preserve local-first execution.\n- Keep provider and tool abstractions explicit.\n- Make outputs resumable from the filesystem.\n\n## Functional Requirements\n- Capture the requested capability.\n- Define the expected workflow boundaries.\n- Describe the required artifacts for downstream stages.\n\n## Non-Functional Requirements\n- Clear failure behavior.\n- Deterministic artifact paths.\n- Testable implementation slices.\n\n## Acceptance Criteria\n- Requirement, design, review, and LLD artifacts can be generated in order.\n- Stage transitions are visible through checkpoint artifacts.\n- Implementation can start from the generated LLD task set.\n", strings.TrimSpace(prompt))
	case "verify_requirements":
		return fmt.Sprintf("# Verify Requirement\n\n## Verification Strategy\n- UT: validate pure logic, parsing, and state transitions.\n- IT: validate provider, runtime, and persistence interaction.\n- E2E: validate CLI generation flow and artifact layout.\n\n## Required Evidence\n- Passing go test ./...\n- Successful CLI smoke test for spec generation\n- Persisted artifacts under the spec directory\n\n## Manual Checks\n- Review checkpoint files and stage order.\n- Confirm LLD tasks are implementation-ready.\n\n## References\n%s\n", strings.Join(sortedPrevious(previous), "\n\n"))
	case "high_level_design":
		return "# High-Level Design\n\n## Architecture\n- Spec engine orchestrates a staged workflow over filesystem artifacts.\n- Provider abstraction supplies stage content generation.\n- State file tracks stage, artifact paths, review rounds, and checkpoints.\n\n## Major Components\n- CLI command layer\n- Spec engine orchestration service\n- Artifact renderer and validator\n- Provider integration layer\n\n## Data Flow\n1. Accept request and resolve agent/provider.\n2. Generate requirement and verification artifacts.\n3. Gate progress with explicit checkpoints.\n4. Produce HLD, HLD review, LLD overview, task docs, and LLD review.\n5. Persist state for resume and validation.\n\n## Affected Files\n- core/specengine/*\n- cmd/spec/*\n- tests and docs as needed\n\n## Testing Strategy\n- Unit test stage generation and validation.\n- Smoke test CLI output.\n"
	case "high_level_design_review":
		return fmt.Sprintf("# High-Level Design Review\n\n## Coverage Check\n- Requirement coverage is explicit.\n- Checkpoints and review rounds are represented.\n- State persistence exists for resume and validation.\n\n## Blocking Issues\n- None in fallback mode.\n\n## Advisory Notes\n- Keep artifact names aligned with the workflow contract.\n- Ensure LLD tasks remain implementation-ready.\n\n## Decision\n- Pass with advisory notes.\n\n## References\n%s\n", strings.Join(sortedPrevious(previous), "\n\n"))
	case "low_level_design_overview":
		return fmt.Sprintf("# Low-Level Design Overview\n\n## Execution Model\n- Tasks execute serially.\n- Status moves from pending to in_progress to completed.\n- Resume starts from the first non-completed task.\n\n## Dependency Rules\n- Respect requirement and HLD ordering.\n- Complete blocking infrastructure tasks before feature tasks.\n\n## Testing Focus\n- Validate state persistence.\n- Validate artifact completeness.\n- Validate CLI and provider integration boundaries.\n\n## References\n%s\n", strings.Join(sortedPrevious(previous), "\n\n"))
	case "low_level_design_review":
		return fmt.Sprintf("# Low-Level Design Review\n\n## Readiness Check\n- Tasks are ordered and implementation-ready.\n- File targets and dependencies are explicit.\n- Test focus is attached to each task.\n\n## Blocking Issues\n- None in fallback mode.\n\n## Advisory Notes\n- Keep tasks atomic and avoid bundling unrelated changes.\n- Preserve task status updates in the overview file.\n\n## Decision\n- Ready for implementation.\n\n## References\n%s\n", strings.Join(sortedPrevious(previous), "\n\n"))
	default:
		return fmt.Sprintf("# Artifact\n\nGenerated fallback content for stage %s.\n\n## Prompt\n%s\n", stage, strings.TrimSpace(prompt))
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
