package main

import (
	"regexp"
	"strings"
	"testing"
)

// ─── generateWordSlug ────────────────────────────────────────────────────
// Ported from upstream words.test.ts

func TestGenerateWordSlugThreeParts(t *testing.T) {
	slug := generateWordSlug()
	parts := strings.Split(slug, "-")
	if len(parts) != 3 {
		t.Errorf("expected 3-part slug, got %d parts: %q", len(parts), slug)
	}
}

func TestGenerateWordSlugNonEmptyParts(t *testing.T) {
	for i := 0; i < 10; i++ {
		slug := generateWordSlug()
		parts := strings.Split(slug, "-")
		for j, part := range parts {
			if part == "" {
				t.Errorf("iteration %d: part %d is empty in slug %q", i, j, slug)
			}
		}
	}
}

func TestGenerateWordSlugAllLowercase(t *testing.T) {
	for i := 0; i < 10; i++ {
		slug := generateWordSlug()
		if slug != strings.ToLower(slug) {
			t.Errorf("slug should be all lowercase, got %q", slug)
		}
	}
}

func TestGenerateWordSlugNoConsecutiveHyphens(t *testing.T) {
	for i := 0; i < 10; i++ {
		slug := generateWordSlug()
		if strings.Contains(slug, "--") {
			t.Errorf("slug should not contain consecutive hyphens, got %q", slug)
		}
	}
}

func TestGenerateWordSlugPattern(t *testing.T) {
	slug := generateWordSlug()
	pattern := regexp.MustCompile(`^[a-z]+-[a-z]+-[a-z]+$`)
	if !pattern.MatchString(slug) {
		t.Errorf("slug should match adjective-verb-noun pattern, got %q", slug)
	}
}

func TestGenerateWordSlugVaried(t *testing.T) {
	// Multiple calls should produce varied results
	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		seen[generateWordSlug()] = true
	}
	if len(seen) <= 10 {
		t.Errorf("expected varied slugs, only got %d unique out of 20", len(seen))
	}
}

// ─── generateShortWordSlug ───────────────────────────────────────────────
// Ported from upstream words.test.ts

func TestGenerateShortWordSlugTwoParts(t *testing.T) {
	slug := generateShortWordSlug()
	parts := strings.Split(slug, "-")
	if len(parts) != 2 {
		t.Errorf("expected 2-part slug, got %d parts: %q", len(parts), slug)
	}
}

func TestGenerateShortWordSlugNonEmptyParts(t *testing.T) {
	for i := 0; i < 10; i++ {
		slug := generateShortWordSlug()
		parts := strings.Split(slug, "-")
		for j, part := range parts {
			if part == "" {
				t.Errorf("iteration %d: part %d is empty in slug %q", i, j, slug)
			}
		}
	}
}

func TestGenerateShortWordSlugAllLowercase(t *testing.T) {
	for i := 0; i < 10; i++ {
		slug := generateShortWordSlug()
		if slug != strings.ToLower(slug) {
			t.Errorf("short slug should be all lowercase, got %q", slug)
		}
	}
}

func TestGenerateShortWordSlugPattern(t *testing.T) {
	slug := generateShortWordSlug()
	pattern := regexp.MustCompile(`^[a-z]+-[a-z]+$`)
	if !pattern.MatchString(slug) {
		t.Errorf("short slug should match adjective-noun pattern, got %q", slug)
	}
}

func TestGenerateShortWordSlugNoConsecutiveHyphens(t *testing.T) {
	for i := 0; i < 10; i++ {
		slug := generateShortWordSlug()
		if strings.Contains(slug, "--") {
			t.Errorf("short slug should not contain consecutive hyphens, got %q", slug)
		}
	}
}

// ─── Invariants ──────────────────────────────────────────────────────────

func TestGenerateWordSlugNeverEmpty(t *testing.T) {
	for i := 0; i < 20; i++ {
		slug := generateWordSlug()
		if slug == "" {
			t.Error("generateWordSlug should never return empty string")
		}
	}
}

func TestGenerateShortWordSlugNeverEmpty(t *testing.T) {
	for i := 0; i < 20; i++ {
		slug := generateShortWordSlug()
		if slug == "" {
			t.Error("generateShortWordSlug should never return empty string")
		}
	}
}

func TestAdjectiveListNotEmpty(t *testing.T) {
	if len(adjectives) == 0 {
		t.Error("adjectives list should not be empty")
	}
}

func TestVerbListNotEmpty(t *testing.T) {
	if len(verbs) == 0 {
		t.Error("verbs list should not be empty")
	}
}

func TestNounListNotEmpty(t *testing.T) {
	if len(nouns) == 0 {
		t.Error("nouns list should not be empty")
	}
}

func TestWordListsLargeEnoughForVariation(t *testing.T) {
	// Should have at least 50 of each type for good variation
	if len(adjectives) < 50 {
		t.Errorf("expected at least 50 adjectives, got %d", len(adjectives))
	}
	if len(verbs) < 50 {
		t.Errorf("expected at least 50 verbs, got %d", len(verbs))
	}
	if len(nouns) < 50 {
		t.Errorf("expected at least 50 nouns, got %d", len(nouns))
	}
}
