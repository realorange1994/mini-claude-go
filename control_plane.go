package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ─── Control Plane + Workspace Adaptor (MiMo-Code 1) ───────────────────────
//
// Full control-plane system for managing remote and local workspaces
// via pluggable "adaptors."
//
// MiMo-Code source: control-plane/workspace.ts (615 lines)

// WorkspaceID represents a workspace identifier.
type WorkspaceID string

// WorkspaceStatus represents workspace status.
type WorkspaceStatus string

const (
	WorkspaceActive   WorkspaceStatus = "active"
	WorkspaceInactive WorkspaceStatus = "inactive"
	WorkspaceSyncing  WorkspaceStatus = "syncing"
)

// Workspace represents a managed workspace.
type Workspace struct {
	ID        WorkspaceID     `json:"id"`
	Name      string          `json:"name"`
	Status    WorkspaceStatus `json:"status"`
	Target    WorkspaceTarget `json:"target"`
	CreatedAt time.Time       `json:"created_at"`
	LastSync  time.Time       `json:"last_sync"`
}

// WorkspaceTarget represents where a workspace points.
type WorkspaceTarget struct {
	Type string // "local" or "remote"
	Path string // local path or remote URL
}

// WorkspaceAdaptor is the interface for workspace backends.
type WorkspaceAdaptor interface {
	Configure(target WorkspaceTarget) error
	Create(id WorkspaceID) error
	Remove(id WorkspaceID) error
	Target() WorkspaceTarget
}

// ControlPlane manages workspaces via adaptors.
type ControlPlane struct {
	mu        sync.Mutex
	workspaces map[WorkspaceID]*Workspace
	adaptors   map[string]WorkspaceAdaptor
	dataDir    string
}

// NewControlPlane creates a new control plane.
func NewControlPlane(dataDir string) *ControlPlane {
	return &ControlPlane{
		workspaces: make(map[WorkspaceID]*Workspace),
		adaptors:   make(map[string]WorkspaceAdaptor),
		dataDir:    dataDir,
	}
}

// RegisterAdaptor registers a workspace adaptor.
func (cp *ControlPlane) RegisterAdaptor(name string, adaptor WorkspaceAdaptor) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.adaptors[name] = adaptor
}

// GetAdaptor returns an adaptor by name.
func (cp *ControlPlane) GetAdaptor(name string) WorkspaceAdaptor {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	return cp.adaptors[name]
}

// ListAdaptors returns all registered adaptor names.
func (cp *ControlPlane) ListAdaptors() []string {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	var names []string
	for name := range cp.adaptors {
		names = append(names, name)
	}
	return names
}

// CreateWorkspace creates a new workspace.
func (cp *ControlPlane) CreateWorkspace(name string, target WorkspaceTarget) (*Workspace, error) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	id := WorkspaceID(fmt.Sprintf("wrk_%s", time.Now().Format("20060102150405")))

	ws := &Workspace{
		ID:        id,
		Name:      name,
		Status:    WorkspaceActive,
		Target:    target,
		CreatedAt: time.Now(),
	}

	cp.workspaces[id] = ws
	cp.saveWorkspace(ws)

	return ws, nil
}

// RemoveWorkspace removes a workspace.
func (cp *ControlPlane) RemoveWorkspace(id WorkspaceID) error {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	ws, exists := cp.workspaces[id]
	if !exists {
		return fmt.Errorf("workspace not found: %s", id)
	}

	ws.Status = WorkspaceInactive
	delete(cp.workspaces, id)

	return nil
}

// GetWorkspace returns a workspace by ID.
func (cp *ControlPlane) GetWorkspace(id WorkspaceID) *Workspace {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	return cp.workspaces[id]
}

// ListWorkspaces returns all active workspaces.
func (cp *ControlPlane) ListWorkspaces() []*Workspace {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	var workspaces []*Workspace
	for _, ws := range cp.workspaces {
		if ws.Status == WorkspaceActive {
			workspaces = append(workspaces, ws)
		}
	}
	return workspaces
}

// SyncWorkspace syncs a workspace.
func (cp *ControlPlane) SyncWorkspace(id WorkspaceID) error {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	ws, exists := cp.workspaces[id]
	if !exists {
		return fmt.Errorf("workspace not found: %s", id)
	}

	ws.Status = WorkspaceSyncing
	ws.LastSync = time.Now()
	ws.Status = WorkspaceActive

	return nil
}

// saveWorkspace persists a workspace to disk.
func (cp *ControlPlane) saveWorkspace(ws *Workspace) {
	if cp.dataDir == "" {
		return
	}

	dir := filepath.Join(cp.dataDir, "workspaces")
	os.MkdirAll(dir, 0755)

	path := filepath.Join(dir, string(ws.ID)+".json")
	data, _ := json.MarshalIndent(ws, "", "  ")
	os.WriteFile(path, data, 0644)
}

// FormatWorkspace formats a workspace for display.
func FormatWorkspace(ws *Workspace) string {
	if ws == nil {
		return "No workspace."
	}
	return fmt.Sprintf("%s (%s) [%s] %s", ws.Name, ws.ID, ws.Status, ws.Target.Path)
}

// FormatControlPlaneStatus formats control plane status for display.
func FormatControlPlaneStatus(cp *ControlPlane) string {
	if cp == nil {
		return "No control plane."
	}

	workspaces := cp.ListWorkspaces()
	adaptors := cp.ListAdaptors()

	return fmt.Sprintf("Workspaces: %d, Adaptors: %d", len(workspaces), len(adaptors))
}
