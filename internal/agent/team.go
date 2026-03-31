package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/termia/termia/internal/config"
)

func TeamDir(cfg *config.Config) string {
	if cfg != nil && strings.TrimSpace(cfg.Agent.TeamsDir) != "" {
		return strings.TrimSpace(cfg.Agent.TeamsDir)
	}
	return config.TeamsDir()
}

func ListTeams(cfg *config.Config) ([]TeamSummary, error) {
	dir := TeamDir(cfg)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read team directory: %w", err)
	}

	var teams []TeamSummary
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".toml" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		spec, err := LoadTeam(path)
		if err != nil {
			teams = append(teams, TeamSummary{
				Name: strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())),
				Path: path,
			})
			continue
		}
		teams = append(teams, TeamSummary{
			Name:        spec.Name,
			Description: spec.Description,
			Path:        path,
		})
	}

	sort.Slice(teams, func(i, j int) bool {
		return strings.ToLower(teams[i].Name) < strings.ToLower(teams[j].Name)
	})
	return teams, nil
}

func LoadTeamByName(cfg *config.Config, name string) (*TeamSpec, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("team name is empty")
	}
	path := filepath.Join(TeamDir(cfg), name+".toml")
	return LoadTeam(path)
}

func LoadTeam(path string) (*TeamSpec, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("team path is empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read team file %s: %w", path, err)
	}

	var spec TeamSpec
	if err := toml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse team file %s: %w", path, err)
	}
	if err := validateTeamSpec(&spec); err != nil {
		return nil, fmt.Errorf("invalid team file %s: %w", path, err)
	}
	return &spec, nil
}

func validateTeamSpec(spec *TeamSpec) error {
	if spec == nil {
		return fmt.Errorf("team spec is nil")
	}
	if strings.TrimSpace(spec.Name) == "" {
		return fmt.Errorf("team name is required")
	}
	if err := validateAgentSpec(&spec.Coordinator, true); err != nil {
		return fmt.Errorf("coordinator: %w", err)
	}

	seen := map[string]bool{strings.ToLower(spec.Coordinator.Name): true}
	for i := range spec.Agents {
		if err := validateAgentSpec(&spec.Agents[i], false); err != nil {
			return fmt.Errorf("agent %d: %w", i, err)
		}
		key := strings.ToLower(spec.Agents[i].Name)
		if seen[key] {
			return fmt.Errorf("duplicate agent name %q", spec.Agents[i].Name)
		}
		seen[key] = true
	}
	return nil
}

func validateAgentSpec(spec *AgentSpec, coordinator bool) error {
	if spec == nil {
		return fmt.Errorf("agent spec is nil")
	}
	if strings.TrimSpace(spec.Name) == "" {
		if coordinator {
			return fmt.Errorf("name is required")
		}
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(spec.Instruction) == "" {
		return fmt.Errorf("instruction is required")
	}
	if strings.TrimSpace(spec.Model.Provider) == "" {
		return fmt.Errorf("model.provider is required")
	}
	if strings.TrimSpace(spec.Model.Model) == "" {
		return fmt.Errorf("model.model is required")
	}
	if len(spec.Tools) == 0 {
		spec.Tools = defaultToolNames()
	}
	return nil
}
