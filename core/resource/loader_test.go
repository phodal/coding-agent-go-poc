package resource

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProjectAgents(t *testing.T) {
	workspace := t.TempDir()
	agentDir := filepath.Join(workspace, ".agent-platform", "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("mkdir agent dir: %v", err)
	}
	content := []byte("name: builder\ndescription: test builder\nmodel: local\ntools:\n  - read_file\n")
	if err := os.WriteFile(filepath.Join(agentDir, "builder.yaml"), content, 0o644); err != nil {
		t.Fatalf("write agent yaml: %v", err)
	}

	loader, err := NewLoader(workspace)
	if err != nil {
		t.Fatalf("new loader: %v", err)
	}
	agents, err := loader.LoadAgents()
	if err != nil {
		t.Fatalf("load agents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Name != "builder" {
		t.Fatalf("unexpected agent %s", agents[0].Name)
	}
}
