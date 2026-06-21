package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSkillDiscoveryService_New(t *testing.T) {
	dir := t.TempDir()
	s := NewSkillDiscoveryService(dir)
	if s == nil {
		t.Error("expected non-nil service")
	}
}

func TestSkillDiscoveryService_Discover(t *testing.T) {
	dir := t.TempDir()
	s := NewSkillDiscoveryService(dir)

	skills, err := s.Discover()
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	// Skills may be nil or empty when no skills are found
	if skills != nil && len(skills) > 0 {
		t.Logf("discovered %d skills", len(skills))
	}
}

func TestSkillDiscoveryService_ScanDirectory(t *testing.T) {
	dir := t.TempDir()
	s := NewSkillDiscoveryService(dir)

	// Create a skill directory
	skillDir := filepath.Join(dir, "test-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Test Skill\nA test skill."), 0644)

	s.scanDirectory(dir, SkillSourceExternal)

	if len(s.discovered) != 1 {
		t.Errorf("expected 1 skill, got %d", len(s.discovered))
	}
	if skill, ok := s.discovered["test-skill"]; ok {
		if skill.Description != "Test Skill" {
			t.Errorf("expected 'Test Skill', got %q", skill.Description)
		}
	}
}

func TestSkillDiscoveryService_SetRemoteURL(t *testing.T) {
	dir := t.TempDir()
	s := NewSkillDiscoveryService(dir)

	s.SetRemoteURL("https://example.com/skills.json")
	if s.remoteURL != "https://example.com/skills.json" {
		t.Errorf("expected URL to be set")
	}
}

func TestSkillDiscoveryService_AddExternalDir(t *testing.T) {
	dir := t.TempDir()
	s := NewSkillDiscoveryService(dir)

 initialCount := len(s.externalDirs)
	s.AddExternalDir("/custom/skills")

	if len(s.externalDirs) != initialCount+1 {
		t.Errorf("expected %d dirs, got %d", initialCount+1, len(s.externalDirs))
	}
}

func TestSkillDiscoveryService_GetDiscovered(t *testing.T) {
	dir := t.TempDir()
	s := NewSkillDiscoveryService(dir)

	skills := s.GetDiscovered()
	if skills != nil {
		t.Errorf("expected nil skills initially, got %d", len(skills))
	}
}

func TestFormatDiscoveryResults(t *testing.T) {
	skills := []*DiscoveredSkill{
		{Name: "test", Description: "A test skill", Source: SkillSourceExternal},
	}

	output := FormatDiscoveryResults(skills)
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatDiscoveryResults_Empty(t *testing.T) {
	output := FormatDiscoveryResults(nil)
	if output != "No skills discovered." {
		t.Errorf("expected 'No skills discovered.', got %q", output)
	}
}
