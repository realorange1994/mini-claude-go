package tools

import (
	"fmt"
	"strings"

	"miniclaudecode-go/skills"
)

// ReadSkillTool reads a skill's SKILL.md content.
type ReadSkillTool struct {
	Loader *skills.Loader
}

func (*ReadSkillTool) Name() string        { return "read_skill" }
func (*ReadSkillTool) Description() string { return "Read a skill's SKILL.md file. Use list_skills first to discover available skills." }

func (*ReadSkillTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Name of the skill to read.",
			},
		},
		"required": []string{"name"},
	}
}

func (*ReadSkillTool) CheckPermissions(params map[string]any) PermissionResult { return PermissionResultPassthrough() }

func (t *ReadSkillTool) Execute(params map[string]any) ToolResult {
	if t.Loader == nil {
		return ToolResult{Output: "Error: skill loader not available", IsError: true}
	}

	name, _ := params["name"].(string)
	if name == "" {
		return ToolResult{Output: "Error: name is required", IsError: true}
	}

	content := t.Loader.LoadSkill(name)
	if content == "" {
		return ToolResult{Output: fmt.Sprintf("Error: Skill not found: %s", name), IsError: true}
	}

	return ToolResult{Output: content}
}

// ListSkillsTool lists available skills.
type ListSkillsTool struct {
	Loader *skills.Loader
}

func (*ListSkillsTool) Name() string        { return "list_skills" }
func (*ListSkillsTool) Description() string { return "List available skills. Shows name, description, and availability status." }

func (*ListSkillsTool) InputSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{},
	}
}

func (*ListSkillsTool) CheckPermissions(params map[string]any) PermissionResult { return PermissionResultPassthrough() }

func (t *ListSkillsTool) Execute(params map[string]any) ToolResult {
	if t.Loader == nil {
		return ToolResult{Output: "Error: skill loader not available", IsError: true}
	}

	skillList := t.Loader.ListSkills(false)
	if len(skillList) == 0 {
		return ToolResult{Output: "No skills found."}
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Skills (%d total)", len(skillList)))
	for _, s := range skillList {
		status := "available"
		if !s.Available {
			status = "unavailable"
		}
		if s.Always {
			status += " (always-on)"
		}
		lines = append(lines, fmt.Sprintf("  %s [%s] - %s", s.Name, status, s.Description))
		if !s.Available && len(s.MissingDeps) > 0 {
			lines = append(lines, fmt.Sprintf("    Missing: %s", s.MissingDeps[0]))
		}
	}

	return ToolResult{Output: strings.Join(lines, "\n")}
}

// --- SearchSkillTool ---

const searchSkillCharBudget = 4000

type scoredSkill struct {
	info  skills.SkillInfo
	score float64
}

// SearchSkillsTool searches available skills by name, description, tags, or
// usage guidance.
type SearchSkillsTool struct {
	Loader *skills.Loader
}

func (*SearchSkillsTool) Name() string {
	return "search_skills"
}

func (*SearchSkillsTool) Description() string {
	return "Search available skills by name, description, tags, or usage guidance. Use this to discover skills relevant to your current task before attempting to build functionality from scratch."
}

func (*SearchSkillsTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query - skill name, topic, or description keyword",
			},
		},
		"required": []string{"query"},
	}
}

func (*SearchSkillsTool) CheckPermissions(params map[string]any) PermissionResult { return PermissionResultPassthrough() }

func (t *SearchSkillsTool) Execute(params map[string]any) ToolResult {
	if t.Loader == nil {
		return ToolResult{Output: "Error: skill loader not available", IsError: true}
	}

	query, _ := params["query"].(string)
	query = strings.TrimSpace(query)
	if query == "" {
		return ToolResult{Output: "Error: query is required", IsError: true}
	}

	skillList := t.Loader.ListSkills(false)
	if len(skillList) == 0 {
		return ToolResult{Output: "No skills available."}
	}

	queryLower := strings.ToLower(query)
	queryTerms := strings.Fields(queryLower)
	if len(queryTerms) == 0 {
		return ToolResult{Output: "Error: query is required", IsError: true}
	}

	// Score each skill
	var scored []scoredSkill
	for _, s := range skillList {
		score := scoreSkill(s, queryTerms)
		if score > 0 {
			scored = append(scored, scoredSkill{info: s, score: score})
		}
	}

	// Sort by score descending
	for i := 0; i < len(scored); i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	if len(scored) == 0 {
		return ToolResult{Output: fmt.Sprintf("No skills found for %q. Use list_skills to see all available skills.", query)}
	}

	return ToolResult{Output: formatSearchResults(scored, query)}
}

// scoreSkill scores a skill against query terms using weighted relevance.
func scoreSkill(skill skills.SkillInfo, queryTerms []string) float64 {
	var score float64
	nameLower := strings.ToLower(skill.Name)
	descLower := strings.ToLower(skill.Description)
	whenLower := strings.ToLower(skill.WhenToUse)
	tagsLower := make([]string, len(skill.Tags))
	for i, t := range skill.Tags {
		tagsLower[i] = strings.ToLower(t)
	}

	for _, term := range queryTerms {
		// Exact name match
		if nameLower == term {
			score += 100
		} else if strings.Contains(nameLower, term) {
			// Name contains term
			score += 50
		}

		// Description contains term
		if strings.Contains(descLower, term) {
			score += 20
		}

		// Tag match (exact or contains)
		for _, tag := range tagsLower {
			if tag == term || strings.Contains(tag, term) {
				score += 30
				break
			}
		}

		// when_to_use contains term
		if whenLower != "" && strings.Contains(whenLower, term) {
			score += 15
		}
	}

	return score
}

// formatSearchResults formats search results within the character budget.
func formatSearchResults(scored []scoredSkill, query string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Matching skills for %q:\n", query))

	included := 0
	for _, s := range scored {
		entry := fmt.Sprintf("  - **%s**: %s", s.info.Name, s.info.Description)
		if s.info.WhenToUse != "" {
			entry += fmt.Sprintf(" (%s)", s.info.WhenToUse)
		}
		if len(s.info.Tags) > 0 {
			entry += fmt.Sprintf(" [%s]", strings.Join(s.info.Tags, ", "))
		}
		if !s.info.Available {
			entry += " (unavailable)"
		}
		entry += "\n"

		if sb.Len()+len(entry) > searchSkillCharBudget {
			break
		}

		sb.WriteString(entry)
		included++
	}

	remaining := len(scored) - included
	sb.WriteString(fmt.Sprintf("\nFound %d skill(s). Use read_skill to load full instructions.", len(scored)))
	if remaining > 0 {
		sb.WriteString(fmt.Sprintf(" (+%d more, use search_skills with different terms)", remaining))
	}

	return sb.String()
}
