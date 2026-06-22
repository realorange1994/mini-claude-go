package main

import (
	"os"
	"path/filepath"
	"testing"
)

// ─── Image/Modality Filter Tests ────────────────────────────────────────────

func TestGetDefaultCapabilities_Claude(t *testing.T) {
	caps := GetDefaultCapabilities("claude-sonnet-4-20250514")
	if !caps.SupportsImages {
		t.Error("expected Claude to support images")
	}
	if !caps.SupportsPDF {
		t.Error("expected Claude to support PDF")
	}
}

func TestGetDefaultCapabilities_GPT4(t *testing.T) {
	caps := GetDefaultCapabilities("gpt-4o")
	if !caps.SupportsImages {
		t.Error("expected GPT-4o to support images")
	}
}

func TestGetDefaultCapabilities_Unknown(t *testing.T) {
	caps := GetDefaultCapabilities("unknown-model")
	if caps.SupportsImages {
		t.Error("expected unknown model to not support images")
	}
}

func TestFilterUnsupportedParts(t *testing.T) {
	caps := ModelCapabilitiesInput{SupportsImages: false}
	parts := []ContentPart{
		{Type: "text", Text: "hello"},
		{Type: "image", MimeType: "image/png"},
	}

	result := FilterUnsupportedParts(parts, caps)
	if len(result) != 2 {
		t.Errorf("expected 2 parts, got %d", len(result))
	}
	if result[1].Type != "text" {
		t.Error("expected image to be replaced with text")
	}
}

func TestLimitImages(t *testing.T) {
	parts := []ContentPart{
		{Type: "text", Text: "hello"},
		{Type: "image"},
		{Type: "image"},
		{Type: "image"},
	}

	result := LimitImages(parts, 2, 0)
	// 1 text + 2 images (dropped 1) + drop message = 4
	if len(result) < 3 {
		t.Errorf("expected at least 3 parts, got %d", len(result))
	}
}

// ─── Instruction Resolution Tests ───────────────────────────────────────────

func TestInstructionResolver_New(t *testing.T) {
	dir := t.TempDir()
	r := NewInstructionResolver(dir)
	if r == nil {
		t.Error("expected non-nil resolver")
	}
}

func TestInstructionResolver_Resolve(t *testing.T) {
	dir := t.TempDir()
	r := NewInstructionResolver(dir)

	// Create CLAUDE.md
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Instructions"), 0644)

	files := r.Resolve(dir)
	if len(files) == 0 {
		t.Error("expected at least 1 instruction file")
	}
}

func TestInstructionResolver_ResolveWithFallback(t *testing.T) {
	dir := t.TempDir()
	r := NewInstructionResolver(dir)

	// Create sparse AGENTS.md
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("short"), 0644)
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Detailed instructions"), 0644)

	files := r.ResolveWithFallback(dir)
	if len(files) < 2 {
		t.Errorf("expected at least 2 files, got %d", len(files))
	}
}

func TestFormatInstructionFiles(t *testing.T) {
	files := []*InstructionFile{
		{Path: "/path/CLAUDE.md", Type: "CLAUDE"},
	}

	output := FormatInstructionFiles(files)
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatInstructionFiles_Empty(t *testing.T) {
	output := FormatInstructionFiles(nil)
	if output != "No instruction files found." {
		t.Errorf("expected 'No instruction files found.', got %q", output)
	}
}

// ─── Team Collaboration Tests ───────────────────────────────────────────────

func TestTeamManager_New(t *testing.T) {
	dir := t.TempDir()
	m := NewTeamManager(dir)
	if m == nil {
		t.Error("expected non-nil manager")
	}
}

func TestTeamManager_CreateTeam(t *testing.T) {
	dir := t.TempDir()
	m := NewTeamManager(dir)

	team := m.CreateTeam("team-1", "Test Team")
	if team == nil {
		t.Fatal("expected non-nil team")
	}
	if team.Name != "Test Team" {
		t.Errorf("expected 'Test Team', got %q", team.Name)
	}
}

func TestTeamManager_AddMember(t *testing.T) {
	dir := t.TempDir()
	m := NewTeamManager(dir)

	m.CreateTeam("team-1", "Test Team")
	err := m.AddMember("team-1", "session-1", "subagent", "worker")
	if err != nil {
		t.Fatalf("add member failed: %v", err)
	}

	members := m.GetTeamMembers("team-1")
	if len(members) != 1 {
		t.Errorf("expected 1 member, got %d", len(members))
	}
}

func TestTeamManager_RemoveMember(t *testing.T) {
	dir := t.TempDir()
	m := NewTeamManager(dir)

	m.CreateTeam("team-1", "Test Team")
	m.AddMember("team-1", "session-1", "subagent", "worker")
	m.RemoveMember("team-1", "session-1")

	members := m.GetTeamMembers("team-1")
	if len(members) != 0 {
		t.Errorf("expected 0 members, got %d", len(members))
	}
}

func TestTeamManager_ListTeams(t *testing.T) {
	dir := t.TempDir()
	m := NewTeamManager(dir)

	m.CreateTeam("team-1", "Team 1")
	m.CreateTeam("team-2", "Team 2")

	teams := m.ListTeams()
	if len(teams) != 2 {
		t.Errorf("expected 2 teams, got %d", len(teams))
	}
}

func TestFormatTeam(t *testing.T) {
	team := &Team{
		ID:   "team-1",
		Name: "Test Team",
		Members: []TeamMember{
			{SessionID: "s1", AgentType: "subagent", Role: "worker"},
		},
	}

	output := FormatTeam(team)
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatTeam_Nil(t *testing.T) {
	output := FormatTeam(nil)
	if output != "No team." {
		t.Errorf("expected 'No team.', got %q", output)
	}
}
