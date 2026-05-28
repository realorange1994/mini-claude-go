package diagnostics

import "testing"

func TestResourceCollision(t *testing.T) {
	c := ResourceCollision{
		ResourceType: "skill",
		Name:         "commit",
		WinnerPath:   "/a/skills/commit.json",
		LoserPath:    "/b/skills/commit.json",
	}
	if c.Name != "commit" {
		t.Error("name should match")
	}
}

func TestResourceDiagnostic(t *testing.T) {
	d := ResourceDiagnostic{
		Type:    DiagCollision,
		Message: `name "commit" collision`,
		Path:    "/b/skills/commit.json",
		Collision: &ResourceCollision{
			Name: "commit",
		},
	}
	if d.Type != DiagCollision {
		t.Errorf("Type = %q", d.Type)
	}
}

func TestDiagnosticTypeConstants(t *testing.T) {
	if DiagWarning != "warning" {
		t.Error("warning type mismatch")
	}
	if DiagError != "error" {
		t.Error("error type mismatch")
	}
	if DiagCollision != "collision" {
		t.Error("collision type mismatch")
	}
}