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

func (*ReadSkillTool) CheckPermissions(params map[string]any) string { return "" }

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

func (*ListSkillsTool) CheckPermissions(params map[string]any) string { return "" }

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
