package main

import "encoding/json"

// TruncationRepairResult represents the result of repairing truncated JSON.
type TruncationRepairResult struct {
	Repaired string   `json:"repaired"`
	Changed  bool     `json:"changed"`
	Notes    []string `json:"notes"`
	Fallback bool     `json:"fallback"` // true if repair failed and fell back to "{}"
}

// repairTruncatedJson attempts to repair truncated JSON by balancing braces,
// closing strings, filling dangling keys with null, and trimming trailing commas.
//
// Matching DeepSeek-Reasonix's repair/truncation.ts pattern:
// - Fast path: return original if already parseable
// - Balance braces and track string state
// - Trim trailing commas
// - Fill dangling keys with null
// - Close unterminated strings
// - Close remaining open structures
// - Fall back to "{}" if all repairs fail
func repairTruncatedJson(input string) TruncationRepairResult {
	notes := []string{}

	// Fast path: already parseable
	if input == "" || len(input) == 0 {
		return TruncationRepairResult{
			Repaired: "{}",
			Changed:  input != "{}",
			Notes:    []string{"empty input → {}"},
			Fallback: false,
		}
	}

	// Try to parse directly first
	if _, err := json.Marshal(parseJSON(input)); err == nil {
		return TruncationRepairResult{Repaired: input, Changed: false, Notes: nil, Fallback: false}
	}

	// Track bracket/string state
	type stackItem struct {
		kind  rune // '{', '[' , '"'
		start int  // position where opened
	}
	stack := make([]stackItem, 0)
	escaped := false
	inString := false
	lastSignificant := -1

	for i, c := range input {
		// Track last non-whitespace position
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			lastSignificant = i
		}

		if escaped {
			escaped = false
			continue
		}

		if inString {
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inString = false
				if len(stack) > 0 && stack[len(stack)-1].kind == '"' {
					stack = stack[:len(stack)-1]
				}
			}
			continue
		}

		// Not in string
		if c == '"' {
			inString = true
			stack = append(stack, stackItem{kind: '"', start: i})
			continue
		}
		if c == '{' || c == '[' {
			stack = append(stack, stackItem{kind: c, start: i})
			continue
		}
		if c == '}' || c == ']' {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}

	// Trim to last significant character
	s := input
	if lastSignificant >= 0 {
		s = input[:lastSignificant+1]
	}

	// Trim trailing comma
	if len(s) > 0 && s[len(s)-1] == ',' {
		s = s[:len(s)-1]
		notes = append(notes, "trimmed trailing comma")
	}

	// If we ended on a key without a value: "foo": → "foo": null
	if len(s) >= 2 {
		trailing := s[len(s)-2:]
		if trailing == `":` {
			s += " null"
			notes = append(notes, "filled dangling key with null")
		}
	}

	// If we're inside a string, close it
	if inString && len(stack) > 0 && stack[len(stack)-1].kind == '"' {
		s += `"`
		stack = stack[:len(stack)-1]
		notes = append(notes, "closed unterminated string")
	}

	// Close remaining open structures in reverse order
	for len(stack) > 0 {
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if top.kind == '{' {
			s += "}"
		} else if top.kind == '[' {
			s += "]"
		} else if top.kind == '"' {
			s += `"`
		}
	}

	// Try to parse the repaired JSON
	if _, err := json.Marshal(parseJSON(s)); err == nil {
		return TruncationRepairResult{
			Repaired: s,
			Changed:  s != input,
			Notes:    notes,
			Fallback: false,
		}
	}

	// Fallback to empty object if all repairs fail
	preview := input
	if len(preview) > 500 {
		preview = input[:500] + " …[+" + itoa(len(input)-500) + " chars]"
	}
	notes = append(notes, "fallback to {}: unrecoverable truncation")
	notes = append(notes, "unrecoverable truncation — original args preview: "+preview)

	return TruncationRepairResult{
		Repaired: "{}",
		Changed:  true,
		Notes:    notes,
		Fallback: true,
	}
}

// parseJSON attempts to parse JSON into interface{}
func parseJSON(s string) any {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil
	}
	return v
}

// itoa is a simple int-to-string converter
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var s string
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}