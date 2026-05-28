// Package diagnostics provides types for resource diagnostics and collisions.
// Aligned to pi's diagnostics.ts.
package diagnostics

// DiagnosticType is the severity of a diagnostic.
type DiagnosticType string

const (
	DiagWarning   DiagnosticType = "warning"
	DiagError     DiagnosticType = "error"
	DiagCollision DiagnosticType = "collision"
)

// ResourceType is the type of resource that collides.
type ResourceType string

const (
	ResourceExtension ResourceType = "extension"
	ResourceSkill     ResourceType = "skill"
	ResourcePrompt    ResourceType = "prompt"
	ResourceTheme     ResourceType = "theme"
)

// ResourceCollision describes two resources competing for the same name.
type ResourceCollision struct {
	ResourceType ResourceType `json:"resourceType"`
	Name         string       `json:"name"`
	WinnerPath   string       `json:"winnerPath"`
	LoserPath    string       `json:"loserPath"`
	WinnerSource string       `json:"winnerSource,omitempty"`
	LoserSource  string       `json:"loserSource,omitempty"`
}

// ResourceDiagnostic is a diagnostic about a resource.
type ResourceDiagnostic struct {
	Type      DiagnosticType    `json:"type"`
	Message   string            `json:"message"`
	Path      string            `json:"path,omitempty"`
	Collision *ResourceCollision `json:"collision,omitempty"`
}