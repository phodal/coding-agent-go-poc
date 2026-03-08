package specengine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type SpecStage string

const (
	StageInitialized                SpecStage = "initialized"
	StageRequirementsComplete       SpecStage = "requirements_complete"
	StageVerifyRequirementsComplete SpecStage = "verify_requirements_complete"
	StageCheckpointDesignApproved   SpecStage = "checkpoint_design_approved"
	StageHLDComplete                SpecStage = "high_level_design_complete"
	StageHLDReviewComplete          SpecStage = "high_level_design_review_complete"
	StageCheckpointLLDApproved      SpecStage = "checkpoint_lld_approved"
	StageLLDOverviewComplete        SpecStage = "low_level_design_overview_complete"
	StageLLDTasksComplete           SpecStage = "low_level_design_tasks_complete"
	StageLLDReviewComplete          SpecStage = "low_level_design_review_complete"
	StageReadyForImplementation     SpecStage = "ready_for_implementation"
)

type CheckpointMode string

const (
	CheckpointModeAuto CheckpointMode = "auto"
	CheckpointModeStop CheckpointMode = "stop"
)

type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusCompleted  TaskStatus = "completed"
)

type ValidationResult struct {
	Valid        bool      `json:"valid"`
	CurrentStage SpecStage `json:"currentStage"`
	CheckedFiles []string  `json:"checkedFiles"`
	Issues       []string  `json:"issues,omitempty"`
}

type CheckpointArtifact struct {
	Name         string    `json:"name"`
	Stage        SpecStage `json:"stage"`
	Approved     bool      `json:"approved"`
	Decision     string    `json:"decision"`
	Description  string    `json:"description"`
	ArtifactPath string    `json:"artifactPath"`
	CreatedAt    time.Time `json:"createdAt"`
}

type TaskArtifact struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	Summary      string     `json:"summary"`
	FilePath     string     `json:"filePath"`
	Dependencies []string   `json:"dependencies,omitempty"`
	Files        []string   `json:"files,omitempty"`
	TestFocus    []string   `json:"testFocus,omitempty"`
	Status       TaskStatus `json:"status"`
}

type SpecState struct {
	SpecID          string               `json:"specId"`
	TaskName        string               `json:"taskName"`
	Prompt          string               `json:"prompt"`
	SpecDir         string               `json:"specDir"`
	RequirementPath string               `json:"requirementPath"`
	VerifyPath      string               `json:"verifyPath"`
	HLDPath         string               `json:"hldPath"`
	HLDReviewPaths  []string             `json:"hldReviewPaths,omitempty"`
	LLDOverviewPath string               `json:"lldOverviewPath"`
	LLDTaskPaths    []string             `json:"lldTaskPaths,omitempty"`
	LLDReviewPaths  []string             `json:"lldReviewPaths,omitempty"`
	CheckpointPaths []string             `json:"checkpointPaths,omitempty"`
	Tasks           []TaskArtifact       `json:"tasks,omitempty"`
	Checkpoints     []CheckpointArtifact `json:"checkpoints,omitempty"`
	CurrentStage    SpecStage            `json:"currentStage"`
	ReviewRound     int                  `json:"reviewRound"`
	CheckpointMode  CheckpointMode       `json:"checkpointMode"`
	GeneratedAt     time.Time            `json:"generatedAt"`
	UpdatedAt       time.Time            `json:"updatedAt"`
	ProviderName    string               `json:"providerName"`
	AgentName       string               `json:"agentName"`
	StatePath       string               `json:"statePath"`
}

type taskPlan struct {
	Title        string   `json:"title"`
	Summary      string   `json:"summary"`
	Files        []string `json:"files"`
	Dependencies []string `json:"dependencies"`
	TestFocus    []string `json:"testFocus"`
}

func (s *Service) LoadState(specDir string) (SpecState, error) {
	path := filepath.Join(specDir, "spec-state.json")
	content, err := os.ReadFile(path)
	if err != nil {
		return SpecState{}, err
	}
	var state SpecState
	if err := json.Unmarshal(content, &state); err != nil {
		return SpecState{}, err
	}
	if state.StatePath == "" {
		state.StatePath = path
	}
	return state, nil
}

func (s *Service) Validate(specDir string) (ValidationResult, error) {
	state, err := s.LoadState(specDir)
	if err != nil {
		return ValidationResult{}, err
	}
	checked := []string{state.RequirementPath, state.VerifyPath, state.HLDPath, state.LLDOverviewPath, state.StatePath}
	checked = append(checked, state.HLDReviewPaths...)
	checked = append(checked, state.LLDTaskPaths...)
	checked = append(checked, state.LLDReviewPaths...)
	checked = append(checked, state.CheckpointPaths...)
	issues := make([]string, 0)
	for _, path := range checked {
		if path == "" {
			issues = append(issues, "empty artifact path detected")
			continue
		}
		if _, err := os.Stat(path); err != nil {
			issues = append(issues, fmt.Sprintf("missing artifact: %s", path))
		}
	}
	if len(state.Tasks) == 0 {
		issues = append(issues, "no low-level design tasks generated")
	}
	if len(state.Checkpoints) != 3 {
		issues = append(issues, fmt.Sprintf("expected 3 checkpoints, got %d", len(state.Checkpoints)))
	}
	if state.CurrentStage != StageReadyForImplementation && state.CheckpointMode == CheckpointModeAuto {
		issues = append(issues, fmt.Sprintf("expected final stage %s, got %s", StageReadyForImplementation, state.CurrentStage))
	}
	return ValidationResult{Valid: len(issues) == 0, CurrentStage: state.CurrentStage, CheckedFiles: checked, Issues: issues}, nil
}

func (s *Service) saveState(state SpecState) error {
	state.UpdatedAt = time.Now().UTC()
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(state.StatePath, content, 0o644)
}

func (s *Service) writeCheckpoint(specDir string, state SpecState, fileName string, nextStage SpecStage, decision string, description string) (CheckpointArtifact, error) {
	artifact := CheckpointArtifact{
		Name:         strings.TrimSuffix(fileName, filepath.Ext(fileName)),
		Stage:        nextStage,
		Approved:     state.CheckpointMode == CheckpointModeAuto,
		Decision:     decision,
		Description:  description,
		ArtifactPath: filepath.Join(specDir, fileName),
		CreatedAt:    time.Now().UTC(),
	}
	content, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return CheckpointArtifact{}, err
	}
	return artifact, os.WriteFile(artifact.ArtifactPath, content, 0o644)
}

func writeArtifact(baseDir string, fileName string, content string) (string, error) {
	path := filepath.Join(baseDir, fileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644)
}

func fallbackTaskPlan(prompt string) []taskPlan {
	title := humanizeTaskName(prompt)
	return []taskPlan{
		{Title: "Define stage state and checkpoint artifacts", Summary: "Model the spec workflow state, checkpoint artifacts, and persisted metadata so the flow is resumable and auditable.", Files: []string{"core/specengine/service.go", "cmd/spec/spec.go"}, TestFocus: []string{"state persistence", "checkpoint generation"}},
		{Title: "Render HLD and LLD artifacts", Summary: "Generate the documented artifact set, including HLD review and LLD overview/task artifacts for the request.", Files: []string{"core/specengine/service.go"}, Dependencies: []string{"Define stage state and checkpoint artifacts"}, TestFocus: []string{"artifact names", "task document rendering"}},
		{Title: "Validate and smoke test " + title, Summary: "Add validation and tests that prove the generated spec follows the documented stage order and file layout.", Files: []string{"core/specengine/service_test.go", "cmd/spec/spec.go"}, Dependencies: []string{"Render HLD and LLD artifacts"}, TestFocus: []string{"validation", "CLI smoke flow"}},
	}
}

func parseTaskPlan(content string) ([]taskPlan, error) {
	cleaned := strings.TrimSpace(content)
	if strings.Contains(cleaned, "```") {
		matches := regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)```").FindStringSubmatch(cleaned)
		if len(matches) == 2 {
			cleaned = strings.TrimSpace(matches[1])
		}
	}
	var plans []taskPlan
	if err := json.Unmarshal([]byte(cleaned), &plans); err != nil {
		return nil, err
	}
	return plans, nil
}

func renderTaskMarkdown(taskID string, task taskPlan) string {
	return fmt.Sprintf("# %s %s\n\n## Summary\n%s\n\n## Files\n%s\n\n## Dependencies\n%s\n\n## Core Logic\n- Implement only the scope required for this task.\n- Keep behavior aligned with the HLD and LLD overview.\n\n## Edge Cases\n- Empty or missing artifacts.\n- Resume behavior after partial execution.\n- Invalid stage transitions.\n\n## Unit Test Design\n%s\n\n## Status\n- pending\n", taskID, task.Title, strings.TrimSpace(task.Summary), renderMarkdownList(task.Files), renderMarkdownList(task.Dependencies), renderMarkdownList(task.TestFocus))
}

func renderLLDOverview(taskName string, body string, tasks []TaskArtifact) string {
	var builder strings.Builder
	builder.WriteString("# Low-Level Design Overview\n\n## Task\n")
	builder.WriteString(taskName)
	builder.WriteString("\n\n")
	builder.WriteString(strings.TrimSpace(body))
	builder.WriteString("\n\n## Task Status\n| Task | Title | Status | Dependencies |\n| --- | --- | --- | --- |\n")
	for _, task := range tasks {
		deps := strings.Join(task.Dependencies, ", ")
		if deps == "" {
			deps = "-"
		}
		builder.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", task.ID, task.Title, task.Status, deps))
	}
	builder.WriteString("\n## Execution Rule\n- Update task status from pending to in_progress to completed during implementation.\n")
	return builder.String()
}

func renderTaskPlanReference(plans []taskPlan) string {
	items := make([]string, 0, len(plans))
	for index, plan := range plans {
		items = append(items, fmt.Sprintf("%d. %s: %s", index+1, plan.Title, plan.Summary))
	}
	return strings.Join(items, "\n")
}

func renderTaskArtifactsReference(tasks []TaskArtifact) string {
	items := make([]string, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, fmt.Sprintf("- %s %s (%s)", task.ID, task.Title, task.FilePath))
	}
	return strings.Join(items, "\n")
}

func renderMarkdownList(items []string) string {
	if len(items) == 0 {
		return "- None"
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, "- "+item)
	}
	return strings.Join(lines, "\n")
}

func humanizeTaskName(input string) string {
	parts := strings.Fields(regexp.MustCompile(`[^A-Za-z0-9]+`).ReplaceAllString(strings.TrimSpace(input), " "))
	if len(parts) == 0 {
		return "Spec Task"
	}
	for index, part := range parts {
		parts[index] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	name := strings.Join(parts, " ")
	if len(name) > 80 {
		return name[:80]
	}
	return name
}
