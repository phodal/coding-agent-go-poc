package session_memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-platform/core/types"
)

type State struct {
	MemoryPath          string
	LastSummarizedMsgID string
	LastUpdatedAt       time.Time
}

type Manager struct {
	workspace string
}

func NewManager(workspace string) *Manager {
	return &Manager{workspace: workspace}
}

func (m *Manager) Update(session types.Session) (State, error) {
	path := filepath.Join(m.workspace, ".agent-platform", "sessions", session.ID, "session_memory.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return State{}, err
	}
	content := buildMemory(session)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return State{}, err
	}
	state := State{MemoryPath: path, LastUpdatedAt: time.Now().UTC()}
	if len(session.Messages) > 0 {
		state.LastSummarizedMsgID = session.Messages[len(session.Messages)-1].ID
	}
	return state, nil
}

func buildMemory(session types.Session) string {
	var b strings.Builder
	b.WriteString("# Session Memory\n\n")
	b.WriteString(fmt.Sprintf("- Session ID: %s\n", session.ID))
	b.WriteString(fmt.Sprintf("- Current Agent: %s\n", session.AgentState.CurrentAgent))
	b.WriteString(fmt.Sprintf("- Current Stop Reason: %s\n", session.AgentState.CurrentStopReason))
	b.WriteString("\n## Tool Breakdown\n")
	if len(session.AgentState.ToolBreakdown) == 0 {
		b.WriteString("- No tools used yet\n")
	} else {
		for name, count := range session.AgentState.ToolBreakdown {
			b.WriteString(fmt.Sprintf("- %s: %d\n", name, count))
		}
	}
	b.WriteString("\n## Recent Messages\n")
	start := max(0, len(session.Messages)-6)
	for _, msg := range session.Messages[start:] {
		if len(msg.Content) > 0 {
			b.WriteString(fmt.Sprintf("- %s: %s\n", msg.Role, joinContent(msg.Content)))
			continue
		}
		if len(msg.ToolCalls) > 0 {
			b.WriteString(fmt.Sprintf("- %s requested tool %s\n", msg.Role, msg.ToolCalls[0].Name))
			continue
		}
		if len(msg.ToolResults) > 0 {
			result := msg.ToolResults[0]
			status := "success"
			if !result.Success {
				status = "failed"
			}
			b.WriteString(fmt.Sprintf("- tool %s %s\n", result.ToolName, status))
		}
	}
	return b.String()
}

func joinContent(parts []types.ContentPart) string {
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		items = append(items, strings.TrimSpace(part.Text))
	}
	return strings.Join(items, " ")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
