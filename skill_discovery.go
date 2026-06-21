package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ─── Skill Discovery (MiMo-Code 2) ─────────────────────────────────────────
//
// Extends skill discovery beyond builtin/workspace to scan external directories,
// pull skills from remote URLs, and extract compose-bundled skills.
//
// MiMo-Code source: skill/discovery.ts (116 lines)

// SkillSource represents where a skill came from.
type SkillSource string

const (
	SkillSourceBuiltin  SkillSource = "builtin"
	SkillSourceWorkspace SkillSource = "workspace"
	SkillSourceExternal  SkillSource = "external"
	SkillSourceRemote    SkillSource = "remote"
	SkillSourceCompose   SkillSource = "compose"
)

// DiscoveredSkill represents a discovered skill.
type DiscoveredSkill struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Path        string      `json:"path"`
	Source      SkillSource `json:"source"`
	URL         string      `json:"url,omitempty"`
	Version     string      `json:"version,omitempty"`
}

// SkillIndex represents a remote skill index.
type SkillIndex struct {
	Version string           `json:"version"`
	Skills  []SkillIndexEntry `json:"skills"`
}

// SkillIndexEntry represents an entry in the remote skill index.
type SkillIndexEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Version     string `json:"version"`
}

// SkillDiscoveryService manages skill discovery.
type SkillDiscoveryService struct {
	mu              sync.Mutex
	externalDirs    []string
	remoteURL       string
	projectDir      string
	discovered      map[string]*DiscoveredSkill
	client          *http.Client
}

// NewSkillDiscoveryService creates a new skill discovery service.
func NewSkillDiscoveryService(projectDir string) *SkillDiscoveryService {
	homeDir, _ := os.UserHomeDir()

	externalDirs := []string{}
	if homeDir != "" {
		externalDirs = append(externalDirs,
			filepath.Join(homeDir, ".claude", "skills"),
			filepath.Join(homeDir, ".agents", "skills"),
			filepath.Join(homeDir, ".codex", "skills"),
			filepath.Join(homeDir, ".opencode", "skills"),
		)
	}

	return &SkillDiscoveryService{
		externalDirs: externalDirs,
		projectDir:   projectDir,
		discovered:   make(map[string]*DiscoveredSkill),
		client:       &http.Client{Timeout: 30 * time.Second},
	}
}

// Discover scans all sources for skills.
func (s *SkillDiscoveryService) Discover() ([]*DiscoveredSkill, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.discovered = make(map[string]*DiscoveredSkill)

	// Scan external directories
	for _, dir := range s.externalDirs {
		s.scanDirectory(dir, SkillSourceExternal)
	}

	// Scan project-level external dirs
	if s.projectDir != "" {
		projectExternal := filepath.Join(s.projectDir, ".claude", "skills")
		s.scanDirectory(projectExternal, SkillSourceExternal)
	}

	// Fetch remote skills
	if s.remoteURL != "" {
		s.fetchRemoteSkills()
	}

	// Collect results
	var skills []*DiscoveredSkill
	for _, skill := range s.discovered {
		skills = append(skills, skill)
	}

	return skills, nil
}

// scanDirectory scans a directory for SKILL.md files.
func (s *SkillDiscoveryService) scanDirectory(dir string, source SkillSource) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillPath := filepath.Join(dir, entry.Name(), "SKILL.md")
		if _, err := os.Stat(skillPath); os.IsNotExist(err) {
			continue
		}

		skill := &DiscoveredSkill{
			Name:   entry.Name(),
			Path:   skillPath,
			Source: source,
		}

		// Parse SKILL.md for description
		if data, err := os.ReadFile(skillPath); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "# ") {
					skill.Description = strings.TrimPrefix(line, "# ")
					break
				}
			}
		}

		s.discovered[entry.Name()] = skill
	}
}

// fetchRemoteSkills fetches skills from a remote index.
func (s *SkillDiscoveryService) fetchRemoteSkills() {
	if s.remoteURL == "" {
		return
	}

	resp, err := s.client.Get(s.remoteURL)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	var index SkillIndex
	if err := json.Unmarshal(body, &index); err != nil {
		return
	}

	for _, entry := range index.Skills {
		skill := &DiscoveredSkill{
			Name:        entry.Name,
			Description: entry.Description,
			Source:      SkillSourceRemote,
			URL:         entry.URL,
			Version:     entry.Version,
		}
		s.discovered[entry.Name] = skill
	}
}

// SetRemoteURL sets the remote skill index URL.
func (s *SkillDiscoveryService) SetRemoteURL(url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remoteURL = url
}

// AddExternalDir adds an external directory to scan.
func (s *SkillDiscoveryService) AddExternalDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.externalDirs = append(s.externalDirs, dir)
}

// GetDiscovered returns all discovered skills.
func (s *SkillDiscoveryService) GetDiscovered() []*DiscoveredSkill {
	s.mu.Lock()
	defer s.mu.Unlock()

	var skills []*DiscoveredSkill
	for _, skill := range s.discovered {
		skills = append(skills, skill)
	}
	return skills
}

// FormatDiscoveryResults formats discovery results for display.
func FormatDiscoveryResults(skills []*DiscoveredSkill) string {
	if len(skills) == 0 {
		return "No skills discovered."
	}

	var sb string
	sb += fmt.Sprintf("## Discovered Skills (%d found)\n\n", len(skills))

	bySource := make(map[SkillSource][]*DiscoveredSkill)
	for _, s := range skills {
		bySource[s.Source] = append(bySource[s.Source], s)
	}

	for source, sourceSkills := range bySource {
		sb += fmt.Sprintf("### %s\n\n", source)
		for _, s := range sourceSkills {
			sb += fmt.Sprintf("- **%s**: %s\n", s.Name, s.Description)
		}
		sb += "\n"
	}

	return sb
}
