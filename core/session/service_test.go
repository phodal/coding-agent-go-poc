package session

import (
	"path/filepath"
	"testing"
)

func TestServiceNewAndLoad(t *testing.T) {
	workspace := t.TempDir()
	service := NewService(workspace)

	session, err := service.New(workspace)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	loaded, err := service.Load(session.ID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	if loaded.ID != session.ID {
		t.Fatalf("unexpected session ID %s", loaded.ID)
	}

	expectedPath := filepath.Join(workspace, ".agent-platform", "sessions", session.ID, "session.json")
	if loaded.Workspace != workspace {
		t.Fatalf("unexpected workspace %s, session file %s", loaded.Workspace, expectedPath)
	}
}
