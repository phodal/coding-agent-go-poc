package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"agent-platform/core/permission"
)

func TestRuntimeOpenAIProviderToolFlow(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			http.NotFound(writer, request)
			return
		}
		requestNumber := requests.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		if requestNumber == 1 {
			_, _ = writer.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"mock-model","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":"","tool_calls":[{"id":"call-1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"README.md\"}"}}]}}]}`))
			return
		}
		_, _ = writer.Write([]byte(`{"id":"chatcmpl-2","object":"chat.completion","created":1,"model":"mock-model","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"README processed"}}]}`))
	}))
	defer server.Close()

	t.Setenv("AGENT_PLATFORM_OPENAI_BASE_URL", server.URL)
	t.Setenv("AGENT_PLATFORM_OPENAI_API_KEY", "test-key")
	t.Setenv("AGENT_PLATFORM_OPENAI_MODEL", "mock-model")

	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("hello openai runtime"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	agentDir := filepath.Join(workspace, ".agent-platform", "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	agentYAML := "name: reviewer\ndescription: mock remote reviewer\nmodel: mock-model\ntools:\n  - read_file\nsystemPrompt: You are a reviewer.\n"
	if err := os.WriteFile(filepath.Join(agentDir, "reviewer.yaml"), []byte(agentYAML), 0o644); err != nil {
		t.Fatalf("write agent yaml: %v", err)
	}

	runtime, err := New(workspace)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	result, err := runtime.Start(context.Background(), StartRequest{
		Workspace: workspace,
		Prompt:    "Read the README and summarize it.",
		AgentName: "reviewer",
		PermissionHandler: func(_ context.Context, _ permission.Request) (permission.Response, error) {
			return permission.Response{Decision: permission.DecisionAllow}, nil
		},
	})
	if err != nil {
		t.Fatalf("runtime start: %v", err)
	}
	if requests.Load() != 2 {
		t.Fatalf("expected 2 provider requests, got %d", requests.Load())
	}
	if len(result.Output.Content) == 0 || !strings.Contains(result.Output.Content[0].Text, "README processed") {
		t.Fatalf("unexpected final output: %+v", result.Output)
	}
	if result.Session.AgentState.ToolBreakdown["read_file"] != 1 {
		t.Fatalf("expected read_file tool count to be 1, got %d", result.Session.AgentState.ToolBreakdown["read_file"])
	}

	content, err := os.ReadFile(filepath.Join(workspace, ".agent-platform", "sessions", result.Session.ID, "session.json"))
	if err != nil {
		t.Fatalf("read session: %v", err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(content, &persisted); err != nil {
		t.Fatalf("unmarshal session: %v", err)
	}
}
