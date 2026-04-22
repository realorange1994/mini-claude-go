package mcp

import (
	"sync"
	"testing"
)

func TestNewManager(t *testing.T) {
	mgr := NewManager()
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	if len(mgr.ListServers()) != 0 {
		t.Error("expected no servers initially")
	}
}

func TestManagerRegister(t *testing.T) {
	mgr := NewManager()
	mgr.Register("test-srv", "echo", []string{"hello"}, nil)

	servers := mgr.ListServers()
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got: %d", len(servers))
	}
	if servers[0] != "test-srv" {
		t.Errorf("expected server 'test-srv', got: %q", servers[0])
	}
}

func TestManagerRegisterMultiple(t *testing.T) {
	mgr := NewManager()
	mgr.Register("srv-a", "echo", []string{"a"}, nil)
	mgr.Register("srv-b", "echo", []string{"b"}, nil)

	servers := mgr.ListServers()
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got: %d", len(servers))
	}
}

func TestManagerGetServerStatus(t *testing.T) {
	mgr := NewManager()
	mgr.Register("test-srv", "echo", []string{}, nil)

	status := mgr.GetServerStatus("test-srv")
	if status != "disconnected" {
		t.Errorf("expected 'disconnected', got: %q", status)
	}

	status = mgr.GetServerStatus("nonexistent")
	if status != "not registered" {
		t.Errorf("expected 'not registered', got: %q", status)
	}
}

func TestManagerListToolsEmpty(t *testing.T) {
	mgr := NewManager()
	tools := mgr.ListTools()
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got: %d", len(tools))
	}
}

func TestManagerCallToolNotFound(t *testing.T) {
	mgr := NewManager()
	_, err := mgr.CallTool(nil, "nonexistent", nil)
	if err == nil {
		t.Error("expected error for nonexistent tool")
	}
}

func TestManagerCallToolWithServerNotFound(t *testing.T) {
	mgr := NewManager()
	_, err := mgr.CallToolWithServer(nil, "nonexistent", "tool", nil)
	if err == nil {
		t.Error("expected error for nonexistent server")
	}
}

func TestManagerStopAllEmpty(t *testing.T) {
	mgr := NewManager()
	mgr.StopAll() // should not panic
}

func TestClientCreation(t *testing.T) {
	client := NewClient("test", "echo", []string{"hello"}, map[string]string{"FOO": "bar"})
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.name != "test" {
		t.Errorf("expected name 'test', got: %q", client.name)
	}
	tools := client.Tools()
	if len(tools) != 0 {
		t.Errorf("expected 0 tools before start, got: %d", len(tools))
	}
}

func TestManagerConcurrency(t *testing.T) {
	mgr := NewManager()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := "srv" + string(rune('0'+n))
			mgr.Register(name, "echo", []string{}, nil)
		}(i)
	}
	wg.Wait()

	servers := mgr.ListServers()
	if len(servers) != 10 {
		t.Errorf("expected 10 servers, got: %d", len(servers))
	}
}

func TestToolWithServer(t *testing.T) {
	tws := ToolWithServer{
		Tool:   Tool{Name: "test-tool", Description: "A test tool"},
		Server: "test-srv",
	}
	if tws.Name != "test-tool" {
		t.Errorf("expected name 'test-tool', got: %q", tws.Name)
	}
	if tws.Server != "test-srv" {
		t.Errorf("expected server 'test-srv', got: %q", tws.Server)
	}
}
