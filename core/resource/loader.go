package resource

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"agent-platform/core/types"
	"gopkg.in/yaml.v3"
)

type Loader struct {
	workspace string
	homeDir   string
}

func NewLoader(workspace string) (*Loader, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user home: %w", err)
	}
	return &Loader{workspace: workspace, homeDir: homeDir}, nil
}

func (l *Loader) LoadAgents() ([]types.AgentDefinition, error) {
	paths := []struct {
		base     string
		location types.ResourceLocation
	}{
		{base: filepath.Join(l.homeDir, ".agent-platform", "agents"), location: types.LocationGlobal},
		{base: filepath.Join(l.workspace, ".agent-platform", "agents"), location: types.LocationProject},
	}

	var items []types.AgentDefinition
	for _, path := range paths {
		loaded, err := loadYAMLFiles[types.AgentDefinition](path.base, path.location)
		if err != nil {
			return nil, err
		}
		items = append(items, loaded...)
	}
	return items, nil
}

func (l *Loader) LoadSkills() ([]types.SkillDefinition, error) {
	paths := []struct {
		base     string
		location types.ResourceLocation
	}{
		{base: filepath.Join(l.homeDir, ".agent-platform", "skills"), location: types.LocationGlobal},
		{base: filepath.Join(l.workspace, ".agent-platform", "skills"), location: types.LocationProject},
	}

	var items []types.SkillDefinition
	for _, path := range paths {
		loaded, err := loadYAMLFiles[types.SkillDefinition](path.base, path.location)
		if err != nil {
			return nil, err
		}
		items = append(items, loaded...)
	}
	return items, nil
}

func loadYAMLFiles[T any](base string, location types.ResourceLocation) ([]T, error) {
	entries := []T{}
	stat, err := os.Stat(base)
	if err != nil {
		if os.IsNotExist(err) {
			return entries, nil
		}
		return nil, err
	}
	if !stat.IsDir() {
		return entries, nil
	}

	err = filepath.WalkDir(base, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var item T
		if err := yaml.Unmarshal(content, &item); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		injectLocation(&item, location, path)
		entries = append(entries, item)
		return nil
	})
	return entries, err
}

func injectLocation[T any](item *T, location types.ResourceLocation, path string) {
	switch value := any(item).(type) {
	case *types.AgentDefinition:
		value.Location = location
		value.StoragePath = path
	case *types.SkillDefinition:
		value.Location = location
		value.StoragePath = path
	}
}
