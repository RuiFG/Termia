package tui

import (
	"strings"

	"github.com/termia/termia/internal/agent"
	"github.com/termia/termia/internal/config"
)

func resolveTeams(cfg *config.Config) ([]agent.TeamSummary, string) {
	if cfg == nil {
		return nil, ""
	}
	teams, err := agent.ListTeams(cfg)
	if err != nil {
		return nil, strings.TrimSpace(cfg.Agent.DefaultTeam)
	}
	active := strings.TrimSpace(cfg.Agent.DefaultTeam)
	if active == "" {
		return teams, ""
	}
	for _, team := range teams {
		if strings.EqualFold(team.Name, active) {
			return teams, team.Name
		}
	}
	return teams, active
}
