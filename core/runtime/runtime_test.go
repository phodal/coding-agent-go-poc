package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-platform/core/permission"
)

func TestRuntimeReadFileFlow(t *testing.T) {
	workspace := t.TempDir()
	readmePath := filepath.Join(workspace, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello runtime"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	runtime, err := New(workspace)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	result, err := runtime.Start(context.Background(), StartRequest{
		Workspace: workspace,
		Prompt:    "read README.md",
		PermissionHandler: func(_ context.Context, _ permission.Request) (permission.Response, error) {
			return permission.Response{Decision: permission.DecisionAllow}, nil
		},
	})
	if err != nil {
		t.Fatalf("runtime start: %v", err)
	}

	if len(result.Output.Content) == 0 {
		t.Fatalf("expected assistant output content")
	}
	if !strings.Contains(result.Output.Content[0].Text, "hello runtime") {
		t.Fatalf("unexpected output %q", result.Output.Content[0].Text)
	}
	if result.Session.AgentState.ToolBreakdown["read_file"] != 1 {
		t.Fatalf("expected read_file tool count to be 1, got %d", result.Session.AgentState.ToolBreakdown["read_file"])
	}
}
