package main

import (
	"runtime"
	"strings"
)

// ─── Protected Paths (MiMo-Code 4) ─────────────────────────────────────────
//
// Platform-specific lists of directories that should never be watched,
// stated, or scanned. Prevents macOS TCC dialogs and Windows UAC prompts.
//
// MiMo-Code source: file/protected.ts (59 lines)

// ProtectedPaths holds platform-specific protected paths.
type ProtectedPaths struct {
	paths []string
}

// NewProtectedPaths creates a new protected paths list.
func NewProtectedPaths() *ProtectedPaths {
	p := &ProtectedPaths{}
	p.init()
	return p
}

// init initializes platform-specific protected paths.
func (p *ProtectedPaths) init() {
	switch runtime.GOOS {
	case "darwin":
		p.paths = []string{
			"~/Music",
			"~/Pictures",
			"~/Documents",
			"~/Desktop",
			"~/Library/AddressBook",
			"~/Library/Mail",
			"~/Library/Calendars",
			"/.DocumentRevisions-V100",
			"/.Spotlight-V100",
			"/.fseventsd",
		}
	case "windows":
		p.paths = []string{
			"~/AppData",
			"~/Downloads",
			"~/Desktop",
			"~/Documents",
			"~/Pictures",
			"~/Music",
			"~/Videos",
			"~/OneDrive",
		}
	case "linux":
		p.paths = []string{
			"/proc",
			"/sys",
			"/dev",
		}
	}
}

// IsProtected checks if a path is protected.
func (p *ProtectedPaths) IsProtected(path string) bool {
	normalized := strings.ToLower(path)
	for _, protected := range p.paths {
		normalizedProtected := strings.ToLower(protected)
		if strings.HasPrefix(normalized, normalizedProtected) {
			return true
		}
	}
	return false
}

// GetProtected returns all protected paths.
func (p *ProtectedPaths) GetProtected() []string {
	result := make([]string, len(p.paths))
	copy(result, p.paths)
	return result
}
