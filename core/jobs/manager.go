package jobs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Status string

const (
	StatusRunning Status = "running"
	StatusStopped Status = "stopped"
)

type Job struct {
	ID        string    `json:"id"`
	Workspace string    `json:"workspace"`
	SessionID string    `json:"sessionId"`
	Status    Status    `json:"status"`
	StartedAt time.Time `json:"startedAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Manager struct {
	workspace string
}

func NewManager(workspace string) *Manager {
	return &Manager{workspace: workspace}
}

func (m *Manager) path() string {
	return filepath.Join(m.workspace, ".agent-platform", "jobs", "jobs.json")
}

func (m *Manager) List() ([]Job, error) {
	content, err := os.ReadFile(m.path())
	if err != nil {
		if os.IsNotExist(err) {
			return []Job{}, nil
		}
		return nil, err
	}
	var jobs []Job
	if err := json.Unmarshal(content, &jobs); err != nil {
		return nil, fmt.Errorf("parse jobs: %w", err)
	}
	return jobs, nil
}

func (m *Manager) Upsert(job Job) error {
	jobs, err := m.List()
	if err != nil {
		return err
	}
	found := false
	for index := range jobs {
		if jobs[index].ID == job.ID {
			jobs[index] = job
			found = true
			break
		}
	}
	if !found {
		jobs = append(jobs, job)
	}
	if err := os.MkdirAll(filepath.Dir(m.path()), 0o755); err != nil {
		return err
	}
	content, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path(), content, 0o644)
}
