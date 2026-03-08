package specengine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateFallbackSpec(t *testing.T) {
	workspace := t.TempDir()
	service, err := New(workspace)
	if err != nil {
		t.Fatalf("new spec service: %v", err)
	}
	result, err := service.Generate(context.Background(), GenerateRequest{
		Workspace:    workspace,
		Prompt:       "Implement a multi-agent CLI with tool calling.",
		ProviderName: "local",
	})
	if err != nil {
		t.Fatalf("generate spec: %v", err)
	}
	for _, name := range []string{
		"state",
		"requirements",
		"verifyRequirements",
		"checkpointRequirementsToDesign",
		"highLevelDesign",
		"highLevelDesignReviewRound1",
		"checkpointHldToLld",
		"lowLevelDesignOverview",
		"lldTask01",
		"lldTask02",
		"lldTask03",
		"lowLevelDesignReviewRound1",
		"checkpointLldToImplementation",
	} {
		path := result.Files[name]
		if path == "" {
			t.Fatalf("missing file path for %s", name)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
	}
	if filepath.Base(result.Directory) != result.SpecID {
		t.Fatalf("expected directory basename %s, got %s", result.SpecID, filepath.Base(result.Directory))
	}
	if result.CurrentStage != StageReadyForImplementation {
		t.Fatalf("expected final stage %s, got %s", StageReadyForImplementation, result.CurrentStage)
	}
	if !result.Validation.Valid {
		t.Fatalf("expected valid spec, got issues: %v", result.Validation.Issues)
	}
	state, err := service.LoadState(result.Directory)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(state.Tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(state.Tasks))
	}
	if len(state.Checkpoints) != 3 {
		t.Fatalf("expected 3 checkpoints, got %d", len(state.Checkpoints))
	}
	if filepath.Base(state.LLDOverviewPath) != "overview.md" {
		t.Fatalf("expected overview.md, got %s", filepath.Base(state.LLDOverviewPath))
	}
	for _, taskPath := range state.LLDTaskPaths {
		if !strings.Contains(filepath.Base(taskPath), "task-") {
			t.Fatalf("unexpected task path %s", taskPath)
		}
	}
}

func TestGenerateFallbackSpecStopsAtCheckpoint(t *testing.T) {
	workspace := t.TempDir()
	service, err := New(workspace)
	if err != nil {
		t.Fatalf("new spec service: %v", err)
	}
	result, err := service.Generate(context.Background(), GenerateRequest{
		Workspace:      workspace,
		Prompt:         "Implement a multi-agent CLI with tool calling.",
		ProviderName:   "local",
		CheckpointMode: CheckpointModeStop,
	})
	if err != nil {
		t.Fatalf("generate spec: %v", err)
	}
	if result.CurrentStage != StageCheckpointDesignApproved {
		t.Fatalf("expected stage %s, got %s", StageCheckpointDesignApproved, result.CurrentStage)
	}
	if result.Validation.Valid {
		t.Fatalf("expected invalid validation for stopped checkpoint flow")
	}
	if _, err := os.Stat(result.Files["checkpointRequirementsToDesign"]); err != nil {
		t.Fatalf("missing checkpoint artifact: %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.Directory, "02-high-level-design.md")); !os.IsNotExist(err) {
		t.Fatalf("expected no HLD artifact when stopped at checkpoint, got err=%v", err)
	}
}
