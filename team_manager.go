package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ─── Team Collaboration (MiMo-Code 6) ──────────────────────────────────────
//
// File-backed team management for coordinating multiple agents.
//
// MiMo-Code source: team/index.ts (113 lines)

// TeamMember represents a team member.
type TeamMember struct {
	SessionID string `json:"session_id"`
	AgentType string `json:"agent_type"`
	Role      string `json:"role"`
	JoinedAt  time.Time `json:"joined_at"`
}

// Team represents a team of agents.
type Team struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Members   []TeamMember  `json:"members"`
	CreatedAt time.Time     `json:"created_at"`
}

// TeamManager manages teams.
type TeamManager struct {
	mu       sync.Mutex
	teams    map[string]*Team
	teamsDir string
}

// NewTeamManager creates a new team manager.
func NewTeamManager(teamsDir string) *TeamManager {
	return &TeamManager{
		teams:    make(map[string]*Team),
		teamsDir: teamsDir,
	}
}

// CreateTeam creates a new team.
func (m *TeamManager) CreateTeam(id, name string) *Team {
	m.mu.Lock()
	defer m.mu.Unlock()

	team := &Team{
		ID:        id,
		Name:      name,
		Members:   make([]TeamMember, 0),
		CreatedAt: time.Now(),
	}

	m.teams[id] = team
	m.saveTeam(team)

	return team
}

// AddMember adds a member to a team.
func (m *TeamManager) AddMember(teamID, sessionID, agentType, role string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	team, exists := m.teams[teamID]
	if !exists {
		return fmt.Errorf("team not found: %s", teamID)
	}

	member := TeamMember{
		SessionID: sessionID,
		AgentType: agentType,
		Role:      role,
		JoinedAt:  time.Now(),
	}

	team.Members = append(team.Members, member)
	m.saveTeam(team)

	return nil
}

// RemoveMember removes a member from a team.
func (m *TeamManager) RemoveMember(teamID, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	team, exists := m.teams[teamID]
	if !exists {
		return fmt.Errorf("team not found: %s", teamID)
	}

	for i, member := range team.Members {
		if member.SessionID == sessionID {
			team.Members = append(team.Members[:i], team.Members[i+1:]...)
			m.saveTeam(team)
			return nil
		}
	}

	return fmt.Errorf("member not found: %s", sessionID)
}

// GetTeam returns a team by ID.
func (m *TeamManager) GetTeam(teamID string) *Team {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.teams[teamID]
}

// ListTeams returns all teams.
func (m *TeamManager) ListTeams() []*Team {
	m.mu.Lock()
	defer m.mu.Unlock()

	var teams []*Team
	for _, t := range m.teams {
		teams = append(teams, t)
	}
	return teams
}

// GetTeamMembers returns all members of a team.
func (m *TeamManager) GetTeamMembers(teamID string) []TeamMember {
	m.mu.Lock()
	defer m.mu.Unlock()

	team, exists := m.teams[teamID]
	if !exists {
		return nil
	}

	result := make([]TeamMember, len(team.Members))
	copy(result, team.Members)
	return result
}

// saveTeam persists a team to disk.
func (m *TeamManager) saveTeam(team *Team) {
	if m.teamsDir == "" {
		return
	}

	dir := filepath.Join(m.teamsDir, team.ID)
	os.MkdirAll(dir, 0755)

	path := filepath.Join(dir, "team.json")
	data, _ := json.MarshalIndent(team, "", "  ")
	os.WriteFile(path, data, 0644)
}

// FormatTeam formats a team for display.
func FormatTeam(team *Team) string {
	if team == nil {
		return "No team."
	}

	var sb string
	sb += fmt.Sprintf("## Team: %s\n\n", team.Name)
	sb += fmt.Sprintf("- ID: %s\n", team.ID)
	sb += fmt.Sprintf("- Members: %d\n\n", len(team.Members))

	for _, m := range team.Members {
		sb += fmt.Sprintf("- %s (%s) - %s\n", m.SessionID, m.AgentType, m.Role)
	}

	return sb
}
