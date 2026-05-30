package main

import (
	"encoding/json"
	"regexp"
	"strings"
)

// ScavengeToolCalls searches assistant text content for tool calls that
// weren't properly structured (e.g., emitted in reasoning_content, DSML
// markup, or raw JSON). Returns additional tool calls to merge.
//
// This matches DeepSeek-Reasonix's scavenge repair pattern: R1 models
// sometimes emit tool-call JSON inside reasoning_content or forget to
// populate the tool_calls field entirely. Without scavenge, the model's
// intended tool invocations are silently lost.
//
// Four scavenge patterns supported:
//   1. DSML invoke blocks: <|DSML|invoke name="read_file">...</|DSML|invoke>
//   2. [TOOL_CALL]...[/TOOL_CALL] with => syntax (hallucinated markup)
//   3. Bare JSON objects: { "name": "read_file", "arguments": {...} }
//   4. OpenAI-style: { "type": "function", "function": { "name": ..., "arguments": ... } }
func ScavengeToolCalls(textParts []string) []map[string]any {
	var scavenged []map[string]any
	seen := make(map[string]bool) // dedup by (name, args) signature

	fullText := strings.Join(textParts, "\n")
	if len(fullText) == 0 {
		return nil
	}

	// Cap input to prevent polynomial-redos on pathological text
	const maxInput = 100_000
	if len(fullText) > maxInput {
		fullText = fullText[:maxInput]
	}

	// Phase 1: DSML invoke blocks
	dsmlCalls := scavengeDSML(fullText)
	for _, call := range dsmlCalls {
		key := callSignature(call)
		if !seen[key] {
			seen[key] = true
			scavenged = append(scavenged, call)
		}
	}

	// Phase 2: [TOOL_CALL]...[/TOOL_CALL] with => syntax
	toolCallBlockCalls := scavengeToolCallBlock(fullText)
	for _, call := range toolCallBlockCalls {
		key := callSignature(call)
		if !seen[key] {
			seen[key] = true
			scavenged = append(scavenged, call)
		}
	}

	// Phase 3: Raw JSON objects
	rawJSONCalls := scavengeRawJSON(fullText)
	for _, call := range rawJSONCalls {
		key := callSignature(call)
		if !seen[key] {
			seen[key] = true
			scavenged = append(scavenged, call)
		}
	}

	return scavenged
}

// scavengeToolCallBlock finds [TOOL_CALL]...[/TOOL_CALL] blocks with => syntax.
// Format: [TOOL_CALL] {tool => "tool_name", args => { ... }} [/TOOL_CALL]
func scavengeToolCallBlock(text string) []map[string]any {
	var calls []map[string]any

	// Extract all [TOOL_CALL]...[/TOOL_CALL] blocks
	blockRe := regexp.MustCompile(`\[(?:TOOL_CALL|tool_call)\]([\s\S]*?)\[(?:/TOOL_CALL|/tool_call)\]`)
	matches := blockRe.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		inner := strings.TrimSpace(m[1])

		// Try to extract tool name and args from => syntax
		// Pattern: {tool => "name", args => {...}}
		// or: tool => "name"\nargs => {...}
		name := extractToolCallBlockName(inner)
		args := extractToolCallBlockArgs(inner)

		if name != "" && len(args) > 0 {
			calls = append(calls, map[string]any{
				"name":  name,
				"input": args,
			})
		} else if name != "" {
			// Name found but no args, still record it
			calls = append(calls, map[string]any{
				"name":  name,
				"input": args,
			})
		}
	}

	return calls
}

// extractToolCallBlockName extracts tool name from => syntax.
// Matches: tool => "read_file" or {tool => "read_file"
func extractToolCallBlockName(s string) string {
	re := regexp.MustCompile(`tool\s*=>\s*"([^"]+)"`)
	if m := re.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	// Also try single quotes
	re2 := regexp.MustCompile(`tool\s*=>\s*'([^']+)'`)
	if m := re2.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

// extractToolCallBlockArgs extracts args from => syntax.
// Matches: args => { --key "value" } or args => { "key": "value" }
func extractToolCallBlockArgs(s string) map[string]any {
	// Find the args => portion, then balance braces to capture the full args block
	re := regexp.MustCompile(`args\s*=>\s*\{`)
	loc := re.FindStringIndex(s)
	if loc == nil {
		return nil
	}

	// From the opening { after args =>, balance braces to find the closing }
	start := loc[1] - 1 // position of the opening {
	depth := 0
	end := -1
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i + 1
				goto found
			}
		}
	}
	return nil

found:
	inner := strings.TrimSpace(s[start:end])
	// Strip outer braces
	if strings.HasPrefix(inner, "{") && strings.HasSuffix(inner, "}") {
		inner = inner[1 : len(inner)-1]
		inner = strings.TrimSpace(inner)
	}

	// Try standard JSON first (in case args are already JSON)
	args := make(map[string]any)
	if err := json.Unmarshal([]byte("{"+inner+"}"), &args); err == nil && len(args) > 0 {
		return args
	}

	// Parse --key "value" or --key 'value' pairs
	argsRe := regexp.MustCompile(`--(\w+)\s+"([^"]*)"`)
	for _, am := range argsRe.FindAllStringSubmatch(inner, -1) {
		args[am[1]] = am[2]
	}

	// Also try --key 'value'
	argsRe2 := regexp.MustCompile(`--(\w+)\s+'([^']*)'`)
	for _, am := range argsRe2.FindAllStringSubmatch(inner, -1) {
		args[am[1]] = am[2]
	}

	// Also try key: "value" (JSON-style inside the => block)
	jsonRe := regexp.MustCompile(`"(\w+)"\s*:\s*"([^"]*)"`)
	for _, jm := range jsonRe.FindAllStringSubmatch(inner, -1) {
		args[jm[1]] = jm[2]
	}

	// Also try number values
	numRe := regexp.MustCompile(`--(\w+)\s+(\d+)`)
	for _, nm := range numRe.FindAllStringSubmatch(inner, -1) {
		args[nm[1]] = nm[2]
	}

	return args
}

// scavengeDSML finds DSML invoke blocks in text.
var dsmlInvokeRe = regexp.MustCompile(`<[\|｜]DSML[\|｜]invoke\s+name="([^"]+)">([\s\S]*?)</[\|｜]DSML[\|｜]invoke>`)
var dsmlParamRe = regexp.MustCompile(`<[\|｜]DSML[\|｜]parameter\s+name="([^"]+)"(?:\s+string="(true|false)")?\s*>([\s\S]*?)</[\|｜]DSML[\|｜]parameter>`)

func scavengeDSML(text string) []map[string]any {
	var calls []map[string]any

	matches := dsmlInvokeRe.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		name := m[1]
		body := m[2]

		input := make(map[string]any)
		// Parse parameters
		paramMatches := dsmlParamRe.FindAllStringSubmatch(body, -1)
		for _, pm := range paramMatches {
			paramName := pm[1]
			stringFlag := pm[2]
			raw := pm[3]

			if stringFlag == "false" {
				// Try JSON parse, fall back to literal
				var val interface{}
				if err := json.Unmarshal([]byte(raw), &val); err == nil {
					input[paramName] = val
					continue
				}
			}
			input[paramName] = raw
		}

		// If no parameters found, try parsing body as a single JSON value
		if len(input) == 0 {
			var val interface{}
			if err := json.Unmarshal([]byte(body), &val); err == nil {
				if m, ok := val.(map[string]interface{}); ok {
					input = m
				}
			}
		}

		calls = append(calls, map[string]any{
			"name":  name,
			"input": input,
		})
	}

	return calls
}

// scavengeRawJSON finds bare JSON objects that look like tool calls.
func scavengeRawJSON(text string) []map[string]any {
	var calls []map[string]any
	seen := make(map[string]bool) // local dedup between pattern1 and pattern2

	// Pattern 2 first: OpenAI-style { "type": "function", "function": { "name": ..., "arguments": ... } }
	pattern2Re := regexp.MustCompile(`\{\s*"type"\s*:\s*"function"\s*,\s*"function"\s*:\s*\{\s*"name"\s*:\s*"([^"]+)"\s*,\s*"arguments"\s*:\s*(\{[^}]*\})\s*\}`)
	for _, m := range pattern2Re.FindAllStringSubmatch(text, -1) {
		name := m[1]
		args := m[2]
		var input map[string]any
		if err := json.Unmarshal([]byte(args), &input); err == nil {
			key := callSignature(map[string]any{"name": name, "input": input})
			seen[key] = true
			calls = append(calls, map[string]any{
				"name":  name,
				"input": input,
			})
		}
	}

	// Pattern 1: { "name": "tool", "arguments": {...} }
	// Skip if already captured by pattern 2
	pattern1Re := regexp.MustCompile(`\{\s*"name"\s*:\s*"([^"]+)"\s*,\s*"arguments"\s*:\s*(\{[^}]*\})\s*\}`)
	for _, m := range pattern1Re.FindAllStringSubmatch(text, -1) {
		name := m[1]
		args := m[2]
		var input map[string]any
		if err := json.Unmarshal([]byte(args), &input); err == nil {
			key := callSignature(map[string]any{"name": name, "input": input})
			if seen[key] {
				continue
			}
			seen[key] = true
			calls = append(calls, map[string]any{
				"name":  name,
				"input": input,
			})
		}
	}

	return calls
}

// callSignature returns a dedup key for a scavenged call.
func callSignature(call map[string]any) string {
	name, _ := call["name"].(string)
	input, _ := call["input"].(map[string]any)
	data, _ := json.Marshal(input)
	return name + "|" + string(data)
}
