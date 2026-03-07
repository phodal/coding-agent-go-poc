package specengine

import (
	"context"
	"os"
	"path/filepath"
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
	for _, name := range []string{"requirements", "design", "tasks", "checkpoint"} {
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
}
