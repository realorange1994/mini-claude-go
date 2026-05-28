package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	all := r.All()
	if len(all) == 0 {
		t.Error("should have builtin skills")
	}
}

func TestGetBuiltin(t *testing.T) {
	r := NewRegistry()
	s, ok := r.Get("commit")
	if !ok {
		t.Fatal("commit skill should exist")
	}
	if s.Type != "builtin" {
		t.Errorf("Type = %q, want builtin", s.Type)
	}
	if s.Prompt == "" {
		t.Error("prompt should not be empty")
	}
}

func TestGetNotFound(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("should not find nonexistent skill")
	}
}

func TestRegister(t *testing.T) {
	r := NewRegistry()
	r.Register(Skill{
		Name:        "my-skill",
		Description: "Custom skill",
		Prompt:      "Do something custom",
		Type:        "user",
	})
	s, ok := r.Get("my-skill")
	if !ok {
		t.Fatal("custom skill should exist")
	}
	if s.Prompt != "Do something custom" {
		t.Errorf("Prompt = %q", s.Prompt)
	}
}

func TestUnregister(t *testing.T) {
	r := NewRegistry()
	r.Register(Skill{Name: "temp", Prompt: "temp"})
	r.Unregister("temp")
	_, ok := r.Get("temp")
	if ok {
		t.Error("skill should be removed")
	}
}

func TestActivePrompt(t *testing.T) {
	r := NewRegistry()
	prompt := r.ActivePrompt([]string{"commit", "nonexistent"})
	if prompt == "" {
		t.Error("should return prompt for commit")
	}
}

func TestLoadFromDir(t *testing.T) {
	tmp := t.TempDir()
	skillJSON := `{"name": "custom-skill", "description": "A custom skill", "prompt": "Do custom things"}`
	if err := os.WriteFile(filepath.Join(tmp, "custom-skill.json"), []byte(skillJSON), 0666); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry()
	if err := r.LoadFromDir(tmp); err != nil {
		t.Fatal(err)
	}

	s, ok := r.Get("custom-skill")
	if !ok {
		t.Fatal("custom-skill should be loaded")
	}
	if s.Type != "user" {
		t.Errorf("Type = %q, want user", s.Type)
	}
}

func TestLoadFromDir_NonExistent(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadFromDir("/nonexistent/path"); err != nil {
		t.Error("should not error for non-existent dir")
	}
}

func TestLoadFromDir_SkipsInvalid(t *testing.T) {
	tmp := t.TempDir()
	// No prompt field
	os.WriteFile(filepath.Join(tmp, "invalid.json"), []byte(`{"name": "bad"}`), 0666)
	// Not JSON
	os.WriteFile(filepath.Join(tmp, "not-json.json"), []byte(`not json`), 0666)

	r := NewRegistry()
	r.LoadFromDir(tmp)

	_, ok := r.Get("bad")
	if ok {
		t.Error("should not load skill with no prompt")
	}
}

func TestAllSorted(t *testing.T) {
	r := NewRegistry()
	r.Register(Skill{Name: "zzz", Prompt: "z"})
	r.Register(Skill{Name: "aaa", Prompt: "a"})

	all := r.All()
	if all[0].Name > all[len(all)-1].Name {
		t.Error("skills should be sorted by name")
	}
}