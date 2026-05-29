// Package sessionservices provides agent session services factory pattern.
// Aligned to pi's agent-session-services.ts.
//
// This provides a service factory pattern that separates service creation
// from session creation, allowing callers to resolve model, thinking, tools,
// and other session inputs against the target cwd before constructing the session.
package sessionservices

import (
	"fmt"
	"os"

	"miniclaudecode-go/pkg/core/agent"
	"miniclaudecode-go/pkg/core/auth"
	"miniclaudecode-go/pkg/core/config"
	"miniclaudecode-go/pkg/core/modelregistry"
	"miniclaudecode-go/pkg/core/resourceloader"
	"miniclaudecode-go/pkg/core/session"
)

// AgentSessionRuntimeDiagnostic represents a non-fatal issue during service creation.
type AgentSessionRuntimeDiagnostic struct {
	Type    string // "info", "warning", "error"
	Message string
}

// ScopedModel pairs a model with an optional thinking level.
type ScopedModel struct {
	Model         modelregistry.ModelInfo
	ThinkingLevel agent.ThinkingLevel
}

// CreateAgentSessionServicesOptions configures service creation.
type CreateAgentSessionServicesOptions struct {
	Cwd       string
	AgentDir  string
	AuthStorage     *auth.AuthStorage
	SettingsManager *config.SettingsManager
	ModelRegistry   *modelregistry.Registry
}

// AgentSessionServices contains all cwd-bound runtime services.
type AgentSessionServices struct {
	Cwd             string
	AgentDir        string
	AuthStorage     *auth.AuthStorage
	SettingsManager *config.SettingsManager
	ModelRegistry   *modelregistry.Registry
	ResourceLoader  *resourceloader.ResourceLoader
	Diagnostics     []AgentSessionRuntimeDiagnostic
}

// CreateAgentSessionServices creates cwd-bound runtime services.
// Returns services plus diagnostics. It does not create an AgentSession.
func CreateAgentSessionServices(opts CreateAgentSessionServicesOptions) (*AgentSessionServices, error) {
	cwd := opts.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	agentDir := opts.AgentDir
	if agentDir == "" {
		home, _ := os.UserHomeDir()
		agentDir = fmt.Sprintf("%s/.miniclaude", home)
	}

	// Auth storage
	authStorage := opts.AuthStorage
	if authStorage == nil {
		authStorage = auth.Create(agentDir + "/auth.json")
	}

	// Settings manager
	settingsMgr := opts.SettingsManager
	if settingsMgr == nil {
		settingsMgr = config.NewSettingsManager(agentDir, cwd+"/.miniclaude")
		if err := settingsMgr.Load(); err != nil {
			// Non-fatal
		}
	}

	// Model registry
	modelReg := opts.ModelRegistry
	if modelReg == nil {
		modelReg = modelregistry.NewRegistry()
		modelsPath := agentDir + "/models.json"
		if err := modelReg.LoadFromJSON(modelsPath); err != nil {
			// Non-fatal
		}
	}

	// Resource loader
	resourceLoader := resourceloader.New(cwd, agentDir)

	return &AgentSessionServices{
		Cwd:             cwd,
		AgentDir:        agentDir,
		AuthStorage:     authStorage,
		SettingsManager: settingsMgr,
		ModelRegistry:   modelReg,
		ResourceLoader:  resourceLoader,
	}, nil
}

// CreateAgentSessionFromServicesOptions configures session creation from services.
type CreateAgentSessionFromServicesOptions struct {
	Services      *AgentSessionServices
	SessionMgr    *session.SessionManager
	Model         *modelregistry.ModelInfo
	ThinkingLevel agent.ThinkingLevel
	ScopedModels  []ScopedModel
	Tools         []string
	NoTools       bool
}

// CreateAgentSessionFromServices creates an AgentSession from already-created services.
func CreateAgentSessionFromServices(opts CreateAgentSessionFromServicesOptions) (*agent.CreateSessionResult, error) {
	sessOpts := agent.CreateSessionOptions{
		Cwd:            opts.Services.Cwd,
		AgentDir:       opts.Services.AgentDir,
		AuthStorage:    opts.Services.AuthStorage,
		ModelRegistry:  opts.Services.ModelRegistry,
		ResourceLoader: opts.Services.ResourceLoader,
	}

	if opts.Model != nil {
		sessOpts.Model = opts.Model.ID
	}
	if opts.ThinkingLevel != "" {
		sessOpts.ThinkingLevel = opts.ThinkingLevel
	}
	if opts.Tools != nil {
		sessOpts.Tools = opts.Tools
	}
	if opts.NoTools {
		sessOpts.NoTools = true
	}

	return agent.CreateSession(sessOpts)
}
