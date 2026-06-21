package main

import (
	"fmt"
	"time"

	"miniclaudecode-go/tools"
)

// ─── Agent Tool Registration ────────────────────────────────────────────────
//
// Tool registration functions extracted from agent_loop.go for better organization.

// registerAgentTool registers the AgentTool with this loop's callback.
func (a *AgentLoop) registerAgentTool() {
	agentTool := &tools.AgentTool{
		SpawnFunc:     a.SpawnSubAgent,
		SpawnSyncFunc: a.SpawnSubAgent, // same callback; sync mode is controlled by the runInBackground flag
		HandleStore:   a.agentHandleStore,
	}
	a.registry.Register(agentTool)
}

// registerSendMessageTool registers the SendMessage tool with this loop's callback.
func (a *AgentLoop) registerSendMessageTool() {
	sendMsgTool := &tools.SendMessageTool{
		SendMessageFunc: a.SendMessageToSubAgent,
		GetStatusFunc:   a.GetSubAgentStatus,
		ResolveNameFunc: a.resolveAgentID,
		HandleStore:     a.agentHandleStore,
	}
	a.registry.Register(sendMsgTool)
}

// registerTodoWriteTool registers the TodoWriteTool with this loop's todo list.
func (a *AgentLoop) registerTodoWriteTool() {
	todoTool := &tools.TodoWriteTool{TodoList: a.todoList}
	a.registry.Register(todoTool)
}

// registerAskUserQuestionTool registers the AskUserQuestion tool.
func (a *AgentLoop) registerAskUserQuestionTool() {
	a.registry.Register(&tools.AskUserQuestionTool{})
}

// registerPlanModeTools registers the EnterPlanMode and ExitPlanMode tools.
func (a *AgentLoop) registerPlanModeTools() {
	a.registry.Register(&tools.EnterPlanModeTool{
		GetMode: func() string { return string(a.config.PermissionMode) },
		SetMode: func(mode string) {
			if a.config.PermissionMode != ModePlan {
				a.config.PrePlanMode = a.config.PermissionMode
			}
			a.config.PermissionMode = PermissionMode(mode)
		},
	})
	a.registry.Register(&tools.ExitPlanModeTool{
		GetMode:        func() string { return string(a.config.PermissionMode) },
		SetMode:        func(mode string) { a.config.PermissionMode = PermissionMode(mode) },
		GetPrePlanMode: func() string { return string(a.config.PrePlanMode) },
	})
}

// registerTaskOutputTool registers the TaskOutputTool with this loop's callback.
func (a *AgentLoop) registerTaskOutputTool() {
	taskOutputTool := &tools.TaskOutputTool{
		GetOutputFunc:   a.GetSubAgentOutput,
		GetProgressFunc: a.GetTaskProgress,
	}
	a.registry.Register(taskOutputTool)
}

// registerTaskStopTool registers the TaskStopTool with this loop's callback.
func (a *AgentLoop) registerTaskStopTool() {
	taskStopTool := &tools.TaskStopTool{
		StopFunc: a.StopBackgroundTask,
		GetFunc: func(id string) (*tools.WorkTaskInfo, bool) {
			if a.workTaskStore == nil {
				return nil, false
			}
			task := a.workTaskStore.GetTask(id)
			if task == nil {
				return nil, false
			}
			info := &tools.WorkTaskInfo{
				ID:          task.ID,
				Subject:     task.Subject,
				Description: task.Description,
				ActiveForm:  task.ActiveForm,
				Status:      string(task.Status),
				Owner:       task.Owner,
				Metadata:    task.Metadata,
				Blocks:      task.Blocks,
				BlockedBy:   task.BlockedBy,
				CreatedAt:   task.CreatedAt.Format(time.RFC3339),
				UpdatedAt:   task.UpdatedAt.Format(time.RFC3339),
			}
			return info, true
		},
	}
	a.registry.Register(taskStopTool)
}

// registerBashBgTool wires the ExecTool's BackgroundTaskCallback to this loop's
// spawnBackgroundBashCommand method, enabling run_in_background support.
// TimeoutCallback is wired to registerExistingProcessAsBgTask for auto-backgrounding
// timed-out exec commands.
// Also wires MCPToolCaller's TimeoutCallback for auto-backgrounding timed-out MCP calls.
func (a *AgentLoop) registerBashBgTool() {
	if tool, ok := a.registry.Get("exec"); ok {
		if execTool, ok := tool.(*tools.ExecTool); ok {
			execTool.BackgroundTaskCallback = a.spawnBackgroundBashCommand
			execTool.TimeoutCallback = a.registerExistingProcessAsBgTask
		}
	}
	if tool, ok := a.registry.Get("mcp_call_tool"); ok {
		if mcpTool, ok := tool.(*tools.MCPToolCaller); ok {
			mcpTool.TimeoutCallback = a.registerMCPTimeoutAsBgTask
		}
	}
}

// registerAgentManagementTools registers the agent_list, agent_get, and agent_kill
// tools wired to the agentTaskStore.
func (a *AgentLoop) registerAgentManagementTools() {
	if a.agentTaskStore == nil {
		return
	}
	a.registry.Register(&tools.AgentListTool{Store: a.agentTaskStore})
	a.registry.Register(&tools.AgentGetTool{Store: a.agentTaskStore})
	a.registry.Register(&tools.AgentKillTool{Store: a.agentTaskStore})
}

// registerWorkTaskTools registers the TaskCreate/List/Get/Update tools
// wired to this loop's WorkTaskStore.
func (a *AgentLoop) registerWorkTaskTools() {
	if a.workTaskStore == nil {
		return
	}

	a.registry.Register(&tools.TaskCreateTool{
		CreateFunc: a.workTaskStore.CreateTask,
	})

	a.registry.Register(&tools.TaskListTool{
		ListFunc: func() []tools.WorkTaskInfo {
			tasks := a.workTaskStore.ListTasks()
			result := make([]tools.WorkTaskInfo, len(tasks))
			for i, t := range tasks {
				result[i] = tools.WorkTaskInfo{
					ID:          t.ID,
					Subject:     t.Subject,
					Description: t.Description,
					ActiveForm:  t.ActiveForm,
					Status:      string(t.Status),
					Owner:       t.Owner,
					Metadata:    t.Metadata,
					Blocks:      t.Blocks,
					BlockedBy:   t.BlockedBy,
					CreatedAt:   t.CreatedAt.Format(time.RFC3339),
					UpdatedAt:   t.UpdatedAt.Format(time.RFC3339),
				}
			}
			return result
		},
	})

	a.registry.Register(&tools.TaskGetTool{
		GetFunc: func(id string) (*tools.WorkTaskInfo, bool) {
			task := a.workTaskStore.GetTask(id)
			if task == nil {
				return nil, false
			}
			info := &tools.WorkTaskInfo{
				ID:          task.ID,
				Subject:     task.Subject,
				Description: task.Description,
				ActiveForm:  task.ActiveForm,
				Status:      string(task.Status),
				Owner:       task.Owner,
				Metadata:    task.Metadata,
				Blocks:      task.Blocks,
				BlockedBy:   task.BlockedBy,
				CreatedAt:   task.CreatedAt.Format(time.RFC3339),
				UpdatedAt:   task.UpdatedAt.Format(time.RFC3339),
			}
			return info, true
		},
	})

	a.registry.Register(&tools.TaskUpdateTool{
		UpdateFunc: a.workTaskStore.UpdateTask,
	})
}

// EnqueueAgentNotification pushes a formatted task notification XML to the notification channel.
func (a *AgentLoop) EnqueueAgentNotification(taskID, status, result, transcriptPath, outputFile string, toolsUsed int, totalTokens int, durationMs int64) {
	notification := fmt.Sprintf(`<task-notification>
<agentId>%s</agentId>
<status>%s</status>
<result>%s</result>
<output_file>%s</output_file>
<transcript_path>%s</transcript_path>
<usage><total_tokens>%d</total_tokens><tool_uses>%d</tool_uses><duration_ms>%d</duration_ms></usage>
</task-notification>`, taskID, status, result, outputFile, transcriptPath, totalTokens, toolsUsed, durationMs)

	select {
	case a.notificationChan <- notification:
	default:
		// Channel is full — log instead of silently dropping.
		// With 64 slots this should only happen with many concurrent sub-agents.
		a.logDebug("[warning] notification channel full, dropping: %s\n", taskID)
	}
}
