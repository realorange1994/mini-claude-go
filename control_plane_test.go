package main

import (
	"testing"
)

// ─── Control Plane Tests ────────────────────────────────────────────────────

func TestControlPlane_New(t *testing.T) {
	dir := t.TempDir()
	cp := NewControlPlane(dir)
	if cp == nil {
		t.Error("expected non-nil control plane")
	}
}

func TestControlPlane_RegisterAdaptor(t *testing.T) {
	dir := t.TempDir()
	cp := NewControlPlane(dir)

	adaptor := &mockAdaptor{}
	cp.RegisterAdaptor("test", adaptor)

	if cp.GetAdaptor("test") == nil {
		t.Error("expected adaptor to be registered")
	}
}

func TestControlPlane_ListAdaptors(t *testing.T) {
	dir := t.TempDir()
	cp := NewControlPlane(dir)

	cp.RegisterAdaptor("a1", &mockAdaptor{})
	cp.RegisterAdaptor("a2", &mockAdaptor{})

	adaptors := cp.ListAdaptors()
	if len(adaptors) != 2 {
		t.Errorf("expected 2 adaptors, got %d", len(adaptors))
	}
}

func TestControlPlane_CreateWorkspace(t *testing.T) {
	dir := t.TempDir()
	cp := NewControlPlane(dir)

	ws, err := cp.CreateWorkspace("test", WorkspaceTarget{Type: "local", Path: "/tmp/test"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if ws.Name != "test" {
		t.Errorf("expected 'test', got %q", ws.Name)
	}
	if ws.Status != WorkspaceActive {
		t.Errorf("expected active, got %s", ws.Status)
	}
}

func TestControlPlane_RemoveWorkspace(t *testing.T) {
	dir := t.TempDir()
	cp := NewControlPlane(dir)

	ws, _ := cp.CreateWorkspace("test", WorkspaceTarget{Type: "local", Path: "/tmp/test"})
	err := cp.RemoveWorkspace(ws.ID)
	if err != nil {
		t.Fatalf("remove failed: %v", err)
	}

	if cp.GetWorkspace(ws.ID) != nil {
		t.Error("expected workspace to be removed")
	}
}

func TestControlPlane_ListWorkspaces(t *testing.T) {
	dir := t.TempDir()
	cp := NewControlPlane(dir)

	cp.CreateWorkspace("ws1", WorkspaceTarget{Type: "local", Path: "/tmp/ws1"})

	workspaces := cp.ListWorkspaces()
	if len(workspaces) < 1 {
		t.Errorf("expected at least 1 workspace, got %d", len(workspaces))
	}
}

func TestControlPlane_SyncWorkspace(t *testing.T) {
	dir := t.TempDir()
	cp := NewControlPlane(dir)

	ws, _ := cp.CreateWorkspace("test", WorkspaceTarget{Type: "local", Path: "/tmp/test"})
	err := cp.SyncWorkspace(ws.ID)
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
}

func TestFormatWorkspace(t *testing.T) {
	ws := &Workspace{
		ID:     "wrk-test",
		Name:   "Test",
		Status: WorkspaceActive,
		Target: WorkspaceTarget{Type: "local", Path: "/tmp/test"},
	}

	output := FormatWorkspace(ws)
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatWorkspace_Nil(t *testing.T) {
	output := FormatWorkspace(nil)
	if output != "No workspace." {
		t.Errorf("expected 'No workspace.', got %q", output)
	}
}

func TestFormatControlPlaneStatus(t *testing.T) {
	dir := t.TempDir()
	cp := NewControlPlane(dir)

	status := FormatControlPlaneStatus(cp)
	if status == "" {
		t.Error("expected non-empty status")
	}
}

// mockAdaptor implements WorkspaceAdaptor for testing.
type mockAdaptor struct {
	target WorkspaceTarget
}

func (m *mockAdaptor) Configure(target WorkspaceTarget) error {
	m.target = target
	return nil
}

func (m *mockAdaptor) Create(id WorkspaceID) error {
	return nil
}

func (m *mockAdaptor) Remove(id WorkspaceID) error {
	return nil
}

func (m *mockAdaptor) Target() WorkspaceTarget {
	return m.target
}
