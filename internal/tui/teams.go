package tui

import "github.com/termia/termia/internal/config"

func resolveTeams(agentCfg config.AgentTeamConfig) ([]config.AgentTeamProfile, string, []string) {
	if len(agentCfg.Teams) > 0 {
		first := agentCfg.Teams[0]
		return agentCfg.Teams, first.Name, append([]string{}, first.Roles...)
	}
	name := "default"
	roles := append([]string{}, agentCfg.Roles...)
	return []config.AgentTeamProfile{{Name: name, Roles: roles}}, name, roles
}
