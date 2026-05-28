package slashcmd

import "testing"

func TestBuiltinSlashCommands(t *testing.T) {
	cmds := BuiltinSlashCommands()
	if len(cmds) < 20 {
		t.Errorf("expected at least 20 builtin commands, got %d", len(cmds))
	}
}

func TestLookup(t *testing.T) {
	cmd, ok := Lookup("compact")
	if !ok {
		t.Fatal("compact should exist")
	}
	if cmd.Name != "compact" {
		t.Errorf("Name = %q, want compact", cmd.Name)
	}
	if cmd.Description == "" {
		t.Error("Description should not be empty")
	}
}

func TestLookupNotFound(t *testing.T) {
	_, ok := Lookup("nonexistent")
	if ok {
		t.Error("should not find nonexistent command")
	}
}

func TestAllNamesUnique(t *testing.T) {
	cmds := BuiltinSlashCommands()
	seen := make(map[string]bool, len(cmds))
	for _, c := range cmds {
		if seen[c.Name] {
			t.Errorf("duplicate command name: %q", c.Name)
		}
		seen[c.Name] = true
	}
}
