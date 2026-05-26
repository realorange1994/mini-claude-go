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
// Three scavenge patterns supported:
//   1. DSML invoke blocks: <|DSML|invoke name="read_file">...</|DSML|invoke>
//   2. Bare JSON objects: { "name": "read_file", "arguments": {...} }
//   3. OpenAI-style: { "type": "function", "function": { "name": ..., "arguments": ... } }
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

	// Phase 2: Raw JSON objects
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

// scavengeDSML finds DSML invoke blocks in text.
var dsmlInvokeRe = regexp.MustCompile(`<[\|｜]DSML[\|｜]invoke\s+name="([^"]+)">([\s\S]*?)<[\|｜]/DSML[\|｜]invoke>`)
var dsmlParamRe = regexp.MustCompile(`<[\|｜]DSML[\|｜]parameter\s+name="([^"]+)"(?:\s+string="(true|false)")?\s*>([\s\S]*?)<[\|｜]/DSML[\|｜]parameter>`)

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

	// Pattern 1: { "name": "tool", "arguments": {...} }
	pattern1Re := regexp.MustCompile(`\{\s*"name"\s*:\s*"([^"]+)"\s*,\s*"arguments"\s*:\s*(\{[^}]*\})\s*\}`)
	for _, m := range pattern1Re.FindAllStringSubmatch(text, -1) {
		name := m[1]
		args := m[2]
		var input map[string]any
		if err := json.Unmarshal([]byte(args), &input); err == nil {
			calls = append(calls, map[string]any{
				"name":  name,
				"input": input,
			})
		}
	}

	// Pattern 2: OpenAI-style { "type": "function", "function": { "name": ..., "arguments": ... } }
	pattern2Re := regexp.MustCompile(`\{\s*"type"\s*:\s*"function"\s*,\s*"function"\s*:\s*\{\s*"name"\s*:\s*"([^"]+)"\s*,\s*"arguments"\s*:\s*(\{[^}]*\})\s*\}`)
	for _, m := range pattern2Re.FindAllStringSubmatch(text, -1) {
		name := m[1]
		args := m[2]
		var input map[string]any
		if err := json.Unmarshal([]byte(args), &input); err == nil {
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
