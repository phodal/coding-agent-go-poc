package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"agent-platform/core/types"
	"github.com/google/uuid"
)

type Service struct {
	root string
}

func NewService(workspace string) *Service {
	return &Service{root: filepath.Join(workspace, ".agent-platform", "sessions")}
}

func (s *Service) New(workspace string) (types.Session, error) {
	id := uuid.NewString()
	now := time.Now().UTC()
	session := types.Session{
		ID:        id,
		Workspace: workspace,
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  []types.Message{},
		AgentState: types.AgentState{
			CurrentMode:   "normal",
			MaxIterations: 8,
			RecoveryLimit: 3,
			ToolBreakdown: map[string]int{},
		},
	}
	return session, s.Save(session)
}

func (s *Service) Save(session types.Session) error {
	dir := filepath.Join(s.root, session.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	session.UpdatedAt = time.Now().UTC()
	content, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := filepath.Join(dir, "session.json.tmp")
	finalPath := filepath.Join(dir, "session.json")
	if err := os.WriteFile(tmpPath, content, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, finalPath)
}

func (s *Service) Load(id string) (types.Session, error) {
	path := filepath.Join(s.root, id, "session.json")
	content, err := os.ReadFile(path)
	if err != nil {
		return types.Session{}, fmt.Errorf("load session %s: %w", id, err)
	}
	var session types.Session
	if err := json.Unmarshal(content, &session); err != nil {
		return types.Session{}, err
	}
	if session.AgentState.ToolBreakdown == nil {
		session.AgentState.ToolBreakdown = map[string]int{}
	}
	return session, nil
}
