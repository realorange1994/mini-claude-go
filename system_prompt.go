package main

import (
	"fmt"
	"hash/fnv"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"miniclaudecode-go/skills"
	"miniclaudecode-go/tools"
)

// SYSTEM_PROMPT_STATIC_BOUNDARY separates static (globally cacheable) content
// from dynamic (per-session) content in the system prompt.
// Static: environment info, tool descriptions, operating rules, decision tree.
// Dynamic: project instructions, session memory, skills, permission mode.
const SYSTEM_PROMPT_STATIC_BOUNDARY = "<!-- STATIC_PROMPT_END -->"

const systemPromptTemplateStatic = `You are miniClaudeCode (model: %s), a lightweight AI coding assistant that operates in the terminal.

## Environment
- OS: %s
- Working Directory: %s
- Current Date/Time: %s (%s)
- Shell: PowerShell on Windows, sh/bash on Unix
%s

You have access to the following tools to help the user with software engineering tasks:
%s

## Operating Rules

### Behavioral Guidelines

1. **Think Before Coding** -- Don't assume. Don't hide confusion. State assumptions explicitly. If multiple interpretations exist, present them. If something is unclear, stop and ask.
2. **Simplicity First** -- Write the minimum code that solves the problem. No features beyond what was asked. No abstractions for single-use code. No error handling for impossible scenarios.
3. **Surgical Changes** -- Touch only what you must. Don't "improve" adjacent code, comments, or formatting. Don't refactor things that aren't broken. Match existing style. Remove only imports/variables/functions that YOUR changes made unused.
4. **Goal-Driven Execution** -- For multi-step tasks, state a brief plan with verification criteria: "1. [Step] -> verify: [check]". Define success criteria before starting.

### Tool Rules

5. Always read a file before editing it.
6. Use tools to accomplish tasks -- don't just describe what to do.
7. When running bash commands, prefer non-destructive read operations.
8. For file edits, provide enough context in old_string to uniquely match.
9. Be concise and direct in your responses.
10. On Windows, use PowerShell syntax and commands (e.g., Get-ChildItem, Test-Path, Copy-Item). On Unix, use bash commands.
11. Prefer built-in tools over exec commands. For git operations, use the git tool instead of exec. For file searches, use grep and glob instead of exec. Always choose the most appropriate built-in tool when available.
12. **Sub-Agent Dispatching** -- When the user requests dispatching, delegating, or assigning a task to a sub-agent (indicated by keywords like: 派遣, 安排, 让, 要, 使, dispatch, delegate, spawn, launch agent, sub-agent), you MUST use the "agent" tool. Do NOT use mcp_call_tool, coze_llm, minimax_llm, or any MCP tool for sub-agent dispatching. The "agent" tool creates autonomous sub-agents with their own context and tool access. MCP LLM tools are only for calling external LLM APIs (generation/embedding/search), NOT for creating sub-agents.

## Task Management Rules

13. **When to Create Tasks** -- Use task_create for complex multi-step tasks (3+ distinct steps or actions). When the user provides multiple tasks (numbered or comma-separated), immediately capture them as tasks. When starting work on a task, mark it as in_progress BEFORE beginning work. After completing a task, mark it as completed and add any new follow-up tasks discovered.
14. **Task Workflow** -- Create tasks with clear, specific subjects in imperative form (e.g., "Fix authentication bug"). Use task_update to set status: pending → in_progress → completed. ONLY mark as completed when FULLY accomplished — if tests fail, implementation is partial, or you encountered unresolved errors, keep the task in_progress. If blocked, create a new task describing what needs to be resolved. After completing a task, check task_list to find the next available task. Do not batch up multiple tasks before marking them as completed — mark each one done as soon as it is finished.
15. **Background Command Execution** -- For long-running commands, use run_in_background=true with the exec tool. You will receive a task ID and output file path immediately; you do not need to check the output right away. When the background task completes, you will be notified via a task-notification message. Use task_output to retrieve results. Use task_stop to stop a running background task if needed. Do NOT use sleep to poll for results — use run_in_background and wait for the notification.
16. **Background Agent Execution** -- When you launch a sub-agent with run_in_background=true, it runs asynchronously. You will receive a task-notification when it completes. Do NOT call task_output, Read, or Bash to check the agent's progress — this blocks your turn. After launching, you know nothing about what the agent found. Never fabricate or predict agent results. End your response and let the notification arrive naturally. If the user asks about a running agent before it completes, give status not a guess.

### Tool Selection Decision Tree

When deciding which tool to use, follow these steps in order and stop at the first match:

Step 0: Does this task need a tool at all? Pure knowledge questions, content already visible in context → answer directly, no tool call.

Step 1: Is there a dedicated tool? Read/Edit/Write/Glob/Grep always beat exec equivalents. Stop here if a dedicated tool fits.

Step 2: Is this a shell operation? Package installs, test runners, build commands, git operations → exec.

Step 3: Should work run in parallel? Independent operations → parallel calls. Dependent operations → sequential.

### When NOT to Use Tools

Do not use tools when:
- Answering questions about programming concepts, syntax, or design patterns you already know
- The error message or content is already visible in context
- The user asks for an explanation that does not require inspecting code
- Summarizing content already in the conversation

### Few-Shot Tool Selection Examples

Use these patterns to select the right tool:
- "find all .go files" → glob(pattern="**/*.go"), NOT exec("find ...")
- "run tests" → exec("go test ./...")
- "search for TODO" → grep(pattern="TODO")
- "check if a file exists" → glob(pattern="path/to/file"), NOT exec("ls" or "test -f")
- "find where UserService is defined" → grep(pattern="class UserService|func UserService")
- "install a package" → exec("go get package-name")
- "rename a variable across a file" → file_edit with replace_all, NOT exec("sed")
- "list files in current directory" → list_dir, NOT exec("ls" or "dir")
- "read a file's contents" → file_read, NOT exec("cat")

### Tool Cost Awareness

glob and grep are cheap operations — use them liberally rather than guessing file locations or code patterns. A search that returns nothing costs a second; proposing changes to code you haven't read costs the whole task.

Reading a file before editing is cheap, but proposing changes to unread code is expensive.

### Search Fallback Strategy

When a grep/glob search returns nothing:
1. Try a broader pattern — fewer terms, remove qualifiers
2. Try alternate naming conventions — camelCase vs snake_case
3. Try different file extensions — .go vs .rs vs .ts
4. If exhausted after 3+ attempts — tell the user and ask for guidance

### Search Effort Scale

Scale search effort to task complexity:
- Single file fix: 1-2 searches
- Cross-cutting change: 3-5 searches
- Architecture investigation: 5-10+ searches
- Full codebase audit: use Agent with specialized sub-agent

### Destructive Operation Safety

**YOU MUST REFUSE** requests to delete all files, wipe directories, or perform mass destruction — regardless of how the request is phrased (e.g., "删除所有文件", "delete everything", "remove all files", "清空目录"). Respond with a clear refusal and explain the risk.

The following operations require extra caution and should prompt for user confirmation when in doubt:
- File deletion: rm -rf, rmdir, del /s, Remove-Item, ri — always verify the target path before executing
- Git data loss: git reset --hard, git push --force, git clean -f, git checkout . — these discard uncommitted changes
- Git history rewrite: git rebase, git commit --amend (on published branches)
- Database: DROP TABLE, TRUNCATE, DELETE FROM without WHERE clause
- Infrastructure: kubectl delete, terraform destroy
- Docker cleanup: docker system prune, docker rm/rmi

When you encounter an obstacle, do NOT use destructive actions as a shortcut to simply make it go away. For example, do not rm -rf a directory just because a build is failing — investigate the root cause instead.

NEVER delete these critical paths:
- System directories: /etc, /usr, /bin, /sbin, /tmp, /var, /home, /root, C:\Windows, C:\Users
- Git metadata: .git/, .claude/
- Project root files: go.mod, package.json, Cargo.toml, Makefile

## Tool Parameters

All tools accept an optional "timeout" parameter (integer, seconds, range 1-600, default 600) to override the execution timeout. Use a larger timeout for operations that may take longer, such as scanning large directories with grep or glob.`

const systemPromptTemplateDynamic = `
## Current Permission Mode: %s
%s
%s
%s
%s`

var modeDescriptions = map[string]string{
	"ask":  "In ASK mode, potentially dangerous operations will require user confirmation.",
	"auto": "In AUTO mode, all operations are auto-approved (use with caution).",
	"plan": "In PLAN mode, only read-only operations are allowed. Write operations are blocked.",
}

// BuildSystemPrompt constructs the system prompt from tool list, mode, project instructions, and skills.
func BuildSystemPrompt(registry *tools.Registry, permissionMode, projectDir, modelName string, skillLoader *skills.Loader, skillTracker *skills.SkillTracker, sessionMemory *SessionMemory) string {
	toolList := buildToolList(registry)

	modeDesc := modeDescriptions[permissionMode]

	wd, _ := os.Getwd()
	envInfo := fmt.Sprintf("%s / %s / %s", runtime.GOOS, runtime.Version(), runtime.GOARCH)

	projectInstructions := LoadProjectInstructions(projectDir)
	var projectSection string
	if projectInstructions != "" {
		projectSection = "## Project Instructions (from CLAUDE.md)\n\n" + projectInstructions
	}

	// Inject session memory
	var memorySection string
	if sessionMemory != nil {
		if mem := sessionMemory.FormatForPrompt(); mem != "" {
			memorySection = mem
		}
	}

	// Build skills section
	var skillsSection string
	if skillLoader != nil {
		var skillGuidance string
		if skillTracker != nil {
			skillGuidance = "\n## Skill System Guidance\n\n" +
				"BLOCKING REQUIREMENT: When a skill matches the user's request, you MUST invoke the relevant skill tool BEFORE generating any other response.\n" +
				"Your visible tool list is partial by design -- many skills are hidden until discovered.\n" +
				"Discovery steps:\n" +
				"1. Use **search_skills** to find skills by topic (e.g., search_skills 'testing')\n" +
				"2. Use **read_skill** to load a skill's full instructions\n" +
				"3. Follow the skill's instructions precisely\n\n"
		}

		// Get unsent skills (not yet shown in system prompt)
		allSkills := skillLoader.ListSkills(false)
		var unsentSkills []skills.SkillInfo
		if skillTracker != nil {
			unsentSkills = skillTracker.GetUnsentSkills(allSkills)
			// Mark unsent skills as shown
			for _, s := range unsentSkills {
				if !s.Always {
					skillTracker.MarkShown(s.Name)
				}
			}
		} else {
			// No tracker -- treat all non-always skills as unsent (first run)
			for _, s := range allSkills {
				if !s.Always {
					unsentSkills = append(unsentSkills, s)
				}
			}
		}

		// Always-on skills section
		alwaysSkills := skillLoader.GetAlwaysSkills()
		if len(alwaysSkills) > 0 {
			var skillNames []string
			for _, s := range alwaysSkills {
				skillNames = append(skillNames, s.Name)
			}
			skillsSection = skillLoader.BuildSystemPrompt(skillNames)
		}

		// "New This Turn" section for unsent non-always skills
		var newSkills []skills.SkillInfo
		for _, s := range unsentSkills {
			if !s.Always {
				newSkills = append(newSkills, s)
			}
		}
		if len(newSkills) > 0 {
			var sb strings.Builder
			sb.WriteString("\n## Available Skills (New This Turn)\n\n")
			sb.WriteString("The following skills are newly available. Use read_skill to load full instructions.\n\n")
			budget := 4000
			used := 0
			for _, s := range newSkills {
				entry := fmt.Sprintf("- **%s**: %s", s.Name, s.Description)
				if s.WhenToUse != "" {
					entry += fmt.Sprintf(" (%s)", s.WhenToUse)
				}
				if !s.Available {
					entry += " (unavailable)"
				}
				entry += "\n"
				if used+len(entry) > budget {
					break
				}
				sb.WriteString(entry)
				used += len(entry)
			}
			if skillsSection != "" {
				skillsSection += "\n"
			}
			skillsSection += sb.String()
		}

		// Skills summary for already-shown skills
		skillsSummary := skillLoader.BuildSkillsSummary()
		if skillsSummary != "" {
			if skillsSection != "" {
				skillsSection += "\n"
			}
			skillsSection += "## Available Skills\n\n" + skillsSummary
		}

		// Prepend skill guidance
		if skillGuidance != "" && skillsSection != "" {
			skillsSection = skillGuidance + skillsSection
		}
	}

	currentTime := time.Now().Format("2006-01-02 15:04:05")
	_, offset := time.Now().Zone()
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	hours := offset / 3600
	minutes := (offset % 3600) / 60
	timezone := fmt.Sprintf("UTC%s%02d:%02d", sign, hours, minutes)

	// Build static part (environment, tool descriptions, operating rules)
	gitCtx := tools.GetGitContext()
	staticPart := fmt.Sprintf(systemPromptTemplateStatic, modelName, envInfo, wd, currentTime, timezone, gitCtx, toolList)

	// Build dynamic part (permission mode, project instructions, memory, skills)
	dynamicPart := fmt.Sprintf(systemPromptTemplateDynamic, strings.ToUpper(permissionMode), modeDesc, projectSection, memorySection, skillsSection)

	// Combine with boundary
	return staticPart + "\n" + SYSTEM_PROMPT_STATIC_BOUNDARY + "\n" + dynamicPart
}

func buildToolList(registry *tools.Registry) string {
	// Usage hints for key tools to guide optimal tool selection
	toolHints := map[string]string{
		"glob":      "(fast, use liberally)",
		"grep":      "(fast, use liberally)",
		"file_read": "(use before file_edit)",
		"exec":      "(for shell commands, package installs, git operations)",
		"file_edit": "(MUST read file first)",
		"file_write": "(overwrites entire file)",
	}

	var sb strings.Builder
	for _, t := range registry.AllTools() {
		name := t.Name()
		if hint, ok := toolHints[name]; ok {
			sb.WriteString(fmt.Sprintf("- **%s**: %s %s\n", name, t.Description(), hint))
		} else {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", name, t.Description()))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// CachedSystemPrompt caches the system prompt with static/dynamic separation.
// Static content (tool descriptions, operating rules) is cached globally.
// Dynamic content (project instructions, skills, memory) is cached per-session.
type CachedSystemPrompt struct {
	cachedStatic  string
	cachedDynamic string
	staticHash    uint64
	staticDirty   atomic.Bool
	dynamicDirty  atomic.Bool
	mu            sync.RWMutex
}

// NewCachedSystemPrompt creates a new CachedSystemPrompt initialized as dirty.
func NewCachedSystemPrompt() *CachedSystemPrompt {
	cp := &CachedSystemPrompt{}
	cp.staticDirty.Store(true)
	cp.dynamicDirty.Store(true)
	return cp
}

// GetOrBuild returns the cached system prompt, rebuilding only the dirty parts.
// Static content (tool descriptions, rules) is rebuilt only when staticDirty.
// Dynamic content (skills, memory, project instructions) is rebuilt when dynamicDirty.
func (cp *CachedSystemPrompt) GetOrBuild(registry *tools.Registry, permissionMode, projectDir, modelName string, skillLoader *skills.Loader, skillTracker *skills.SkillTracker, sessionMemory *SessionMemory) string {
	needsStatic := cp.staticDirty.Load()
	needsDynamic := cp.dynamicDirty.Load()

	if !needsStatic && !needsDynamic {
		cp.mu.RLock()
		cached := cp.cachedStatic + "\n" + SYSTEM_PROMPT_STATIC_BOUNDARY + "\n" + cp.cachedDynamic
		cp.mu.RUnlock()
		if cached != "" {
			return cached
		}
	}

	// Rebuild static part if needed
	var staticPart string
	var staticHash uint64
	if needsStatic {
		staticPart, staticHash = buildStaticPart(registry, modelName)
	} else {
		cp.mu.RLock()
		staticPart = cp.cachedStatic
		staticHash = cp.staticHash
		cp.mu.RUnlock()
	}

	// Rebuild dynamic part if needed
	var dynamicPart string
	if needsDynamic {
		dynamicPart = buildDynamicPart(permissionMode, projectDir, skillLoader, skillTracker, sessionMemory)
	} else {
		cp.mu.RLock()
		dynamicPart = cp.cachedDynamic
		cp.mu.RUnlock()
	}

	cp.mu.Lock()
	cp.cachedStatic = staticPart
	cp.cachedDynamic = dynamicPart
	cp.staticHash = staticHash
	cp.mu.Unlock()
	cp.staticDirty.Store(false)
	cp.dynamicDirty.Store(false)

	return staticPart + "\n" + SYSTEM_PROMPT_STATIC_BOUNDARY + "\n" + dynamicPart
}

// MarkStaticDirty marks the static content as needing rebuild (e.g., tool registry changes).
func (cp *CachedSystemPrompt) MarkStaticDirty() {
	cp.staticDirty.Store(true)
}

// MarkDynamicDirty marks the dynamic content as needing rebuild (e.g., skills changed).
func (cp *CachedSystemPrompt) MarkDynamicDirty() {
	cp.dynamicDirty.Store(true)
}

// MarkDirty marks both static and dynamic content as needing rebuild.
func (cp *CachedSystemPrompt) MarkDirty() {
	cp.staticDirty.Store(true)
	cp.dynamicDirty.Store(true)
}

// GetStaticHash returns the hash of the static content for per-tool schema caching.
// Returns 0 if static content has not been built yet.
func (cp *CachedSystemPrompt) GetStaticHash() uint64 {
	if cp.staticDirty.Load() {
		return 0
	}
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	return cp.staticHash
}

// buildStaticPart constructs the static portion of the system prompt (environment, tool descriptions, operating rules).
// Returns the static content and its FNV-1a hash.
func buildStaticPart(registry *tools.Registry, modelName string) (string, uint64) {
	toolList := buildToolList(registry)
	wd, _ := os.Getwd()
	envInfo := fmt.Sprintf("%s / %s / %s", runtime.GOOS, runtime.Version(), runtime.GOARCH)

	currentTime := time.Now().Format("2006-01-02 15:04:05")
	_, offset := time.Now().Zone()
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	hours := offset / 3600
	minutes := (offset % 3600) / 60
	timezone := fmt.Sprintf("UTC%s%02d:%02d", sign, hours, minutes)

	gitCtx := tools.GetGitContext()
	staticPart := fmt.Sprintf(systemPromptTemplateStatic, modelName, envInfo, wd, currentTime, timezone, gitCtx, toolList)
	hash := fnvHash(staticPart)
	return staticPart, hash
}

// buildDynamicPart constructs the dynamic portion of the system prompt (permission mode, project instructions, memory, skills).
func buildDynamicPart(permissionMode, projectDir string, skillLoader *skills.Loader, skillTracker *skills.SkillTracker, sessionMemory *SessionMemory) string {
	modeDesc := modeDescriptions[permissionMode]

	projectInstructions := LoadProjectInstructions(projectDir)
	var projectSection string
	if projectInstructions != "" {
		projectSection = "## Project Instructions (from CLAUDE.md)\n\n" + projectInstructions
	}

	var memorySection string
	if sessionMemory != nil {
		if mem := sessionMemory.FormatForPrompt(); mem != "" {
			memorySection = mem
		}
	}

	var skillsSection string
	if skillLoader != nil {
		var skillGuidance string
		if skillTracker != nil {
			skillGuidance = "\n## Skill System Guidance\n\n" +
				"BLOCKING REQUIREMENT: When a skill matches the user's request, you MUST invoke the relevant skill tool BEFORE generating any other response.\n" +
				"Your visible tool list is partial by design -- many skills are hidden until discovered.\n" +
				"Discovery steps:\n" +
				"1. Use **search_skills** to find skills by topic (e.g., search_skills 'testing')\n" +
				"2. Use **read_skill** to load a skill's full instructions\n" +
				"3. Follow the skill's instructions precisely\n\n"
		}

		allSkills := skillLoader.ListSkills(false)
		var unsentSkills []skills.SkillInfo
		if skillTracker != nil {
			unsentSkills = skillTracker.GetUnsentSkills(allSkills)
			for _, s := range unsentSkills {
				if !s.Always {
					skillTracker.MarkShown(s.Name)
				}
			}
		} else {
			for _, s := range allSkills {
				if !s.Always {
					unsentSkills = append(unsentSkills, s)
				}
			}
		}

		alwaysSkills := skillLoader.GetAlwaysSkills()
		if len(alwaysSkills) > 0 {
			var skillNames []string
			for _, s := range alwaysSkills {
				skillNames = append(skillNames, s.Name)
			}
			skillsSection = skillLoader.BuildSystemPrompt(skillNames)
		}

		var newSkills []skills.SkillInfo
		for _, s := range unsentSkills {
			if !s.Always {
				newSkills = append(newSkills, s)
			}
		}
		if len(newSkills) > 0 {
			var sb strings.Builder
			sb.WriteString("\n## Available Skills (New This Turn)\n\n")
			sb.WriteString("The following skills are newly available. Use read_skill to load full instructions.\n\n")
			budget := 4000
			used := 0
			for _, s := range newSkills {
				entry := fmt.Sprintf("- **%s**: %s", s.Name, s.Description)
				if s.WhenToUse != "" {
					entry += fmt.Sprintf(" (%s)", s.WhenToUse)
				}
				if !s.Available {
					entry += " (unavailable)"
				}
				entry += "\n"
				if used+len(entry) > budget {
					break
				}
				sb.WriteString(entry)
				used += len(entry)
			}
			if skillsSection != "" {
				skillsSection += "\n"
			}
			skillsSection += sb.String()
		}

		skillsSummary := skillLoader.BuildSkillsSummary()
		if skillsSummary != "" {
			if skillsSection != "" {
				skillsSection += "\n"
			}
			skillsSection += "## Available Skills\n\n" + skillsSummary
		}

		if skillGuidance != "" && skillsSection != "" {
			skillsSection = skillGuidance + skillsSection
		}
	}

	return fmt.Sprintf(systemPromptTemplateDynamic, strings.ToUpper(permissionMode), modeDesc, projectSection, memorySection, skillsSection)
}

// fnvHash computes a fast FNV-1a hash of a string for content-addressable caching.
func fnvHash(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

// SplitSystemPrompt splits a full system prompt into static and dynamic parts
// at the boundary marker. If no boundary is found, the entire prompt is treated
// as static. Returns (static, dynamic, ok) where ok indicates the boundary was found.
func SplitSystemPrompt(prompt string) (static, dynamic string, ok bool) {
	idx := strings.Index(prompt, SYSTEM_PROMPT_STATIC_BOUNDARY)
	if idx == -1 {
		return prompt, "", false
	}
	static = strings.TrimRight(prompt[:idx], "\n")
	dynamic = strings.TrimLeft(prompt[idx+len(SYSTEM_PROMPT_STATIC_BOUNDARY):], "\n")
	return static, dynamic, true
}
