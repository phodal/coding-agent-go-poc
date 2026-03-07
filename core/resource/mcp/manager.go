package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Scope string

const (
	ScopeLocal   Scope = "local"
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
)

type Server struct {
	Name        string            `json:"name"`
	Endpoint    string            `json:"endpoint"`
	Transport   string            `json:"transport"`
	Headers     map[string]string `json:"headers,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	Scope       Scope             `json:"scope"`
}

type Manager struct {
	workspace string
	homeDir   string
}

func NewManager(workspace string, homeDir string) *Manager {
	return &Manager{workspace: workspace, homeDir: homeDir}
}

func (m *Manager) path(scope Scope) string {
	switch scope {
	case ScopeUser:
		return filepath.Join(m.homeDir, ".agent-platform", "mcp", "servers.json")
	case ScopeProject, ScopeLocal:
		return filepath.Join(m.workspace, ".agent-platform", "mcp", "servers.json")
	default:
		return filepath.Join(m.workspace, ".agent-platform", "mcp", "servers.json")
	}
}

func (m *Manager) List(scope Scope) ([]Server, error) {
	content, err := os.ReadFile(m.path(scope))
	if err != nil {
		if os.IsNotExist(err) {
			return []Server{}, nil
		}
		return nil, err
	}
	var servers []Server
	if err := json.Unmarshal(content, &servers); err != nil {
		return nil, fmt.Errorf("parse mcp servers: %w", err)
	}
	return servers, nil
}

func (m *Manager) Add(server Server) error {
	servers, err := m.List(server.Scope)
	if err != nil {
		return err
	}
	for _, existing := range servers {
		if existing.Name == server.Name {
			return fmt.Errorf("mcp server %s already exists", server.Name)
		}
	}
	servers = append(servers, server)
	return m.save(server.Scope, servers)
}

func (m *Manager) Get(scope Scope, name string) (Server, error) {
	servers, err := m.List(scope)
	if err != nil {
		return Server{}, err
	}
	for _, server := range servers {
		if server.Name == name {
			return server, nil
		}
	}
	return Server{}, fmt.Errorf("mcp server %s not found", name)
}

func (m *Manager) Remove(scope Scope, name string) error {
	servers, err := m.List(scope)
	if err != nil {
		return err
	}
	filtered := make([]Server, 0, len(servers))
	for _, server := range servers {
		if server.Name != name {
			filtered = append(filtered, server)
		}
	}
	return m.save(scope, filtered)
}

func (m *Manager) ListAll() ([]Server, error) {
	scopes := []Scope{ScopeUser, ScopeProject, ScopeLocal}
	items := make([]Server, 0)
	seen := map[string]struct{}{}
	for _, scope := range scopes {
		servers, err := m.List(scope)
		if err != nil {
			return nil, err
		}
		for _, server := range servers {
			key := string(scope) + ":" + server.Name
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			items = append(items, server)
		}
	}
	return items, nil
}

func (m *Manager) save(scope Scope, servers []Server) error {
	path := m.path(scope)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content, err := json.MarshalIndent(servers, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}
