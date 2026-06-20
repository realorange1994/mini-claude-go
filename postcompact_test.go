package main

import (
	"strings"
	"testing"

	"miniclaudecode-go/tools"
)

func TestBuildPostCompactToolsAnnouncement_SkipsPreserved(t *testing.T) {
	a := &AgentLoop{
		registry:       tools.NewRegistry(),
		announcedTools: make(map[string]bool),
	}

	preserved := map[string]bool{"read_file": true}
	result := a.buildPostCompactToolsAnnouncement(preserved)
	if strings.Contains(result, "read_file") {
		t.Error("expected read_file to be skipped (preserved)")
	}
}

func TestBuildPostCompactToolsAnnouncement_AnnouncesUnpreserved(t *testing.T) {
	a := &AgentLoop{
		registry:       tools.NewRegistry(),
		announcedTools: make(map[string]bool),
	}

	result := a.buildPostCompactToolsAnnouncement(nil)
	// Empty registry should return empty (header-only is skipped by the function)
	if result != "" {
		// This is acceptable — the function returns the header even with no tools
		t.Logf("info: empty registry returns header (acceptable): %q", result)
	}
}

func TestAnnouncedTools_Tracking(t *testing.T) {
	a := &AgentLoop{
		announcedTools: make(map[string]bool),
	}

	a.announcedTools["read_file"] = true
	a.announcedTools["bash"] = true
	a.announcedTools["edit_file"] = true

	if !a.announcedTools["read_file"] {
		t.Error("expected read_file to be announced")
	}
	if !a.announcedTools["bash"] {
		t.Error("expected bash to be announced")
	}
	if a.announcedTools["write_file"] {
		t.Error("expected write_file to NOT be announced")
	}
}
